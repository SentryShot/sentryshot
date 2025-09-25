// SPDX-License-Identifier: GPL-2.0-or-later

use crate::log_monitor;
use async_trait::async_trait;
use common::{
    ArcLogger, ArcMsgLogger, ArcStreamerMuxer, DynError, H264Data, LogEntry, LogLevel, MonitorId,
    MsgLogger, StreamType, TrackParameters,
    monitor::{
        ArcStreamer, DecoderError, DynH264Writer, Feed, FeedDecoded, Protocol, RtspUrl, Source,
        SourceRtspConfig, SubscribeDecodedError,
    },
    recording::FrameRateLimiter,
    time::{DtsOffset, H264_SECOND, UnixH264, UnixNano},
};
use futures_lite::StreamExt;
use retina::{
    client::{InitialTimestampPolicy, Stream},
    codec::{ParametersRef, VideoFrame, VideoParameters},
};
use sentryshot_convert::Frame;
use sentryshot_ffmpeg_h264::{
    H264Decoder, H264DecoderBuilder, Packet, PaddedBytes, Ready, ReceiveFrameError, SendPacketError,
};
use std::sync::Arc;
use thiserror::Error;
use tokio::{
    runtime::Handle,
    sync::{broadcast, mpsc, oneshot},
};
use tokio_util::{sync::CancellationToken, task::task_tracker::TaskTrackerToken};
use url::Url;

#[allow(clippy::module_name_repetitions)]
pub struct MonitorSource {
    stream_type: StreamType,
    get_muxer_tx: mpsc::Sender<oneshot::Sender<ArcStreamerMuxer>>,
    subscribe_tx: mpsc::Sender<oneshot::Sender<Feed>>,
}

impl MonitorSource {
    #[must_use]
    pub fn new(
        stream_type: StreamType,
        get_muxer_tx: mpsc::Sender<oneshot::Sender<ArcStreamerMuxer>>,
        subscribe_tx: mpsc::Sender<oneshot::Sender<Feed>>,
    ) -> Self {
        Self {
            stream_type,
            get_muxer_tx,
            subscribe_tx,
        }
    }
}

#[async_trait]
impl Source for MonitorSource {
    #[must_use]
    fn stream_type(&self) -> &StreamType {
        &self.stream_type
    }

    // Returns the HLS muxer for this source. Will block until the source has started.
    // Returns None if cancelled.
    async fn muxer(&self) -> Option<ArcStreamerMuxer> {
        let (res_tx, res_rx) = oneshot::channel();
        if self.get_muxer_tx.send(res_tx).await.is_err() {
            return None;
        }
        let Ok(muxer) = res_rx.await else {
            return None;
        };
        Some(muxer)
    }

    // Subscribe to the raw feed. Will block until the source has started.
    async fn subscribe(&self) -> Option<Feed> {
        let (res_tx, res_rx) = oneshot::channel();
        if self.subscribe_tx.send(res_tx).await.is_err() {
            return None;
        }
        let Ok(feed) = res_rx.await else {
            return None;
        };
        Some(feed)
    }

    // Subscribe to a decoded feed. Currently creates a new decoder for each
    // call but this may change. Will block until the source has started.
    // Will close channel when cancelled.
    async fn subscribe_decoded(
        &self,
        rt_handle: Handle,
        logger: ArcMsgLogger,
        limiter: Option<FrameRateLimiter>,
    ) -> Option<Result<FeedDecoded, SubscribeDecodedError>> {
        let feed = self.subscribe().await?;

        // We could grab the extradata strait from the source instead.
        let muxer = self.muxer().await?;
        let extradata = muxer.params().extra_data.clone();

        let h264_decoder = match H264DecoderBuilder::new().avcc(PaddedBytes::new(extradata)) {
            Ok(v) => v,
            Err(e) => return Some(Err(SubscribeDecodedError::NewH264Decoder(e))),
        };
        Some(Ok(new_decoder(
            rt_handle,
            logger,
            feed,
            h264_decoder,
            limiter,
        )))
    }
}

struct SourceLogger {
    logger: ArcLogger,

    monitor_id: MonitorId,
    source_name: String,
    stream_type: StreamType,
}

impl SourceLogger {
    fn new(
        logger: ArcLogger,
        monitor_id: MonitorId,
        source_name: String,
        stream_type: StreamType,
    ) -> Self {
        Self {
            logger,
            monitor_id,
            source_name,
            stream_type,
        }
    }
}

impl MsgLogger for SourceLogger {
    fn log(&self, level: LogLevel, msg: &str) {
        self.logger.log(LogEntry::new(
            level,
            "monitor",
            &self.monitor_id,
            &format!(
                "({}) {} source: {}",
                self.stream_type.name(),
                self.source_name,
                msg
            ),
        ));
    }
}

#[allow(clippy::module_name_repetitions)]
pub struct SourceRtsp {
    msg_logger: ArcMsgLogger,
    streamer: ArcStreamer,

    monitor_id: MonitorId,
    config: SourceRtspConfig,
    stream_type: StreamType,
}

impl SourceRtsp {
    #[allow(clippy::new_ret_no_self, clippy::too_many_arguments)]
    pub fn new(
        token: CancellationToken,
        task_token: TaskTrackerToken,
        logger: ArcLogger,
        streamer: ArcStreamer,
        monitor_id: MonitorId,
        config: SourceRtspConfig,
        stream_type: StreamType,
    ) -> Option<MonitorSource> {
        if stream_type.is_sub() && config.sub_stream.is_none() {
            log_monitor(&logger, LogLevel::Debug, &monitor_id, "no sub stream");
            return None;
        }

        let msg_logger = Arc::new(SourceLogger::new(
            logger,
            monitor_id.clone(),
            "rtsp".to_owned(),
            stream_type,
        ));

        let source = Self {
            msg_logger,
            streamer,
            monitor_id,
            config,
            stream_type,
        };

        let (started_tx, mut started_rx) = mpsc::channel(1);

        let task_token2 = task_token.clone();
        let token2 = token.clone();
        tokio::spawn(async move {
            let _task_token = task_token2;
            loop {
                if token2.is_cancelled() {
                    source.log(LogLevel::Info, "stopped");
                    return;
                }

                match source.run(token2.child_token(), started_tx.clone()).await {
                    Ok(()) => source.log(LogLevel::Debug, "cancelled"),
                    Err(e) => source.log(LogLevel::Error, &format!("crashed: {e}")),
                };

                tokio::select! {
                    () = token2.cancelled() => {}
                    () = tokio::time::sleep(tokio::time::Duration::from_secs(10)) => {}
                }
            }
        });

        let (get_muxer_tx, mut get_muxer_rx) =
            mpsc::channel::<oneshot::Sender<ArcStreamerMuxer>>(1);
        let (subscribe_tx, mut subscribe_rx) = mpsc::channel::<oneshot::Sender<Feed>>(1);

        tokio::spawn(async move {
            let _task_token = task_token;
            let mut muxer = None;
            let mut feed_tx = None;
            let mut get_muxer_requests: Vec<oneshot::Sender<_>> = Vec::new();
            let mut subscribe_requests: Vec<oneshot::Sender<_>> = Vec::new();
            loop {
                tokio::select! {
                    () = token.cancelled() => return,
                   res = started_rx.recv() => {
                        let Some((m, f)) = res else {
                            return
                        };
                        while let Some(res) = get_muxer_requests.pop() {
                            _ = res.send(m.clone());
                        }
                        while let Some(res) = subscribe_requests.pop() {
                            _ = res.send(f.subscribe());
                        }
                        muxer = Some(m);
                        feed_tx = Some(f);
                    }

                    res = get_muxer_rx.recv() => {
                        let Some(res) = res else {
                            return
                        };
                        if let Some(muxer) = &muxer {
                            _ = res.send(muxer.clone());
                        } else {
                            get_muxer_requests.push(res);
                        }
                    }
                    res = subscribe_rx.recv() => {
                        let Some(res) = res else {
                            return
                        };
                        if let Some(feed_tx) = &feed_tx {
                            _ = res.send(feed_tx.subscribe());
                        } else {
                            subscribe_requests.push(res);
                        }
                    }
                }
            }
        });

        Some(MonitorSource::new(stream_type, get_muxer_tx, subscribe_tx))
    }

    fn log(&self, level: LogLevel, msg: &str) {
        self.msg_logger.log(level, msg);
    }

    #[allow(clippy::too_many_lines, clippy::similar_names)]
    async fn run(
        &self,
        token: CancellationToken,
        started_tx: mpsc::Sender<(ArcStreamerMuxer, broadcast::Sender<H264Data>)>,
    ) -> Result<(), SourceRtspRunError> {
        use SourceRtspRunError::*;

        let url: &Url = self.stream_url();
        let creds = creds_from_url(url);
        let url = remove_creds_from_url(url.to_owned())?;

        let session_group = Arc::new(retina::client::SessionGroup::default());
        let mut session = retina::client::Session::describe(
            url.clone(),
            retina::client::SessionOptions::default()
                .creds(creds)
                .session_group(session_group.clone())
                .teardown(retina::client::TeardownPolicy::Always),
        )
        .await
        .map_err(Describe)?;

        let video_stream_i = {
            let s = session.streams().iter().position(|s| {
                if s.media() == "video" {
                    if s.encoding_name() == "h264" {
                        self.log(LogLevel::Debug, "using h264 video stream");
                        return true;
                    }

                    self.log(
                        LogLevel::Debug,
                        &format!(
                            "ignoring {} video stream because it's unsupported",
                            s.encoding_name()
                        ),
                    );
                }
                false
            });
            let Some(s) = s else {
                return Err(NoVideoStreamFound(format_streams(session.streams())));
            };
            s
        };

        let transport = match self.config.protocol {
            Protocol::Tcp => {
                retina::client::Transport::Tcp(retina::client::TcpTransportOptions::default())
            }
            Protocol::Udp => {
                retina::client::Transport::Udp(retina::client::UdpTransportOptions::default())
            }
        };

        session
            .setup(
                video_stream_i,
                retina::client::SetupOptions::default().transport(transport),
            )
            .await
            .map_err(SourceRtspRunError::Setup)?;

        let mut session = session
            .play(
                retina::client::PlayOptions::default()
                    .initial_timestamp(InitialTimestampPolicy::Permissive),
            )
            .await
            .map_err(Play)?
            .demuxed()
            .map_err(Demuxed)?;

        // Buffer 10 frame to reduce dropped frames.
        let (feed_tx, _) = broadcast::channel(10);

        let mut stream_started: Option<StreamStarted> = None;
        loop {
            tokio::select! {
                () = token.cancelled() => {
                    return Ok(());
                },
                pkt = session.next() => {
                    let Some(pkt) = pkt else {
                        return Err(Eof);
                    };
                    match pkt {
                        Ok(retina::codec::CodecItem::VideoFrame(frame)) => {
                            if let Some(stream_started) = &mut stream_started {
                                let data = parse_frame(
                                    frame,
                                    stream_started.start_time,
                                    stream_started.first_sample_pts,
                                )?;
                                check_clock_drift(data.pts)?;
                                stream_started.hls_writer.write_h264(data.clone()).await
                                    .map_err(SourceRtspRunError::WriteH264 )?;
                                _ = feed_tx.send(data);

                            } else {
                                if !frame.is_random_access_point() {
                                    // Wait for IDR.
                                    continue
                                }

                                let stream = &session.streams()[frame.stream_id()];
                                if let Some(ParametersRef::Video(params)) = stream.parameters() {
                                    let start_time = UnixNano::now();
                                    let first_sample_pts = UnixH264::new(frame.timestamp().pts());
                                    let first_sample = parse_frame(frame, start_time, first_sample_pts)?;
                                    let result = self.streamer.new_muxer(
                                        token.clone(),
                                        self.monitor_id.clone(),
                                        self.stream_type.is_sub(),
                                        track_params_from_video_params(params)?,
                                        start_time.into(),
                                        first_sample.clone(),
                                    )
                                        .await
                                        .map_err(SourceRtspRunError::NewMuxer)?;
                                    let Some((muxer, hls_writer)) = result else {
                                        // Cancelled.
                                        return Ok(());
                                    };
                                    stream_started = Some(StreamStarted{ hls_writer, start_time, first_sample_pts });
                                    // Notify successful start.
                                    _ = started_tx.send((muxer, feed_tx.clone())).await;
                                };
                            }
                        },
                        Ok(_) => {},
                        Err(e) => return Err(Stream(e)),
                    }
                }
            }
        }
    }

    fn stream_url(&self) -> &RtspUrl {
        if self.stream_type.is_main() {
            &self.config.main_stream
        } else {
            self.config
                .sub_stream
                .as_ref()
                .expect("sub_stream to be `Some`")
        }
    }
}

struct StreamStarted {
    hls_writer: DynH264Writer,
    start_time: UnixNano,
    first_sample_pts: UnixH264,
}

#[derive(Debug, Error)]
enum ParseFrameError {
    #[error("subtract first sample pts")]
    SubFirstSample,

    #[error("add start time")]
    AddStartTime,
}

#[allow(clippy::similar_names)]
fn parse_frame(
    frame: VideoFrame,
    start_time: UnixNano,
    first_sample_time: UnixH264,
) -> Result<H264Data, ParseFrameError> {
    use ParseFrameError::*;
    let timestamp = frame.timestamp();

    let pts = timestamp.pts();
    let dts = timestamp.dts();
    let dts_offset = DtsOffset::new(i32::try_from(pts - dts).unwrap_or(0));

    let pts = UnixH264::new(pts)
        .checked_sub(first_sample_time)
        .ok_or(SubFirstSample)?
        .checked_add(UnixH264::from(start_time))
        .ok_or(AddStartTime)?;

    Ok(H264Data {
        pts,
        dts_offset,
        random_access_present: frame.is_random_access_point(),
        avcc: Arc::new(PaddedBytes::new(frame.into_data())),
    })
}

fn check_clock_drift(pts: UnixH264) -> Result<(), SourceRtspRunError> {
    let now = UnixH264::now();
    let diff = (pts - now).abs();
    // This shouldnt be more than one or two frames, but we will try 30 sec for now.
    if diff > 30 * H264_SECOND {
        let diff_secs = diff / H264_SECOND;
        return if now < pts {
            Err(SourceRtspRunError::CameraClockAhead(diff_secs))
        } else {
            Err(SourceRtspRunError::CameraClockBehind(diff_secs))
        };
    }
    Ok(())
}

#[derive(Debug, Error)]
pub enum ParseParamsError {
    #[error("{0}")]
    TryFromInt(#[from] std::num::TryFromIntError),
}

pub fn track_params_from_video_params(
    params: &VideoParameters,
) -> Result<TrackParameters, ParseParamsError> {
    let (width, height) = params.pixel_dimensions();
    Ok(TrackParameters {
        width: u16::try_from(width)?,
        height: u16::try_from(height)?,
        codec: params.rfc6381_codec().to_owned(),
        extra_data: params.extra_data().to_owned(),
    })
}

#[derive(Debug, Error)]
enum SourceRtspRunError {
    #[error("end of file")]
    Eof,

    #[error("describe: {0}")]
    Describe(retina::Error),

    #[error("remove credentials: {0}")]
    RemoveCreds(#[from] RemoveCreds),

    #[error("no suitable video stream found, streams:{0}")]
    NoVideoStreamFound(String),

    #[error("setup: {0}")]
    Setup(retina::Error),

    #[error("play: {0}")]
    Play(retina::Error),

    #[error("demuxed: {0}")]
    Demuxed(retina::Error),

    #[error("stream: {0}")]
    Stream(retina::Error),

    #[error("write h264: {0}")]
    WriteH264(DynError),

    #[error("convert params: {0}")]
    ConvertTrackParams(#[from] ParseParamsError),

    #[error("new muxer: {0}")]
    NewMuxer(DynError),

    #[error("parse frame: {0}")]
    ParseFrame(#[from] ParseFrameError),

    #[error("camera clock drifted {0} seconds ahead")]
    CameraClockAhead(i64),

    #[error("camera clock drifted {0} seconds behind")]
    CameraClockBehind(i64),
}

#[derive(Debug, Error)]
enum RemoveCreds {
    #[error("set password")]
    SetUsername,

    #[error("set username")]
    SetPassword,
}

fn remove_creds_from_url(mut url: Url) -> Result<Url, RemoveCreds> {
    url.set_username("")
        .map_err(|()| RemoveCreds::SetUsername)?;
    url.set_password(None)
        .map_err(|()| RemoveCreds::SetPassword)?;
    Ok(url)
}

fn creds_from_url(url: &Url) -> Option<retina::client::Credentials> {
    let username = url.username();
    let password = url.password();

    if let Some(password) = password {
        Some(retina::client::Credentials {
            username: username.to_owned(),
            password: password.to_owned(),
        })
    } else if !username.is_empty() {
        Some(retina::client::Credentials {
            username: username.to_owned(),
            password: String::new(),
        })
    } else {
        None
    }
}

fn format_streams(streams: &[Stream]) -> String {
    streams
        .iter()
        .enumerate()
        .map(|(i, s)| {
            format!(
                " {}:{{media=\"{}\", name=\"{}\"}}",
                i,
                s.media(),
                s.encoding_name()
            )
        })
        .collect::<Vec<_>>()
        .concat()
}

fn new_decoder(
    rt_handle: Handle,
    logger: ArcMsgLogger,
    mut feed: Feed,
    mut h264_decoder: H264Decoder<Ready>,
    mut frame_rate_limiter: Option<FrameRateLimiter>,
) -> FeedDecoded {
    let (frame_tx, frame_rx) = mpsc::channel(1);

    rt_handle.clone().spawn(async move {
        use DecoderError::*;
        loop {
            use broadcast::error::RecvError;
            let frame = match feed.recv().await {
                Ok(v) => v,
                Err(RecvError::Closed) => {
                    // Close receiver by dropping sender.
                    return;
                }
                Err(RecvError::Lagged(_)) => {
                    _ = frame_tx.send(Err(DroppedFrames)).await;
                    return;
                }
            };

            // State juggling to avoid lifetime issue.
            let avcc = frame.avcc.clone();

            let result: Result<(), SendPacketError>;
            (h264_decoder, result) = rt_handle
                .spawn_blocking(move || {
                    let result = h264_decoder.send_packet(&Packet::new(&avcc).with_pts(*frame.pts));
                    (h264_decoder, result)
                })
                .await
                .expect("join");
            if let Err(e) = result {
                if let SendPacketError::Invaliddata = e {
                    logger.log(LogLevel::Warning, "h264 decoder: send_packet: invalid data");
                    continue;
                }
                _ = frame_tx.send(Err(SendFrame(e))).await;
                return;
            };

            loop {
                let mut frame_decoded = Frame::new();
                match h264_decoder.receive_frame(&mut frame_decoded) {
                    Ok(()) => {}
                    Err(ReceiveFrameError::Eagain) => break,
                    Err(e) => {
                        _ = frame_tx.send(Err(ReceiveFrame(e))).await;
                        return;
                    }
                };
                let pts = match u64::try_from(frame_decoded.pts()) {
                    Ok(v) => v,
                    Err(e) => {
                        _ = frame_tx.send(Err(TryFrom(e))).await;
                        return;
                    }
                };

                let discard = if let Some(limiter) = &mut frame_rate_limiter {
                    match limiter.discard(pts) {
                        Ok(v) => v,
                        Err(e) => {
                            _ = frame_tx.send(Err(FrameRateLimiter(e))).await;
                            return;
                        }
                    }
                } else {
                    false
                };
                if !discard {
                    _ = frame_tx.send(Ok(frame_decoded)).await;
                }
            }
        }
    });

    frame_rx
}
