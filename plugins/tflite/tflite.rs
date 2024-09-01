// SPDX-License-Identifier: GPL-2.0-or-later

mod config;
mod detector;
mod label;
mod model;

use crate::{config::TfliteConfig, detector::DetectorManager};
use async_trait::async_trait;
use axum::{
    extract::{Path, State},
    http::StatusCode,
    middleware,
    response::{IntoResponse, Response},
    routing::patch,
    Router,
};
use common::{
    recording::{vertex_inside_poly2, FrameRateLimiter},
    time::{DurationH264, UnixNano},
    Detection, Detections, DynAuth, DynEnvConfig, DynLogger, DynMsgLogger, Event, LogEntry,
    LogLevel, LogSource, MonitorId, MsgLogger, RectangleNormalized, Region,
};
use config::{set_enable, Crop, Mask};
use detector::{DetectError, Detector, DetectorName, Thresholds};
use hyper::{body::HttpBody, http::uri::InvalidUri};
use hyper_rustls::HttpsConnectorBuilder;
use monitor::{DecoderError, Monitor, MonitorManager, Source, SubscribeDecodedError};
use plugin::{
    types::{admin, Assets},
    Application, Plugin, PreLoadPlugin,
};
use sentryshot_convert::{
    ConvertError, Frame, NewConverterError, PixelFormat, PixelFormatConverter,
};
use sentryshot_filter::{crop, pad, CropError, PadError};
use sentryshot_scale::{CreateScalerError, Scaler, ScalerError};
use sentryshot_util::ImageCopyToBufferError;
use std::{
    borrow::Cow,
    num::{NonZeroU16, NonZeroU32, TryFromIntError},
    sync::Arc,
    time::Duration,
};
use thiserror::Error;
use tokio::{io::AsyncWriteExt, runtime::Handle, sync::mpsc};
use tokio_util::sync::CancellationToken;
use url::Url;

#[no_mangle]
pub extern "Rust" fn version() -> String {
    plugin::get_version()
}

#[no_mangle]
pub extern "Rust" fn pre_load() -> Box<dyn PreLoadPlugin> {
    Box::new(PreLoadAuthNone)
}
struct PreLoadAuthNone;
impl PreLoadPlugin for PreLoadAuthNone {
    fn add_log_source(&self) -> Option<LogSource> {
        #[allow(clippy::unwrap_used)]
        Some("tflite".try_into().unwrap())
    }
}

#[no_mangle]
pub extern "Rust" fn load(app: &dyn Application) -> Arc<dyn Plugin> {
    // This is very dirty and may break horribly.
    // Tokio normally forbids multiple runtimes, but plugins have a different
    // static namespace and tokio can't access the globals from the other runtime.
    tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .expect("runtime should start")
        .block_on(async {
            Arc::new(
                TflitePlugin::new(
                    app.rt_handle(),
                    app.shutdown_complete_tx(),
                    app.logger(),
                    app.env(),
                    app.auth(),
                    app.monitor_manager(),
                )
                .await,
            )
        })
}

pub struct TflitePlugin {
    rt_handle: Handle,
    _shutdown_complete_tx: mpsc::Sender<()>,
    logger: DynLogger,
    auth: DynAuth,
    monitor_manager: MonitorManager,
    detector_manager: DetectorManager,
}

impl TflitePlugin {
    async fn new(
        rt_handle: Handle,
        shutdown_complete_tx: mpsc::Sender<()>,
        logger: DynLogger,
        env: DynEnvConfig,
        auth: DynAuth,
        monitor_manager: MonitorManager,
    ) -> Self {
        let tflite_logger = Arc::new(TfliteLogger {
            logger: logger.clone(),
        });
        let detector_manager = match DetectorManager::new(
            rt_handle.clone(),
            shutdown_complete_tx.clone(),
            tflite_logger,
            &Fetch,
            env.config_dir(),
        )
        .await
        {
            Ok(v) => v,
            Err(e) => {
                eprintln!("Failed to create tflite detector manager: {e}");
                std::process::exit(1);
            }
        };

        Self {
            rt_handle,
            _shutdown_complete_tx: shutdown_complete_tx,
            logger,
            auth,
            monitor_manager,
            detector_manager,
        }
    }
}

const TFLITE_MJS_FILE: &[u8] = include_bytes!("./js/tflite.js");

#[async_trait]
impl Plugin for TflitePlugin {
    fn edit_assets(&self, assets: &mut Assets) {
        let detectors = self.detector_manager.detectors();
        let detectors_json = serde_json::to_string_pretty(detectors).expect("infallible");

        let js = assets["scripts/settings.js"].to_vec();
        *assets.get_mut("scripts/settings.js").expect("exist") = Cow::Owned(modify_settings_js(js));

        assets.insert(
            "scripts/tflite.js".to_owned(),
            Cow::Owned(
                String::from_utf8(TFLITE_MJS_FILE.to_owned())
                    .expect("js file should be valid utf8")
                    .replace("$detectorsJSON", &detectors_json)
                    .as_bytes()
                    .to_owned(),
            ),
        );
    }

    async fn on_monitor_start(&self, token: CancellationToken, monitor: Arc<Monitor>) {
        let msg_logger = Arc::new(TfliteMonitorLogger {
            logger: self.logger.clone(),
            monitor_id: monitor.config().id().to_owned(),
        });

        if let Err(e) = self.start(&token, msg_logger.clone(), monitor).await {
            msg_logger.log(LogLevel::Error, &format!("start: {e}"));
        };
    }

    fn route(&self, router: Router) -> Router {
        let state = HandlerState {
            logger: self.logger.clone(),
            monitor_manager: self.monitor_manager.clone(),
        };
        router
            .route(
                "/api/monitor/:id/tflite/enable",
                patch(enable_handler)
                    .with_state(state.clone())
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), admin))
                    .with_state(self.auth.clone()),
            )
            .route(
                "/api/monitor/:id/tflite/disable",
                patch(disable_handler)
                    .with_state(state)
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), admin))
                    .with_state(self.auth.clone()),
            )
    }
}

#[derive(Debug, Error)]
enum StartError {
    #[error("parse config: {0}")]
    ParseConfig(#[from] serde_json::Error),

    #[error("get detector '{0}'")]
    GetDetector(DetectorName),

    #[error("failed to get sub-stream")]
    GetSubStream,
}

#[derive(Debug, Error)]
enum RunError {
    #[error("subscribe: {0}")]
    Subscribe(#[from] SubscribeDecodedError),

    #[error("decoder: {0}")]
    DecoderError(#[from] DecoderError),

    #[error("input size zero")]
    InputSizeZero,

    #[error("try_from: {0}")]
    TryFrom(#[from] std::num::TryFromIntError),

    #[error("calculate outputs: {0}")]
    CalculateOutputs(#[from] CalculateOutputsError),

    #[error("process frame: {0}")]
    ProcessFrame(#[from] ProcessFrameError),

    #[error("detect: {0}")]
    Detect(#[from] DetectError),

    #[error("parse detections: {0}")]
    ParseDetections(#[from] ParseDetectionsError),
}

impl TflitePlugin {
    async fn start(
        &self,
        token: &CancellationToken,
        msg_logger: DynMsgLogger,
        monitor: Arc<Monitor>,
    ) -> Result<(), StartError> {
        use StartError::*;
        let config = monitor.config();

        let Some(config) = TfliteConfig::parse(config.raw().clone(), msg_logger.clone())? else {
            // Object detection is disabled.
            return Ok(());
        };

        let source = if config.use_sub_stream {
            match monitor.source_sub().await {
                Some(Some(v)) => v,
                Some(None) => return Err(GetSubStream),
                None => {
                    // Cancelled.
                    return Ok(());
                }
            }
        } else {
            match monitor.source_main().await {
                Some(v) => v,
                None => {
                    // Cancelled.
                    return Ok(());
                }
            }
        };

        msg_logger.log(
            LogLevel::Info,
            &format!("using {}-stream", source.stream_type().name()),
        );

        let detector_name = config.detector_name.clone();
        let detector = self
            .detector_manager
            .get_detector(&detector_name)
            .ok_or(GetDetector(detector_name))?;

        loop {
            msg_logger.log(LogLevel::Debug, "run");
            if let Err(e) = self
                .run(&msg_logger, &monitor, &config, &source, &detector)
                .await
            {
                msg_logger.log(LogLevel::Error, &format!("run: {e}"));
            }

            let sleep = || {
                let _enter = self.rt_handle.enter();
                tokio::time::sleep(Duration::from_secs(3))
            };
            tokio::select! {
                () = token.cancelled() => return Ok(()),
                () = sleep() => {}
            }
        }
    }

    async fn run(
        &self,
        msg_logger: &DynMsgLogger,
        monitor: &Arc<Monitor>,
        config: &TfliteConfig,
        source: &Arc<Source>,
        detector: &Detector,
    ) -> Result<(), RunError> {
        use RunError::*;
        let Some(muxer) = source.muxer().await else {
            // Cancelled.
            return Ok(());
        };
        let params = muxer.params();
        let width = params.width;
        let height = params.height;

        let inputs = Inputs {
            input_width: NonZeroU16::new(width).ok_or(InputSizeZero)?,
            input_height: NonZeroU16::new(height).ok_or(InputSizeZero)?,
            output_width: detector.width(),
            output_height: detector.height(),
        };

        let rate_limiter =
            FrameRateLimiter::new(u64::try_from(*DurationH264::from(*config.feed_rate))?);
        let Some(feed) = source
            .subscribe_decoded(self.rt_handle.clone(), Some(rate_limiter))
            .await
        else {
            // Cancelled.
            return Ok(());
        };
        let mut feed = feed?;

        let (outputs, uncrop) = calculate_outputs(config.crop, &inputs)?;

        let mut state = DetectorState {
            frame_processed: vec![0; outputs.output_size],
            outputs,
        };

        loop {
            let Some(frame) = feed.recv().await else {
                // Feed was cancelled.
                return Ok(());
            };
            let frame = frame?;

            let time = UnixNano::now();

            state = self
                .rt_handle
                .spawn_blocking(move || process_frame(&mut state, frame).map(|()| state))
                .await
                .expect("join")?;

            let Some(detections) = detector.detect(state.frame_processed.clone()).await? else {
                // Canceled.
                return Ok(());
            };
            let detections =
                parse_detections(&config.thresholds, &config.mask, &uncrop, detections)?;

            // Continue if there are no detections.
            let Some(d) = detections.first() else {
                continue;
            };

            msg_logger.log(
                LogLevel::Debug,
                &format!("trigger: label:{} score:{:.1}", d.label, d.score),
            );

            monitor
                .send_event(Event {
                    time,
                    duration: *config.feed_rate,
                    rec_duration: *config.duration,
                    detections,
                })
                .await;
        }
    }
}

struct DetectorState {
    outputs: Outputs,
    frame_processed: Vec<u8>,
}

#[derive(Debug)]
struct Inputs {
    input_width: NonZeroU16,
    input_height: NonZeroU16,
    output_width: NonZeroU16,
    output_height: NonZeroU16,
}

#[derive(Debug)]
struct Outputs {
    padded_width: NonZeroU16,
    padded_height: NonZeroU16,
    scaled_width: NonZeroU16,
    scaled_height: NonZeroU16,
    crop_x: u16,
    crop_y: u16,
    output_width: NonZeroU16,
    output_height: NonZeroU16,
    output_size: usize,
}

type UncropFn = Box<dyn Fn(u32) -> u32 + Send>;

pub(crate) struct Uncrop {
    uncrop_x_fn: UncropFn,
    uncrop_y_fn: UncropFn,
}

#[derive(Debug, Error)]
enum CalculateOutputsError {
    #[error("input width is less than output width, {0}/{1}")]
    OutputWidth(u32, u32),

    #[error("input height is less than output height, {0}/{1}")]
    OutputHeight(u32, u32),

    #[error("cropSize={0}% is less than {1}%")]
    CropSizeTooSmall(u32, u32),

    #[error("input width is less than scaled width, {0}/{1}")]
    ScaledWidth(u16, f64),

    #[error("try from int: {0}")]
    TryFromInt(#[from] TryFromIntError),

    #[error("zero")]
    Zero,
}

#[allow(clippy::items_after_statements, clippy::similar_names)]
fn calculate_outputs(crop: Crop, i: &Inputs) -> Result<(Outputs, Uncrop), CalculateOutputsError> {
    use CalculateOutputsError::*;
    let crop_x = u32::from(crop.x.get());
    let crop_y = u32::from(crop.y.get());
    let crop_size = u32::from(crop.size.get());
    let input_width = u32::from(i.input_width.get());
    let input_height = u32::from(i.input_height.get());
    let output_width = i.output_width.get();
    let output_height = i.output_height.get();
    let output_width2 = u32::from(output_width);
    let output_height2 = u32::from(output_height);

    if input_width < output_width2 {
        return Err(OutputWidth(input_width, output_width2));
    }

    if i.input_height.get() < output_height {
        return Err(OutputHeight(input_height, output_height2));
    }

    let padded_width = u16::try_from(output_width2 * 100 / crop_size)?;
    let padded_width2 = u32::from(padded_width);

    let padded_height = u16::try_from(output_height2 * 100 / crop_size)?;
    let padded_height2 = u32::from(padded_height);

    let crop_out_x = u16::try_from(padded_width2 * crop_x / 100)?;
    let crop_out_y = u16::try_from(padded_height2 * crop_y / 100)?;

    let width_ratio = input_width * output_height2;
    let height_ratio = input_height * output_width2;

    let mut scaled_width = padded_width;
    let mut scaled_height = padded_height;

    let mut padding_x_multiplier: u64 = 10000;
    let mut padding_y_multiplier: u64 = 10000;

    #[allow(clippy::comparison_chain)]
    if width_ratio > height_ratio {
        // Landscape.
        if input_width * crop_size / 100 < output_width2 {
            let min_crop_size = (output_width2 * 100).div_ceil(input_width);
            return Err(CropSizeTooSmall(crop_size, min_crop_size));
        }

        scaled_height = u16::try_from(input_height * padded_width2 / input_width)?;
        padding_y_multiplier = u64::from((10000 * padded_height2) / u32::from(scaled_height));
    } else if width_ratio < height_ratio {
        // Portrait.
        if input_height * crop_size / 100 < output_height2 {
            let min_crop_size = (output_height2 * 100).div_ceil(input_height);
            return Err(CropSizeTooSmall(crop_size, min_crop_size));
        }

        scaled_width = u16::try_from(input_width * padded_height2 / input_height)?;
        padding_x_multiplier = u64::from((10000 * padded_width2) / u32::from(scaled_width));
    }

    if i.input_width.get() < scaled_width {
        return Err(ScaledWidth(i.input_width.get(), f64::from(scaled_width)));
    }

    let crop_size = u64::from(crop_size);
    let uncrop_x_fn = move |input: u32| -> u32 {
        let input = u64::from(input);
        let crop_x = u64::from(crop_x);
        let output = ((padding_x_multiplier * input * crop_size) / 1_000_000)
            + (padding_x_multiplier * crop_x);
        #[allow(clippy::unwrap_used)]
        u32::try_from(output).unwrap()
    };

    let uncrop_y_fn = move |input: u32| -> u32 {
        let input = u64::from(input);
        let crop_y = u64::from(crop_y);
        let output = ((padding_y_multiplier * input * crop_size) / 1_000_000)
            + (padding_y_multiplier * crop_y);
        #[allow(clippy::unwrap_used)]
        u32::try_from(output).unwrap()
    };

    // Add 1 to odd inputs.
    fn even(input: u16) -> u16 {
        if input & 1 != 0 {
            input + 1
        } else {
            input
        }
    }

    Ok((
        Outputs {
            padded_width: NonZeroU16::new(even(padded_width)).ok_or(Zero)?,
            padded_height: NonZeroU16::new(even(padded_height)).ok_or(Zero)?,
            scaled_width: NonZeroU16::new(even(scaled_width)).ok_or(Zero)?,
            scaled_height: NonZeroU16::new(even(scaled_height)).ok_or(Zero)?,
            crop_x: crop_out_x,
            crop_y: crop_out_y,
            output_width: NonZeroU16::new(output_width).ok_or(Zero)?,
            output_height: NonZeroU16::new(output_height).ok_or(Zero)?,
            output_size: usize::from(output_width) * usize::from(output_height) * 3,
        },
        Uncrop {
            uncrop_x_fn: Box::new(uncrop_x_fn),
            uncrop_y_fn: Box::new(uncrop_y_fn),
        },
    ))
}

#[derive(Debug, Error)]
enum ProcessFrameError {
    #[error("unsupported pixel format: {0}")]
    UnsupportedPixelFormat(PixelFormat),

    #[error("create converter: {0}")]
    CreateConverter(#[from] NewConverterError),

    #[error("convert: {0}")]
    Convert(#[from] ConvertError),

    #[error("copy to buffer: {0}")]
    CopyToBuffer(#[from] ImageCopyToBufferError),

    #[error("create scaler: {0}")]
    CreateScaler(#[from] CreateScalerError),

    #[error("scale: {0}")]
    Scale(#[from] ScalerError),

    #[error("pad: {0}")]
    Pad(#[from] PadError),

    #[error("crop: {0}")]
    Crop(#[from] CropError),
}

fn process_frame(s: &mut DetectorState, mut frame: Frame) -> Result<(), ProcessFrameError> {
    use ProcessFrameError::*;
    if !frame.pix_fmt().is_yuv() {
        return Err(UnsupportedPixelFormat(frame.pix_fmt()));
    }

    // Remove color.
    let data = frame.data_mut();
    data[1].fill(128);
    data[2].fill(128);

    // Downscale.
    let mut frame_scaled = Frame::new();
    let mut scaler = Scaler::new(
        frame.width(),
        frame.height(),
        frame.pix_fmt(),
        s.outputs.scaled_width,
        s.outputs.scaled_height,
    )?;
    scaler.scale(&frame, &mut frame_scaled)?;

    // Convert to rgb24.
    let mut frame_converted = Frame::new();
    let mut converter = PixelFormatConverter::new(
        frame_scaled.width(),
        frame_scaled.height(),
        frame_scaled.color_range(),
        frame_scaled.pix_fmt(),
        PixelFormat::RGB24,
    )?;
    converter.convert(&frame_scaled, &mut frame_converted)?;

    // Add Padding.
    let mut frame_padded = Frame::new();
    pad(
        &frame_converted,
        &mut frame_padded,
        s.outputs.padded_width,
        s.outputs.padded_height,
        0,
        0,
    )?;

    // Crop to final size.
    let mut frame_cropped = Frame::new();
    crop(
        &frame_padded,
        &mut frame_cropped,
        s.outputs.crop_x,
        s.outputs.crop_y,
        s.outputs.output_width,
        s.outputs.output_height,
    )?;

    frame_cropped.copy_to_buffer(&mut s.frame_processed, 1)?;

    Ok(())
}

#[derive(Debug, Error)]
enum ParseDetectionsError {
    #[error("detection doesn't have a rectangle")]
    NoRectangle,

    #[error("size is zero")]
    Zero,
}

fn parse_detections(
    thresholds: &Thresholds,
    mask: &Mask,
    uncrop: &Uncrop,
    detections: Detections,
) -> Result<Detections, ParseDetectionsError> {
    use ParseDetectionsError::*;
    let mut parsed = Vec::new();
    for detection in detections {
        let Some(threshold) = thresholds.get(&detection.label) else {
            continue;
        };
        if detection.score < threshold.as_f32() {
            continue;
        }

        let rect = detection.region.rectangle.ok_or(NoRectangle)?;

        let top = (uncrop.uncrop_y_fn)(rect.y);
        let left = (uncrop.uncrop_x_fn)(rect.x);
        let bottom = (uncrop.uncrop_y_fn)(rect.y + rect.height.get());
        let right = (uncrop.uncrop_x_fn)(rect.x + rect.width.get());

        // Width and height must be calculated after uncropping.
        let width = right - left;
        let height = bottom - top;

        if mask.enable {
            let center_y = top + (height / 2);
            let center_x = left + (width / 2);

            let center_inside_mask = vertex_inside_poly2(center_x, center_y, &mask.area);
            if center_inside_mask {
                continue;
            }
        }

        parsed.push(Detection {
            label: detection.label,
            score: detection.score,
            region: Region {
                rectangle: Some(RectangleNormalized {
                    x: left,
                    y: top,
                    width: NonZeroU32::new(width).ok_or(Zero)?,
                    height: NonZeroU32::new(height).ok_or(Zero)?,
                }),
                polygon: None,
            },
        });
    }
    Ok(parsed)
}

fn modify_settings_js(tpl: Vec<u8>) -> Vec<u8> {
    const IMPORT_STATEMENT: &str = "import { tflite } from \"./tflite.js\";";
    const TARGET: &str = "/* SETTINGS_LAST_MONITOR_FIELD */";

    let tpl = String::from_utf8(tpl).expect("template should be valid utf8");
    let tpl = tpl.replace(
        TARGET,
        &("monitorFields.tflite = tflite(getMonitorId);\n".to_owned() + TARGET),
    );
    let tpl = IMPORT_STATEMENT.to_owned() + &tpl;
    tpl.as_bytes().to_owned()
}

struct TfliteMonitorLogger {
    logger: DynLogger,
    monitor_id: MonitorId,
}

impl MsgLogger for TfliteMonitorLogger {
    fn log(&self, level: LogLevel, msg: &str) {
        self.logger.log(LogEntry::new(
            level,
            "tflite",
            Some(self.monitor_id.clone()),
            msg.to_owned(),
        ));
    }
}

#[async_trait]
trait Fetcher {
    async fn fetch(&self, url: &Url) -> Result<Vec<u8>, FetchError>;
}

#[derive(Debug, Error)]
pub enum FetchError {
    #[error("parse uri")]
    ParseUri(#[from] InvalidUri),

    #[error("get: {0}")]
    Get(hyper::Error),

    #[error("Chunk: {0}")]
    Chunk(hyper::Error),
}

struct TfliteLogger {
    logger: DynLogger,
}

impl MsgLogger for TfliteLogger {
    fn log(&self, level: LogLevel, msg: &str) {
        self.logger
            .log(LogEntry::new(level, "tflite", None, msg.to_owned()));
    }
}

#[allow(clippy::similar_names)]
async fn fetch(url: &Url) -> Result<Vec<u8>, FetchError> {
    use FetchError::*;

    let uri = url.as_str().parse()?;
    let https = HttpsConnectorBuilder::new()
        .with_webpki_roots()
        .https_or_http()
        .enable_http1()
        .build();
    let client = hyper::client::Client::builder().build::<_, hyper::Body>(https);
    let mut res = client.get(uri).await.map_err(Get)?;
    let mut body = Vec::new();
    while let Some(chunk) = res.body_mut().data().await {
        let chunk = chunk.map_err(Chunk)?;
        body.write_all(&chunk).await.expect("write");
    }
    Ok(body)
}

struct Fetch;

#[async_trait]
impl Fetcher for Fetch {
    async fn fetch(&self, url: &Url) -> Result<Vec<u8>, FetchError> {
        fetch(url).await
    }
}

#[derive(Clone)]
struct HandlerState {
    logger: DynLogger,
    monitor_manager: MonitorManager,
}

async fn enable_handler(
    State(s): State<HandlerState>,
    Path(monitor_id): Path<MonitorId>,
) -> Response {
    let Some(old_config) = s.monitor_manager.monitor_config(monitor_id.clone()).await else {
        return (
            StatusCode::NOT_FOUND,
            format!("monitor '{monitor_id}' does not exist"),
        )
            .into_response();
    };

    let Some(new_config) = set_enable(&old_config, true) else {
        return (
            StatusCode::INTERNAL_SERVER_ERROR,
            "failed to find enable field",
        )
            .into_response();
    };

    if let Err(e) = s.monitor_manager.monitor_set_and_restart(new_config).await {
        return (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response();
    }

    s.logger.log(LogEntry::new(
        LogLevel::Info,
        "tflite",
        Some(monitor_id),
        "detector enabled".to_owned(),
    ));

    StatusCode::OK.into_response()
}

async fn disable_handler(
    State(s): State<HandlerState>,
    Path(monitor_id): Path<MonitorId>,
) -> Response {
    let Some(old_config) = s.monitor_manager.monitor_config(monitor_id.clone()).await else {
        return (
            StatusCode::NOT_FOUND,
            format!("monitor '{monitor_id}' does not exist"),
        )
            .into_response();
    };

    let new_config = match set_enable(&old_config, false) {
        Some(v) => v,
        // None means that it's already disabled in some way.
        None => old_config.clone(),
    };

    if let Err(e) = s.monitor_manager.monitor_set_and_restart(new_config).await {
        return (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response();
    }

    s.logger.log(LogEntry::new(
        LogLevel::Info,
        "tflite",
        Some(monitor_id),
        "detector disabled".to_owned(),
    ));

    StatusCode::OK.into_response()
}

#[allow(clippy::too_many_arguments, clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use std::collections::HashMap;

    use super::*;
    use crate::config::{Crop, CropSize, CropValue};
    use common::{
        recording::{denormalize, normalize},
        Label, PointNormalized,
    };
    use pretty_assertions::assert_eq;
    use test_case::test_case;

    #[test_case(600, 400, 0, 0, 100, 300, 300, "300x200 300x300 0:0 50:75";)]
    #[test_case(400, 600, 0, 0, 100, 300, 300, "200x300 300x300 0:0 75:50";)]
    #[test_case(640, 480, 0, 0, 100, 420, 280, "374x280 420x280 0:0 57:50";)]
    #[test_case(480, 640, 0, 0, 100, 280, 420, "280x374 280x420 0:0 50:57";)]
    #[test_case(100, 100, 5, 5,  90,  90,  90, "100x100 100x100 5:5 50:50";)]
    #[test_case(100, 200, 5, 5,  90,  90,  90,  "50x100 100x100 5:5 100:50";)]
    #[test_case(200, 100, 5, 5,  90,  90,  90,  "100x50 100x100 5:5 50:100";)]
    #[test_case(200, 100, 0, 0,  90,  90,  90,  "100x50 100x100 0:0 45:90";)]
    #[test_case(200, 100, 0, 20, 80,  80,  80,  "100x50 100x100 0:20 40:120";)]
    #[test_case(854, 480, 20, 10, 60, 300, 300, "500x282 500x500 100:50 50:72";)]
    fn test_calculate_outputs(
        input_width: u16,
        input_height: u16,
        crop_x: u16,
        crop_y: u16,
        crop_size: u16,
        output_width: u16,
        output_height: u16,
        want: &str,
    ) {
        let (outputs, reverse) = calculate_outputs(
            Crop {
                x: CropValue::new_testing(crop_x),
                y: CropValue::new_testing(crop_y),
                size: CropSize::new_testing(crop_size.try_into().unwrap()),
            },
            &Inputs {
                input_width: NonZeroU16::new(input_width).unwrap(),
                input_height: NonZeroU16::new(input_height).unwrap(),
                output_width: NonZeroU16::new(output_width).unwrap(),
                output_height: NonZeroU16::new(output_height).unwrap(),
            },
        )
        .unwrap();

        let got = format!(
            "{}x{} {}x{} {}:{} {}:{}",
            outputs.scaled_width,
            outputs.scaled_height,
            outputs.padded_width,
            outputs.padded_height,
            outputs.crop_x,
            outputs.crop_y,
            denormalize((reverse.uncrop_x_fn)(normalize(50, 100)), 100),
            denormalize((reverse.uncrop_y_fn)(normalize(50, 100)), 100),
        );

        assert_eq!(want, got);
    }

    fn label(s: &str) -> Label {
        s.to_owned().try_into().unwrap()
    }

    #[test]
    #[allow(clippy::items_after_statements)]
    fn test_parse_detections() {
        let reverse = Uncrop {
            uncrop_x_fn: Box::new(|v| v * 2),
            uncrop_y_fn: Box::new(|v| v * 2),
        };
        let detections = vec![Detection {
            label: label("b"),
            score: 5.0,
            region: Region {
                rectangle: Some(RectangleNormalized {
                    x: normalize(10, 100),
                    y: normalize(10, 100),
                    width: NonZeroU32::new(normalize(30, 100)).unwrap(),
                    height: NonZeroU32::new(normalize(30, 100)).unwrap(),
                }),
                polygon: None,
            },
        }];
        let mask = Mask {
            enable: false,
            area: Vec::new(),
        };
        let thresholds = HashMap::from([(label("b"), 1.try_into().unwrap())]);
        let got = parse_detections(&thresholds, &mask, &reverse, detections).unwrap();
        let want = vec![Detection {
            label: label("b"),
            score: 5.0,
            region: Region {
                rectangle: Some(RectangleNormalized {
                    x: normalize(20, 100),
                    y: normalize(20, 100),
                    width: NonZeroU32::new(normalize(60, 100)).unwrap(),
                    height: NonZeroU32::new(normalize(60, 100)).unwrap(),
                }),
                polygon: None,
            },
        }];
        assert_eq!(want, got);
    }

    #[test]
    #[allow(clippy::items_after_statements)]
    fn test_parse_detections_mask() {
        let reverse = Uncrop {
            uncrop_x_fn: Box::new(|v| v),
            uncrop_y_fn: Box::new(|v| v),
        };
        let detections = vec![Detection {
            label: label("b"),
            score: 5.0,
            region: Region {
                rectangle: Some(RectangleNormalized {
                    x: normalize(22, 100),
                    y: normalize(62, 100),
                    width: NonZeroU32::new(normalize(16, 100)).unwrap(),
                    height: NonZeroU32::new(normalize(16, 100)).unwrap(),
                }),
                polygon: None,
            },
        }];

        fn p(x: u16, y: u16) -> PointNormalized {
            PointNormalized {
                x: normalize(x, 100),
                y: normalize(y, 100),
            }
        }
        let thresholds = HashMap::from([(label("b"), 1.try_into().unwrap())]);
        let mask = Mask {
            enable: true,
            area: vec![p(20, 60), p(20, 80), p(40, 80), p(40, 60)],
        };
        assert!(
            parse_detections(&thresholds, &mask, &reverse, detections)
                .unwrap()
                .is_empty(),
            "detection should have been filtered"
        );
    }

    #[test]
    fn test_parse_detections_thresholds() {
        let reverse = Uncrop {
            uncrop_x_fn: Box::new(|v| v * 2),
            uncrop_y_fn: Box::new(|v| v * 2),
        };
        let detections = vec![Detection {
            label: label("b"),
            score: 5.0,
            region: Region {
                rectangle: Some(RectangleNormalized {
                    x: normalize(10, 100),
                    y: normalize(10, 100),
                    width: NonZeroU32::new(normalize(30, 100)).unwrap(),
                    height: NonZeroU32::new(normalize(30, 100)).unwrap(),
                }),
                polygon: None,
            },
        }];
        let thresholds = HashMap::from([(label("b"), 100.try_into().unwrap())]);
        let mask = Mask {
            enable: false,
            area: Vec::new(),
        };
        assert!(parse_detections(&thresholds, &mask, &reverse, detections)
            .unwrap()
            .is_empty());
    }

    #[test]
    fn test_crop_size_eroor() {
        // Landscape.
        let result = calculate_outputs(
            Crop {
                x: CropValue::new_testing(0),
                y: CropValue::new_testing(0),
                size: CropSize::new_testing(50.try_into().unwrap()),
            },
            &Inputs {
                input_width: NonZeroU16::new(640).unwrap(),
                input_height: NonZeroU16::new(480).unwrap(),
                output_width: NonZeroU16::new(340).unwrap(),
                output_height: NonZeroU16::new(340).unwrap(),
            },
        );
        match result {
            Ok(_) => panic!("expected error"),
            Err(e) => assert_eq!("cropSize=50% is less than 54%", e.to_string()),
        };

        // Portrait.
        let result = calculate_outputs(
            Crop {
                x: CropValue::new_testing(0),
                y: CropValue::new_testing(0),
                size: CropSize::new_testing(50.try_into().unwrap()),
            },
            &Inputs {
                input_width: NonZeroU16::new(480).unwrap(),
                input_height: NonZeroU16::new(640).unwrap(),
                output_width: NonZeroU16::new(340).unwrap(),
                output_height: NonZeroU16::new(340).unwrap(),
            },
        );
        match result {
            Ok(_) => panic!("expected error"),
            Err(e) => assert_eq!("cropSize=50% is less than 54%", e.to_string()),
        };
    }
}
