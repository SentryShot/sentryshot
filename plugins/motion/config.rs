// SPDX-License-Identifier: GPL-2.0-or-later

use common::PolygonNormalized;
use recording::{DurationSec, FeedRateSec};
use serde::Deserialize;

#[derive(Clone, Debug, PartialEq)]
pub(crate) struct MotionConfig {
    pub(crate) feed_rate: FeedRateSec,
    pub(crate) duration: DurationSec,
    //scale           int
    pub(crate) zones: Vec<ZoneConfig>,
}

impl MotionConfig {
    // Returns `None` if unset or disabled.
    pub(crate) fn parse(raw: serde_json::Value) -> Result<Option<Self>, serde_json::Error> {
        #[derive(Deserialize)]
        struct Temp {
            motion: serde_json::Value,
        }
        let Ok(temp) = serde_json::from_value::<Temp>(raw) else {
            return Ok(None)
        };
        if temp.motion == serde_json::Value::Object(serde_json::Map::new()) {
            return Ok(None);
        }

        let config = RawConfigV0::from_raw_motion(temp.motion)?;
        if !config.enable {
            return Ok(None);
        }

        Ok(Some(Self {
            feed_rate: config.feed_rate,
            duration: config.duration,
            zones: config.zones,
        }))
    }
}

#[derive(Debug, Deserialize, PartialEq)]
struct RawConfigV0 {
    enable: bool,

    #[serde(rename = "feedRate")]
    feed_rate: FeedRateSec,
    //#[serde(rename = "frameScale")]
    //frame_scale: String,
    //FrameScale string `json:"frameScale"`
    duration: DurationSec,
    zones: Vec<ZoneConfig>,
}

impl RawConfigV0 {
    pub(crate) fn from_raw_motion(
        raw_motion: serde_json::Value,
    ) -> Result<RawConfigV0, serde_json::Error> {
        let config: RawConfigV0 = serde_json::from_value(raw_motion)?;
        Ok(config)
    }
}

#[derive(Clone, Debug, Deserialize, PartialEq)]
pub(crate) struct ZoneConfig {
    pub(crate) enable: bool,
    pub(crate) sensitivity: f64,

    #[serde(rename = "thresholdMin")]
    pub(crate) threshold_min: f32, //`json:"thresholdMin"`a
    //
    #[serde(rename = "thresholdMax")]
    pub(crate) threshold_max: f32, //`json:"thresholdMax"`

    pub(crate) area: PolygonNormalized,
}

#[cfg(test)]
mod tests {
    use super::*;
    use common::{time::Duration, PointNormalized};
    use pretty_assertions::assert_eq;
    use serde_json::json;

    #[test]
    fn test_parse() {
        let raw = json!({
            "motion": {
                "enable":     true,
                "feedRate":   5,
                "frameScale": "full",
                "duration":   6,
                "zones":[
                    {
                        "enable": true,
                        "sensitivity": 7,
                        "thresholdMin": 8,
                        "thresholdMax": 9,
                        "area":[[10,11],[12,13],[14,15]]
                    }
                ]
            }
        });

        let got = MotionConfig::parse(raw).unwrap().unwrap();
        let want = MotionConfig {
            feed_rate: FeedRateSec::new(Duration::from_millis(200)),
            duration: DurationSec::new(Duration::from_secs(6)),
            zones: vec![ZoneConfig {
                enable: true,
                sensitivity: 7.0,
                threshold_min: 8.0,
                threshold_max: 9.0,
                area: vec![
                    PointNormalized { x: 10, y: 11 },
                    PointNormalized { x: 12, y: 13 },
                    PointNormalized { x: 14, y: 15 },
                ],
            }],
        };

        assert_eq!(want, got)
    }

    #[test]
    fn test_parse_empty() {
        let raw = serde_json::Value::String("".to_owned());
        assert!(MotionConfig::parse(raw).unwrap().is_none());
    }

    #[test]
    fn test_parse_config_empty2() {
        let raw = json!({"motion": {}});
        assert!(MotionConfig::parse(raw).unwrap().is_none());
    }

    #[test]
    fn test_parse_disabled() {
        let raw = json!({
            "motion": {
                "enable":     false,
                "feedRate":   5,
                "frameScale": "full",
                "duration":   6,
                "zones":[
                    {
                        "enable": true,
                        "sensitivity": 7,
                        "thresholdMin": 8,
                        "thresholdMax": 9,
                        "area":[[10,11],[12,13],[14,15]]
                    }
                ]
            }
        });
        assert!(MotionConfig::parse(raw).unwrap().is_none());
    }
}
