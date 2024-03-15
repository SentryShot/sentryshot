// SPDX-License-Identifier: GPL-2.0-or-later

mod config;
mod zone;

use crate::{config::MotionConfig, zone::Zone};
use async_trait::async_trait;
use common::{
    time::UnixNano, Cancelled, DynLogger, DynMsgLogger, Event, LogEntry, LogLevel, LogSource,
    MonitorId, MsgLogger,
};
use monitor::{DecoderError, Monitor, Source, SubscribeDecodedError};
use plugin::{types::Assets, Application, Plugin, PreLoadPlugin};
use recording::FrameRateLimiter;
use sentryshot_convert::{
    ConvertError, Frame, NewConverterError, PixelFormat, PixelFormatConverter,
};
use sentryshot_util::ImageCopyToBufferError;
use std::{borrow::Cow, sync::Arc, time::Duration};
use thiserror::Error;
use tokio::{runtime::Handle, sync::mpsc};
use tokio_util::sync::CancellationToken;
use zone::Zones;

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
        Some("motion".try_into().unwrap())
    }
}

#[no_mangle]
pub extern "Rust" fn load(app: &dyn Application) -> Arc<dyn Plugin> {
    Arc::new(MotionPlugin {
        rt_handle: app.rt_handle(),
        _shutdown_complete_tx: app.shutdown_complete_tx(),
        logger: app.logger(),
    })
}

pub struct MotionPlugin {
    rt_handle: Handle,
    _shutdown_complete_tx: mpsc::Sender<()>,
    logger: DynLogger,
}

const MOTION_MJS_FILE: &[u8] = include_bytes!("./js/motion.js");

#[async_trait]
impl Plugin for MotionPlugin {
    #[allow(clippy::unwrap_used)]
    fn edit_assets(&self, assets: &mut Assets) {
        let js = assets["scripts/settings.js"].to_vec();
        *assets.get_mut("scripts/settings.js").unwrap() = Cow::Owned(modify_settings_js(js));

        assets.insert(
            "scripts/motion.js".to_owned(),
            Cow::Borrowed(MOTION_MJS_FILE),
        );
    }

    async fn on_monitor_start(&self, token: CancellationToken, monitor: Arc<Monitor>) {
        let msg_logger = Arc::new(MotionLogger {
            logger: self.logger.clone(),
            monitor_id: monitor.config().id().to_owned(),
        });

        match self.start(token, msg_logger.clone(), monitor).await {
            Ok(()) => {}
            Err(StartError::Cancelled(_)) => {
                msg_logger.log(LogLevel::Debug, "cancelled");
            }
            Err(e) => {
                msg_logger.log(LogLevel::Error, &format!("start: {e}"));
            }
        };
    }
}

struct MotionLogger {
    logger: DynLogger,
    monitor_id: MonitorId,
}

impl MsgLogger for MotionLogger {
    fn log(&self, level: LogLevel, msg: &str) {
        self.logger.log(LogEntry::new(
            level,
            "motion",
            Some(self.monitor_id.clone()),
            msg.to_owned(),
        ));
    }
}

#[derive(Debug, Error)]
enum StartError {
    #[error("parse config: {0}")]
    ParseConfig(#[from] serde_json::Error),

    #[error("{0}")]
    Cancelled(#[from] Cancelled),
}

#[derive(Debug, Error)]
enum RunError {
    #[error("try_from: {0}")]
    TryFrom(#[from] std::num::TryFromIntError),

    #[error("subscribe: {0}")]
    Subscribe(#[from] SubscribeDecodedError),

    #[error("decoder: {0}")]
    DecoderError(#[from] DecoderError),

    #[error("convert frame: {0}")]
    ConvertFrame(#[from] ConvertFrameError),
}

impl MotionPlugin {
    async fn start(
        &self,
        token: CancellationToken,
        msg_logger: DynMsgLogger,
        monitor: Arc<Monitor>,
    ) -> Result<(), StartError> {
        use StartError::*;

        let config = monitor.config();

        let Some(source) = monitor.get_smallest_source().await else {
            // Cancelled.
            return Ok(());
        };

        let Some(config) = MotionConfig::parse(config.raw.clone())? else {
            // Motion detection is disabled.
            return Ok(());
        };

        msg_logger.log(
            LogLevel::Info,
            &format!("using {}-stream", source.stream_type().name()),
        );

        loop {
            msg_logger.log(LogLevel::Debug, "run");
            if let Err(e) = self.run(&msg_logger, &monitor, &config, &source).await {
                msg_logger.log(LogLevel::Error, &format!("run: {e}"));
            }

            let sleep = || {
                let _enter = self.rt_handle.enter();
                tokio::time::sleep(Duration::from_secs(3))
            };
            tokio::select! {
                () = token.cancelled() => {
                    return Err(Cancelled(common::Cancelled));
                }
                () = sleep() => {}
            }
        }
    }

    async fn run(
        &self,
        msg_logger: &DynMsgLogger,
        monitor: &Arc<Monitor>,
        config: &MotionConfig,
        source: &Arc<Source>,
    ) -> Result<(), RunError> {
        let Some(muxer) = source.muxer().await else {
            // Cancelled.
            return Ok(());
        };
        let params = muxer.params();
        let width = params.width;
        let height = params.height;

        let zones: Vec<Zone> = config
            .zones
            .iter()
            .filter_map(|zone_config| {
                if !zone_config.enable {
                    return None;
                }
                Some(Zone::new(width, height, zone_config))
            })
            .collect();

        let limiter = FrameRateLimiter::new(u64::try_from(*config.feed_rate.as_h264())?);

        let raw_frame_size = usize::from(width) * usize::from(height);
        let mut state = State {
            zones: Zones(zones),
            raw_frame: vec![0; raw_frame_size],
            prev_raw_frame: vec![0; raw_frame_size],
            raw_frame_diff: vec![0; raw_frame_size],
        };

        let Some(feed) = source
            .subscribe_decoded(self.rt_handle.clone(), Some(limiter))
            .await
        else {
            // Cancelled.
            return Ok(());
        };
        let mut feed = feed?;

        let mut first_frame = true;
        loop {
            let Some(frame) = feed.recv().await else {
                // Feed was cancelled.
                return Ok(());
            };

            let detections: Vec<_>;
            (detections, state) = self
                .rt_handle
                .spawn_blocking(move || -> Result<_, RunError> {
                    convert_frame(&mut state.raw_frame, &frame?)?;
                    let detections = state.zones.analyze(
                        &state.raw_frame,
                        &state.prev_raw_frame,
                        &mut state.raw_frame_diff,
                    );
                    Ok((detections, state))
                })
                .await
                .expect("join")?;

            std::mem::swap(&mut state.raw_frame, &mut state.prev_raw_frame);

            // First frame is compared to an empty frame and reports 99% motion.
            if first_frame {
                first_frame = false;
                continue;
            }

            for (zone, score) in detections {
                msg_logger.log(
                    LogLevel::Debug,
                    &format!("detection: zone:{zone} score:{score:.2}"),
                );

                let time = UnixNano::now();
                //t := time.Now().Add(-d.config.timestampOffset)
                monitor
                    .send_event(Event {
                        time,
                        duration: *config.feed_rate,
                        rec_duration: *config.duration,
                        detections: Vec::new(),
                    })
                    .await;
            }
        }
    }
}

struct State {
    zones: Zones,
    raw_frame: Vec<u8>,
    prev_raw_frame: Vec<u8>,
    raw_frame_diff: Vec<u8>,
}

#[derive(Debug, Error)]
enum ConvertFrameError {
    #[error("new converter: {0}")]
    NewConverter(#[from] NewConverterError),

    #[error("convert: {0}")]
    Convert(#[from] ConvertError),

    #[error("copy to buffer: {0}")]
    CopyToBuffer(#[from] ImageCopyToBufferError),
}

fn convert_frame(raw_frame: &mut Vec<u8>, frame_buf: &Frame) -> Result<(), ConvertFrameError> {
    let mut converter = PixelFormatConverter::new(
        frame_buf.width(),
        frame_buf.height(),
        frame_buf.color_range(),
        frame_buf.pix_fmt(),
        PixelFormat::GRAY8,
    )?;

    let mut gray_frame = Frame::new();
    converter.convert(frame_buf, &mut gray_frame)?;

    gray_frame.copy_to_buffer(raw_frame, 1)?;

    Ok(())
}

#[allow(clippy::unwrap_used)]
fn modify_settings_js(tpl: Vec<u8>) -> Vec<u8> {
    const IMPORT_STATEMENT: &str = "import { motion } from \"./motion.js\";";
    const TARGET: &str = "/* SETTINGS_LAST_FIELD */";

    let tpl = String::from_utf8(tpl).unwrap();
    let tpl = tpl.replace(TARGET, &("motion: motion(),\n".to_owned() + TARGET));
    let tpl = IMPORT_STATEMENT.to_owned() + &tpl;
    tpl.as_bytes().to_owned()
}
