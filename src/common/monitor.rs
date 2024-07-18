// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{
    time::{Duration, MINUTE},
    MonitorId, NonEmptyString,
};
use serde::{Deserialize, Deserializer, Serialize, Serializer};
use std::{collections::HashMap, ops::Deref, str::FromStr};
use thiserror::Error;
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
    pub fn name(&self) -> &NonEmptyString {
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
    pub name: NonEmptyString,
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
