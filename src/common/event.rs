// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{
    impl_deserialize_try_from_and_display,
    time::{Duration, UnixNano, UnixNanoString},
};
use serde::{Deserialize, Serialize};
use std::num::NonZeroU32;
use thiserror::Error;

// Recording trigger event.
#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
pub struct Event {
    pub time: UnixNano,
    pub duration: Duration,
    pub detections: Detections,

    // BREAKING: make this mandatory.
    pub source: Option<EventSource>,
}

// Recording trigger event.
#[derive(Clone, Debug, PartialEq, Serialize)]
pub struct Event2 {
    pub time: UnixNanoString,
    pub duration: Duration,
    pub detections: Detections,

    // BREAKING: make this mandatory.
    pub source: Option<EventSource>,
}

impl From<Event> for Event2 {
    fn from(value: Event) -> Self {
        Self {
            time: value.time.into(),
            duration: value.duration,
            detections: value.detections,
            source: value.source,
        }
    }
}

pub type Detections = Vec<Detection>;

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
pub struct Detection {
    pub label: Label,
    pub score: f32,
    pub region: Region,
}

// Region where detection occurred.
#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct Region {
    pub rectangle: Option<RectangleNormalized>,
    pub polygon: Option<PolygonNormalized>,
}

// 1_000_000 is 100%
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct RectangleNormalized {
    pub x: u32,
    pub y: u32,
    pub width: NonZeroU32,
    pub height: NonZeroU32,
}

pub type Polygon = Vec<Point>;
pub type PolygonNormalized = Vec<PointNormalized>;

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct Point {
    pub x: u16,
    pub y: u16,
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct PointNormalized {
    pub x: u32,
    pub y: u32,
}

pub type Labels = Vec<Label>;

#[derive(Clone, Debug, Hash, PartialEq, Eq, Serialize)]
pub struct Label(String);
impl_deserialize_try_from_and_display!(Label);

#[derive(Debug, Error, PartialEq, Eq)]
pub enum ParseLabelError {
    #[error("max length 64: {0}")]
    Len(String),

    #[error("bad char: '{0}'")]
    BadChar(char),
}

impl TryFrom<String> for Label {
    type Error = ParseLabelError;

    fn try_from(s: String) -> Result<Self, Self::Error> {
        let chars: Vec<char> = s.chars().collect();
        if chars.len() > 64 {
            return Err(Self::Error::Len(s));
        }
        for c in chars {
            if c != ' ' && !c.is_alphanumeric() {
                return Err(Self::Error::BadChar(c));
            }
        }
        Ok(Self(s))
    }
}

const EVENT_SOURCE_MAX_LENGTH: usize = 7;

#[repr(transparent)]
#[allow(clippy::module_name_repetitions)]
#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
pub struct EventSource(String);
impl_deserialize_try_from_and_display!(EventSource);

#[derive(Debug, Error, PartialEq, Eq)]
pub enum ParseEventSourceError {
    // Feel free to increase.
    #[error("max length {EVENT_SOURCE_MAX_LENGTH}: {0}")]
    Len(String),

    #[error("bad char: '{0}'")]
    BadChar(char),
}

impl TryFrom<String> for EventSource {
    type Error = ParseEventSourceError;

    fn try_from(s: String) -> Result<Self, Self::Error> {
        let chars: Vec<char> = s.chars().collect();
        if chars.len() > EVENT_SOURCE_MAX_LENGTH {
            return Err(Self::Error::Len(s));
        }
        for c in chars {
            if c != ' ' && !c.is_alphanumeric() {
                return Err(Self::Error::BadChar(c));
            }
        }
        Ok(Self(s))
    }
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {
    use super::*;

    #[test]
    fn test_label() {
        Label::try_from("abc".to_owned()).unwrap();
        Label::try_from("123".to_owned()).unwrap();
        Label::try_from("1a2b".to_owned()).unwrap();
        Label::try_from(
            "1234567890123456789012345678901234567890123456789012345678901234".to_owned(),
        )
        .unwrap();
        Label::try_from(
            "12345678901234567890123456789012345678901234567890123456789012345".to_owned(),
        )
        .unwrap_err();
        Label::try_from("<".to_owned()).unwrap_err();
        Label::try_from("{".to_owned()).unwrap_err();
    }

    #[test]
    fn test_event_source() {
        EventSource::try_from("abcdefg".to_owned()).unwrap();
        EventSource::try_from("1234567".to_owned()).unwrap();
        EventSource::try_from("1a2b3c".to_owned()).unwrap();
        EventSource::try_from("12345678".to_owned()).unwrap_err();
        EventSource::try_from("<".to_owned()).unwrap_err();
        EventSource::try_from("{".to_owned()).unwrap_err();
    }
}
