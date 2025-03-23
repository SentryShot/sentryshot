// SPDX-License-Identifier: GPL-2.0-or-later

#![allow(clippy::unwrap_used)]

use crate::{
    error::{CreateSegmenterError, GenerateInitError, GenerateMoofError},
    init::generate_init,
    part::generate_moof,
    WriteFrameError,
};
use async_trait::async_trait;
use bytes::Bytes;
use common::{
    time::{DurationH264, UnixH264, UnixNano},
    ArcLogger, H264Data, Segment, SegmentImpl, TrackParameters, VideoSample,
};
use futures_lite::{stream, Stream};
use std::{collections::VecDeque, fmt::Formatter, iter, ops::Deref, sync::Arc};
use tokio::sync::{broadcast, oneshot, Mutex, MutexGuard};
use tokio_stream::{wrappers::BroadcastStream, StreamExt};
use tokio_util::{
    io::{ReaderStream, StreamReader},
    sync::{CancellationToken, DropGuard},
};

#[allow(clippy::module_name_repetitions)]
pub struct Muxer {
    token: CancellationToken,
    params: TrackParameters,
    state: Arc<Mutex<MuxerState>>,
}

impl Muxer {
    pub(crate) const MAX_SESSIONS: usize = 9;
    pub(crate) const MAX_GOPS: usize = 3;
    const HTTP_BUFFER_SIZE: usize = 65536;
    const FRAME_CACHE_SIZE: usize = 256;

    pub(crate) fn new(
        parent_token: &CancellationToken,
        _logger: &ArcLogger,
        params: TrackParameters,
        id: u16,
        start_time: UnixNano,
        first_frame: H264Data,
    ) -> Result<(Self, H264Writer), CreateSegmenterError> {
        let token = parent_token.child_token();

        let state = MuxerState::new(token.clone(), id, &params, start_time, first_frame)?;
        let muxer = Self {
            token: token.clone(),
            params,
            state: state.clone(),
        };
        let writer = H264Writer::new(state, token.drop_guard());

        Ok((muxer, writer))
    }

    async fn get_state_lock(&self) -> Option<MutexGuard<MuxerState>> {
        let state = self.state.lock().await;
        if state.cancelled {
            return None;
        }
        Some(state)
    }

    // Starts a session and returns the date time of the first frame.
    pub(crate) async fn start_session(&self, session_id: u32) -> StartSessionReponse {
        use StartSessionReponse::*;
        let Some(mut state) = self.get_state_lock().await else {
            return MuxerCancelled;
        };
        let state = state.split_borrow();

        for (id, _) in &mut *state.sessions {
            if *id == session_id {
                return SessionAlreadyExist;
            }
        }

        let Some(last_gop) = state.gops.back() else {
            return NotReady;
        };
        let start_frame = &last_gop.frames[0];
        let start_time = start_frame.pts;
        let start_frame_id = start_frame.id;

        if state.sessions.len() >= Self::MAX_SESSIONS {
            state.sessions.pop_front();
        }

        state.sessions.push_back((
            session_id,
            Session::new(start_time, start_frame_id, state.init_content.len()),
        ));
        Ready(start_time.into())
    }

    #[allow(clippy::too_many_lines)]
    pub(crate) async fn play(&self, req: PlayRequest) {
        use PlayReponse::*;
        let Some(mut state2) = self.get_state_lock().await else {
            _ = req.res_tx.send(MuxerCancelled);
            return;
        };
        let state = state2.split_borrow();

        let session = state
            .sessions
            .iter_mut()
            .find(|(id, _)| *id == req.session_id);
        let Some((_, ref mut session)) = session else {
            _ = req.res_tx.send(SessionNotExist);
            return;
        };

        match req.range {
            Range::Start(start) => {
                let start2 = usize::try_from(start).expect("fit usize");
                if start2 <= state.init_content.len() {
                    let start_frame_index = start_frame_index(state.frames, session.start_frame_id);
                    let Some(start_frame_index) = start_frame_index else {
                        _ = req.res_tx.send(FramesExpired);
                        return;
                    };

                    let length: usize = state.init_content.slice(start2..).len()
                        + state
                            .frames
                            .iter()
                            .skip(start_frame_index)
                            .map(|f| f.muxed_size)
                            .sum::<usize>();
                    assert_ne!(0, length);

                    let init_content = state.init_content.clone().slice(start2..);

                    let cached_frames = state
                        .frames
                        .iter()
                        .skip(start_frame_index)
                        .map(|f| f.muxed(session.start_time.into()).unwrap());

                    let cached_data: Vec<std::io::Result<Bytes>> = iter::once(init_content)
                        .chain(cached_frames)
                        .map(Ok)
                        .collect();

                    let buffered_cached_data_stream = ReaderStream::with_capacity(
                        StreamReader::new(stream::iter(cached_data)),
                        Self::HTTP_BUFFER_SIZE,
                    );

                    let start_time = session.start_time;
                    let new_frames =
                        BroadcastStream::new(state.frames_tx.subscribe()).map(move |v| match v {
                            Ok(v) => Ok(v.muxed(start_time.into()).unwrap()),
                            Err(e) => Err(std::io::Error::new(std::io::ErrorKind::BrokenPipe, e)),
                        });

                    session.cursor.advance_to_end(state.frames).unwrap();

                    let body = Box::new(buffered_cached_data_stream.chain(new_frames));
                    let response = Ready(PlayResponseReady {
                        start,
                        length: u64::try_from(length).unwrap(),
                        body,
                    });
                    _ = req.res_tx.send(response);
                    return;
                }

                let start_frame_index = session.cursor.seek(start, state.frames);
                let (start_frame_index, frame_pos) = match start_frame_index {
                    SeekResult::BeforeFrames => {
                        _ = req.res_tx.send(FramesExpired);
                        return;
                    }
                    SeekResult::Ok(v) => v,
                    SeekResult::AfterFrames => {
                        println!("HOLD");
                        let (tx, rx) = oneshot::channel();
                        state.frames_on_hold.push(tx);
                        let start_time = session.start_time;
                        // State lock must be released at this point.
                        drop(state2);
                        let Ok(frame) = rx.await else {
                            _ = req.res_tx.send(MuxerCancelled);
                            return;
                        };
                        let muxed = frame.muxed(start_time.into()).unwrap();
                        let response = Ready(PlayResponseReady {
                            start,
                            length: u64::try_from(muxed.len()).unwrap(),
                            body: Box::new(stream::once(Ok(muxed))),
                        });
                        _ = req.res_tx.send(response);
                        return;
                    }
                };

                let length: usize = state
                    .frames
                    .iter()
                    .skip(start_frame_index)
                    .map(|f| f.muxed_size)
                    .sum::<usize>()
                    - usize::try_from(frame_pos).expect("fit usize");

                assert_ne!(0, length);

                let frame_pos = usize::try_from(frame_pos).expect("fit usize");
                let first_frame = iter::once(&state.frames[start_frame_index]).map(|f| {
                    f.muxed(session.start_time.into())
                        .unwrap()
                        .slice(frame_pos..)
                });
                let cached_frames = state
                    .frames
                    .iter()
                    .skip(start_frame_index + 1)
                    .map(|f| f.muxed(session.start_time.into()).unwrap());

                let cached_data: Vec<std::io::Result<Bytes>> =
                    first_frame.chain(cached_frames).map(Ok).collect();

                let buffered_cached_data_stream = ReaderStream::with_capacity(
                    StreamReader::new(stream::iter(cached_data)),
                    Self::HTTP_BUFFER_SIZE,
                );

                let start_time = session.start_time;
                let new_frames =
                    BroadcastStream::new(state.frames_tx.subscribe()).map(move |v| match v {
                        Ok(v) => Ok(v.muxed(start_time.into()).unwrap()),
                        Err(e) => Err(std::io::Error::new(std::io::ErrorKind::BrokenPipe, e)),
                    });

                session.cursor.advance_to_end(state.frames).unwrap();

                let body = Box::new(buffered_cached_data_stream.chain(new_frames));
                let response = Ready(PlayResponseReady {
                    start,
                    length: u64::try_from(length).unwrap(),
                    body,
                });
                _ = req.res_tx.send(response);
            }
        }
    }

    #[cfg(test)]
    pub async fn debug_state(&self) -> DebugState {
        let state = self.get_state_lock().await.unwrap();
        DebugState {
            num_sessions: state.sessions.len(),
            gop_count: state.gops.len(),
            next_seg_on_hold_count: state.next_segments_on_hold.len(),
            frame_on_hold_count: state.frames_on_hold.len(),
        }
    }

    pub(crate) async fn cancel(&self) {
        self.token.cancel();
        let mut state = self.state.lock().await;

        state.next_segments_on_hold.clear();
        state.frames_on_hold.clear();
        state.gops.clear();
        state.gop_in_progress.clear();
        state.frames.clear();
    }
}

#[async_trait]
impl common::StreamerMuxer for Muxer {
    fn params(&self) -> &TrackParameters {
        &self.params
    }

    // Returns the first segment with a ID greater than prevID.
    // Will wait for new segments if the next segment isn't cached.
    async fn next_segment(&self, prev_seg: Option<Segment>) -> Option<Segment> {
        let res_rx: oneshot::Receiver<Segment>;
        {
            let mut state = self.get_state_lock().await?;

            let prev_id = match prev_seg {
                Some(seg) if seg.muxer_id() == state.id && seg.id() < state.gop_count => seg.id(),
                Some(_) | None => 0,
            };

            let seg = || state.gops.iter().find(|&gop| prev_id < gop.id);

            if let Some(seg) = seg() {
                return Some(seg.clone());
            }

            let res_tx: oneshot::Sender<Segment>;
            (res_tx, res_rx) = oneshot::channel();
            state
                .next_segments_on_hold
                .push(NextSegmentRequest { prev_id, res_tx });
        }

        // Lock must be released at this point.
        res_rx.await.ok()
    }
}

struct NextSegmentRequest {
    prev_id: u64,
    res_tx: oneshot::Sender<Segment>,
}

struct MuxerState {
    id: u16,
    frame_count: u64,
    next_frame: H264Data,

    muxer_start_time: UnixNano,
    sessions: VecDeque<(u32, Session)>,

    frames_tx: broadcast::Sender<Arc<Frame>>,
    frames: VecDeque<Arc<Frame>>,
    gop_in_progress: Vec<Arc<Frame>>,
    // Independant groups of pictures.
    gops: VecDeque<Arc<Gop>>,
    gop_count: u64,

    next_segments_on_hold: Vec<NextSegmentRequest>,
    frames_on_hold: Vec<oneshot::Sender<Arc<Frame>>>,

    // The is the first bytes in the mp4 file that contain the decoding parameters.
    init_content: Bytes,
    cancelled: bool,
}

impl MuxerState {
    fn new(
        token: CancellationToken,
        id: u16,
        params: &TrackParameters,
        muxer_start_time: UnixNano,
        mut first_frame: H264Data,
    ) -> Result<Arc<Mutex<Self>>, GenerateInitError> {
        assert!(
            first_frame.random_access_present,
            "first frame must be an idr"
        );

        first_frame.pts = muxer_start_time.into();
        let (frames_tx, _) = broadcast::channel(Muxer::FRAME_CACHE_SIZE);

        let state = Arc::new(Mutex::new(Self {
            id,
            frame_count: 0,
            next_frame: first_frame,
            muxer_start_time,
            sessions: VecDeque::new(),
            frames_tx,
            frames: VecDeque::new(),
            gop_in_progress: Vec::new(),
            gops: VecDeque::new(),
            gop_count: 1,
            next_segments_on_hold: Vec::new(),
            frames_on_hold: Vec::new(),
            init_content: generate_init(params)?,
            cancelled: false,
        }));

        let state2 = state.clone();
        tokio::spawn(async move {
            token.cancelled().await;
            let mut state = state2.lock().await;
            state.cancelled = true;
        });

        Ok(state)
    }

    pub fn write_frame(&mut self, frame: H264Data) -> Result<(), WriteFrameError> {
        use WriteFrameError::*;

        let next_frame_dts = frame.dts().ok_or(Dts)?;
        let next_frame_is_idr = frame.random_access_present;

        // Queue one frame in order to compute frame duration.
        let frame = std::mem::replace(&mut self.next_frame, frame);

        // frame_duration = next_frame.dts() - frame.dts()
        let frame_duration = next_frame_dts
            .checked_sub(frame.dts().ok_or(Dts)?)
            .ok_or(ComputeFrameDuration)?
            .into();

        self.frame_count += 1;
        let frame = Arc::new(Frame::new(
            self.frame_count,
            frame,
            frame_duration,
            self.muxer_start_time,
        )?);

        if self.frames.len() >= Muxer::FRAME_CACHE_SIZE {
            self.frames.pop_front();
        }
        self.frames.push_back(frame.clone());
        self.gop_in_progress.push(frame.clone());

        for req in std::mem::take(&mut self.frames_on_hold) {
            _ = req.send(frame.clone());
        }
        _ = self.frames_tx.send(frame.clone());

        // Switch GOPs.
        if next_frame_is_idr {
            if self.gops.len() >= Muxer::MAX_GOPS {
                self.gops.pop_front();
            }

            let frames = std::mem::take(&mut self.gop_in_progress);
            let gop = Arc::new(Gop::new(self.gop_count, self.id, frames));
            self.gops.push_back(gop.clone());
            self.gop_count += 1;

            // RUSTC: extract_if.
            let mut i = 0;
            while i < self.next_segments_on_hold.len() {
                if gop.id() > self.next_segments_on_hold[i].prev_id {
                    let req = self.next_segments_on_hold.swap_remove(i);
                    _ = req.res_tx.send(gop.clone());
                } else {
                    i += 1;
                }
            }
        }

        Ok(())
    }

    fn split_borrow(&mut self) -> MuxerStateRef {
        MuxerStateRef {
            sessions: &mut self.sessions,
            frames_tx: &self.frames_tx,
            frames: &mut self.frames,
            gops: &self.gops,
            frames_on_hold: &mut self.frames_on_hold,
            init_content: &self.init_content,
        }
    }
}

struct MuxerStateRef<'a> {
    sessions: &'a mut VecDeque<(u32, Session)>,
    frames_tx: &'a broadcast::Sender<Arc<Frame>>,
    frames: &'a mut VecDeque<Arc<Frame>>,
    gops: &'a VecDeque<Arc<Gop>>,
    frames_on_hold: &'a mut Vec<oneshot::Sender<Arc<Frame>>>,
    init_content: &'a Bytes,
}

fn start_frame_index(frames: &VecDeque<Arc<Frame>>, frame_id: u64) -> Option<usize> {
    (0..frames.len()).find(|&i| frames[i].id == frame_id)
}

#[derive(Debug, PartialEq, Eq)]
pub enum StartSessionReponse {
    // Date time of first frame.
    Ready(UnixNano),
    NotReady,
    SessionAlreadyExist,
    MuxerNotExist,
    StreamerCancelled,
    MuxerCancelled,
}

impl StartSessionReponse {
    #[cfg(test)]
    #[track_caller]
    #[must_use]
    pub fn unwrap(self) -> i64 {
        let StartSessionReponse::Ready(time) = self else {
            panic!("not ok: {self:?}")
        };
        *time
    }
}

pub struct PlayRequest {
    pub session_id: u32,
    pub range: Range,

    pub res_tx: oneshot::Sender<PlayReponse>,
}

#[derive(Debug, PartialEq)]
pub enum PlayReponse {
    Ready(PlayResponseReady),
    NotReady,
    NotImplemented(String),
    FramesExpired,
    SessionNotExist,
    MuxerNotExist,
    StreamerCancelled,
    MuxerCancelled,
}

impl PlayReponse {
    #[cfg(test)]
    #[track_caller]
    #[must_use]
    pub fn unwrap(self) -> PlayResponseReady {
        let PlayReponse::Ready(res) = self else {
            panic!("not ok: {self:?}")
        };
        res
    }
}

pub struct PlayResponseReady {
    pub start: u64,
    pub length: u64,
    pub body: Box<dyn Stream<Item = std::io::Result<Bytes>> + Send + Unpin>,
}

impl PlayResponseReady {
    pub fn end(&self) -> u64 {
        self.start + self.length - 1
    }

    #[cfg(test)]
    pub async fn collect_body(self) -> Vec<u8> {
        use futures_lite::StreamExt;
        let bytes: Vec<Bytes> = self.body.try_collect().await.unwrap();
        bytes.into_iter().flatten().collect()
    }
}

impl std::fmt::Debug for PlayResponseReady {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        write!(f, "start: {} length: {}", self.start, self.length)
    }
}

impl PartialEq for PlayResponseReady {
    fn eq(&self, _other: &Self) -> bool {
        true
    }
}

#[derive(Debug)]
pub enum Range {
    Start(u64),
}

struct Session {
    start_time: UnixH264,
    start_frame_id: u64,
    cursor: Cursor,
}

impl Session {
    fn new(start_time: UnixH264, start_frame_id: u64, init_content_len: usize) -> Self {
        Self {
            start_time,
            start_frame_id,
            cursor: Cursor {
                file_pos: u64::try_from(init_content_len).expect("fit u64"),
                frame_id: start_frame_id,
            },
        }
    }
}

#[derive(Debug)]
struct Cursor {
    file_pos: u64,
    frame_id: u64,
}

#[derive(Debug)]
enum SeekResult {
    BeforeFrames,
    // Frame index and position.
    Ok((usize, u64)),
    AfterFrames,
}

impl Cursor {
    fn seek(&mut self, pos: u64, frames: &VecDeque<Arc<Frame>>) -> SeekResult {
        use std::cmp::Ordering::*;
        let Some(mut index) = frames.iter().position(|f| f.id == self.frame_id) else {
            return SeekResult::BeforeFrames;
        };

        loop {
            let frame = &frames[index];
            match pos.cmp(&self.file_pos) {
                Less => {
                    index -= 1;
                    self.frame_id -= 1;
                    self.file_pos -= frames[index].muxed_size();
                }
                Greater => {
                    if self.file_pos + frame.muxed_size() <= pos {
                        index += 1;
                        if index == frames.len() {
                            return SeekResult::AfterFrames;
                        }
                        self.frame_id += 1;
                        self.file_pos += frame.muxed_size();
                    } else {
                        let frame_pos = pos - self.file_pos;
                        return SeekResult::Ok((index, frame_pos));
                    }
                }
                Equal => {
                    return SeekResult::Ok((index, 0));
                }
            };
        }
    }

    fn advance_to_end(&mut self, frames: &VecDeque<Arc<Frame>>) -> Option<()> {
        let Some(mut index) = frames.iter().position(|f| f.id == self.frame_id) else {
            return None;
        };
        while index + 1 < frames.len() {
            let frame = frames.get(index)?;
            index += 1;
            self.frame_id += 1;
            self.file_pos += frame.muxed_size();
        }
        Some(())
    }
}

struct Frame {
    id: u64,
    sample: VideoSample,

    muxed_size: usize,
}

impl Frame {
    fn new(
        id: u64,
        data: H264Data,
        mut duration: DurationH264,
        muxer_start_time: UnixNano,
    ) -> Result<Self, GenerateMoofError> {
        if *duration < 0 {
            duration = DurationH264::new(0);
        }
        let sample = VideoSample {
            pts: data.pts,
            dts_offset: data.dts_offset,
            duration,
            random_access_present: data.random_access_present,
            avcc: data.avcc.clone(),
        };
        let muxed_size = generate_moof(muxer_start_time.into(), Arc::new(vec![sample]))?.len();

        Ok(Frame {
            id,
            sample: VideoSample {
                pts: data.pts,
                dts_offset: data.dts_offset,
                duration,
                random_access_present: data.random_access_present,
                avcc: data.avcc,
            },
            muxed_size,
        })
    }

    fn muxed_size(&self) -> u64 {
        u64::try_from(self.muxed_size).expect("fit u64")
    }

    fn muxed(&self, start_time: UnixNano) -> Result<Bytes, GenerateMoofError> {
        let muxed = generate_moof(start_time.into(), Arc::new(vec![self.sample.clone()]))?;
        assert_eq!(self.muxed_size, muxed.len());
        Ok(muxed)
    }
}

impl Deref for Frame {
    type Target = VideoSample;

    fn deref(&self) -> &Self::Target {
        &self.sample
    }
}

// Group of pictures.
struct Gop {
    id: u64,
    muxer_id: u16,
    duration: DurationH264,
    frames: Vec<Arc<Frame>>,
}

impl Gop {
    fn new(id: u64, muxer_id: u16, frames: Vec<Arc<Frame>>) -> Self {
        assert_eq!(*frames[0].dts_offset, 0);
        Self {
            id,
            muxer_id,
            duration: frames.iter().map(|f| f.duration).sum(),
            frames,
        }
    }
}

impl SegmentImpl for Gop {
    fn id(&self) -> u64 {
        self.id
    }

    fn muxer_id(&self) -> u16 {
        self.muxer_id
    }

    fn frames(&self) -> Box<dyn Iterator<Item = &VideoSample> + Send + '_> {
        Box::new(self.frames.iter().map(|f| &f.sample))
    }

    fn duration(&self) -> DurationH264 {
        self.duration
    }

    fn start_time(&self) -> UnixH264 {
        self.frames[0].sample.pts
    }
}

// Opaque wrapper around segmenter that will cancel the muxer when dropped.
pub struct H264Writer {
    state: Arc<Mutex<MuxerState>>,
    _guard: DropGuard,
}

impl H264Writer {
    #[must_use]
    fn new(state: Arc<Mutex<MuxerState>>, guard: DropGuard) -> Self {
        Self {
            state,
            _guard: guard,
        }
    }
    pub async fn write_h264(&self, data: H264Data) -> Result<(), WriteFrameError> {
        let mut state = self.state.lock().await;
        if state.cancelled {
            return Ok(());
        }
        state.write_frame(data)
    }

    #[cfg(test)]
    #[allow(clippy::unwrap_used)]
    pub async fn test_write(&mut self, pts: i64, avcc: Vec<u8>, random_access: bool) {
        use common::time::DtsOffset;
        use sentryshot_padded_bytes::PaddedBytes;

        self.write_h264(H264Data {
            pts: common::time::UnixH264::new(pts),
            dts_offset: DtsOffset::new(0),
            avcc: Arc::new(PaddedBytes::new(avcc)),
            random_access_present: random_access,
        })
        .await
        .unwrap();
    }
}

#[cfg(test)]
pub struct DebugState {
    pub num_sessions: usize,
    pub gop_count: usize,
    pub next_seg_on_hold_count: usize,
    pub frame_on_hold_count: usize,
}
