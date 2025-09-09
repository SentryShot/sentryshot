// SPDX-License-Identifier: GPL-2.0-or-later

use crate::ArcMonitorHooks;
use common::{
    ArcLogger, ArcMsgLogger, ArcStreamerMuxer, Event, LogEntry, LogLevel, MonitorId, MsgLogger,
    Segment, TrackParameters,
    monitor::{ArcSource, MonitorConfig},
    recording::{RecordingData, RecordingId},
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
    sync::{Mutex, mpsc},
    time::sleep,
};
use tokio_util::sync::CancellationToken;

#[allow(clippy::too_many_arguments, clippy::module_name_repetitions)]
pub fn new_recorder(
    token: CancellationToken,
    shutdown_complete: mpsc::Sender<()>,
    hooks: ArcMonitorHooks,
    logger: ArcLogger,
    monitor_id: MonitorId,
    source_main: ArcSource,
    config: MonitorConfig,
    rec_db: Arc<RecDb>,
) -> mpsc::Sender<(Duration, Event)> {
    let (send_event_tx, mut send_event_rx) = mpsc::channel::<(Duration, Event)>(1);
    let c = RecordingContext {
        hooks,
        logger: Arc::new(RecorderMsgLogger::new(logger, monitor_id)),
        source_main,
        prev_seg: Arc::new(Mutex::new(None)),
        config,
        rec_db,
    };

    // Recorder actor.
    tokio::spawn(async move {
        let shutdown_complete = shutdown_complete;

        let mut recording_session: Option<RecordingSession> = None;
        if c.config.always_record() {
            c.log(LogLevel::Debug, "alwaysRecord=true");
            recording_session = Some(RecordingSession::new(
                &token,
                None,
                c.clone(),
                shutdown_complete.clone(),
            ));
        } else {
            c.log(LogLevel::Debug, "alwaysRecord=false");
        }

        loop {
            // Is recording.
            if let Some(session) = &mut recording_session {
                tokio::select! {
                    () = token.cancelled() => {
                        // Wait for session to exit.
                        _ = session.on_exit_rx.recv().await;
                        return;
                    }

                    event = send_event_rx.recv() => { // Incomming events.
                        let Some((trigger_duration, event)) = event else {
                            continue
                        };


                        let Some(end) = event.time.checked_add(trigger_duration.into()) else {
                            continue
                        };

                        // Update timer if the monitor isn't set to always record.
                        if let Some(timer_end) = session.timer_end {
                            if end.after(timer_end) {
                                c.log(LogLevel::Debug, "new event, already recording, updating timer");
                                session.timer_end = Some(end);
                            }
                        }
                    }

                    // This should never complete if monitor is set to always record.
                    () = session.sleep_until_timer_end() => {
                        assert!(!c.config.always_record());

                        c.log(LogLevel::Debug, "timer reached end, canceling session");
                        session.token.cancel();

                        // Avoid sleeping again.
                        session.on_exit_rx.recv().await;
                        c.log(LogLevel::Debug, "session stopped");
                        recording_session = None;
                    }

                    _ = session.on_exit_rx.recv() => {
                        c.log(LogLevel::Debug, "session stopped2");
                        recording_session = None;
                    }
                }
            } else {
                tokio::select! {
                    () = token.cancelled() => return,
                    event = send_event_rx.recv() => { // Incomming events.
                        let Some((trigger_duration, event)) = event else {
                            return
                        };
                        let Some(end) = event.time.checked_add(trigger_duration.into()) else {
                            continue
                        };
                        if !end.after(UnixNano::now()) {
                            continue
                        }

                        recording_session = Some(RecordingSession::new(
                            &token,
                            Some(end),
                            c.clone(),
                            shutdown_complete.clone()),
                        );
                    }
                }
            }
        }
    });

    send_event_tx
}

struct RecordingSession {
    token: CancellationToken,
    logger: ArcMsgLogger,
    timer_end: Option<UnixNano>,
    on_exit_rx: mpsc::Receiver<()>,
}

impl RecordingSession {
    fn new(
        parent_token: &CancellationToken,
        timer_end: Option<UnixNano>,
        c: RecordingContext,
        shutdown_complete: mpsc::Sender<()>,
    ) -> Self {
        c.log(LogLevel::Debug, "starting recording session");

        let token = parent_token.child_token();
        let (on_session_exit_tx, on_session_exit_rx) = mpsc::channel::<()>(1);
        let recording_session = RecordingSession {
            token: token.clone(),
            logger: c.logger.clone(),
            timer_end,
            on_exit_rx: on_session_exit_rx,
        };

        tokio::spawn(async move {
            run_recording_session(
                token,
                c,
                shutdown_complete,
                std::time::Duration::from_secs(3),
            )
            .await;
            _ = on_session_exit_tx.send(()).await;
        });

        recording_session
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

async fn run_recording_session(
    session_token: CancellationToken,
    c: RecordingContext,
    _shutdown_complete: mpsc::Sender<()>,
    restart_sleep: std::time::Duration,
) {
    loop {
        if let Err(e) = run_recording(session_token.clone(), c.clone()).await {
            c.log(LogLevel::Error, &format!("recording crashed: {e}"));

            tokio::select! {
                () = session_token.cancelled() => return,
                () = sleep(restart_sleep) => {}
            }
            c.log(LogLevel::Debug, "recovering after crash");
            continue;
        }
        c.log(LogLevel::Debug, "recording finished");
        // Recoding reached videoLength and exited normally. The timer
        // is still active, so continue the loop and start another one.
        tokio::select! {
            () = session_token.cancelled() => return,
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

    #[error("save recording: {0}")]
    SaveRecording(#[from] SaveRecordingError),
}

#[derive(Clone)]
struct RecordingContext {
    hooks: ArcMonitorHooks,
    logger: ArcMsgLogger,
    source_main: ArcSource,
    prev_seg: Arc<Mutex<Option<Segment>>>,
    config: MonitorConfig,
    rec_db: Arc<RecDb>,
}

impl RecordingContext {
    fn log(&self, level: LogLevel, msg: &str) {
        self.logger.log(level, msg);
    }
}

async fn run_recording(
    token: CancellationToken,
    c: RecordingContext,
) -> Result<(), RunRecordingError> {
    let Some(muxer) = c.source_main.muxer().await else {
        c.log(LogLevel::Debug, "source cancelled");
        return Ok(());
    };

    let Some(first_segment) = muxer.next_segment(c.prev_seg.lock().await.clone()).await else {
        c.log(LogLevel::Debug, "muxer cancelled");
        return Ok(());
    };

    let start_time = first_segment.start_time();

    let monitor_id = c.config.id().to_owned();
    let recording = c
        .rec_db
        .new_recording(monitor_id.clone(), start_time)
        .await?;

    let video_length = DurationH264::from(c.config.video_length());

    c.log(
        LogLevel::Info,
        &format!("starting recording: {:?}", recording.id()),
    );

    let params = muxer.params();

    let result = generate_thumbnail(
        c.hooks.clone(),
        &c.logger,
        c.config.clone(),
        &recording,
        &first_segment,
        muxer.params().extra_data.clone(),
    )
    .await;
    if let Err(e) = result {
        c.log(
            LogLevel::Error,
            &format!("failed to generate thumbnail: {}", &e),
        );
    }

    let (new_prev_seg, end_time) = generate_video(
        token,
        &recording,
        &muxer,
        first_segment,
        params,
        video_length,
    )
    .await?;
    *c.prev_seg.lock().await = Some(new_prev_seg);

    c.log(
        LogLevel::Debug,
        &format!("video generated: {:?}", recording.id()),
    );

    save_recording(
        c.logger.clone(),
        recording.id(),
        &recording,
        UnixNano::from(start_time),
        UnixNano::from(end_time),
    )
    .await?;

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
) -> Result<(Segment, UnixH264), GenerateVideoError> {
    use GenerateVideoError::*;

    let start_time = first_segment.start_time();

    let stop_time = first_segment
        .start_time()
        .checked_add(max_duration.into())
        .ok_or(GenerateVideoError::Add)?;

    let mut meta = recording.new_file("meta").await?;
    let mut meta = BufWriter::with_capacity(64 * 1024, &mut *meta);

    let mut mdat = recording.new_file("mdat").await?;
    let mut mdat = BufWriter::with_capacity(64 * 1024, &mut *mdat);

    let header = MetaHeader {
        start_time,
        width: params.width,
        height: params.height,
        extra_data: params.extra_data.clone(),
    };

    let mut w = VideoWriter::new(&mut meta, &mut mdat, header).await?;

    w.write_frames(first_segment.frames()).await?;

    let mut prev_seg = first_segment.clone();
    let mut end_time = first_segment
        .start_time()
        .checked_add(first_segment.duration().into())
        .ok_or(Add)?;

    loop {
        if token.is_cancelled() {
            return Ok((prev_seg, end_time));
        }

        let Some(seg) = muxer.next_segment(Some(prev_seg.clone())).await else {
            return Ok((prev_seg, end_time));
        };

        if seg.id() != prev_seg.id() + 1 {
            return Err(SkippedSegment(seg.id(), prev_seg.id() + 1));
        }

        prev_seg = seg.clone();
        w.write_frames(seg.frames()).await?;
        end_time = seg
            .start_time()
            .checked_add(seg.duration().into())
            .ok_or(Add)?;
        recording.set_end_time(end_time);

        if seg.start_time().after(stop_time) {
            return Ok((seg, end_time));
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
    extradata: Vec<u8>,
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
    let jpeg_buf = tokio::task::spawn_blocking(move || {
        avcc_to_jpeg(&hooks, &config, &avcc, PaddedBytes::new(extradata))
    })
    .await
    .expect("join")?;

    let mut file = recording.new_file("jpeg").await?;
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

#[derive(Debug, Error)]
enum SaveRecordingError {
    #[error("serialize data: {0}")]
    Serialize(#[from] serde_json::Error),

    #[error("open file: {0}")]
    OpenFile(#[from] OpenFileError),

    #[error("write data file: {0}")]
    Write(std::io::Error),

    #[error("flush data file: {0}")]
    Flush(std::io::Error),
}

async fn save_recording(
    logger: ArcMsgLogger,
    rec_id: &RecordingId,
    recording: &RecordingHandle,
    start_time: UnixNano,
    end_time: UnixNano,
) -> Result<(), SaveRecordingError> {
    use SaveRecordingError::*;
    logger.log(LogLevel::Debug, &format!("saving recording: {rec_id:?}"));

    let data = RecordingData {
        start: start_time,
        end: end_time,
        events: Vec::new(),
    };

    let json = serde_json::to_vec_pretty(&data)?;

    let mut data_file = recording.new_file("json").await?;
    data_file.write_all(&json).await.map_err(Write)?;
    data_file.flush().await.map_err(Flush)?;

    //go r.hooks.RecSaved(r, filePath, data)

    logger.log(LogLevel::Info, &format!("recording saved: {rec_id:?}"));

    Ok(())
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

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use bytesize::ByteSize;
    use common::{DummyLogger, time::MINUTE};
    use pretty_assertions::assert_eq;
    use recdb::DiskImpl;
    use std::path::Path;
    use tempfile::tempdir;
    use tokio::io::AsyncReadExt;
    /*
    func newTestRecorder(t *testing.T) *Recorder {
        t.Helper()
        tempDir := t.TempDir()
        t.Cleanup(func() {
            os.Remove(tempDir)
        })

        logf := func(level log.Level, format string, a ...interface{}) {}
        return &Recorder{
            Config: NewConfig(RawConfig{
                "timestampOffset": "0",
                "videoLength":     "0.0003",
            }),

            events:     &storage.Events{},
            eventsLock: sync.Mutex{},
            eventChan:  make(chan storage.Event),

            logf:       logf,
            runSession: runRecording,
            NewProcess: ffmock.NewProcess,

            input: &InputProcess{
                isSubInput: false,

                serverPath: video.ServerPath{
                    HlsAddress: "hls.m3u8",
                    HLSMuxer: newMockMuxerFunc(
                        &mockMuxer{videoTrack: &gortsplib.TrackH264{SPS: []byte{0, 0, 0}}},
                    ),
                },

                logf: logf,

                runInputProcess: stubRunInputProcess,
                newProcess:      ffmock.NewProcess,
            },
            wg: &sync.WaitGroup{},
            Env: storage.ConfigEnv{
                TempDir:    tempDir,
                StorageDir: tempDir,
            },
            hooks: stubHooks(),
        }
    }

    type mockMuxer struct {
        videoTrack  *gortsplib.TrackH264
        audioTrack  *gortsplib.TrackMPEG4Audio
        getMuxerErr error
        segCount    int
    }

    func newMockMuxerFunc(muxer *mockMuxer) func(context.Context) (video.IHLSMuxer, error) {
        return func(ctx context.Context) (video.IHLSMuxer, error) {
            return muxer, muxer.getMuxerErr
        }
    }

    func (m *mockMuxer) VideoTrack() *gortsplib.TrackH264 {
        return m.videoTrack
    }

    func (m *mockMuxer) AudioTrack() *gortsplib.TrackMPEG4Audio {
        return m.audioTrack
    }

    func (m *mockMuxer) NextSegment(prevID uint64) (*hls.Segment, error) {
        seg := &hls.Segment{
            ID:        uint64(m.segCount),
            StartTime: time.Unix(1*int64(m.segCount), 0),
        }
        m.segCount++
        return seg, nil
    }

    func (m *mockMuxer) WaitForSegFinalized() {}

    func TestStartRecorder(t *testing.T) {
        t.Run("timeout", func(t *testing.T) {
            onRunRecording := make(chan struct{})
            onCanceled := make(chan struct{})
            mockRunRecording := func(ctx context.Context, _ *Recorder) error {
                close(onRunRecording)
                <-ctx.Done()
                close(onCanceled)
                return nil
            }

            ctx, cancel := context.WithCancel(context.Background())
            defer cancel()

            r := newTestRecorder(t)
            r.sleep = 1 * time.Hour
            r.wg.Add(1)
            r.runSession = mockRunRecording
            go r.start(ctx)

            err := r.sendEvent(ctx, storage.Event{
                Time:        time.Now().Add(time.Duration(-1) * time.Hour),
                RecDuration: 1,
            })
            require.NoError(t, err)

            <-onRunRecording
            <-onCanceled
        })
        t.Run("timeoutUpdate", func(t *testing.T) {
            onRunRecording := make(chan struct{})
            mockRunRecording := func(ctx context.Context, _ *Recorder) error {
                close(onRunRecording)
                <-ctx.Done()
                return nil
            }

            ctx, cancel := context.WithCancel(context.Background())
            defer cancel()

            r := newTestRecorder(t)
            r.wg.Add(1)
            r.runSession = mockRunRecording
            go r.start(ctx)

            now := time.Now()
            r.eventChan <- storage.Event{Time: now, RecDuration: 20 * time.Millisecond}
            r.eventChan <- storage.Event{Time: now, RecDuration: 60 * time.Millisecond}

            <-onRunRecording
        })
        t.Run("recordingCheck", func(t *testing.T) {
            onRunRecording := make(chan struct{})
            mockRunRecording := func(ctx context.Context, _ *Recorder) error {
                close(onRunRecording)
                <-ctx.Done()
                return nil
            }

            ctx, cancel := context.WithCancel(context.Background())
            defer cancel()

            r := newTestRecorder(t)
            r.wg.Add(1)
            r.runSession = mockRunRecording
            go r.start(ctx)

            now := time.Now()
            r.eventChan <- storage.Event{Time: now, RecDuration: 10 * time.Millisecond}
            r.eventChan <- storage.Event{Time: now, RecDuration: 11 * time.Millisecond}
            r.eventChan <- storage.Event{Time: now, RecDuration: 0 * time.Millisecond}

            <-onRunRecording
        })
        // Only update timeout if new time is after current time.
        t.Run("updateTimeout", func(t *testing.T) {
            onCancel := make(chan struct{})
            mockRunRecording := func(ctx context.Context, _ *Recorder) error {
                <-ctx.Done()
                close(onCancel)
                return nil
            }

            ctx, cancel := context.WithCancel(context.Background())
            defer cancel()

            r := newTestRecorder(t)
            r.wg.Add(1)
            r.runSession = mockRunRecording
            go r.start(ctx)

            now := time.Now()
            r.eventChan <- storage.Event{Time: now, RecDuration: 30 * time.Millisecond}
            r.eventChan <- storage.Event{Time: now, RecDuration: 1 * time.Millisecond}

            select {
            case <-time.After(15 * time.Millisecond):
            case <-onCancel:
                t.Fatal("the second trigger reset the timeout")
            }
        })
        t.Run("normalExit", func(t *testing.T) {
            onRunRecording := make(chan struct{})
            exitProcess := make(chan error)
            mockRunRecording := func(ctx context.Context, _ *Recorder) error {
                onRunRecording <- struct{}{}
                return <-exitProcess
            }

            ctx, cancel := context.WithCancel(context.Background())
            defer cancel()

            r := newTestRecorder(t)
            r.sleep = 1 * time.Hour
            r.wg.Add(1)
            r.runSession = mockRunRecording
            go r.start(ctx)

            now := time.Now()
            r.eventChan <- storage.Event{Time: now, RecDuration: 1 * time.Hour}

            <-onRunRecording
            exitProcess <- nil
            <-onRunRecording
            exitProcess <- nil
            <-onRunRecording
            close(onRunRecording)
            exitProcess <- ffmock.ErrMock
        })
        t.Run("canceled", func(t *testing.T) {
            ctx, cancel := context.WithCancel(context.Background())
            cancel()

            r := newTestRecorder(t)
            r.wg.Add(1)
            r.start(ctx)
        })
        t.Run("canceled2", func(t *testing.T) {
            ctx, cancel := context.WithCancel(context.Background())
            defer cancel()

            mockRunRecording := func(context.Context, *Recorder) error {
                cancel()
                return nil
            }

            r := newTestRecorder(t)
            r.wg.Add(1)
            r.runSession = mockRunRecording
            go r.start(ctx)

            now := time.Now()
            r.eventChan <- storage.Event{Time: now, RecDuration: 1 * time.Hour}
        })
        t.Run("canceledRecording", func(t *testing.T) {
            onCancel := make(chan struct{})
            mockRunRecording := func(ctx context.Context, _ *Recorder) error {
                <-ctx.Done()
                close(onCancel)
                return nil
            }

            ctx, cancel := context.WithCancel(context.Background())
            defer cancel()

            r := newTestRecorder(t)
            r.wg.Add(1)
            r.runSession = mockRunRecording
            go r.start(ctx)

            now := time.Now()
            r.eventChan <- storage.Event{Time: now, RecDuration: 0}
            <-onCancel
        })
        t.Run("crashAndRestart", func(t *testing.T) {
            onRunRecording := make(chan struct{})
            mockRunRecording := func(ctx context.Context, _ *Recorder) error {
                onRunRecording <- struct{}{}
                return errors.New("mock")
            }

            ctx, cancel := context.WithCancel(context.Background())
            defer cancel()

            r := newTestRecorder(t)
            r.wg.Add(1)
            r.runSession = mockRunRecording
            go r.start(ctx)

            now := time.Now()
            r.eventChan <- storage.Event{Time: now, RecDuration: 1 * time.Hour}
            <-onRunRecording
            <-onRunRecording
            <-onRunRecording
        })
    }

    func createTempDir(t *testing.T, r *Recorder) {
    }

    func TestRunRecording(t *testing.T) {
        t.Run("saveRecordingAsync", func(t *testing.T) {
            ctx, cancel := context.WithCancel(context.Background())
            defer cancel()

            r := newTestRecorder(t)
            r.NewProcess = ffmock.NewProcessNil
            r.hooks.RecSave = func(*Recorder, *string) {
                <-ctx.Done()
            }
            err := runRecording(ctx, r)
            require.NoError(t, err)
        })
    }*/

    fn new_test_recdb(recordings_dir: &Path) -> RecDb {
        let disk = DiskImpl::new(recordings_dir.to_path_buf(), ByteSize(0));
        RecDb::new(DummyLogger::new(), recordings_dir.to_path_buf(), disk)
    }

    #[tokio::test]
    async fn test_save_recording() {
        let start = UnixNano::new(MINUTE);
        let end = UnixNano::new(11 * MINUTE);
        let tempdir = tempdir().unwrap();

        let rec_db = new_test_recdb(&tempdir.path().join("recordings"));
        let recording = rec_db.test_recording().await;

        save_recording(
            DummyLogger::new(),
            &"2000-01-01_01-01-01_x".to_owned().try_into().unwrap(),
            &recording,
            start,
            end,
        )
        .await
        .unwrap();

        let mut data_file = recording.open_file("json").await.unwrap();
        let mut got = String::new();
        data_file.read_to_string(&mut got).await.unwrap();

        let want = "{
  \"start\": 60000000000,
  \"end\": 660000000000,
  \"events\": []
}";
        assert_eq!(want, got);
    }
}
