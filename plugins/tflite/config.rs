// SPDX-License-Identifier: GPL-2.0-or-later

use crate::detector::{DetectorName, Thresholds};
use common::{
    monitor::MonitorConfig,
    recording::{denormalize, DurationSec, FeedRateSec},
    ArcMsgLogger, LogLevel, PolygonNormalized,
};
use serde::Deserialize;
use serde_json::Value;
use std::{num::NonZeroU16, ops::Deref};
use thiserror::Error;

#[derive(Clone, Debug, PartialEq)]
pub(crate) struct TfliteConfig {
    //timestampOffset: time.Duration,
    pub thresholds: Thresholds,
    pub crop: Crop,
    pub mask: Mask,
    pub detector_name: DetectorName,
    pub feed_rate: FeedRateSec,
    pub trigger_duration: DurationSec,
    pub use_sub_stream: bool,
}

#[derive(Deserialize)]
struct RawConfigV1 {
    enable: bool,
    thresholds: Thresholds,
    crop: Crop,
    mask: Mask,

    #[serde(rename = "detectorName")]
    detector_name: DetectorName,

    #[serde(rename = "feedRate")]
    feed_rate: FeedRateSec,
    // Trigger duration.
    duration: DurationSec,

    #[serde(rename = "useSubStream")]
    use_sub_stream: bool,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Eq)]
pub(crate) struct Mask {
    pub enable: bool,
    pub area: PolygonNormalized,
}

impl TfliteConfig {
    #[allow(clippy::needless_pass_by_value)]
    pub(crate) fn parse(
        raw: serde_json::Value,
        logger: ArcMsgLogger,
    ) -> Result<Option<TfliteConfig>, serde_json::Error> {
        #[derive(Deserialize)]
        struct Temp {
            tflite: serde_json::Value,
        }
        let Ok(temp) = serde_json::from_value::<Temp>(raw) else {
            return Ok(None);
        };
        if temp.tflite == serde_json::Value::Object(serde_json::Map::new()) {
            return Ok(None);
        }

        let c: RawConfigV1 = serde_json::from_value(temp.tflite)?;

        let enable = c.enable;
        if !enable {
            return Ok(None);
        }

        if c.thresholds.is_empty() {
            logger.log(LogLevel::Warning, "no thresholds are set");
        }

        //timestampOffset, err := ffmpeg.ParseTimestampOffset(c.Get("timestampOffset"))

        Ok(Some(TfliteConfig {
            //timestampOffset: timestampOffset,
            thresholds: c.thresholds,
            crop: c.crop,
            mask: c.mask,
            detector_name: c.detector_name,
            feed_rate: c.feed_rate,
            trigger_duration: c.duration,
            use_sub_stream: c.use_sub_stream,
        }))
    }
}

#[derive(Debug, Error)]
#[error("value is greater than 100")]
pub(crate) struct ParsePercentError;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) struct Percent(u8);

impl Percent {
    pub(crate) fn as_f32(self) -> f32 {
        f32::from(self.0)
    }
}

impl TryFrom<u8> for Percent {
    type Error = ParsePercentError;

    fn try_from(value: u8) -> Result<Self, Self::Error> {
        if value > 100 {
            Err(ParsePercentError)
        } else {
            Ok(Self(value))
        }
    }
}

impl<'de> Deserialize<'de> for Percent {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        u8::deserialize(deserializer)?
            .try_into()
            .map_err(serde::de::Error::custom)
    }
}

impl Deref for Percent {
    type Target = u8;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

#[derive(Clone, Copy, Debug, Deserialize, PartialEq, Eq)]
pub(crate) struct Crop {
    pub x: CropValue,
    pub y: CropValue,
    pub size: CropSize,
}

#[repr(transparent)]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) struct CropValue(u16);

impl CropValue {
    #[cfg(test)]
    pub(crate) fn new_testing(value: u16) -> Self {
        Self(value)
    }

    pub(crate) fn get(self) -> u16 {
        self.0
    }
}

#[derive(Debug, Error)]
pub(crate) enum ParseCropValueError {
    #[error("crop size cannot be larger than 99")]
    TooLarge(u16),
}

impl TryFrom<u32> for CropValue {
    type Error = ParseCropValueError;

    fn try_from(value: u32) -> Result<Self, Self::Error> {
        use ParseCropValueError::*;
        let value = denormalize(value, 100);
        if value > 99 {
            return Err(TooLarge(value));
        }
        Ok(Self(value))
    }
}

impl<'de> Deserialize<'de> for CropValue {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        u32::deserialize(deserializer)?
            .try_into()
            .map_err(serde::de::Error::custom)
    }
}

#[repr(transparent)]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) struct CropSize(NonZeroU16);

impl CropSize {
    #[cfg(test)]
    pub(crate) fn new_testing(value: NonZeroU16) -> Self {
        Self(value)
    }

    pub(crate) fn get(self) -> u16 {
        self.0.get()
    }
}

#[derive(Debug, Error)]
pub(crate) enum ParseCropSizeError {
    #[error("crop size cannot be larger than 100")]
    TooLarge(u16),

    #[error("crop size cannot be zero")]
    Zero,
}

impl TryFrom<u32> for CropSize {
    type Error = ParseCropSizeError;

    fn try_from(value: u32) -> Result<Self, Self::Error> {
        use ParseCropSizeError::*;
        let value = denormalize(value, 100);
        if value > 100 {
            return Err(TooLarge(value));
        }
        Ok(Self(NonZeroU16::new(value).ok_or(Zero)?))
    }
}

impl<'de> Deserialize<'de> for CropSize {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        u32::deserialize(deserializer)?
            .try_into()
            .map_err(serde::de::Error::custom)
    }
}

pub(crate) fn set_enable(config: &MonitorConfig, value: bool) -> Option<MonitorConfig> {
    let mut raw = config.raw().clone();
    let Value::Object(root) = &mut raw else {
        return None;
    };
    let Value::Object(tflite) = root.get_mut("tflite")? else {
        return None;
    };
    let Value::Bool(enable) = tflite.get_mut("enable")? else {
        return None;
    };
    *enable = value;

    serde_json::from_value(raw).expect("config should still be valid after toggling")
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use common::{time::Duration, DummyLogger, PointNormalized};
    use pretty_assertions::assert_eq;
    use serde_json::json;
    use std::collections::HashMap;

    fn parse(raw: &serde_json::Value) -> Option<TfliteConfig> {
        TfliteConfig::parse(raw.clone(), DummyLogger::new()).unwrap()
    }

    #[test]
    fn test_parse_config_ok() {
        let raw = json!({
            "tflite": {
                "enable":       true,
                "thresholds":   {"5": 6},
                "crop":         [7, 8, 9],
                "mask":         {"enable": true, "area": [[10,11],[12,13]]},
                "detectorName": "14",
                "feedRate":     0.2,
                "duration":     15,
                "useSubStream": true
            }
        });

        let got = parse(&raw).unwrap();

        let want = TfliteConfig {
            thresholds: HashMap::from([(
                "5".to_owned().try_into().unwrap(),
                6.try_into().unwrap(),
            )]),
            crop: Crop {
                x: 7.try_into().unwrap(),
                y: 8.try_into().unwrap(),
                size: 9.try_into().unwrap(),
            },
            mask: Mask {
                enable: true,
                area: vec![
                    PointNormalized { x: 10, y: 11 },
                    PointNormalized { x: 12, y: 13 },
                ],
            },
            detector_name: "14".to_owned().try_into().unwrap(),
            feed_rate: FeedRateSec::new(Duration::from_secs(5)),
            trigger_duration: DurationSec::new(Duration::from_secs(15)),
            use_sub_stream: true,
        };
        assert_eq!(want, got);
    }

    #[test]
    fn test_parse_config_empty() {
        let raw = serde_json::Value::String(String::new());
        assert!(parse(&raw).is_none());
    }

    #[test]
    fn test_parse_config_empty2() {
        let raw = json!({"tflite": {}});
        assert!(parse(&raw).is_none());
    }

    #[test]
    fn test_set_enable() {
        let y = json!({
            "id": "123",
            "name": "test",
            "enable": false,
            "alwaysRecord": false,
            "videoLength": 0,
            "source": "rtsp",
            "sourcertsp": {
                "protocol": "tcp",
                "mainStream": "rtsp://x"
            },
            "tflite": {
                "enable": true,
                "thresholds": {},
                "crop": { "size": 100, "x": 0, "y": 0 },
                "mask": { "enable": false, "area": [] },
                "detectorName": "test",
                "feedRate": 0,
                "duration": 0,
                "useSubStream": true,
            }
        });
        let config: MonitorConfig = serde_json::from_value(y).unwrap();
        assert!(parse(config.raw()).is_some());

        let config = set_enable(&config, false).unwrap();
        assert!(parse(config.raw()).is_none());

        let config = set_enable(&config, true).unwrap();
        assert!(parse(config.raw()).is_some());
    }
}
