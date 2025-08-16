// SPDX-License-Identifier: GPL-2.0-or-later

mod config;
mod detector;

use crate::detector::DetectorManager;
use async_trait::async_trait;
use common::{
    ArcLogger, ArcMsgLogger, DynError, Event, LogEntry, LogLevel, LogSource, MonitorId, MsgLogger,
    monitor::{ArcMonitor, ArcSource, CreateEventDbError, DecoderError, SubscribeDecodedError},
    recording::FrameRateLimiter,
    time::{DurationH264, UnixH264, UnixNano},
};
use config::{Crop, OpenvinoConfig, Config};
use detector::ArcDetector;
use plugin::{Application, Plugin, PreLoadPlugin};
use sentryshot_convert::{
    ConvertError, Frame, NewConverterError, PixelFormat, PixelFormatConverter,
};
use sentryshot_filter::{CropError, PadError, crop, pad};
use sentryshot_scale::{CreateScalerError, Scaler, ScalerError};
use sentryshot_util::ImageCopyToBufferError;
use std::{
    ffi::c_char,
    num::{NonZeroU16, TryFromIntError},
    sync::Arc,
    time::Duration,
};
use thiserror::Error;
use tokio::runtime::{Handle, Runtime};
use tokio_util::sync::CancellationToken;

#[unsafe(no_mangle)]
pub extern "C" fn version() -> *const c_char {
    plugin::get_version()
}

#[unsafe(no_mangle)]
pub extern "Rust" fn pre_load() -> Box<dyn PreLoadPlugin> {
    Box::new(PreLoadOpenvino)
}

struct PreLoadOpenvino;

impl PreLoadPlugin for PreLoadOpenvino {
    fn add_log_source(&self) -> Option<LogSource> {
        #[allow(clippy::unwrap_used)]
        Some("openvino".try_into().unwrap())
    }
}

#[unsafe(no_mangle)]
pub extern "Rust" fn load(app: &dyn Application) -> Arc<dyn Plugin> {
    Arc::new(
        OpenvinoPlugin::new(
            app.logger(),
            app.env().raw(),
        )
    )
}

struct OpenvinoPlugin {
    runtime: Runtime,
    logger: ArcLogger,
    detector_manager: DetectorManager,
}

impl OpenvinoPlugin {
    fn new(logger: ArcLogger, raw_env_config: &str) -> Self {
        let config = match raw_env_config.parse::<Config>() {
            Ok(config) => config,
            Err(e) => {
                eprintln!("failed to parse openvino config: {e}");
                std::process::exit(1);
            }
        };
        let openvino_logger = Arc::new(OpenvinoLogger {
            logger: logger.clone(),
        });
        let detector_manager = DetectorManager::new(openvino_logger, &config);
        Self {
            runtime: tokio::runtime::Runtime::new().unwrap(),
            logger,
            detector_manager,
        }
    }
}

#[async_trait]
impl Plugin for OpenvinoPlugin {
    async fn on_monitor_start(&self, token: CancellationToken, monitor: ArcMonitor) {
        let detector = self.detector_manager.get_detector();
        let logger = self.logger.clone();
        let rt_handle = self.runtime.handle().clone();

        // Since on_monitor_start is not guaranteed to be run in tokio runtime, immediately spawn a new tokio task in the runtime we created.
        self.runtime.spawn(async move {
            let msg_logger = Arc::new(OpenvinoMsgLogger {
                logger,
                monitor_id: monitor.config().id().to_owned(),
            });

            if let Err(e) = start(&token, msg_logger.clone(), monitor, detector, rt_handle).await {
                msg_logger.log(LogLevel::Error, &format!("start: {e}"));
            };
        });
    }
}

#[derive(Debug, Error)]
enum StartError {
    #[error("parse config: {0}")]
    ParseConfig(#[from] serde_json::Error),
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
    Detect(DynError),
    #[error("send event: {0}")]
    SendEvent(#[from] CreateEventDbError),
}

async fn start(
    token: &CancellationToken,
    msg_logger: ArcMsgLogger,
    monitor: ArcMonitor,
    detector: ArcDetector,
    rt_handle: Handle,
) -> Result<(), StartError> {
        use StartError::*;
        let config = monitor.config();

        let Some(config) = OpenvinoConfig::parse(config.raw().clone(), msg_logger.clone())? else {
            return Ok(());
        };

        let source = if config.use_sub_stream {
            match monitor.source_sub().await {
                Some(Some(v)) => v,
                Some(None) => return Err(GetSubStream),
                None => {
                    return Ok(());
                }
            }
        } else {
            match monitor.source_main().await {
                Some(v) => v,
                None => {
                    return Ok(());
                }
            }
        };

        msg_logger.log(
            LogLevel::Info,
            &format!("using {}-stream", source.stream_type().name()),
        );

        loop {
            msg_logger.log(LogLevel::Debug, "run");
            if let Err(e) = 
                run(&msg_logger, &monitor, &config, &source, &detector, &rt_handle)
                .await
            {
                msg_logger.log(LogLevel::Error, &format!("run: {e}"));
            }

            let sleep = || {
                let _enter = rt_handle.enter();
                tokio::time::sleep(Duration::from_secs(3))
            };
            tokio::select! {
                () = token.cancelled() => return Ok(()),
                () = sleep() => {}
            }
        }
    }

async fn run(
        msg_logger: &ArcMsgLogger,
        monitor: &ArcMonitor,
        config: &OpenvinoConfig,
        source: &ArcSource,
        detector: &ArcDetector,
        rt_handle: &Handle,
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
            .subscribe_decoded(
                rt_handle.clone(),
                msg_logger.clone(),
                Some(rate_limiter),
            )
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

            let time = UnixNano::from(UnixH264::new(frame.pts()));

            state = rt_handle
                .spawn_blocking(move || process_frame(&mut state, frame).map(|()| state))
                .await
                .expect("join")?;

            let detector = Arc::clone(detector);
            let frame_processed = state.frame_processed.clone();
            let Some(detections) = rt_handle.spawn(async move {
                detector
                    .detect(frame_processed)
                    .await
            })
            .await
            .expect("task join failed")
            .map_err(Detect)?
            else {
                return Ok(());
            };

            if detections.is_empty() {
                continue;
            }

            // monitor
            //     .trigger(
            //         *config.trigger_duration,
            //         Event {
            //             time,
            //             duration: *config.feed_rate,
            //             detections,
            //             source: Some("object".to_owned().try_into().expect("valid")),
            //         },
            //     )
            //     .await?;
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
        if input & 1 != 0 { input + 1 } else { input }
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

struct OpenvinoMsgLogger {
    logger: ArcLogger,
    monitor_id: MonitorId,
}

impl MsgLogger for OpenvinoMsgLogger {
    fn log(&self, level: LogLevel, msg: &str) {
        self.logger
            .log(LogEntry::new(level, "openvino", &self.monitor_id, msg));
    }
}

struct OpenvinoLogger {
    logger: ArcLogger,
}

impl MsgLogger for OpenvinoLogger {
    fn log(&self, level: LogLevel, msg: &str) {
        self.logger.log(LogEntry::new2(level, "openvino", msg));
    }
}
