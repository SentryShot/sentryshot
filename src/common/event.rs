// SPDX-License-Identifier: GPL-2.0-or-later

use crate::time::{Duration, UnixNano};
use serde::{Deserialize, Serialize};
use std::{fmt::Display, num::NonZeroU32, ops::Deref};
use thiserror::Error;

// Recording trigger event.
#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
pub struct Event {
    pub time: UnixNano,
    pub duration: Duration,

    #[serde(skip)]
    pub rec_duration: Duration,

    pub detections: Detections,
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

#[derive(Clone, Debug, Hash, PartialEq, Eq)]
pub struct Label(String);

#[derive(Debug, Error, PartialEq, Eq)]
pub enum ParseLabelError {
    #[error("bad char: '{0}'")]
    BadChar(char),
}

impl TryFrom<String> for Label {
    type Error = ParseLabelError;

    fn try_from(s: String) -> Result<Self, Self::Error> {
        for c in s.chars() {
            if c != ' ' && !c.is_alphanumeric() {
                return Err(Self::Error::BadChar(c));
            }
        }
        Ok(Self(s))
    }
}

impl Serialize for Label {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        self.0.serialize(serializer)
    }
}

impl<'de> Deserialize<'de> for Label {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        String::deserialize(deserializer)?
            .try_into()
            .map_err(serde::de::Error::custom)
    }
}

impl Deref for Label {
    type Target = str;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

impl Display for Label {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.0)
    }
}
