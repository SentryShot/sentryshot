// SPDX-License-Identifier: GPL-2.0-or-later

use async_trait::async_trait;
use common::{ArcMsgLogger, Detections, DynError, LogLevel};
use std::num::NonZeroU16;
use std::sync::Arc;

#[async_trait]
pub(crate) trait Detector: Send + Sync {
    async fn detect(&self, data: Vec<u8>) -> Result<Option<Detections>, DynError>;
    fn width(&self) -> NonZeroU16;
    fn height(&self) -> NonZeroU16;
}

pub(crate) type ArcDetector = Arc<dyn Detector>;

pub(crate) struct DummyDetector {
    logger: ArcMsgLogger,
    width: NonZeroU16,
    height: NonZeroU16,
}

impl DummyDetector {
    pub(crate) fn new(logger: ArcMsgLogger) -> Self {
        Self {
            logger,
            width: NonZeroU16::new(300).unwrap(),
            height: NonZeroU16::new(300).unwrap(),
        }
    }
}

#[async_trait]
impl Detector for DummyDetector {
    async fn detect(&self, _data: Vec<u8>) -> Result<Option<Detections>, DynError> {
        self.logger
            .log(LogLevel::Info, "DummyDetector: received detection request.");
        Ok(Some(Vec::new()))
    }

    fn width(&self) -> NonZeroU16 {
        self.width
    }

    fn height(&self) -> NonZeroU16 {
        self.height
    }
}

pub(crate) struct DetectorManager {
    detector: ArcDetector,
}

impl DetectorManager {
    pub(crate) fn new(logger: ArcMsgLogger) -> Self {
        Self {
            detector: Arc::new(DummyDetector::new(logger)),
        }
    }

    pub(crate) fn get_detector(&self) -> ArcDetector {
        self.detector.clone()
    }
}
