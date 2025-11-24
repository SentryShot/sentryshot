// SPDX-License-Identifier: GPL-2.0-or-later

use async_trait::async_trait;
use common::{
    ArcMsgLogger, Detection, Detections, DynError, LogLevel, RectangleNormalized, Region,
    recording::normalize,
};
use plugin::object_detection::{
    ArcDetector, Detector, DetectorName, DynOpenVinoServerBackend, OpenVinoServerBackend,
};
use std::{
    ffi::c_char,
    num::{NonZeroU16, NonZeroU32},
    sync::Arc,
};
use tokio::runtime::Handle;
use tokio_util::task::task_tracker::TaskTrackerToken;

#[unsafe(no_mangle)]
pub extern "C" fn version() -> *const c_char {
    plugin::get_version()
}

#[unsafe(no_mangle)]
pub extern "Rust" fn new_open_vino_server_backend(rt_handle: Handle) -> DynOpenVinoServerBackend {
    Box::new(OpenVinoServerBackendImpl { rt_handle })
}

pub(crate) struct OpenVinoServerBackendImpl {
    rt_handle: Handle,
}

impl OpenVinoServerBackend for OpenVinoServerBackendImpl {
    fn new_open_vino_server_detector(
        &self,
        _task_token: &TaskTrackerToken,
        logger: &ArcMsgLogger,
        name: &DetectorName,
        _host: String,
        _input_tensor: String,
        _output_tensor: String,
        input_width: NonZeroU16,
        input_height: NonZeroU16,
    ) -> Result<ArcDetector, DynError> {
        logger.log(LogLevel::Info, &format!("starting detector '{name}'"));

        Ok(Arc::new(OpenVinoServerDetector {
            _rt_handle: self.rt_handle.clone(),
            input_width,
            input_height,
        }))
    }
}

struct OpenVinoServerDetector {
    _rt_handle: Handle,
    input_width: NonZeroU16,
    input_height: NonZeroU16,
}

#[async_trait]
impl Detector for OpenVinoServerDetector {
    async fn detect(&self, _data: Vec<u8>) -> Result<Option<Detections>, DynError> {
        Ok(Some(vec![Detection {
            label: "person".to_owned().try_into().expect(""),
            score: 100.0,
            region: Region {
                rectangle: Some(RectangleNormalized {
                    x: normalize(10, 100),
                    y: normalize(10, 100),
                    width: NonZeroU32::new(normalize(40, 100)).expect(""),
                    height: NonZeroU32::new(normalize(40, 100)).expect(""),
                }),
                polygon: None,
            },
        }]))
    }
    fn width(&self) -> NonZeroU16 {
        self.input_width
    }
    fn height(&self) -> NonZeroU16 {
        self.input_height
    }
}
