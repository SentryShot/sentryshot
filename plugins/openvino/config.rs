// SPDX-License-Identifier: GPL-2.0-or-later

use common::{
    ArcMsgLogger,
    recording::{DurationSec, FeedRateSec, denormalize},
};
use serde::Deserialize;
use serde_json::Value;
use std::num::NonZeroU16;
use thiserror::Error;

#[derive(Clone, Debug, PartialEq)]
pub(crate) struct OpenvinoConfig {
    pub(crate) feed_rate: FeedRateSec,
    pub(crate) trigger_duration: DurationSec,
    pub(crate) use_sub_stream: bool,
    pub(crate) crop: Crop,
}

impl OpenvinoConfig {
    pub(crate) fn parse(
        raw: serde_json::Value,
        _logger: ArcMsgLogger,
    ) -> Result<Option<Self>, serde_json::Error> {
        #[derive(Deserialize)]
        struct Temp {
            openvino: Value,
        }
        let Ok(temp) = serde_json::from_value::<Temp>(raw) else {
            return Ok(None);
        };
        if temp.openvino.is_null() {
            return Ok(None);
        }

        #[derive(Deserialize)]
        struct RawConfig {
            enable: bool,
            #[serde(rename = "feedRate")]
            feed_rate: FeedRateSec,
            duration: DurationSec,
            #[serde(rename = "useSubStream")]
            use_sub_stream: bool,
            crop: Crop,
        }

        let config: RawConfig = serde_json::from_value(temp.openvino)?;
        if !config.enable {
            return Ok(None);
        }

        Ok(Some(Self {
            feed_rate: config.feed_rate,
            trigger_duration: config.duration,
            use_sub_stream: config.use_sub_stream,
            crop: config.crop,
        }))
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
