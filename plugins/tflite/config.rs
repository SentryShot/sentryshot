// SPDX-License-Identifier: GPL-2.0-or-later

use crate::detector::{DetectorName, Thresholds};
use common::PolygonNormalized;
use recording::{DurationSec, FeedRateSec};
use serde::Deserialize;
use std::ops::Deref;
use thiserror::Error;

#[derive(Clone, Debug, PartialEq)]
pub(crate) struct TfliteConfig {
    //timestampOffset: time.Duration,
    pub thresholds: Thresholds,
    pub crop: Crop,
    pub mask: Mask,
    pub detector_name: DetectorName,
    pub feed_rate: FeedRateSec,
    pub duration: DurationSec,
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
    pub(crate) fn parse(raw: serde_json::Value) -> Result<Option<TfliteConfig>, serde_json::Error> {
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

        //timestampOffset, err := ffmpeg.ParseTimestampOffset(c.Get("timestampOffset"))

        Ok(Some(TfliteConfig {
            //timestampOffset: timestampOffset,
            thresholds: c.thresholds,
            crop: c.crop,
            mask: c.mask,
            detector_name: c.detector_name,
            feed_rate: c.feed_rate,
            duration: c.duration,
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
    pub(crate) fn new(v: u8) -> Result<Self, ParsePercentError> {
        if v > 100 {
            Err(ParsePercentError)
        } else {
            Ok(Self(v))
        }
    }

    pub(crate) fn as_f32(self) -> f32 {
        f32::from(self.0)
    }
}

impl<'de> Deserialize<'de> for Percent {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        let s = u8::deserialize(deserializer)?;
        Self::new(s).map_err(serde::de::Error::custom)
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
    pub x: u32,
    pub y: u32,
    pub size: u32,
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use common::{time::Duration, PointNormalized};
    use pretty_assertions::assert_eq;
    use serde_json::json;
    use std::collections::HashMap;

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

        let got = TfliteConfig::parse(raw).unwrap().unwrap();

        let want = TfliteConfig {
            thresholds: HashMap::from([("5".parse().unwrap(), Percent::new(6).unwrap())]),
            crop: Crop {
                x: 7,
                y: 8,
                size: 9,
            },
            mask: Mask {
                enable: true,
                area: vec![
                    PointNormalized { x: 10, y: 11 },
                    PointNormalized { x: 12, y: 13 },
                ],
            },
            detector_name: "14".parse().unwrap(),
            feed_rate: FeedRateSec::new(Duration::from_secs(5)),
            duration: DurationSec::new(Duration::from_secs(15)),
            use_sub_stream: true,
        };
        assert_eq!(want, got);
    }

    #[test]
    fn test_parse_config_empty() {
        let raw = serde_json::Value::String(String::new());
        assert!(TfliteConfig::parse(raw).unwrap().is_none());
    }

    #[test]
    fn test_parse_config_empty2() {
        let raw = json!({"tflite": {}});
        assert!(TfliteConfig::parse(raw).unwrap().is_none());
    }
}
