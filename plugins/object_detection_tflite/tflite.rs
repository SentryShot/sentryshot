use async_trait::async_trait;
use common::{ArcMsgLogger, Detections, DynError, LogLevel, RectangleNormalized, Region};
use plugin::object_detection::{
    ArcTfliteDetector, DetectorName, DynTfliteBackend, LabelMap, TfliteBackend, TfliteDetector,
    TfliteFormat,
};
use std::{
    ffi::c_char,
    num::{NonZeroU8, NonZeroU16, NonZeroU32},
    path::Path,
    sync::Arc,
};
use tflite_lib::{
    EdgetpuDevice, ModelFormat, NewDetectorError, debug_device, edgetpu_verbosity,
    list_edgetpu_devices,
};
use thiserror::Error;
use tokio::{runtime::Handle, sync::oneshot};
use tokio_util::task::task_tracker::TaskTrackerToken;

#[unsafe(no_mangle)]
pub extern "C" fn version() -> *const c_char {
    plugin::get_version()
}

#[unsafe(no_mangle)]
pub extern "Rust" fn new_tflite_backend(rt_handle: Handle) -> DynTfliteBackend {
    edgetpu_verbosity(get_log_level());
    Box::new(TfliteBackendImpl {
        rt_handle,
        device_cache: DeviceCache::new(),
    })
}

pub(crate) struct TfliteBackendImpl {
    rt_handle: Handle,
    device_cache: DeviceCache,
}

impl TfliteBackend for TfliteBackendImpl {
    #[allow(clippy::too_many_arguments)]
    fn new_tflite_detector(
        &self,
        task_token: &TaskTrackerToken,
        logger: &ArcMsgLogger,
        name: &DetectorName,
        width: NonZeroU16,
        height: NonZeroU16,
        model_path: &Path,
        format: TfliteFormat,
        label_map: &LabelMap,
        threads: NonZeroU8,
    ) -> Result<ArcTfliteDetector, DynError> {
        let (detect_tx, detect_rx) = async_channel::bounded::<DetectRequest>(1);
        for i in 0..threads.get() {
            logger.log(LogLevel::Info, &format!("starting detector '{name}' T{i}"));
            let task_token = task_token.clone();
            let rt_handle2 = self.rt_handle.clone();
            let detect_rx = detect_rx.clone();
            let mut detector = tflite_lib::TfliteDetector::new(
                model_path,
                convert_format(format),
                None,
                width,
                height,
            )?;
            let label_map = label_map.clone();

            self.rt_handle.spawn(async move {
                let _task_token = task_token;
                while let Ok(mut req) = detect_rx.recv().await {
                    let result;
                    (detector, result) = rt_handle2
                        .spawn_blocking(move || {
                            let result = detector.detect(&mut req.data);
                            (detector, result)
                        })
                        .await
                        .expect("join");
                    let result = result.map(|v| parse_detections(&label_map, v));
                    _ = req.res.send(result);
                }
            });
        }
        Ok(Arc::new(TfliteDetectorImpl {
            rt_handle: self.rt_handle.clone(),
            detect_tx,
            width,
            height,
        }))
    }

    #[allow(clippy::too_many_arguments)]
    fn new_edgetpu_detector(
        &mut self,
        task_token: TaskTrackerToken,
        logger: &ArcMsgLogger,
        name: &DetectorName,
        width: NonZeroU16,
        height: NonZeroU16,
        model_path: &Path,
        format: TfliteFormat,
        label_map: LabelMap,
        device_path: String,
    ) -> Result<ArcTfliteDetector, DynError> {
        let device_cache: &mut DeviceCache = &mut self.device_cache;
        logger.log(LogLevel::Info, &format!("starting detector '{name}'"));

        let Some(device) = device_cache.device(&device_path) else {
            let err = debug_device(device_path, device_cache.devices());
            return Err(NewDetectorError::DebugDevice(err).into());
        };
        let mut detector = match tflite_lib::TfliteDetector::new(
            model_path,
            convert_format(format),
            Some(device),
            width,
            height,
        ) {
            Ok(v) => v,
            Err(e) => {
                if matches!(e, NewDetectorError::EdgetpuDelegateCreate) {
                    let _ = debug_device(device_path, device_cache.devices());
                }
                return Err(Box::new(e));
            }
        };

        let (detect_tx, detect_rx) = async_channel::bounded::<DetectRequest>(1);
        let rt_handle2 = self.rt_handle.clone();
        self.rt_handle.spawn(async move {
            let _task_token = task_token;
            while let Ok(mut req) = detect_rx.recv().await {
                let result;
                (detector, result) = rt_handle2
                    .spawn_blocking(move || {
                        let result = detector.detect(&mut req.data);
                        (detector, result)
                    })
                    .await
                    .expect("join");
                let result = result.map(|v| parse_detections(&label_map, v));
                _ = req.res.send(result);
            }
        });
        Ok(Arc::new(TfliteDetectorImpl {
            rt_handle: self.rt_handle.clone(),
            detect_tx,
            width,
            height,
        }))
    }
}

fn parse_detections(label_map: &LabelMap, input: Vec<tflite_lib::Detection>) -> Detections {
    let get_label = |class| {
        if let Some(label) = label_map.get(&class) {
            label.to_owned()
        } else {
            #[allow(clippy::unwrap_used)]
            format!("unknown{class}").try_into().unwrap()
        }
    };
    input
        .into_iter()
        .filter_map(|d| {
            let rect = parse_rect(d.top, d.left, d.bottom, d.right)?;
            Some(common::Detection {
                label: get_label(d.class),
                score: d.score * 100.0,
                region: Region {
                    rectangle: Some(rect),
                    polygon: None,
                },
            })
        })
        .collect()
}

fn parse_rect(top: f32, left: f32, bottom: f32, right: f32) -> Option<RectangleNormalized> {
    #[allow(
        clippy::cast_sign_loss,
        clippy::cast_possible_truncation,
        clippy::as_conversions
    )]
    fn scale(v: f32) -> u32 {
        (v * 1_000_000.0) as u32
    }
    let top = scale(top);
    let left = scale(left);
    let bottom = scale(bottom);
    let right = scale(right);
    if top > bottom || left > right {
        return None;
    }
    Some(RectangleNormalized {
        x: left,
        y: top,
        width: NonZeroU32::new(right - left)?,
        height: NonZeroU32::new(bottom - top)?,
    })
}

struct TfliteDetectorImpl {
    rt_handle: Handle,
    detect_tx: async_channel::Sender<DetectRequest>,
    width: NonZeroU16,
    height: NonZeroU16,
}

#[async_trait]
impl TfliteDetector for TfliteDetectorImpl {
    #[allow(clippy::similar_names)]
    async fn detect(&self, data: Vec<u8>) -> Result<Option<Detections>, DynError> {
        use DetectError::*;
        let (res_tx, res_rx) = oneshot::channel();
        let req = DetectRequest { data, res: res_tx };

        let sleep = |secs: u64| {
            let _enter = self.rt_handle.enter();
            tokio::time::sleep(std::time::Duration::from_secs(secs))
        };
        tokio::select!(
            _ = self.detect_tx.send(req) => {},
            () = sleep(1) => return Err(Box::new(DetectorTimeout)),
        );

        let res = tokio::select!(
            v = res_rx => v,
            () = sleep(3) => return Err(Box::new(DetectionTimeout)),
        );
        if let Ok(res) = res {
            Ok(Some(res?))
        } else {
            // Detector was dropped.
            Ok(None)
        }
    }
    fn width(&self) -> NonZeroU16 {
        self.width
    }
    fn height(&self) -> NonZeroU16 {
        self.height
    }
}

#[derive(Debug)]
struct DetectRequest {
    data: Vec<u8>,
    res: oneshot::Sender<Result<Detections, tflite_lib::DetectError>>,
}

#[derive(Debug, Error)]
pub(crate) enum DetectError {
    #[error["{0}"]]
    Detect(#[from] tflite_lib::DetectError),

    #[error("detector did not repond in 1 second")]
    DetectorTimeout,

    #[error("detection took longer than 3 second")]
    DetectionTimeout,
}

pub(crate) struct DeviceCache(Option<Vec<EdgetpuDevice>>);

impl DeviceCache {
    pub(crate) fn new() -> Self {
        Self(None)
    }
    fn devices(&mut self) -> &[EdgetpuDevice] {
        self.0.get_or_insert_with(list_edgetpu_devices)
    }
    fn device(&mut self, path: &str) -> Option<&EdgetpuDevice> {
        self.devices().iter().find(|device| device.path == path)
    }
}

fn convert_format(val: TfliteFormat) -> ModelFormat {
    match val {
        TfliteFormat::OdAPi => ModelFormat::OdAPi,
        TfliteFormat::Nolo => ModelFormat::Nolo,
    }
}

fn get_log_level() -> u8 {
    if let Ok(log_level) = std::env::var("EDGETPU_LOG_LEVEL") {
        let log_level: u8 = log_level
            .parse()
            .expect("EDGETPU_LOG_LEVEL is not a valid number");
        assert!(
            log_level <= 10,
            "EDGETPU_LOG_LEVEL is not a number between 0 and 10"
        );
        log_level
    } else {
        0
    }
}
