// SPDX-License-Identifier: GPL-2.0-or-later

mod event;
pub mod monitor;
pub mod recording;
pub mod time;

pub use event::*;

use async_trait::async_trait;
use bytes::Bytes;
use bytesize::{ByteSize, MB};
use http::{HeaderMap, HeaderValue};
use sentryshot_padded_bytes::PaddedBytes;
use serde::{Deserialize, Serialize};
use std::{
    borrow::Cow, collections::HashMap, convert::TryFrom, fmt, io::Cursor, ops::Deref, path::Path,
    str::FromStr, sync::Arc, task::Poll,
};
use thiserror::Error;
use time::{DtsOffset, DurationH264, UnixH264};
use tokio::io::AsyncRead;

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
pub struct EnvPlugin {
    name: String,
    enable: bool,
}

impl EnvPlugin {
    #[must_use]
    pub fn name(&self) -> &str {
        &self.name
    }

    #[must_use]
    pub fn enable(&self) -> bool {
        self.enable
    }
}

pub type DynEnvConfig = Box<dyn EnvConfig + Send + Sync>;

pub trait EnvConfig {
    fn port(&self) -> u16;
    fn storage_dir(&self) -> &Path;
    fn recordings_dir(&self) -> &Path;
    fn config_dir(&self) -> &Path;
    fn plugin_dir(&self) -> &Path;
    fn max_disk_usage(&self) -> ByteSize;
    fn plugins(&self) -> &Option<Vec<EnvPlugin>>;
}

impl NonZeroGb {
    #[must_use]
    pub fn new(size: ByteSize) -> Option<Self> {
        if size.0 == 0 {
            None
        } else {
            Some(Self(size))
        }
    }
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct NonZeroGb(ByteSize);

impl Deref for NonZeroGb {
    type Target = ByteSize;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

impl<'de> Deserialize<'de> for NonZeroGb {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        #[derive(Deserialize)]
        struct Temp(f32);

        let temp = Temp::deserialize(deserializer)?;
        if temp.0 == 0.0 {
            return Err(serde::de::Error::custom("cannot be zero"));
        }
        #[allow(
            clippy::cast_sign_loss,
            clippy::cast_possible_truncation,
            clippy::as_conversions
        )]
        Ok(Self(ByteSize((temp.0 * 1000.0) as u64 * MB)))
    }
}

/// Thread safe dyn '`ILogger`'.
pub type DynLogger = Arc<dyn ILogger + Send + Sync>;

pub trait ILogger {
    /// Send log.
    fn log(&self, _: LogEntry) {}
}

/// Log entry. See `EntryWithTime`.
#[derive(Clone, Debug)]
pub struct LogEntry {
    pub level: LogLevel,
    pub source: LogSource,
    pub monitor_id: Option<MonitorId>,
    pub message: NonEmptyString,
}

impl LogEntry {
    #[allow(clippy::unwrap_used, clippy::needless_pass_by_value)]
    #[must_use]
    pub fn new(
        level: LogLevel,
        source: &'static str,
        monitor_id: Option<MonitorId>,
        message: String,
    ) -> Self {
        let source: LogSource = source
            .to_owned()
            .try_into()
            .expect("source should be valid");
        let message: NonEmptyString = message
            .try_into()
            .unwrap_or("invalid_message".to_owned().try_into().unwrap());
        Self {
            level,
            source,
            monitor_id,
            message,
        }
    }
}

/// Severity of the log message.
#[derive(Clone, Copy, Debug, Hash, PartialEq, Eq, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum LogLevel {
    /// Something requires attention.
    Error,

    /// Something may require attention.
    Warning,

    /// Standard information.
    Info,

    /// Verbose debugging information.
    Debug,
}

impl LogLevel {
    #[must_use]
    pub fn as_u8(&self) -> u8 {
        match self {
            LogLevel::Error => 16,
            LogLevel::Warning => 24,
            LogLevel::Info => 32,
            LogLevel::Debug => 48,
        }
    }
}

#[derive(Debug, Error)]
pub enum ParseLogLevelError {
    #[error("invalid value: '{0}'")]
    InvalidValue(u8),

    #[error("unknown log level: '{0}'")]
    UnknownLevel(String),
}

impl TryFrom<u8> for LogLevel {
    type Error = ParseLogLevelError;

    fn try_from(value: u8) -> Result<Self, ParseLogLevelError> {
        match value {
            16 => Ok(Self::Error),
            24 => Ok(Self::Warning),
            32 => Ok(Self::Info),
            48 => Ok(Self::Debug),
            _ => Err(ParseLogLevelError::InvalidValue(value)),
        }
    }
}

impl<'de> Deserialize<'de> for LogLevel {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        let s = String::deserialize(deserializer)?;
        FromStr::from_str(&s).map_err(serde::de::Error::custom)
    }
}

impl FromStr for LogLevel {
    type Err = ParseLogLevelError;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s {
            "error" => Ok(LogLevel::Error),
            "warning" => Ok(LogLevel::Warning),
            "info" => Ok(LogLevel::Info),
            "debug" => Ok(LogLevel::Debug),
            _ => Err(ParseLogLevelError::UnknownLevel(s.to_owned())),
        }
    }
}

pub const MONITOR_ID_MAX_LENGTH: usize = 24;

#[derive(Clone, Debug, Hash, PartialEq, Eq, Serialize, Deserialize)]
pub struct MonitorId(String);

impl fmt::Display for MonitorId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.0)
    }
}

#[derive(Debug, Error, PartialEq, Eq)]
pub enum ParseMonitorIdError {
    #[error("empty string")]
    Empty,

    #[error("invalid characters: '{0}'")]
    InvalidChars(String),

    #[error("too long")]
    TooLong,
}

impl TryFrom<String> for MonitorId {
    type Error = ParseMonitorIdError;

    fn try_from(s: String) -> Result<Self, Self::Error> {
        use ParseMonitorIdError::*;
        if s.is_empty() {
            return Err(Empty);
        }
        if !s.chars().all(char::is_alphanumeric) {
            return Err(InvalidChars(s));
        }
        if s.len() > MONITOR_ID_MAX_LENGTH {
            return Err(TooLong);
        }
        Ok(Self(s))
    }
}

impl Deref for MonitorId {
    type Target = str;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

pub const LOG_SOURCE_MAX_LENGTH: usize = 8;

#[repr(transparent)]
#[derive(Clone, Debug, Hash, PartialEq, Eq, Serialize, PartialOrd, Ord)]
pub struct LogSource(Cow<'static, str>);

impl LogSource {
    #[must_use]
    pub fn len(&self) -> usize {
        self.0.len()
    }

    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }
}

impl<'de> Deserialize<'de> for LogSource {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        let s = String::deserialize(deserializer)?;
        s.try_into().map_err(serde::de::Error::custom)
    }
}

#[derive(Debug, Error, PartialEq, Eq)]
pub enum ParseLogSourceError {
    #[error("empty string")]
    Empty,

    #[error("invalid characters: '{0}'")]
    InvalidChars(String),

    #[error("too long")]
    TooLong,
}

impl TryFrom<String> for LogSource {
    type Error = ParseLogSourceError;

    fn try_from(s: String) -> Result<Self, Self::Error> {
        use ParseLogSourceError::*;
        if s.is_empty() {
            return Err(Empty);
        }
        if !s.chars().all(char::is_alphanumeric) {
            return Err(InvalidChars(s));
        }
        if s.len() > MONITOR_ID_MAX_LENGTH {
            return Err(TooLong);
        }
        Ok(Self(Cow::Owned(s)))
    }
}

impl TryFrom<&'static str> for LogSource {
    type Error = ParseLogSourceError;

    fn try_from(s: &'static str) -> Result<Self, Self::Error> {
        use ParseLogSourceError::*;
        if s.is_empty() {
            return Err(Empty);
        }
        if !s.chars().all(char::is_alphanumeric) {
            return Err(InvalidChars(s.to_owned()));
        }
        if s.len() > MONITOR_ID_MAX_LENGTH {
            return Err(TooLong);
        }
        Ok(Self(Cow::Borrowed(s)))
    }
}

impl Deref for LogSource {
    type Target = str;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

#[repr(transparent)]
#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize)]
pub struct NonEmptyString(String);

impl<'de> Deserialize<'de> for NonEmptyString {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        String::deserialize(deserializer)?
            .try_into()
            .map_err(serde::de::Error::custom)
    }
}

impl fmt::Display for NonEmptyString {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.0)
    }
}

#[derive(Debug, Error, PartialEq, Eq)]
pub enum ParseNonEmptyStringError {
    #[error("empty string")]
    Empty,
}

impl TryFrom<String> for NonEmptyString {
    type Error = ParseNonEmptyStringError;

    fn try_from(s: String) -> Result<Self, Self::Error> {
        if s.is_empty() {
            return Err(ParseNonEmptyStringError::Empty);
        }
        Ok(Self(s))
    }
}

impl Deref for NonEmptyString {
    type Target = str;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

pub struct DummyLogger;

impl DummyLogger {
    #[must_use]
    pub fn new() -> Arc<Self> {
        Arc::new(DummyLogger {})
    }
}

impl ILogger for DummyLogger {
    fn log(&self, _: LogEntry) {}
}

// Thread safe dyn `Authenticator`.
pub type DynAuth = Arc<dyn Authenticator + Send + Sync>;

pub type AccountsMap = HashMap<String, AccountObfuscated>;

// Username is lowercase only.
#[derive(Clone, Debug, Serialize, PartialEq, Eq)]
pub struct Username(String);

impl<'de> Deserialize<'de> for Username {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        Ok(String::deserialize(deserializer)?.into())
    }
}

impl From<String> for Username {
    fn from(value: String) -> Self {
        Self(value.to_lowercase())
    }
}

impl Deref for Username {
    type Target = str;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

impl Username {
    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }
}

/// Account without sensitive information.
#[derive(Debug, Serialize, PartialEq, Eq)]
pub struct AccountObfuscated {
    pub id: String,
    pub username: Username,

    #[serde(rename = "isAdmin")]
    pub is_admin: bool,
}

// Authenticator is responsible for blocking all
// unauthenticated requests and storing user information.
#[async_trait]
pub trait Authenticator {
    // ValidateRequest validates raw http requests.
    async fn validate_request(&self, req: &HeaderMap<HeaderValue>) -> Option<ValidateResponse>;

    // Returns obfuscated account map.
    async fn accounts(&self) -> AccountsMap;

    // Set the information of an account.
    async fn account_set(&self, req: AccountSetRequest) -> Result<bool, AuthAccountSetError>;

    // Deletes account by id.
    async fn account_delete(&self, id: &str) -> Result<(), AuthAccountDeleteError>;

    // Handlers.
    //MyToken() http.Handler
    //Logout() http.Handler
}

/// Set account details request.
#[derive(Clone, Debug, Deserialize)]
pub struct AccountSetRequest {
    pub id: String,
    pub username: Username,

    #[serde(rename = "plainPassword")]
    pub plain_password: Option<String>,

    #[serde(rename = "isAdmin")]
    pub is_admin: bool,
}

#[derive(Debug, Error)]
pub enum AuthAccountSetError {
    #[error("missing ID")]
    IdMissing(),

    #[error("missing username")]
    UsernameMissing(),

    #[error("password is required for new accounts")]
    PasswordMissing(),

    #[error("save accounts: {0}")]
    SaveAccounts(#[from] AuthSaveToFileError),
}

#[derive(Debug, Error)]
pub enum AuthAccountDeleteError {
    #[error("account does not exist '{0}'")]
    AccountNotExist(String),

    #[error("save accounts: {0}")]
    SaveAccounts(#[from] AuthSaveToFileError),
}

#[derive(Debug, Error)]
pub enum AuthSaveToFileError {
    #[error("serialize accounts: {0}")]
    Serialize(#[from] serde_json::Error),

    #[error("open file: {0}")]
    OpenFile(std::io::Error),

    #[error("write file: {0}")]
    WriteFile(std::io::Error),

    #[error("sync file: {0}")]
    SyncFile(std::io::Error),

    #[error("rename file: {0}")]
    RenameFile(std::io::Error),
}

/// Main account definition.
#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
pub struct Account {
    pub id: String,
    pub username: Username,
    pub password: String, // Hashed password PHC string.

    #[serde(rename = "isAdmin")]
    pub is_admin: bool,

    #[serde(skip)]
    pub token: String,
}

#[derive(Clone)]
pub struct ValidateResponse {
    pub is_admin: bool,
    pub token: String,
    pub token_valid: bool,
}

#[derive(Clone)]
pub struct ValidateLoginResponse {
    pub is_admin: bool,
    pub token: String,
}

#[derive(Clone, Copy, PartialEq, Eq)]
pub enum StreamType {
    Main,
    Sub,
}

impl StreamType {
    #[must_use]
    pub fn is_main(&self) -> bool {
        *self == StreamType::Main
    }
    #[must_use]
    pub fn is_sub(&self) -> bool {
        *self == StreamType::Sub
    }

    #[must_use]
    pub fn name(&self) -> &str {
        if self.is_main() {
            "main"
        } else {
            "sub"
        }
    }
}

#[derive(Clone, Debug, Default)]
pub struct VideoSample {
    pub pts: UnixH264,         // Relative presentation timestamp.
    pub dts_offset: DtsOffset, // Composition time offset.
    pub avcc: Arc<PaddedBytes>,
    pub random_access_present: bool,

    pub duration: DurationH264,
}

impl VideoSample {
    #[must_use]
    pub fn dts(&self) -> Option<UnixH264> {
        self.pts.checked_sub(self.dts_offset.into())
    }
}

impl std::fmt::Debug for StreamType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.name())
    }
}

#[derive(Clone, Debug, Default)]
pub struct PartFinalized {
    pub id: u64,

    pub is_independent: bool,
    pub video_samples: Arc<Vec<VideoSample>>,
    pub rendered_content: Option<Bytes>,
    pub rendered_duration: DurationH264,
}

impl PartFinalized {
    pub fn name(&self) -> String {
        part_name(self.id)
    }

    pub fn reader(&self) -> Box<dyn AsyncRead + Send + Unpin> {
        let Some(rendered_content) = &self.rendered_content else {
            return Box::new(Cursor::new(Vec::new()));
        };
        Box::new(Cursor::new(rendered_content.clone()))
    }
}

#[must_use]
pub fn part_name(id: u64) -> String {
    ["part", &id.to_string()].join("")
}

#[derive(Debug)]
pub struct SegmentFinalized {
    id: u64,
    muxer_id: u16,
    start_time: UnixH264,
    //pub start_dts: i64,
    //muxer_start_time: i64,
    //playlist: Arc<Playlist>,
    name: String,
    //size: u64,
    parts: Vec<Arc<PartFinalized>>,
    duration: DurationH264,
}

impl SegmentFinalized {
    #[must_use]
    pub fn new(
        id: u64,
        muxer_id: u16,
        start_time: UnixH264,
        name: String,
        parts: Vec<Arc<PartFinalized>>,
        duration: DurationH264,
    ) -> Self {
        Self {
            id,
            muxer_id,
            start_time,
            name,
            parts,
            duration,
        }
    }

    #[must_use]
    pub fn id(&self) -> u64 {
        self.id
    }

    #[must_use]
    pub fn muxer_id(&self) -> u16 {
        self.muxer_id
    }

    #[must_use]
    pub fn name(&self) -> &str {
        &self.name
    }

    #[must_use]
    pub fn parts(&self) -> &Vec<Arc<PartFinalized>> {
        &self.parts
    }

    #[must_use]
    pub fn duration(&self) -> DurationH264 {
        self.duration
    }

    #[must_use]
    pub fn reader(&self) -> Box<dyn AsyncRead + Send + Unpin> {
        Box::new(PartsReader::new(self.parts.clone()))
    }

    #[must_use]
    pub fn start_time(&self) -> UnixH264 {
        self.start_time
    }
}

pub struct PartsReader {
    parts: Vec<Arc<PartFinalized>>,
    cur_part: usize,
    cur_pos: usize,
}

impl PartsReader {
    #[must_use]
    pub fn new(parts: Vec<Arc<PartFinalized>>) -> Self {
        Self {
            parts,
            cur_part: 0,
            cur_pos: 0,
        }
    }
}

impl AsyncRead for PartsReader {
    fn poll_read(
        mut self: std::pin::Pin<&mut Self>,
        _cx: &mut std::task::Context<'_>,
        buf: &mut tokio::io::ReadBuf<'_>,
    ) -> std::task::Poll<std::io::Result<()>> {
        let mut n = 0;
        let buf_len = buf.remaining();

        loop {
            if self.cur_part >= self.parts.len() {
                // EOF.
                return Poll::Ready(Ok(()));
            }

            let Some(part) = &self.parts[self.cur_part].rendered_content else {
                panic!("expected part to exist");
            };

            let part_len = part.len();

            let start = self.cur_pos;
            let amt = std::cmp::min(part.len() - start, buf.remaining());
            let end = start + amt;

            buf.put_slice(&part[start..end]);

            self.cur_pos += amt;
            n += amt;

            if self.cur_pos == part_len {
                self.cur_part += 1;
                self.cur_pos = 0;
            }

            // If buffer is full.
            if n == buf_len {
                return Poll::Ready(Ok(()));
            }
        }
    }
}

pub type DynHlsMuxer = Arc<dyn HlsMuxer + Send + Sync>;

#[async_trait]
pub trait HlsMuxer {
    fn params(&self) -> &TrackParameters;

    // Returns none if cancelled.
    async fn next_segment(
        &self,
        prev_seg: Option<&SegmentFinalized>,
    ) -> Option<Arc<SegmentFinalized>>;
}

#[derive(Clone, Debug)]
pub struct TrackParameters {
    pub width: u16,
    pub height: u16,
    pub codec: String,
    pub extra_data: Vec<u8>,
}

#[derive(Clone, Debug, Default)]
pub struct H264Data {
    pub pts: UnixH264,         // Absolute presentation timestamp.
    pub dts_offset: DtsOffset, // Composition time offset.
    pub avcc: Arc<PaddedBytes>,
    pub random_access_present: bool,
}

impl std::fmt::Display for H264Data {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "pts: {:?}, dts_offset: {:?}, IDR: {}",
            self.pts, self.dts_offset, self.random_access_present
        )
    }
}

pub type DynMsgLogger = Arc<dyn MsgLogger + Send + Sync>;

pub trait MsgLogger {
    fn log(&self, level: LogLevel, msg: &str);
}

pub struct DummyMsgLogger;

impl MsgLogger for DummyMsgLogger {
    fn log(&self, _: LogLevel, _: &str) {}
}

#[must_use]
pub fn new_dummy_msg_logger() -> Arc<impl MsgLogger> {
    Arc::new(DummyMsgLogger {})
}
