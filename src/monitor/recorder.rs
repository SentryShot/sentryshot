// SPDX-License-Identifier: GPL-2.0-or-later

use crate::ArcMonitorHooks;
use common::{
    ArcLogger, ArcMsgLogger, ArcStreamerMuxer, Event, LogEntry, LogLevel, MonitorId, MsgLogger,
    Segment, TrackParameters,
    monitor::{ArcSource, MonitorConfig},
    time::{Duration, DurationH264, UnixH264, UnixNano},
};
use futures_lite::Future;
use recdb::{NewRecordingError, OpenFileError, RecDb, RecordingHandle};
use recording::{CreateVideoWriterError, MetaHeader, VideoWriter, WriteSampleError};
use sentryshot_convert::{
    ConvertError, Frame, NewConverterError, PixelFormat, PixelFormatConverter,
};
use sentryshot_ffmpeg_h264::{
    DrainError, H264BuilderError, H264DecoderBuilder, Packet, PaddedBytes, ReceiveFrameError,
    SendPacketError,
};
use sentryshot_util::ImageCopyToBufferError;
use std::{pin::Pin, sync::Arc, task::Poll};
use thiserror::Error;
use tokio::{
    io::{AsyncWriteExt, BufWriter},
    sync::mpsc,
    time::sleep,
};
use tokio_util::{sync::CancellationToken, task::task_tracker::TaskTrackerToken};

#[allow(clippy::too_many_arguments, clippy::module_name_repetitions)]
pub fn new_recorder(
    token: CancellationToken,
    task_token: TaskTrackerToken,
    hooks: ArcMonitorHooks,
    logger: ArcLogger,
    monitor_id: MonitorId,
    source_main: ArcSource,
    source_sub: Option<ArcSource>,
    config: MonitorConfig,
    recdb_main: Arc<RecDb>,
    recdb_sub: Arc<RecDb>,
) -> mpsc::Sender<(Duration, Event)> {
    let (send_event_tx, mut send_event_rx) = mpsc::channel::<(Duration, Event)>(1);
    let c = RecordingContext {
        hooks,
        logger: Arc::new(RecorderMsgLogger::new(logger, monitor_id)),
        source_main,
        source_sub,
        config,
        recdb_main,
        recdb_sub,
    };

    // Recorder actor.
    tokio::spawn(async move {
        let _task_token = task_token;

        let mut state = if c.config.always_record() {
            c.log(LogLevel::Debug, "alwaysRecord=true");
            State::Recording(RecordingSession::new(&token, None, &c, PrevSegs::default()))
        } else {
            c.log(LogLevel::Debug, "alwaysRecord=false");
            State::NotRecording(PrevSegs::default())
        };

        loop {
            match state {
                State::Recording(mut session) => {
                    tokio::select! {
                        () = token.cancelled() => {
                            // Wait for session to exit.
                            _ = session.on_exit_rx.recv().await;
                            return;
                        }

                        event = send_event_rx.recv() => { // Incomming events.
                            session.handle_event(event);
                            state = State::Recording(session);
                        }

                        () = session.sleep_until_timer_end() => {
                            assert!(!c.config.always_record());

                            c.log(LogLevel::Debug, "timer reached end, canceling session");
                            let prev_segs = session.cancel().await;
                            c.log(LogLevel::Debug, "session stopped");

                            state = State::NotRecording(prev_segs);
                        }

                        prev_seg = session.on_exit_rx.recv() => {
                            c.log(LogLevel::Debug, "session stopped2");
                            state = State::NotRecording(prev_seg.expect("complete"));
                        }
                    }
                }
                State::NotRecording(prev_seg) => {
                    tokio::select! {
                        () = token.cancelled() => return,
                        event = send_event_rx.recv() => { // Incomming events.
                            let Some((trigger_duration, event)) = event else {
                                return
                            };
                            let Some(end) = event.time.checked_add(trigger_duration.into()) else {
                                state = State::NotRecording(prev_seg);
                                continue
                            };
                            if !end.after(UnixNano::now()) {
                                state = State::NotRecording(prev_seg);
                                continue
                            }

                            state = State::Recording(RecordingSession::new(
                                &token,
                                Some(end),
                                &c,
                                prev_seg,
                            ));
                        }
                    };
                }
            }
        }
    });

    send_event_tx
}

enum State {
    Recording(RecordingSession),
    NotRecording(PrevSegs),
}

type PrevSeg = Option<Segment>;

#[derive(Default)]
struct PrevSegs {
    main: PrevSeg,
    sub: PrevSeg,
}

struct RecordingSession {
    token: CancellationToken,
    logger: ArcMsgLogger,
    timer_end: Option<UnixNano>, // None if always record.
    on_exit_rx: mpsc::Receiver<PrevSegs>,
}

impl RecordingSession {
    fn new(
        parent_token: &CancellationToken,
        timer_end: Option<UnixNano>,
        c: &RecordingContext,
        prev_segs: PrevSegs,
    ) -> Self {
        c.log(LogLevel::Debug, "starting recording session");
        let token = parent_token.child_token();
        let (mut prev_main_seg, mut prev_sub_seg) = (prev_segs.main, prev_segs.sub);

        // Spawn main recording.
        let token2 = token.clone();
        let c2 = c.clone();
        let (on_main_exit_tx, mut on_main_exit_rx) = mpsc::channel::<PrevSeg>(1);
        tokio::spawn(async move {
            run_main_recording_session(token2, c2, &mut prev_main_seg).await;
            on_main_exit_tx
                .send(prev_main_seg)
                .await
                .expect("task should receive");
        });

        // Spawn sub recording.
        let token2 = token.clone();
        let c2 = c.clone();
        let (on_sub_exit_tx, mut on_sub_exit_rx) = mpsc::channel::<PrevSeg>(1);
        tokio::spawn(async move {
            run_sub_recording_session(token2, c2, &mut prev_sub_seg).await;

            on_sub_exit_tx
                .send(prev_sub_seg)
                .await
                .expect("task should receive");
        });

        // Await recordings.
        let (on_exit_tx, on_exit_rx) = mpsc::channel(1);
        tokio::spawn(async move {
            let prev_segs = PrevSegs {
                main: on_main_exit_rx.recv().await.expect("main should exit"),
                sub: on_sub_exit_rx.recv().await.expect("sub should exit"),
            };
            on_exit_tx
                .send(prev_segs)
                .await
                .expect("recorder should wait for the session to exit");
        });

        RecordingSession {
            token: token.clone(),
            logger: c.logger.clone(),
            timer_end,
            on_exit_rx,
        }
    }

    fn handle_event(&mut self, event: Option<(Duration, Event)>) {
        let Some((trigger_duration, event)) = event else {
            return;
        };

        // Update timer if the monitor isn't set to always record.
        if let Some(timer_end) = self.timer_end {
            let Some(end) = event.time.checked_add(trigger_duration.into()) else {
                return;
            };
            if end.after(timer_end) {
                self.logger.log(
                    LogLevel::Debug,
                    "new event, already recording, updating timer",
                );
                self.timer_end = Some(end);
            }
        }
    }

    fn sleep_until_timer_end(&self) -> Sleep {
        let Some(timer_end) = self.timer_end else {
            return Sleep(None);
        };
        let Some(timer_end) = UnixNano::until(timer_end) else {
            self.logger.log(LogLevel::Error, "Duration::until failed");
            return Sleep(None);
        };
        let Some(timer_end) = timer_end.as_std() else {
            self.logger.log(LogLevel::Error, "timer_end.as_std failed");
            return Sleep(None);
        };
        Sleep(Some(Box::pin(tokio::time::sleep(timer_end))))
    }

    async fn cancel(mut self) -> PrevSegs {
        self.token.cancel();
        self.on_exit_rx.recv().await.expect("should exit")
    }
}

// Future will always return pending if this is None.
struct Sleep(Option<Pin<Box<tokio::time::Sleep>>>);

impl Future for Sleep {
    type Output = ();

    fn poll(
        mut self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
    ) -> std::task::Poll<Self::Output> {
        match &mut self.0 {
            Some(v) => Pin::new(v).poll(cx),
            None => Poll::Pending,
        }
    }
}

async fn run_main_recording_session(
    recording_session_token: CancellationToken,
    c: RecordingContext,
    prev_seg: &mut PrevSeg,
) {
    let restart_sleep = std::time::Duration::from_secs(3);
    loop {
        let result = run_main_recording(recording_session_token.clone(), c.clone(), prev_seg).await;
        if let Err(e) = result {
            c.log(LogLevel::Error, &format!("recording crashed: {e}"));
            tokio::select! {
                () = recording_session_token.cancelled() => return,
                () = sleep(restart_sleep) => {}
            }
            c.log(LogLevel::Debug, "recovering after crash");
            continue;
        }
        c.log(LogLevel::Debug, "recording finished");
        // Recoding reached videoLength and exited normally. The timer
        // is still active, so continue the loop and start another recording.
        tokio::select! {
            () = recording_session_token.cancelled() => return,
            () = sleep(restart_sleep) => {}
        }
    }
}

async fn run_sub_recording_session(
    recording_session_token: CancellationToken,
    c: RecordingContext,
    prev_seg: &mut PrevSeg,
) {
    let Some(source_sub) = c.clone().source_sub else {
        c.log(LogLevel::Debug, "substream disabled");
        return;
    };
    let restart_sleep = std::time::Duration::from_secs(3);
    loop {
        let result = run_sub_recording(
            recording_session_token.clone(),
            &c.logger,
            &c.recdb_sub,
            c.config.id().to_owned(),
            &source_sub,
            prev_seg,
        )
        .await;
        if let Err(e) = result {
            c.log(LogLevel::Error, &format!("sub recording crashed: {e}"));
            tokio::select! {
                () = recording_session_token.cancelled() => return,
                () = sleep(restart_sleep) => {}
            }
            c.log(LogLevel::Debug, "recovering sub after crash");
            continue;
        }
        c.log(LogLevel::Debug, "sub recording finished");
        // Recoding reached videoLength and exited normally. The timer
        // is still active, so continue the loop and start another recording.
        tokio::select! {
            () = recording_session_token.cancelled() => return,
            () = sleep(restart_sleep) => {}
        }
    }
}

#[derive(Debug, Error)]
enum RunRecordingError {
    #[error("new recording: {0}")]
    NewRecording(#[from] NewRecordingError),

    #[error("generate video: {0}")]
    GenerateVideo(#[from] GenerateVideoError),

    #[error("save end file: {0}")]
    SaveEndFile(#[from] OpenFileError),
}

#[derive(Clone)]
struct RecordingContext {
    hooks: ArcMonitorHooks,
    logger: ArcMsgLogger,
    source_main: ArcSource,
    source_sub: Option<ArcSource>,
    config: MonitorConfig,
    recdb_main: Arc<RecDb>,
    recdb_sub: Arc<RecDb>,
}

impl RecordingContext {
    fn log(&self, level: LogLevel, msg: &str) {
        self.logger.log(level, msg);
    }
}

async fn run_main_recording(
    recording_session_token: CancellationToken,
    c: RecordingContext,
    prev_seg: &mut PrevSeg,
) -> Result<(), RunRecordingError> {
    let Some(muxer) = c.source_main.muxer().await else {
        c.log(LogLevel::Debug, "main source cancelled");
        return Ok(());
    };

    let Some(first_segment) = muxer.next_segment(prev_seg.clone()).await else {
        c.log(LogLevel::Debug, "main muxer cancelled");
        return Ok(());
    };

    let start_time = first_segment.start_time();

    let monitor_id = c.config.id().to_owned();
    let recording = c
        .recdb_main
        .new_recording(monitor_id.clone(), start_time)
        .await?;

    let video_length = DurationH264::from(c.config.video_length());

    c.log(
        LogLevel::Info,
        &format!("starting recording: {}", recording.id()),
    );

    let params = muxer.params();

    let result = generate_thumbnail(
        c.hooks.clone(),
        &c.logger,
        c.config.clone(),
        &recording,
        &first_segment,
        params,
    )
    .await;
    if let Err(e) = result {
        c.log(
            LogLevel::Error,
            &format!("failed to generate thumbnail: {}", &e),
        );
    }

    let end_time = generate_video(
        recording_session_token,
        &recording,
        &muxer,
        first_segment,
        params,
        video_length,
        prev_seg,
    )
    .await?;

    c.log(
        LogLevel::Debug,
        &format!("video generated: {}", recording.id()),
    );

    recording
        .new_file(&format!("{}.end", UnixNano::from(end_time)))
        .await
        .map_err(RunRecordingError::SaveEndFile)?;

    Ok(())
}

async fn run_sub_recording(
    recording_session_token: CancellationToken,
    logger: &ArcMsgLogger,
    recdb_sub: &Arc<RecDb>,
    monitor_id: MonitorId,
    source_sub: &ArcSource,
    prev_seg: &mut PrevSeg,
) -> Result<(), RunRecordingError> {
    let Some(muxer) = source_sub.muxer().await else {
        logger.log(LogLevel::Debug, "main source cancelled");
        return Ok(());
    };

    let Some(first_segment) = muxer.next_segment(prev_seg.clone()).await else {
        logger.log(LogLevel::Debug, "main muxer cancelled");
        return Ok(());
    };

    let start_time = first_segment.start_time();
    let recording = recdb_sub
        .new_recording(monitor_id.clone(), start_time)
        .await?;

    logger.log(
        LogLevel::Debug,
        &format!("starting sub recording: {}", recording.id()),
    );

    /* 17 minutes */
    let max_duration = Duration::from_secs(1000).into();
    let params = muxer.params();

    let end_time = generate_video(
        recording_session_token,
        &recording,
        &muxer,
        first_segment,
        params,
        max_duration,
        prev_seg,
    )
    .await?;

    logger.log(
        LogLevel::Debug,
        &format!("sub video generated: {}", recording.id()),
    );

    recording
        .new_file(&format!("{}.end", UnixNano::from(end_time)))
        .await
        .map_err(RunRecordingError::SaveEndFile)?;

    Ok(())
}

#[derive(Debug, Error)]
enum GenerateVideoError {
    #[error("open file: {0}")]
    OpenFile(#[from] OpenFileError),

    #[error("new video writer: {0}")]
    NewVideoWriter(#[from] CreateVideoWriterError),

    #[error("add")]
    Add,

    #[error("write sample: {0}")]
    WriteSample(#[from] WriteSampleError),

    #[error("skipped segment: expected: {0}, got: {1}. this may be a disk issue")]
    SkippedSegment(u64, u64),
}

async fn generate_video(
    token: CancellationToken,
    recording: &RecordingHandle,
    muxer: &ArcStreamerMuxer,
    first_segment: Segment,
    params: &TrackParameters,
    max_duration: DurationH264,
    prev_seg: &mut PrevSeg,
) -> Result<UnixH264, GenerateVideoError> {
    use GenerateVideoError::*;

    let name = format!("video_{}x{}", params.width, params.height);
    let mut meta = recording.new_file(&format!("{name}.meta")).await?;
    let mut meta = BufWriter::with_capacity(64 * 1024, &mut *meta);

    let mut mdat = recording.new_file(&format!("{name}.mdat")).await?;
    let mut mdat = BufWriter::with_capacity(64 * 1024, &mut *mdat);

    let start_time = first_segment.start_time();
    let stop_time = first_segment
        .start_time()
        .checked_add(max_duration.into())
        .ok_or(GenerateVideoError::Add)?;

    let header = MetaHeader {
        start_time,
        width: params.width,
        height: params.height,
        extra_data: params.extra_data.clone(),
    };

    let mut w = VideoWriter::new(&mut meta, &mut mdat, header).await?;

    w.write_frames(first_segment.frames()).await?;
    let end_time = first_segment
        .start_time()
        .checked_add(first_segment.duration().into())
        .ok_or(Add)?;
    recording.set_end_time(end_time);
    let mut prev_seg_id = first_segment.id();
    *prev_seg = Some(first_segment); // prev_seg should be updated after frames have been written.

    loop {
        if token.is_cancelled() {
            return Ok(end_time);
        }

        let Some(seg) = muxer.next_segment(prev_seg.clone()).await else {
            return Ok(end_time);
        };

        if seg.id() != prev_seg_id + 1 {
            return Err(SkippedSegment(seg.id(), prev_seg_id + 1));
        }

        w.write_frames(seg.frames()).await?;
        let end_time = seg
            .start_time()
            .checked_add(seg.duration().into())
            .ok_or(Add)?;
        recording.set_end_time(end_time);
        prev_seg_id = seg.id();
        let seg_start_time = seg.start_time();
        *prev_seg = Some(seg);

        if seg_start_time.after(stop_time) {
            return Ok(end_time);
        }
    }
}

#[derive(Debug, Error)]
enum GenerateThumbnailError {
    #[error("no frame")]
    NoFrame,

    #[error("sample is not an IDR")]
    SampleNotIdr,

    #[error("avcc to jpeg: {0}")]
    AvccToJpeg(#[from] AvccToJpegError),

    #[error("open file: {0}")]
    OpenFile(#[from] OpenFileError),

    #[error("write file: {0}")]
    WriteFile(std::io::Error),

    #[error("flush jpeg file: {0}")]
    FlushFile(std::io::Error),
}

// The first h264 frame in firstSegment is wrapped in a mp4
// container and piped into FFmpeg and then converted to jpeg.
async fn generate_thumbnail(
    hooks: ArcMonitorHooks,
    logger: &ArcMsgLogger,
    config: MonitorConfig,
    recording: &RecordingHandle,
    first_segment: &Segment,
    params: &TrackParameters,
) -> Result<(), GenerateThumbnailError> {
    use GenerateThumbnailError::*;

    logger.log(LogLevel::Debug, "generating thumbnail");

    let Some(first_sample) = first_segment.frames().next() else {
        return Err(NoFrame);
    };
    if !first_sample.random_access_present {
        return Err(SampleNotIdr);
    }

    let avcc = first_sample.avcc.clone();
    let extra_data = params.extra_data.clone();
    let jpeg_buf = tokio::task::spawn_blocking(move || {
        avcc_to_jpeg(&hooks, &config, &avcc, PaddedBytes::new(extra_data))
    })
    .await
    .expect("join")?;

    let name = format!("thumb_{}x{}.jpeg", params.width, params.height);
    let mut file = recording.new_file(&name).await?;
    file.write_all(&jpeg_buf).await.map_err(WriteFile)?;
    file.flush().await.map_err(FlushFile)?;

    logger.log(
        LogLevel::Debug,
        &format!("thumbnail generated: {:?}", file.path()),
    );

    Ok(())
}

#[derive(Debug, Error)]
enum AvccToJpegError {
    #[error("new h264 decoder: {0}")]
    NewH264Decoder(#[from] H264BuilderError),

    #[error("send avcc packet: {0}")]
    SendPacket(#[from] SendPacketError),

    #[error("drain h264 decoder: {0}")]
    Drain(#[from] DrainError),

    #[error("receive frame: {0}")]
    ReceiveFrame(#[from] ReceiveFrameError),

    #[error("new converter: {0}")]
    NewConverter(#[from] NewConverterError),

    #[error("convert: {0}")]
    Convert(#[from] ConvertError),

    #[error("copy to buffer: {0}")]
    CopyToBuffer(#[from] ImageCopyToBufferError),

    #[error("encode jpeg: {0}")]
    EncodeJpeg(#[from] jpeg_encoder::EncodingError),
}

fn avcc_to_jpeg(
    hooks: &ArcMonitorHooks,
    config: &MonitorConfig,
    avcc: &PaddedBytes,
    extradata: PaddedBytes,
) -> Result<Vec<u8>, AvccToJpegError> {
    let mut decoder = H264DecoderBuilder::new().avcc(extradata)?;

    decoder.send_packet(&Packet::new(avcc))?;

    let mut h264_decoder = decoder.drain()?;

    let mut frame = Frame::new();
    h264_decoder.receive_frame(&mut frame)?;

    let frame = hooks.on_thumb_save(config, frame);

    let mut converter = PixelFormatConverter::new(
        frame.width(),
        frame.height(),
        frame.color_range(),
        frame.pix_fmt(),
        PixelFormat::RGB24,
    )?;

    let mut rgb_frame = Frame::new();

    converter.convert(&frame, &mut rgb_frame)?;

    let mut raw_rgb_frame = Vec::new();
    rgb_frame.copy_to_buffer(&mut raw_rgb_frame, 1)?;

    let mut jpeg_buf = Vec::new();
    let jpeg_encoder = jpeg_encoder::Encoder::new(&mut jpeg_buf, 75);

    jpeg_encoder.encode(
        &raw_rgb_frame,
        rgb_frame.width().get(),
        rgb_frame.height().get(),
        jpeg_encoder::ColorType::Rgb,
    )?;

    Ok(jpeg_buf)
}

struct RecorderMsgLogger {
    logger: ArcLogger,
    monitor_id: MonitorId,
}

impl RecorderMsgLogger {
    fn new(logger: ArcLogger, monitor_id: MonitorId) -> Self {
        Self { logger, monitor_id }
    }
}

impl MsgLogger for RecorderMsgLogger {
    fn log(&self, level: LogLevel, msg: &str) {
        self.logger.log(LogEntry::new(
            level,
            "monitor",
            &self.monitor_id,
            &format!("recorder: {msg}"),
        ));
    }
}
