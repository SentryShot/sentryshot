// SPDX-License-Identifier: GPL-2.0-or-later

mod event;
pub mod monitor;
pub mod time;

use async_trait::async_trait;
use bytes::Bytes;
use bytesize::{ByteSize, MB};
pub use event::*;
use http::{HeaderMap, HeaderValue};
use sentryshot_padded_bytes::PaddedBytes;
use serde::{Deserialize, Serialize};
use std::{
    collections::HashMap,
    convert::{Infallible, TryFrom},
    fmt,
    io::Cursor,
    ops::Deref,
    path::Path,
    str::FromStr,
    sync::Arc,
    task::Poll,
};
use thiserror::Error;
use time::{DurationH264, UnixH264};
use tokio::{
    io::AsyncRead,
    sync::{broadcast, mpsc, oneshot},
};

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
pub struct EnvPlugin {
    name: String,
    enable: bool,
}

impl EnvPlugin {
    pub fn name(&self) -> &str {
        &self.name
    }

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
        Ok(Self(ByteSize((temp.0 * 1000.0) as u64 * MB)))
    }
}

/// Thread safe dyn 'ILogger'.
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

impl FromStr for MonitorId {
    type Err = ParseMonitorIdError;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        use ParseMonitorIdError::*;
        if s.is_empty() {
            return Err(Empty);
        }
        if !s.chars().all(char::is_alphanumeric) {
            return Err(InvalidChars(s.to_owned()));
        }
        if s.len() > MONITOR_ID_MAX_LENGTH {
            return Err(TooLong);
        }
        Ok(Self(s.to_owned()))
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
pub struct LogSource(String);

impl LogSource {
    pub fn len(&self) -> usize {
        self.0.len()
    }

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
        FromStr::from_str(&s).map_err(serde::de::Error::custom)
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

impl FromStr for LogSource {
    type Err = ParseLogSourceError;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
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
        Ok(Self(s.to_owned()))
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
        let s = String::deserialize(deserializer)?;
        FromStr::from_str(&s).map_err(serde::de::Error::custom)
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

impl FromStr for NonEmptyString {
    type Err = ParseNonEmptyStringError;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        if s.is_empty() {
            return Err(ParseNonEmptyStringError::Empty);
        }
        Ok(Self(s.to_owned()))
    }
}

impl Deref for NonEmptyString {
    type Target = str;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

pub struct DummyLogger {}

pub fn new_dummy_logger() -> Arc<DummyLogger> {
    Arc::new(DummyLogger {})
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
        Ok(Username(String::deserialize(deserializer)?.to_lowercase()))
    }
}

impl FromStr for Username {
    type Err = Infallible;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        Ok(Self(s.to_lowercase()))
    }
}

impl Deref for Username {
    type Target = str;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

impl Username {
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

//// Set account details request.
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
    pub fn is_main(&self) -> bool {
        *self == StreamType::Main
    }
    pub fn is_sub(&self) -> bool {
        *self == StreamType::Sub
    }

    pub fn name(&self) -> &str {
        if self.is_main() {
            "main"
        } else {
            "sub"
        }
    }
}

#[derive(Clone, Debug)]
pub struct VideoSample {
    pub ntp: UnixH264,
    pub pts: DurationH264, // Presentation time.
    pub dts: DurationH264, // Display time.
    pub avcc: Arc<PaddedBytes>,
    pub random_access_present: bool,

    pub duration: DurationH264,
}

impl std::fmt::Debug for StreamType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.name())
    }
}

#[derive(Clone, Debug, Default)]
pub struct PartFinalized {
    pub muxer_start_time: i64,
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

pub fn part_name(id: u64) -> String {
    ["part", &id.to_string()].join("")
}

#[derive(Debug)]
pub struct SegmentFinalized {
    id: u64,
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
    pub fn new(
        id: u64,
        start_time: UnixH264,
        name: String,
        parts: Vec<Arc<PartFinalized>>,
        duration: DurationH264,
    ) -> Self {
        Self {
            id,
            start_time,
            name,
            parts,
            duration,
        }
    }

    pub fn id(&self) -> u64 {
        self.id
    }

    pub fn name(&self) -> &str {
        &self.name
    }

    pub fn parts(&self) -> &Vec<Arc<PartFinalized>> {
        &self.parts
    }

    pub fn duration(&self) -> DurationH264 {
        self.duration
    }

    pub fn reader(&self) -> Box<dyn AsyncRead + Send + Unpin> {
        Box::new(PartsReader::new(self.parts.clone()))
    }

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
                self.cur_pos = 0
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
    async fn next_segment(&self, prev_id: u64) -> Option<Arc<SegmentFinalized>>;
}

#[derive(Debug)]
pub struct TrackParameters {
    pub width: u16,
    pub height: u16,
    pub codec: String,
    pub extra_data: Vec<u8>,
}

// A 'broadcast' channel is used instead of a 'watch' channel to detect dropped frames.
pub type Feed = broadcast::Receiver<H264Data>;

pub struct Source {
    stream_type: StreamType,
    get_muxer_tx: mpsc::Sender<oneshot::Sender<DynHlsMuxer>>,
    subscribe_tx: mpsc::Sender<oneshot::Sender<Feed>>,
}

impl Source {
    pub fn new(
        stream_type: StreamType,
        get_muxer_tx: mpsc::Sender<oneshot::Sender<DynHlsMuxer>>,
        subscribe_tx: mpsc::Sender<oneshot::Sender<Feed>>,
    ) -> Self {
        Self {
            stream_type,
            get_muxer_tx,
            subscribe_tx,
        }
    }

    pub fn stream_type(&self) -> &StreamType {
        &self.stream_type
    }

    // Returns the HLS muxer for this source. Will block until the source has started.
    pub async fn muxer(&self) -> Result<DynHlsMuxer, Cancelled> {
        let (res_tx, res_rx) = oneshot::channel();
        if self.get_muxer_tx.send(res_tx).await.is_err() {
            return Err(Cancelled);
        }
        let Ok(muxer) = res_rx.await else {
            return Err(Cancelled);
        };
        Ok(muxer)
    }

    // Subscribe to the raw feed. Will block until the source has started.
    #[allow(unused)]
    pub async fn subscribe(&self) -> Result<Feed, Cancelled> {
        let (res_tx, res_rx) = oneshot::channel();
        if self.subscribe_tx.send(res_tx).await.is_err() {
            return Err(Cancelled);
        }
        let Ok(channel) = res_rx.await else {
            return Err(Cancelled);
        };
        Ok(channel)
    }
}

#[derive(Debug, Error)]
#[error("cancelled")]
pub struct Cancelled;

#[derive(Clone, Debug, Default)]
pub struct H264Data {
    //pub ntp: i64,
    pub pts: DurationH264, // Presentation time.
    pub dts: DurationH264, // Composition time offset.
    pub avcc: Arc<PaddedBytes>,
    pub random_access_present: bool,
}

pub type DynMsgLogger = Arc<dyn MsgLogger + Send + Sync>;

pub trait MsgLogger {
    fn log(&self, level: LogLevel, msg: &str);
}

pub struct DummyMsgLogger;

impl MsgLogger for DummyMsgLogger {
    fn log(&self, _: LogLevel, _: &str) {}
}

pub fn new_dummy_msg_logger() -> Arc<impl MsgLogger> {
    Arc::new(DummyMsgLogger {})
}
