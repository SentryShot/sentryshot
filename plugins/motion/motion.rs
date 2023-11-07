// SPDX-License-Identifier: GPL-2.0-or-later

// False positive: https://github.com/dtolnay/async-trait/issues/228#issuecomment-1374848487
// RUSTC: remove in 1.69
#![allow(where_clauses_object_safety)]

mod config;
mod zone;

use crate::{config::MotionConfig, zone::Zone};
use async_trait::async_trait;
use common::{
    time::UnixNano, Cancelled, DynLogger, DynMonitor, DynMsgLogger, Event, LogEntry, LogLevel,
    LogSource, MonitorId, MsgLogger, Source,
};
use plugin::{types::Assets, Application, Plugin, PreLoadPlugin};
use recording::{FrameRateLimiter, FrameRateLimiterError};
use sentryshot_convert::{
    ConvertError, Frame, NewConverterError, PixelFormat, PixelFormatConverter,
};
use sentryshot_ffmpeg_h264::{
    H264BuilderError, H264Decoder, H264DecoderBuilder, Packet, PaddedBytes, Ready,
    ReceiveFrameError, SendPacketError,
};
use sentryshot_util::ImageCopyToBufferError;
use std::{borrow::Cow, sync::Arc, time::Duration};
use thiserror::Error;
use tokio::{
    runtime::Handle,
    sync::{broadcast, mpsc},
};
use tokio_util::sync::CancellationToken;
use zone::Zones;

#[no_mangle]
pub fn version() -> String {
    plugin::get_version()
}

#[no_mangle]
pub fn pre_load() -> Box<dyn PreLoadPlugin> {
    Box::new(PreLoadAuthNone)
}
struct PreLoadAuthNone;
impl PreLoadPlugin for PreLoadAuthNone {
    fn add_log_source(&self) -> Option<LogSource> {
        Some("motion".parse().unwrap())
    }
}

#[no_mangle]
pub fn load(app: &dyn Application) -> Arc<dyn Plugin> {
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
    fn edit_assets(&self, assets: &mut Assets) {
        let js = assets["scripts/settings.js"].to_vec();
        *assets.get_mut("scripts/settings.js").unwrap() = Cow::Owned(modify_settings_js(js));

        assets.insert(
            "scripts/motion.js".to_owned(),
            Cow::Borrowed(MOTION_MJS_FILE),
        );
    }

    async fn on_monitor_start(&self, token: CancellationToken, monitor: DynMonitor) {
        let msg_logger = Arc::new(MotionLogger {
            logger: self.logger.to_owned(),
            monitor_id: monitor.config().id().to_owned(),
        });

        match self.start(token, msg_logger.clone(), monitor).await {
            Ok(()) => {}
            Err(StartError::Cancelled(_)) => {
                msg_logger.log(LogLevel::Debug, "cancelled");
            }
            Err(e) => {
                msg_logger.log(LogLevel::Error, &format!("start: {}", e));
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
        self.logger.log(LogEntry {
            level,
            source: "motion".parse().unwrap(),
            monitor_id: Some(self.monitor_id.to_owned()),
            message: msg.parse().unwrap(),
        })
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
    #[error("{0}")]
    Cancelled(#[from] Cancelled),

    #[error("try_from: {0}")]
    TryFrom(#[from] std::num::TryFromIntError),

    #[error("new h264 decoder: {0}")]
    NewH264Decoder(#[from] H264BuilderError),

    #[error("dropped frames")]
    DroppedFrames,

    #[error("{0}")]
    SendFrame(#[from] SendPacketError),

    #[error("process frame: {0}")]
    ProcessFrame(#[from] DecodeFrameError),
}

impl MotionPlugin {
    async fn start(
        &self,
        token: CancellationToken,
        msg_logger: DynMsgLogger,
        monitor: DynMonitor,
    ) -> Result<(), StartError> {
        use StartError::*;

        let config = monitor.config();

        let source = {
            if let Some(sub_stream) = monitor.source_sub().await? {
                sub_stream
            } else {
                monitor.source_main().await?
            }
        };

        msg_logger.log(
            LogLevel::Info,
            &format!("using {}-stream", source.stream_type().name()),
        );

        let Some(config) = MotionConfig::parse(config.raw.to_owned())? else {
            // Motion detection is disabled.
            return Ok(())
        };

        loop {
            msg_logger.log(LogLevel::Debug, "run");
            match self.run(&msg_logger, &monitor, &config, &source).await {
                Ok(_) => {}
                Err(RunError::Cancelled(_)) => {}
                Err(e) => msg_logger.log(LogLevel::Error, &format!("run: {}", e)),
            }
            let _enter = self.rt_handle.enter();
            tokio::select! {
                _ = token.cancelled() => {
                    return Err(Cancelled(common::Cancelled));
                }
                _ = tokio::time::sleep(Duration::from_secs(3)) => {}
            }
        }
    }

    async fn run(
        &self,
        msg_logger: &DynMsgLogger,
        monitor: &DynMonitor,
        config: &MotionConfig,
        source: &Arc<Source>,
    ) -> Result<(), RunError> {
        use RunError::*;

        let muxer = source.muxer().await?;
        let params = muxer.params();
        let width = params.width;
        let height = params.height;
        let extradata = params.extra_data.to_owned();
        let h264_decoder = H264DecoderBuilder::new().avcc(PaddedBytes::new(extradata))?;

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

        let raw_frame_size = usize::from(width) * usize::from(height);
        let mut state = State {
            h264_decoder,
            frame_rate_limiter: FrameRateLimiter::new(u64::try_from(*config.feed_rate.as_h264())?),
            zones: Zones(zones),
            raw_frame: vec![0; raw_frame_size],
            prev_raw_frame: vec![0; raw_frame_size],
            raw_frame_diff: vec![0; raw_frame_size],
        };

        let mut feed = source.subscribe().await?;
        let mut first_frame = true;
        loop {
            use broadcast::error::RecvError;
            let frame = match feed.recv().await {
                Ok(v) => v,
                Err(RecvError::Closed) => {
                    msg_logger.log(LogLevel::Debug, "feed closed");
                    return Err(Cancelled(common::Cancelled));
                }
                Err(RecvError::Lagged(_)) => {
                    return Err(DroppedFrames);
                }
            };

            // State juggling to avoid lifetime issue.
            let avcc = frame.avcc.clone();
            state = self
                .rt_handle
                .spawn_blocking(move || {
                    state
                        .h264_decoder
                        .send_packet(Packet::new(&avcc).with_pts(*frame.pts))
                        .map(|_| (state))
                })
                .await
                .unwrap()?;

            loop {
                let has_new_frame: bool;
                (state, has_new_frame) = self
                    .rt_handle
                    .spawn_blocking(move || decode_frame(&mut state).map(|v| (state, v)))
                    .await
                    .unwrap()?;
                if !has_new_frame {
                    break;
                }

                let detections: Vec<_>;
                (detections, state) = self
                    .rt_handle
                    .spawn_blocking(|| {
                        (
                            state.zones.analyze(
                                &state.raw_frame,
                                &state.prev_raw_frame,
                                &mut state.raw_frame_diff,
                            ),
                            state,
                        )
                    })
                    .await
                    .unwrap();

                std::mem::swap(&mut state.raw_frame, &mut state.prev_raw_frame);

                // First frame will be compared to an empty frame and report 99% motion.
                if first_frame {
                    first_frame = false;
                    break;
                }

                for (zone, score) in detections {
                    msg_logger.log(
                        LogLevel::Debug,
                        &format!("detection: zone:{} score:{:.2}", zone, score),
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
}

struct State {
    h264_decoder: H264Decoder<Ready>,
    frame_rate_limiter: FrameRateLimiter,
    zones: Zones,
    raw_frame: Vec<u8>,
    prev_raw_frame: Vec<u8>,
    raw_frame_diff: Vec<u8>,
}

#[derive(Debug, Error)]
enum DecodeFrameError {
    #[error("receive frame: {0}")]
    ReceiveFrame(#[from] ReceiveFrameError),

    #[error("try from: {0}")]
    TryFrom(#[from] std::num::TryFromIntError),

    #[error("frame rate limiter: {0}")]
    FrameRateLimiter(#[from] FrameRateLimiterError),

    #[error("new converter: {0}")]
    NewConverter(#[from] NewConverterError),

    #[error("convert: {0}")]
    Convert(#[from] ConvertError),

    #[error("copy to buffer: {0}")]
    CopyToBuffer(#[from] ImageCopyToBufferError),
}

fn decode_frame(s: &mut State) -> Result<bool, DecodeFrameError> {
    let mut frame_buf = Frame::new();
    match s.h264_decoder.receive_frame(&mut frame_buf) {
        Ok(_) => {}
        Err(ReceiveFrameError::Eagain) => {
            return Ok(false);
        }
        Err(e) => Err(e)?,
    };
    if s.frame_rate_limiter
        .discard(u64::try_from(frame_buf.pts())?)?
    {
        return Ok(false);
    }

    let mut gray_frame = Frame::new();

    let mut converter = PixelFormatConverter::new(
        frame_buf.width(),
        frame_buf.height(),
        frame_buf.color_range(),
        frame_buf.pix_fmt(),
        PixelFormat::GRAY8,
    )?;

    converter.convert(&frame_buf, &mut gray_frame)?;

    gray_frame.copy_to_buffer(&mut s.raw_frame, 1)?;

    Ok(true)
}

fn modify_settings_js(tpl: Vec<u8>) -> Vec<u8> {
    const IMPORT_STATEMENT: &str = "import { motion } from \"./motion.js\";";
    const TARGET: &str = "/* SETTINGS_LAST_FIELD */";

    let tpl = String::from_utf8(tpl).unwrap();
    let tpl = tpl.replace(TARGET, &("motion: motion(),\n".to_owned() + TARGET));
    let tpl = IMPORT_STATEMENT.to_owned() + &tpl;
    tpl.as_bytes().to_owned()
}
