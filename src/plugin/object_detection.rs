// SPDX-License-Identifier: GPL-2.0-or-later

use async_trait::async_trait;
use common::{ArcMsgLogger, Detections, DynError, Label};
use serde::{Deserialize, Serialize};
use std::{
    collections::HashMap,
    num::{NonZeroU8, NonZeroU16},
    ops::Deref,
    path::Path,
    sync::Arc,
};
use thiserror::Error;
use tokio_util::task::task_tracker::TaskTrackerToken;

pub type ArcDetector = Arc<dyn Detector + Send + Sync>;

#[async_trait]
pub trait Detector {
    // Data is a raw rgb24 frame with size determined by the width and height methods.
    // Returns `Ok(None)` when cancelled.
    async fn detect(&self, data: Vec<u8>) -> Result<Option<Detections>, DynError>;
    fn width(&self) -> NonZeroU16;
    fn height(&self) -> NonZeroU16;
}

pub type DynTfliteBackend = Box<dyn TfliteBackend>;

pub trait TfliteBackend {
    #[allow(clippy::too_many_arguments)]
    fn new_tflite_detector(
        &self,
        task_tracker: &TaskTrackerToken,
        logger: &ArcMsgLogger,
        name: &DetectorName,
        width: NonZeroU16,
        height: NonZeroU16,
        model_path: &Path,
        format: TfliteFormat,
        label_map: &LabelMap,
        threads: NonZeroU8,
    ) -> Result<ArcDetector, DynError>;

    #[allow(clippy::too_many_arguments)]
    fn new_edgetpu_detector(
        &mut self,
        task_tracker: TaskTrackerToken,
        logger: &ArcMsgLogger,
        name: &DetectorName,
        width: NonZeroU16,
        height: NonZeroU16,
        model_path: &Path,
        format: TfliteFormat,
        label_map: LabelMap,
        device_path: String,
    ) -> Result<ArcDetector, DynError>;
}

pub type DynOpenVinoServerBackend = Box<dyn OpenVinoServerBackend>;

pub trait OpenVinoServerBackend {
    #[allow(clippy::too_many_arguments)]
    fn new_open_vino_server_detector(
        &self,
        task_token: &TaskTrackerToken,
        logger: &ArcMsgLogger,
        name: &DetectorName,
        host: String,
        input_tensor: String,
        output_tensor: String,
        input_width: NonZeroU16,
        input_height: NonZeroU16,
    ) -> Result<ArcDetector, DynError>;
}

pub type LabelMap = HashMap<u16, Label>;

#[derive(Copy, Clone, Debug, Default, Deserialize, PartialEq, Eq)]
pub enum TfliteFormat {
    #[default]
    #[serde(rename = "odapi")]
    OdAPi,

    #[serde(rename = "nolo")]
    Nolo,
}

#[derive(Clone, Debug, Hash, PartialEq, Eq, Serialize)]
pub struct DetectorName(String);

impl<'de> Deserialize<'de> for DetectorName {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        String::deserialize(deserializer)?
            .try_into()
            .map_err(serde::de::Error::custom)
    }
}

impl std::fmt::Display for DetectorName {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.0)
    }
}

#[derive(Debug, Error, PartialEq, Eq)]
pub enum ParseDetectorNameError {
    #[error("empty string")]
    Empty,

    #[error("bad char: {0}")]
    BadChar(char),

    #[error("white space not allowed")]
    WhiteSpace,
}

impl TryFrom<String> for DetectorName {
    type Error = ParseDetectorNameError;

    fn try_from(s: String) -> Result<Self, Self::Error> {
        if s.is_empty() {
            return Err(Self::Error::Empty);
        }
        for c in s.chars() {
            if c.is_whitespace() {
                return Err(Self::Error::WhiteSpace);
            }
            if !c.is_alphanumeric() && c != '-' && c != '_' {
                return Err(Self::Error::BadChar(c));
            }
        }
        Ok(Self(s))
    }
}

impl Deref for DetectorName {
    type Target = str;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}
