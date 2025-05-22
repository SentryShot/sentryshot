// SPDX-License-Identifier: GPL-2.0-or-later

#![allow(clippy::module_name_repetitions)]

use crate::{
    ArcMsgLogger, ArcStreamerMuxer, DynError, Event, H264Data, MonitorId, MonitorName, StreamType,
    TrackParameters,
    recording::{FrameRateLimiter, FrameRateLimiterError},
    time::{Duration, MINUTE, UnixH264},
};
use async_trait::async_trait;
use sentryshot_ffmpeg_h264::{H264BuilderError, ReceiveFrameError, SendPacketError};
use sentryshot_util::Frame;
use serde::{Deserialize, Deserializer, Serialize, Serializer};
use std::{collections::HashMap, ops::Deref, path::PathBuf, str::FromStr, sync::Arc};
use thiserror::Error;
use tokio::{
    runtime::Handle,
    sync::{broadcast, mpsc},
};
use tokio_util::sync::CancellationToken;
use url::Url;

#[allow(clippy::module_name_repetitions)]
pub type MonitorConfigs = HashMap<MonitorId, MonitorConfig>;

#[derive(Clone, Debug, PartialEq)]
#[allow(clippy::module_name_repetitions)]
pub struct MonitorConfig {
    config: Config,
    source: SourceConfig,

    // Raw json used for unknown values.
    raw: serde_json::Value,
}

impl MonitorConfig {
    #[must_use]
    pub fn new(config: Config, source: SourceConfig, raw: serde_json::Value) -> Self {
        Self {
            config,
            source,
            raw,
        }
    }

    #[must_use]
    pub fn config(&self) -> &Config {
        &self.config
    }
    #[must_use]
    pub fn source(&self) -> &SourceConfig {
        &self.source
    }
    #[must_use]
    pub fn raw(&self) -> &serde_json::Value {
        &self.raw
    }

    /*
    // Get config value by key.
    func (c Config) Get(key string) string {
        return c.v[key]
    }
    */

    // Monitor ID.
    #[must_use]
    pub fn id(&self) -> &MonitorId {
        &self.config.id
    }

    // Name returns the monitor name.
    #[must_use]
    pub fn name(&self) -> &MonitorName {
        &self.config.name
    }

    #[must_use]
    pub fn enabled(&self) -> bool {
        self.config.enable
    }

    #[must_use]
    pub fn has_sub_stream(&self) -> bool {
        match &self.source {
            SourceConfig::Rtsp(v) => v.sub_stream.is_some(),
        }
    }

    /*
        // video length is seconds.
        func (c Config) videoLength() string {
            return c.v["videoLength"]
        }
    */
    #[must_use]
    pub fn always_record(&self) -> bool {
        self.config.always_record
    }

    #[must_use]
    #[allow(clippy::cast_precision_loss, clippy::as_conversions)]
    pub fn video_length(&self) -> Duration {
        Duration::from_f64(self.config.video_length * (MINUTE as f64))
    }

    /*
        // TimestampOffset returns the timestamp offset.
        func (c Config) TimestampOffset() string {
            return c.v["timestampOffset"]
        }

        // LogLevel returns the ffmpeg log level.
        func (c Config) LogLevel() string {
            return c.v["logLevel"]
        }
    */
}

#[derive(Clone, Debug, Deserialize, PartialEq)]
pub struct Config {
    pub id: MonitorId,
    pub name: MonitorName,
    pub enable: bool,
    pub source: SelectedSource,

    #[serde(rename = "alwaysRecord")]
    pub always_record: bool,

    #[serde(rename = "videoLength")]
    pub video_length: f64,
}

impl Serialize for MonitorConfig {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: Serializer,
    {
        self.raw.serialize(serializer)
    }
}

impl<'de> Deserialize<'de> for MonitorConfig {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        use serde::de::Error;
        let value = serde_json::Value::deserialize(deserializer)?;
        let config: Config = serde_json::from_value(value.clone()).map_err(Error::custom)?;

        let sources: SourcesDeserializer =
            serde_json::from_value(value.clone()).map_err(Error::custom)?;

        let source = match config.source {
            SelectedSource::Rtsp => SourceConfig::Rtsp(sources.source_rtsp.map_err(Error::custom)?),
        };

        Ok(Self {
            config,
            source,
            raw: value,
        })
    }
}

#[derive(Clone, Debug, Deserialize, PartialEq, Eq)]
pub enum SelectedSource {
    #[serde(rename = "rtsp")]
    Rtsp,
}

#[derive(Deserialize)]
pub struct SourcesDeserializer {
    #[serde(rename = "sourcertsp", deserialize_with = "try_deserialize")]
    pub source_rtsp: Result<SourceRtspConfig, serde_json::Error>,
}

fn try_deserialize<'de, D, T>(deserializer: D) -> Result<Result<T, serde_json::Error>, D::Error>
where
    D: Deserializer<'de>,
    T: Deserialize<'de>,
{
    let v = serde_json::Value::deserialize(deserializer)?;
    Ok(T::deserialize(v))
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub enum SourceConfig {
    Rtsp(SourceRtspConfig),
}

#[derive(Clone, Debug, Deserialize, PartialEq, Eq)]
pub struct SourceRtspConfig {
    pub protocol: Protocol,

    #[serde(rename = "mainStream")]
    pub main_stream: RtspUrl,

    #[serde(rename = "subStream")]
    pub sub_stream: Option<RtspUrl>,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Eq)]
pub enum Protocol {
    #[serde(rename = "tcp")]
    Tcp,

    #[serde(rename = "udp")]
    Udp,
}

#[derive(Debug, Error)]
pub enum ParseRtspUrl {
    #[error("{0}")]
    ParseUrl(#[from] url::ParseError),

    #[error("bad url '{0}' only scheme rtsp supported")]
    BadScheme(String),
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RtspUrl(Url);

impl TryFrom<Url> for RtspUrl {
    type Error = ParseRtspUrl;

    fn try_from(url: Url) -> Result<Self, Self::Error> {
        if url.scheme() != "rtsp" {
            return Err(ParseRtspUrl::BadScheme(url.to_string()));
        }
        Ok(Self(url))
    }
}

impl FromStr for RtspUrl {
    type Err = ParseRtspUrl;

    fn from_str(value: &str) -> Result<Self, Self::Err> {
        let url = Url::try_from(value)?;
        RtspUrl::try_from(url)
    }
}

impl<'de> Deserialize<'de> for RtspUrl {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        let s = String::deserialize(deserializer)?;
        FromStr::from_str(&s).map_err(serde::de::Error::custom)
    }
}

impl Deref for RtspUrl {
    type Target = Url;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

#[derive(Debug, Error)]
pub enum DecoderError {
    #[error("dropped frames")]
    DroppedFrames,

    #[error("{0}")]
    SendFrame(#[from] SendPacketError),

    #[error("receive frame: {0}")]
    ReceiveFrame(#[from] ReceiveFrameError),

    #[error("try from: {0}")]
    TryFrom(#[from] std::num::TryFromIntError),

    #[error("frame rate limiter: {0}")]
    FrameRateLimiter(#[from] FrameRateLimiterError),
}

#[derive(Debug, Error)]
pub enum SubscribeDecodedError {
    #[error("new h264 decoder: {0}")]
    NewH264Decoder(#[from] H264BuilderError),
}

// A 'broadcast' channel is used instead of a 'watch' channel to detect dropped frames.
pub type Feed = broadcast::Receiver<H264Data>;
pub type FeedDecoded = mpsc::Receiver<Result<Frame, DecoderError>>;

pub type ArcSource = Arc<dyn Source + Send + Sync>;

#[async_trait]
pub trait Source {
    #[must_use]
    fn stream_type(&self) -> &StreamType;

    // Returns the HLS muxer for this source. Will block until the source has started.
    // Returns None if cancelled.
    async fn muxer(&self) -> Option<ArcStreamerMuxer>;

    // Subscribe to the raw feed. Will block until the source has started.
    async fn subscribe(&self) -> Option<Feed>;

    // Subscribe to a decoded feed. Currently creates a new decoder for each
    // call but this may change. Will block until the source has started.
    // Will close channel when cancelled.
    async fn subscribe_decoded(
        &self,
        rt_handle: Handle,
        logger: ArcMsgLogger,
        limiter: Option<FrameRateLimiter>,
    ) -> Option<Result<FeedDecoded, SubscribeDecodedError>>;
}

pub type ArcMonitor = Arc<dyn MonitorImpl + Send + Sync>;

#[async_trait]
pub trait MonitorImpl {
    fn config(&self) -> &MonitorConfig;

    async fn stop(&self) {}

    // Return sub stream if it exists otherwise returns main stream.
    // Returns None if cancelled
    async fn get_smallest_source(&self) -> Option<ArcSource>;

    // Returns None if cancelled.
    async fn source_main(&self) -> Option<ArcSource>;

    // Returns None if cancelled and Some(None) if sub stream doesn't exist.
    async fn source_sub(&self) -> Option<Option<ArcSource>>;

    async fn trigger(
        &self,
        trigger_duration: Duration,
        event: Event,
    ) -> Result<(), CreateEventDbError>;
}

pub type ArcMonitorHooks = Arc<dyn MonitorHooks + Send + Sync>;

#[async_trait]
pub trait MonitorHooks {
    async fn on_monitor_start(&self, token: CancellationToken, monitor: ArcMonitor);
    // Blocking.
    fn on_thumb_save(&self, config: &MonitorConfig, frame: Frame) -> Frame;
    async fn on_event(&self, event: Event, config: MonitorConfig);
}

#[derive(Debug, Error)]
pub enum MonitorRestartError {
    #[error("monitor does not exist '{0}'")]
    NotExist(String),
}

#[derive(Debug, Error)]
pub enum MonitorSetError {
    #[error("serialize config: {0}")]
    Serialize(#[from] serde_json::Error),

    #[error("write config to file: {0}")]
    WriteFile(std::io::Error),
}

#[derive(Debug, Error)]
pub enum MonitorSetAndRestartError {
    #[error(transparent)]
    Set(MonitorSetError),

    #[error(transparent)]
    Restart(MonitorRestartError),
}

#[derive(Debug, Error)]
pub enum MonitorDeleteError {
    #[error("monitor does not exist '{0}'")]
    NotExist(String),

    #[error("remove file: {0}")]
    RemoveFile(#[from] std::io::Error),
}

#[derive(Debug, Error)]
pub enum CreateEventDbError {
    #[error("create eventdb directory: {0} {1}")]
    CreateDir(PathBuf, std::io::Error),
}

#[derive(Debug, Serialize, PartialEq, Eq)]
pub struct MonitorInfo {
    id: MonitorId,
    name: MonitorName,
    enable: bool,

    #[serde(rename = "hasSubStream")]
    has_sub_stream: bool,
}

impl MonitorInfo {
    #[must_use]
    pub fn new(id: MonitorId, name: MonitorName, enable: bool, has_sub_stream: bool) -> Self {
        Self {
            id,
            name,
            enable,
            has_sub_stream,
        }
    }
}

pub type ArcMonitorManager = Arc<dyn IMonitorManager + Send + Sync>;

#[async_trait]
pub trait IMonitorManager {
    async fn start_monitors(&self, hooks: ArcMonitorHooks);
    async fn monitor_restart(&self, monitor_id: MonitorId) -> Result<(), MonitorRestartError>;
    async fn monitor_set(&self, config: MonitorConfig) -> Result<bool, MonitorSetError>;
    async fn monitor_set_and_restart(
        &self,
        config: MonitorConfig,
    ) -> Result<bool, MonitorSetAndRestartError>;
    async fn monitor_delete(&self, id: MonitorId) -> Result<(), MonitorDeleteError>;
    async fn monitors_info(&self) -> HashMap<MonitorId, MonitorInfo>;
    async fn monitor_config(&self, monitor_id: MonitorId) -> Option<MonitorConfig>;
    async fn monitor_configs(&self) -> MonitorConfigs;
    async fn stop(&self);
    async fn monitor_is_running(&self, monitor_id: MonitorId) -> bool;
}

#[async_trait]
pub trait H264WriterImpl {
    // TODO: replace &mut with &.
    async fn write_h264(&mut self, data: H264Data) -> Result<(), DynError>;
}

pub type DynH264Writer = Box<dyn H264WriterImpl + Send>;

#[async_trait]
pub trait StreamerImpl {
    // Creates muxer and returns a H264Writer to it.
    // Stops and replaces existing muxer if present.
    // Returns None if cancelled.
    async fn new_muxer(
        &self,
        token: CancellationToken,
        monitor_id: MonitorId,
        sub_stream: bool,
        params: TrackParameters,
        start_time: UnixH264,
        first_sample: H264Data,
    ) -> Result<Option<(ArcStreamerMuxer, DynH264Writer)>, DynError>;
}

pub type ArcStreamer = Arc<dyn StreamerImpl + Send + Sync>;
