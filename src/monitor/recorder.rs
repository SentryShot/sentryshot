// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{source::Source, DynMonitorHooks};
use common::{
    monitor::MonitorConfig,
    time::{Duration, DurationH264, UnixH264, UnixNano, MINUTE},
    Cancelled, DynHlsMuxer, DynLogger, DynMsgLogger, Event, LogEntry, LogLevel, MonitorId,
    MsgLogger, SegmentFinalized, TrackParameters,
};
use recording::{Header, NewVideoWriterError, RecordingData, VideoWriter, WriteSampleError};
use sentryshot_convert::{
    ConvertError, Frame, NewConverterError, PixelFormat, PixelFormatConverter,
};
use sentryshot_ffmpeg_h264::{
    DrainError, H264BuilderError, H264DecoderBuilder, Packet, PaddedBytes, ReceiveFrameError,
    SendPacketError,
};
use sentryshot_util::ImageCopyToBufferError;
use std::{
    path::{Path, PathBuf},
    sync::Arc,
};
use thiserror::Error;
use tokio::{
    io::AsyncWriteExt,
    sync::{mpsc, Mutex},
    time::sleep,
};
use tokio_util::sync::CancellationToken;

struct RecordingSession {
    token: CancellationToken,
    timer_end: UnixNano,
    on_exit_rx: mpsc::Receiver<()>,
}

#[allow(
    clippy::too_many_arguments,
    clippy::too_many_lines,
    clippy::module_name_repetitions
)]
pub fn new_recorder(
    token: CancellationToken,
    shutdown_complete: mpsc::Sender<()>,
    hooks: DynMonitorHooks,
    logger: DynLogger,
    monitor_id: MonitorId,
    source_main: Arc<Source>,
    config: MonitorConfig,
    recordings_dir: PathBuf,
) -> mpsc::Sender<Event> {
    let (send_event_tx, mut send_event_rx) = mpsc::channel::<Event>(1);

    let logger = Arc::new(RecorderMsgLogger::new(logger, monitor_id));

    let logger2 = logger.clone();
    let log = move |level: LogLevel, msg: &str| logger2.log(level, msg);

    // Recorder actor.
    tokio::spawn(async move {
        let shutdown_complete = shutdown_complete;

        let prev_seg = Arc::new(Mutex::new(0));
        let event_cache = Arc::new(EventCache::new());

        #[allow(clippy::items_after_statements)]
        fn get_timer_end(timer_end: UnixNano) -> std::time::Duration {
            let Some(timer_end) = Duration::until(timer_end) else {
                return std::time::Duration::MAX;
            };
            let Some(timer_end) = timer_end.as_std() else {
                return std::time::Duration::from_nanos(0);
            };
            timer_end
        }

        let mut recording_session: Option<RecordingSession> = None;
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
                        let Some(event) = event else {
                            continue
                        };
                        //r.hooks.Event(r, &event)

                        let Some(end) = event.time.add_duration(event.rec_duration) else {
                            continue
                        };

                        if end.after(session.timer_end) {
                            log(LogLevel::Debug, "new event, already recording, updating timer");
                            session.timer_end = end;
                        }
                        event_cache.push(event).await;
                    }

                    () = sleep(get_timer_end(session.timer_end)) => {
                        log(LogLevel::Debug, "timer reached end, canceling session");
                        session.token.cancel();

                        // Avoid sleeping again.
                        session.on_exit_rx.recv().await;
                        log(LogLevel::Debug, "session stopped");
                        recording_session = None;
                    }

                    _ = session.on_exit_rx.recv() => {
                        log(LogLevel::Debug, "session stopped2");
                        recording_session = None;
                    }
                }
            } else {
                tokio::select! {
                    () = token.cancelled() => {
                        return
                    }

                    event = send_event_rx.recv() => { // Incomming events.
                        let Some(event) = event else {
                            return
                        };
                        //r.hooks.Event(r, &event)

                        let Some(end) = event.time.add_duration(event.rec_duration) else {
                            continue
                        };
                        if !end.after(UnixNano::now()) {
                            continue
                        }

                        event_cache.push(event).await;

                        log(LogLevel::Debug, "starting recording session");
                        let session_token = token.child_token();

                        let (on_session_exit_tx, on_session_exit_rx) = mpsc::channel::<()>(1);
                        recording_session =  Some(RecordingSession{
                            token: session_token.clone(),
                            timer_end: end,
                            on_exit_rx: on_session_exit_rx,
                        });

                        let recording_context = RecordingContext {
                            token: session_token.clone(),
                            hooks: hooks.clone(),
                            logger: logger.clone(),
                            source_main: source_main.clone(),
                            prev_seg: prev_seg.clone(),
                            config: config.clone(),
                            recordings_dir: recordings_dir.clone(),
                            event_cache: event_cache.clone(),
                        };

                        let recording_context = recording_context.clone();
                        let shutdown_complete = shutdown_complete.clone();

                        tokio::spawn(async move {
                            run_recording_session(
                                recording_context,
                                shutdown_complete,
                                Duration::from_secs(3),
                            ).await;
                            _ = on_session_exit_tx.send(()).await;
                        });
                    }
                }
            }
        }
    });

    send_event_tx
}

struct RecorderMsgLogger {
    logger: DynLogger,
    monitor_id: MonitorId,
}

impl RecorderMsgLogger {
    fn new(logger: DynLogger, monitor_id: MonitorId) -> Self {
        Self { logger, monitor_id }
    }
}

impl MsgLogger for RecorderMsgLogger {
    fn log(&self, level: LogLevel, msg: &str) {
        self.logger.log(LogEntry {
            level,
            source: "monitor".parse().unwrap(),
            monitor_id: Some(self.monitor_id.clone()),
            message: format!("recorder: {msg}").parse().unwrap(),
        });
    }
}

async fn run_recording_session(
    c: RecordingContext,
    _shutdown_complete: mpsc::Sender<()>,
    restart_sleep: Duration,
) {
    loop {
        if let Err(e) = run_recording(c.clone()).await {
            if !matches!(e, RunRecordingError::Cancelled(_)) {
                c.logger
                    .log(LogLevel::Error, &format!("recording crashed: {e}"));
            }

            tokio::select! {
                () = c.token.cancelled() => {
                    return
                }
                () = sleep(restart_sleep.as_std().unwrap()) => {
                    c.logger.log(LogLevel::Debug, "recovering after crash");
                }
            }
        } else {
            c.logger.log(LogLevel::Info, "recording finished");
            if c.token.is_cancelled() {
                return;
            }
            // Recoding reached videoLength and exited normally. The timer
            // is still active, so continue the loop and start another one.
            continue;
        }
    }
}

#[derive(Debug, Error)]
enum RunRecordingError {
    #[error("{0}")]
    Cancelled(#[from] Cancelled),

    #[error("create directory for recording: {0}")]
    CreateDir(std::io::Error),

    #[error("generate video: {0}")]
    GenerateVideo(#[from] GenerateVideoError),

    #[error("save recording: {0}")]
    SaveRecording(#[from] SaveRecordingError),
}

#[derive(Clone)]
struct RecordingContext {
    token: CancellationToken,
    hooks: DynMonitorHooks,
    logger: DynMsgLogger,
    source_main: Arc<Source>,
    prev_seg: Arc<Mutex<u64>>,
    config: MonitorConfig,
    recordings_dir: PathBuf,
    event_cache: Arc<EventCache>,
}

async fn run_recording(c: RecordingContext) -> Result<(), RunRecordingError> {
    use RunRecordingError::*;
    /*timestampOffsetInt, err := strconv.Atoi(r.Config.TimestampOffset())
    if err != nil {
        return fmt.Errorf("parse timestamp offset %w", err)
    }*/

    let muxer = c.source_main.muxer().await?;

    let first_segment = muxer
        .next_segment(*c.prev_seg.lock().await)
        .await
        .ok_or(common::Cancelled)?;

    //offset := 0 + time.Duration(timestampOffsetInt)*time.Millisecond
    //startTime := firstSegment.StartTime.Add(-offset)
    let start_time = first_segment.start_time();

    let monitor_id = c.config.id().to_owned();
    let fmt1 = start_time
        .as_nanos()
        .as_chrono()
        .unwrap()
        .format("%Y/%m/%d")
        .to_string();

    let file_dir = c.recordings_dir.join(fmt1).join(&*monitor_id);
    /*fileDir := filepath.Join(
        r.Env.RecordingsDir(),
        startTime.Format("2006/01/02/")+monitorID,
    )*/

    let fmt2 = start_time
        .as_nanos()
        .as_chrono()
        .unwrap()
        .format("%Y-%m-%d_%H-%M-%S_")
        .to_string();
    let file_path = file_dir.join(fmt2 + &*monitor_id);
    /*filePath := filepath.Join(
        fileDir,
        startTime.Format("2006-01-02_15-04-05_")+monitorID,
    )*/

    let rec_id = file_path.file_name().unwrap();
    //basePath := filepath.Base(filePath)

    tokio::fs::create_dir_all(file_dir)
        .await
        .map_err(CreateDir)?;

    #[allow(clippy::cast_precision_loss)]
    let video_length = Duration::from(c.config.video_length() * (MINUTE as f64));

    c.logger
        .log(LogLevel::Info, &format!("starting recording: {rec_id:?}"));

    let params = muxer.params();

    let result = generate_thumbnail(
        c.hooks.clone(),
        &c.logger,
        c.config,
        file_path.clone(),
        &first_segment,
        muxer.params().extra_data.clone(),
    )
    .await;
    if let Err(e) = result {
        c.logger.log(
            LogLevel::Error,
            &format!("failed to generate thumbnail: {}", &e),
        );
    }

    let (new_prev_seg, end_time) = generate_video(
        c.token,
        &file_path,
        &muxer,
        &first_segment,
        params,
        video_length.as_h264(),
    )
    .await?;
    *c.prev_seg.lock().await = new_prev_seg;

    c.logger
        .log(LogLevel::Info, &format!("video generated: {rec_id:?}"));

    save_recording(
        c.logger.clone(),
        file_path,
        c.event_cache,
        start_time.as_nanos(),
        end_time.as_nanos(),
    )
    .await?;

    Ok(())
}

//type nextSegmentFunc func(uint64) (*hls.Segment, error)

#[derive(Debug, Error)]
enum GenerateVideoError {
    #[error("open file: {0} {1}")]
    OpenFile(String, std::io::Error),

    #[error("new video writer: {0}")]
    NewVideoWriter(#[from] NewVideoWriterError),

    #[error("add")]
    Add,

    #[error("write sample: {0}")]
    WriteSample(#[from] WriteSampleError),

    #[error("{0}")]
    Cancelled(#[from] Cancelled),

    #[error("skipped segment: expected: {0}, got: {0}")]
    SkippedSegment(u64, u64),
}

async fn generate_video(
    token: CancellationToken,
    file_path: &Path,
    muxer: &DynHlsMuxer,
    first_segment: &SegmentFinalized,
    params: &TrackParameters,
    max_duration: DurationH264,
) -> Result<(u64, UnixH264), GenerateVideoError> {
    use GenerateVideoError::*;

    let start_time = first_segment.start_time();

    let stop_time = first_segment
        .start_time()
        .checked_add_duration(max_duration)
        .unwrap();
    //stopTime := firstSegment.StartTime.Add(maxDuration)

    let mut meta_path = file_path.to_owned();
    meta_path.set_extension("meta");

    let mut mdat_path = file_path.to_owned();
    mdat_path.set_extension("mdat");

    let mut meta = tokio::fs::OpenOptions::new()
        .create_new(true)
        .write(true)
        .open(&meta_path)
        .await
        .map_err(|e| OpenFile(format!("{meta_path:?}"), e))?;

    let mut mdat = tokio::fs::OpenOptions::new()
        .create_new(true)
        .write(true)
        .open(&mdat_path)
        .await
        .map_err(|e| OpenFile(mdat_path.to_string_lossy().to_string(), e))?;

    let header = Header {
        start_time,
        width: params.width,
        height: params.height,
        extra_data: params.extra_data.clone(),
    };

    let mut w = VideoWriter::new(&mut meta, &mut mdat, header).await?;

    w.write_parts(first_segment.parts()).await?;
    let mut prev_seg_id = first_segment.id();
    let mut end_time = first_segment
        .start_time()
        .checked_add_duration(first_segment.duration())
        .ok_or(Add)?;

    /*if err := writeSegment(firstSegment); err != nil {
        return 0, nil, err
    }*/

    loop {
        if token.is_cancelled() {
            return Ok((prev_seg_id, end_time));
        }

        let Some(seg) = muxer.next_segment(prev_seg_id).await else {
            return Ok((prev_seg_id, end_time));
        };

        if seg.id() != prev_seg_id + 1 {
            return Err(SkippedSegment(seg.id(), prev_seg_id + 1));
        }

        w.write_parts(seg.parts()).await?;
        prev_seg_id = seg.id();
        end_time = seg
            .start_time()
            .checked_add_duration(seg.duration())
            .ok_or(Add)?;

        if seg.start_time().after(stop_time) {
            return Ok((prev_seg_id, end_time));
        }
    }
}

#[derive(Debug, Error)]
enum GenerateThumbnailError {
    #[error("no part")]
    NoPart,

    #[error("no sample")]
    NoSample,

    #[error("sample is not an IDR")]
    SampleNotIdr,

    #[error("avcc to jpeg: {0}")]
    AvccToJpeg(#[from] AvccToJpegError),

    #[error("open file: {0}")]
    OpenFile(std::io::Error),

    #[error("write file: {0}")]
    WriteFile(std::io::Error),

    #[error("flush jpeg file: {0}")]
    FlushFile(std::io::Error),
}

// The first h264 frame in firstSegment is wrapped in a mp4
// container and piped into FFmpeg and then converted to jpeg.
#[allow(unused)]
async fn generate_thumbnail(
    hooks: DynMonitorHooks,
    logger: &DynMsgLogger,
    config: MonitorConfig,
    file_path: PathBuf,
    first_segment: &Arc<SegmentFinalized>,
    extradata: Vec<u8>,
) -> Result<(), GenerateThumbnailError> {
    use GenerateThumbnailError::*;
    let mut thumb_path = file_path;
    thumb_path.set_extension("jpeg");
    //thumbPath := filePath + ".jpeg"

    logger.log(
        LogLevel::Info,
        &format!("generating thumbnail: {thumb_path:?}"),
    );
    //r.logf(log.LevelInfo, "generating thumbnail: %v", thumbPath)

    let Some(first_part) = first_segment.parts().first() else {
        return Err(NoPart);
    };
    let Some(first_sample) = first_part.video_samples.first() else {
        return Err(NoSample);
    };
    if !first_sample.random_access_present {
        return Err(SampleNotIdr);
    }

    let mut avcc = first_sample.avcc.clone();
    let jpeg_buf = tokio::task::spawn_blocking(move || {
        avcc_to_jpeg(&hooks, &config, &avcc, PaddedBytes::new(extradata))
    })
    .await
    .unwrap()?;

    let mut file = tokio::fs::OpenOptions::new()
        .create_new(true)
        .write(true)
        .open(&thumb_path)
        .await
        .map_err(OpenFile)?;
    file.write_all(&jpeg_buf).await.map_err(WriteFile)?;
    file.flush().await.map_err(FlushFile)?;

    logger.log(
        LogLevel::Debug,
        &format!(
            "thumbnail generated: {:?}",
            thumb_path.file_name().unwrap_or_default()
        ),
    );
    //r.logf(log.LevelDebug, "thumbnail generated: %v", filepath.Base(thumbPath))

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
    hooks: &DynMonitorHooks,
    config: &MonitorConfig,
    avcc: &PaddedBytes,
    extradata: PaddedBytes,
) -> Result<Vec<u8>, AvccToJpegError> {
    let mut decoder = H264DecoderBuilder::new().avcc(extradata)?;

    decoder.send_packet(Packet::new(avcc))?;

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

    #[error("open data file: {0}")]
    Open(std::io::Error),

    #[error("write data file: {0}")]
    Write(std::io::Error),

    #[error("flush data file: {0}")]
    Flush(std::io::Error),
}

async fn save_recording(
    logger: DynMsgLogger,
    file_path: PathBuf,
    event_cache: Arc<EventCache>,
    start_time: UnixNano,
    end_time: UnixNano,
) -> Result<(), SaveRecordingError> {
    use SaveRecordingError::*;
    let rec_id = file_path.file_name().unwrap().to_string_lossy().to_string();
    logger.log(LogLevel::Info, &format!("saving recording: {rec_id:?}"));
    //r.logf(log.LevelInfo, "saving recording: %v", filepath.Base(filePath))

    let events = event_cache.query_and_prune(start_time, end_time).await;

    let data = RecordingData {
        start: start_time,
        end: end_time,
        events,
    };

    let json = serde_json::to_vec_pretty(&data)?;
    /*json, err := json.MarshalIndent(data, "", "    ")
    if err != nil {
        r.logf(log.LevelError, "marshal event data: %w", err)
        return
    }*/

    let mut data_path = file_path.clone();
    data_path.set_extension("json");

    let mut data_file = tokio::fs::OpenOptions::new()
        .create_new(true)
        .write(true)
        .open(data_path)
        .await
        .map_err(Open)?;

    data_file.write_all(&json).await.map_err(Write)?;
    data_file.flush().await.map_err(Flush)?;

    //go r.hooks.RecSaved(r, filePath, data)

    logger.log(LogLevel::Info, &format!("recording saved: {rec_id:?}"));

    Ok(())
}

struct EventCache(Mutex<Vec<Event>>);

impl EventCache {
    fn new() -> Self {
        Self(Mutex::new(Vec::new()))
    }

    async fn push(&self, event: Event) {
        self.0.lock().await.push(event);
    }

    async fn query_and_prune(&self, start: UnixNano, end: UnixNano) -> Vec<Event> {
        let mut new_events: Vec<Event> = Vec::new();
        let mut return_events: Vec<Event> = Vec::new();
        let mut events = self.0.lock().await;
        for event in events.drain(..) {
            if event.time.before(start) {
                // Discard events before start time.
                continue;
            }

            if event.time.before(end) {
                return_events.push(event.clone());
            }

            new_events.push(event);
        }
        *events = new_events;

        return_events
    }
}

#[cfg(test)]
mod tests {
    use std::num::NonZeroU32;

    use super::*;
    use common::{new_dummy_msg_logger, Detection, PointNormalized, RectangleNormalized, Region};
    use pretty_assertions::assert_eq;
    use tempfile::tempdir;
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

    #[tokio::test]
    async fn test_save_recording() {
        let event_cache = Arc::new(EventCache(Mutex::new(vec![
            Event {
                time: UnixNano::from(0),
                duration: Duration::from(0),
                rec_duration: Duration::from(0),
                detections: Vec::new(),
            },
            Event {
                time: UnixNano::from(2 * MINUTE),
                duration: Duration::from(11),
                rec_duration: Duration::from(0),
                detections: vec![Detection {
                    label: "10".parse().unwrap(),
                    score: 9.0,
                    region: Region {
                        rectangle: Some(RectangleNormalized {
                            x: 1,
                            y: 2,
                            width: NonZeroU32::new(3).unwrap(),
                            height: NonZeroU32::new(4).unwrap(),
                        }),
                        polygon: Some(vec![
                            PointNormalized { x: 5, y: 6 },
                            PointNormalized { x: 7, y: 8 },
                        ]),
                    },
                }],
            },
            Event {
                time: UnixNano::from(11 * MINUTE),
                duration: Duration::from(0),
                rec_duration: Duration::from(0),
                detections: Vec::new(),
            },
        ])));
        /*r.events = &storage.Events{
            storage.Event{
                Time: time.Time{},
            },
            storage.Event{
                Time: time.Time{}.Add(2 * time.Minute),
                Detections: []storage.Detection{
                    {
                        Label: "10",
                        Score: 9,
                        Region: &storage.Region{
                            Rect: &ffmpeg.Rect{1, 2, 3, 4},
                            Polygon: &ffmpeg.Polygon{
                                ffmpeg.Point{5, 6},
                                ffmpeg.Point{7, 8},
                            },
                        },
                    },
                },
                Duration: 11,
            },
            storage.Event{
                Time: time.Time{}.Add(11 * time.Minute),
            },
        }
        */

        let start = UnixNano::from(MINUTE);
        //start := time.Time{}.Add(1 * time.Minute)
        let end = UnixNano::from(11 * MINUTE);
        //end := time.Time{}.Add(11 * time.Minute)
        let tempdir = tempdir().unwrap();
        //tempdir := r.Env.TempDir
        let file_path = tempdir.path().join("file");
        //filePath := tempdir + "file"

        save_recording(
            new_dummy_msg_logger(),
            file_path.clone(),
            event_cache,
            start,
            end,
        )
        .await
        .unwrap();
        //r.saveRecording(filePath, start, end)

        let mut data_path = file_path.clone();
        data_path.set_extension("json");
        let b = std::fs::read(data_path).unwrap();
        //b, err := os.ReadFile(filePath + ".json")

        let want = "{
  \"start\": 60000000000,
  \"end\": 660000000000,
  \"events\": [
    {
      \"time\": 120000000000,
      \"duration\": 11,
      \"detections\": [
        {
          \"label\": \"10\",
          \"score\": 9.0,
          \"region\": {
            \"rectangle\": {
              \"x\": 1,
              \"y\": 2,
              \"width\": 3,
              \"height\": 4
            },
            \"polygon\": [
              {
                \"x\": 5,
                \"y\": 6
              },
              {
                \"x\": 7,
                \"y\": 8
              }
            ]
          }
        }
      ]
    }
  ]
}";
        let got = String::from_utf8(b).unwrap();
        assert_eq!(want, got);

        /*actual := string(b)
        actual = strings.ReplaceAll(actual, " ", "")
        actual = strings.ReplaceAll(actual, "\n", "")

        expected := `{"start":"0001-01-01T00:01:00Z","end":"0001-01-01T00:11:00Z",` +
            `"events":[{"time":"0001-01-01T00:02:00Z","detections":` +
            `[{"label":"10","score":9,"region":{"rect":[1,2,3,4],` +
            `"polygon":[[5,6],[7,8]]}}],"duration":11}]}`

        require.Equal(t, actual, expected)
        */
    }
}
