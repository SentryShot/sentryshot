// SPDX-License-Identifier: GPL-2.0-or-later

use crate::log_monitor;
use common::{
    monitor::{Protocol, RtspUrl, SourceRtspConfig},
    time::DurationH264,
    Cancelled, DynHlsMuxer, DynLogger, DynMsgLogger, H264Data, LogEntry, LogLevel, MonitorId,
    MsgLogger, StreamType,
};
use futures::StreamExt;
use hls::{
    track_params_from_video_params, H264Writer, HlsServer, ParseParamsError,
    SegmenterWriteH264Error,
};
use recording::{FrameRateLimiter, FrameRateLimiterError};
use retina::codec::ParametersRef;
use sentryshot_convert::Frame;
use sentryshot_ffmpeg_h264::{
    H264BuilderError, H264Decoder, H264DecoderBuilder, Packet, PaddedBytes, Ready,
    ReceiveFrameError, SendPacketError,
};
use std::sync::Arc;
use thiserror::Error;
use tokio::{
    runtime::Handle,
    sync::{broadcast, mpsc, oneshot},
};
use tokio_util::sync::CancellationToken;
use url::Url;

// A 'broadcast' channel is used instead of a 'watch' channel to detect dropped frames.
pub type Feed = broadcast::Receiver<H264Data>;
pub type FeedDecoded = mpsc::Receiver<Result<Frame, DecoderError>>;

pub struct Source {
    stream_type: StreamType,
    get_muxer_tx: mpsc::Sender<oneshot::Sender<DynHlsMuxer>>,
    subscribe_tx: mpsc::Sender<oneshot::Sender<Feed>>,
}

impl Source {
    pub fn new(
        stream_type: StreamType,
        get_muxer_tx: mpsc::Sender<oneshot::Sender<DynHlsMuxer>>,
        subscribe_tx: mpsc::Sender<oneshot::Sender<Feed>>,
    ) -> Self {
        Self {
            stream_type,
            get_muxer_tx,
            subscribe_tx,
        }
    }

    pub fn stream_type(&self) -> &StreamType {
        &self.stream_type
    }

    // Returns the HLS muxer for this source. Will block until the source has started.
    pub async fn muxer(&self) -> Result<DynHlsMuxer, Cancelled> {
        let (res_tx, res_rx) = oneshot::channel();
        if self.get_muxer_tx.send(res_tx).await.is_err() {
            return Err(Cancelled);
        }
        let Ok(muxer) = res_rx.await else {
            return Err(Cancelled);
        };
        Ok(muxer)
    }

    // Subscribe to the raw feed. Will block until the source has started.
    pub async fn subscribe(&self) -> Result<Feed, Cancelled> {
        let (res_tx, res_rx) = oneshot::channel();
        if self.subscribe_tx.send(res_tx).await.is_err() {
            return Err(Cancelled);
        }
        let Ok(feed) = res_rx.await else {
            return Err(Cancelled);
        };
        Ok(feed)
    }

    // Subscribe to a decoded feed. Currently creates a new decoder for each
    // call but this may change. Will block until the source has started.
    pub async fn subscribe_decoded(
        &self,
        rt_handle: Handle,
        limiter: Option<FrameRateLimiter>,
    ) -> Result<FeedDecoded, SubscribeDecodedError> {
        let feed = self.subscribe().await?;

        // We could grab the extradata strait from the source instead.
        let muxer = self.muxer().await?;
        let extradata = muxer.params().extra_data.clone();

        let h264_decoder = H264DecoderBuilder::new().avcc(PaddedBytes::new(extradata))?;
        Ok(new_decoder(rt_handle, feed, h264_decoder, limiter))
    }
}

#[derive(Debug, Error)]
pub enum SubscribeDecodedError {
    #[error("{0}")]
    Cancelled(#[from] Cancelled),

    #[error("new h264 decoder: {0}")]
    NewH264Decoder(#[from] H264BuilderError),
}

struct SourceLogger {
    logger: DynLogger,

    monitor_id: MonitorId,
    source_name: String,
    stream_type: StreamType,
}

impl SourceLogger {
    fn new(
        logger: DynLogger,
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
        self.logger.log(LogEntry {
            level,
            source: "monitor".parse().unwrap(),
            monitor_id: Some(self.monitor_id.to_owned()),
            message: format!(
                "({}) {} source: {}",
                self.stream_type.name(),
                self.source_name,
                msg
            )
            .parse()
            .unwrap(),
        })
    }
}

pub struct SourceRtsp {
    msg_logger: DynMsgLogger,
    hls_server: Arc<HlsServer>,

    monitor_id: MonitorId,
    config: SourceRtspConfig,
    stream_type: StreamType,
}

impl SourceRtsp {
    #[allow(clippy::new_ret_no_self, clippy::too_many_arguments)]
    pub async fn new(
        token: CancellationToken,
        shutdown_complete: mpsc::Sender<()>,
        logger: DynLogger,
        hls_server: Arc<HlsServer>,
        monitor_id: MonitorId,
        config: SourceRtspConfig,
        stream_type: StreamType,
    ) -> Option<Source> {
        if stream_type.is_sub() && config.sub_stream.is_none() {
            log_monitor(&logger, LogLevel::Debug, &monitor_id, "no sub stream");
            return None;
        }

        let msg_logger = Arc::new(SourceLogger::new(
            logger,
            monitor_id.to_owned(),
            "rtsp".to_owned(),
            stream_type,
        ));

        let source = Self {
            msg_logger,
            hls_server,
            monitor_id,
            config,
            stream_type,
        };

        let (started_tx, mut started_rx) = mpsc::channel(1);

        let shutdown_complete2 = shutdown_complete.clone();
        let token2 = token.clone();
        tokio::spawn(async move {
            let _shutdown_complete = shutdown_complete2;
            loop {
                if token2.is_cancelled() {
                    source.log(LogLevel::Info, "stopped");
                    return;
                }

                match source.run(token2.child_token(), started_tx.clone()).await {
                    Ok(()) => {}
                    Err(SourceRtspRunError::Cancelled(_)) => {
                        source.log(LogLevel::Debug, "cancelled");
                    }
                    Err(e) => {
                        source.log(LogLevel::Error, &format!("crashed: {}", e));
                    }
                };

                tokio::select! {
                    _ = token2.cancelled() => {}
                    _ = tokio::time::sleep(tokio::time::Duration::from_secs(20)) => {}
                }
            }
        });

        let (get_muxer_tx, mut get_muxer_rx) = mpsc::channel::<oneshot::Sender<DynHlsMuxer>>(1);
        let (subscribe_tx, mut subscribe_rx) = mpsc::channel::<oneshot::Sender<Feed>>(1);

        tokio::spawn(async move {
            let _shutdown_complete = shutdown_complete;
            let mut muxer = None;
            let mut feed_tx = None;
            let mut get_muxer_requests: Vec<oneshot::Sender<_>> = Vec::new();
            let mut subscribe_requests: Vec<oneshot::Sender<_>> = Vec::new();
            loop {
                tokio::select! {
                    _ = token.cancelled() => return,
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

        Some(Source::new(stream_type, get_muxer_tx, subscribe_tx))
    }

    fn log(&self, level: LogLevel, msg: &str) {
        self.msg_logger.log(level, msg)
    }

    async fn run(
        &self,
        token: CancellationToken,
        started_tx: mpsc::Sender<(DynHlsMuxer, broadcast::Sender<H264Data>)>,
    ) -> Result<(), SourceRtspRunError> {
        let url: &Url = self.stream_url();
        let creds = creds_from_url(url);
        let url = remove_creds_from_url(url.to_owned())?;

        let session_group = Arc::new(retina::client::SessionGroup::default());
        let mut session = retina::client::Session::describe(
            url.to_owned(),
            retina::client::SessionOptions::default()
                .creds(creds)
                .session_group(session_group.clone())
                .user_agent("temp".to_string())
                .teardown(retina::client::TeardownPolicy::Always),
        )
        .await
        .map_err(SourceRtspRunError::Describe)?;

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
                return Err(SourceRtspRunError::NoVideoStreamFound);
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
            .play(retina::client::PlayOptions::default())
            .await
            .map_err(SourceRtspRunError::Play)?
            .demuxed()
            .map_err(SourceRtspRunError::Demuxed)?;

        // Buffer 10 frame to reduce dropped frames.
        let (feed_tx, _) = broadcast::channel(10);

        let mut hls_writer: Option<H264Writer> = None;
        loop {
            tokio::select! {
                _ = token.cancelled() => {
                    return Err(SourceRtspRunError::Cancelled(Cancelled));
                },
                pkt = session.next() => {
                    let Some(pkt) = pkt else {
                        return Err(SourceRtspRunError::Eof);
                    };
                    match pkt {
                        Ok(retina::codec::CodecItem::VideoFrame(frame)) => {
                            let stream = &session.streams()[frame.stream_id()];

                            if let Some(hls_writer) = &mut hls_writer {
                                let timestamp = frame.timestamp();

                                let pts = timestamp.timestamp();
                                let Some(dts) = timestamp.dts() else {
                                    continue;
                                };
                                let dts_offset = pts - dts;
                                let pts = timestamp.timestamp();
                                let dts = pts - dts_offset;

                                let data = H264Data{
                                    //ntp: now,
                                    pts: DurationH264::from(pts),
                                    dts: DurationH264::from(dts),
                                    random_access_present: frame.is_random_access_point(),
                                    avcc: Arc::new(PaddedBytes::new(frame.into_data())),
                                };

                                hls_writer.write_h264(data.clone()).await?;
                                _ = feed_tx.send(data);

                            } else if let Some(ParametersRef::Video(params)) = stream.parameters() {
                                let (muxer, hls_writer2) = self.hls_server.new_muxer(
                                    token.clone(),
                                    self.hls_name(),
                                    track_params_from_video_params(params)?,
                                ).await?;
                                hls_writer = Some(hls_writer2);
                                // Notify successful start.
                                _ = started_tx.send((muxer, feed_tx.clone())).await;
                            };
                        },
                        Ok(_) => {},
                        Err(e) => return Err(SourceRtspRunError::Stream(e)),
                    }
                }
            }
        }
    }

    fn hls_name(&self) -> String {
        if self.stream_type.is_main() {
            self.monitor_id.to_string()
        } else {
            self.monitor_id.to_string() + "_sub"
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

#[derive(Debug, Error)]
enum SourceRtspRunError {
    #[error("{0}")]
    Cancelled(#[from] Cancelled),

    #[error("end of file")]
    Eof,

    #[error("describe: {0}")]
    Describe(retina::Error),

    #[error("remove credentials: {0}")]
    RemoveCreds(#[from] RemoveCreds),

    #[error("no suitable video stream found")]
    NoVideoStreamFound,

    #[error("setup: {0}")]
    Setup(retina::Error),

    #[error("play: {0}")]
    Play(retina::Error),

    #[error("demuxed: {0}")]
    Demuxed(retina::Error),

    #[error("stream: {0}")]
    Stream(retina::Error),

    #[error("write h264: {0}")]
    WriteH264(#[from] SegmenterWriteH264Error),

    #[error("convert params: {0}")]
    ConvertTrackParams(#[from] ParseParamsError),
}

#[derive(Debug, Error)]
enum RemoveCreds {
    #[error("set password")]
    SetUsername,

    #[error("set username")]
    SetPassword,
}

fn remove_creds_from_url(mut url: Url) -> Result<Url, RemoveCreds> {
    url.set_username("").map_err(|_| RemoveCreds::SetUsername)?;
    url.set_password(None)
        .map_err(|_| RemoveCreds::SetPassword)?;
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
            password: "".to_owned(),
        })
    } else {
        None
    }
}

#[derive(Debug, Error)]
pub enum DecoderError {
    #[error("{0}")]
    Cancelled(#[from] Cancelled),

    #[error("dropped frames")]
    DroppedFrames,

    #[error("{0}")]
    SendFrame(#[from] SendPacketError),

    #[error("receive frame: {0}")]
    ReceiveFrame(#[from] ReceiveFrameError),

    #[error("try from: {0}")]
    TryFrom(#[from] std::num::TryFromIntError),

    #[error("frame rate limiter: {0}")]
    FrameRateLimiter(#[from] FrameRateLimiterError),
}

fn new_decoder(
    rt_handle: Handle,
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
                    _ = frame_tx.send(Err(Cancelled(common::Cancelled))).await;
                    return;
                }
                Err(RecvError::Lagged(_)) => {
                    _ = frame_tx.send(Err(DroppedFrames)).await;
                    return;
                }
            };

            // State juggling to avoid lifetime issue.
            let avcc = frame.avcc.clone();
            let result = rt_handle
                .spawn_blocking(move || {
                    h264_decoder
                        .send_packet(Packet::new(&avcc).with_pts(*frame.pts))
                        .map(|_| h264_decoder)
                })
                .await
                .unwrap();
            h264_decoder = match result {
                Ok(v) => v,
                Err(e) => {
                    _ = frame_tx.send(Err(SendFrame(e))).await;
                    return;
                }
            };

            loop {
                let mut frame_decoded = Frame::new();
                match h264_decoder.receive_frame(&mut frame_decoded) {
                    Ok(_) => {}
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
