// SPDX-License-Identifier: GPL-2.0-or-later

use async_trait::async_trait;
use bytes::Bytes;
use common::{
    monitor::H264WriterImpl,
    time::{DurationH264, UnixH264, UnixNano},
    ArcLogger, DynError, H264Data, Segment, SegmentImpl, TrackParameters, VideoSample,
};
use futures_lite::{stream, Stream};
use sentryshot_padded_bytes::PaddedBytes;
use serde::Serialize;
use std::{collections::VecDeque, fmt::Formatter, iter, ops::Deref, sync::Arc};
use thiserror::Error;
use tokio::sync::{oneshot, Mutex, MutexGuard};
use tokio_util::{
    io::{ReaderStream, StreamReader},
    sync::{CancellationToken, DropGuard},
};

use crate::boxes::{generate_init, generate_moof_and_empty_mdat, GenerateMoofError};

#[allow(clippy::module_name_repetitions)]
pub struct Muxer {
    token: CancellationToken,
    params: TrackParameters,
    state: Arc<Mutex<MuxerState>>,
}

#[derive(Debug, Error)]
pub enum CreateMuxerError {
    #[error("first sample is not an IDR")]
    NotIdr,

    #[error("Dts is not zero")]
    DtsNotZero,

    #[error("generate init: {0}")]
    GenerateInit(#[from] mp4::Mp4Error),
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
        start_time: UnixH264,
        first_frame: H264Data,
    ) -> Result<(Self, H264Writer), CreateMuxerError> {
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
            Session {
                start_time,
                first_request: true,
                next_frame_id: start_frame_id,
            },
        ));
        Ready(StartSessionResponseReady {
            start_time: start_time.into(),
            codecs: self.params.codec.clone(),
        })
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

        let last_frame = state.frames.back().expect("frames not empty");
        if session.next_frame_id <= last_frame.id {
            let next_frame_index = frame_position(state.frames, session.next_frame_id);
            let Some(next_frame_index) = next_frame_index else {
                _ = req.res_tx.send(FramesExpired);
                return;
            };

            let cached_frames = state
                .frames
                .iter()
                .skip(next_frame_index)
                .flat_map(|f| f.muxed(session.start_time));

            let cached_data: Vec<std::io::Result<Bytes>> = {
                if session.first_request {
                    iter::once(state.init_content.clone())
                        .map(Ok)
                        .chain(cached_frames)
                        .collect()
                } else {
                    cached_frames.collect()
                }
            };

            let buffered_cached_data_stream = ReaderStream::with_capacity(
                StreamReader::new(stream::iter(cached_data)),
                Self::HTTP_BUFFER_SIZE,
            );

            let response = Ready(PlayResponseReady(Box::new(buffered_cached_data_stream)));
            _ = req.res_tx.send(response);

            session.first_request = false;
            session.next_frame_id = last_frame.id + 1;
            return;
        }

        let (tx, rx) = oneshot::channel();
        state.frames_on_hold.push(tx);
        let start_time = session.start_time;
        // State lock must be released at this point.
        drop(state2);
        let Ok(frame) = rx.await else {
            _ = req.res_tx.send(MuxerCancelled);
            return;
        };
        let response = Ready(PlayResponseReady(Box::new(stream::iter(
            frame.muxed(start_time),
        ))));
        _ = req.res_tx.send(response);
    }

    #[cfg(test)]
    #[allow(clippy::unwrap_used)]
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

    muxer_start_time: UnixH264,
    sessions: VecDeque<(u32, Session)>,

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
        muxer_start_time: UnixH264,
        mut first_frame: H264Data,
    ) -> Result<Arc<Mutex<Self>>, CreateMuxerError> {
        if !first_frame.random_access_present {
            return Err(CreateMuxerError::NotIdr);
        }
        if *first_frame.dts_offset != 0 {
            return Err(CreateMuxerError::DtsNotZero);
        }

        first_frame.pts = muxer_start_time;

        let state = Arc::new(Mutex::new(Self {
            id,
            frame_count: 0,
            next_frame: first_frame,
            muxer_start_time,
            sessions: VecDeque::new(),
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
            frames: &mut self.frames,
            gops: &self.gops,
            frames_on_hold: &mut self.frames_on_hold,
            init_content: &self.init_content,
        }
    }
}

#[derive(Debug, Error)]
pub enum WriteFrameError {
    #[error("generate mp4 boxes: {0}")]
    GenerateMp4Boxes(#[from] GenerateMoofError),

    #[error("calculate sample duration")]
    ComputeFrameDuration,

    #[error("dts")]
    Dts,
}

struct MuxerStateRef<'a> {
    sessions: &'a mut VecDeque<(u32, Session)>,
    frames: &'a mut VecDeque<Arc<Frame>>,
    gops: &'a VecDeque<Arc<Gop>>,
    frames_on_hold: &'a mut Vec<oneshot::Sender<Arc<Frame>>>,
    init_content: &'a Bytes,
}

fn frame_position(frames: &VecDeque<Arc<Frame>>, frame_id: u64) -> Option<usize> {
    (0..frames.len()).find(|&i| frames[i].id == frame_id)
}

#[derive(Debug, PartialEq, Eq)]
pub enum StartSessionReponse {
    // Date time of first frame.
    Ready(StartSessionResponseReady),
    NotReady,
    SessionAlreadyExist,
    MuxerNotExist,
    StreamerCancelled,
    MuxerCancelled,
}

impl StartSessionReponse {
    #[must_use]
    #[cfg(test)]
    #[track_caller]
    pub fn unwrap(self) -> StartSessionResponseReady {
        let StartSessionReponse::Ready(ready) = self else {
            panic!("not ok: {self:?}")
        };
        ready
    }
}

#[derive(Debug, PartialEq, Eq, Serialize)]
pub struct StartSessionResponseReady {
    #[serde(rename = "startTimeNs")]
    pub(crate) start_time: UnixNano,
    pub(crate) codecs: String,
}

pub struct PlayRequest {
    pub session_id: u32,
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

pub struct PlayResponseReady(pub Box<dyn Stream<Item = std::io::Result<Bytes>> + Send + Unpin>);

impl PlayResponseReady {
    #[cfg(test)]
    #[allow(clippy::unwrap_used)]
    pub async fn collect(self) -> Vec<u8> {
        use futures_lite::StreamExt;
        let bytes: Vec<Bytes> = self.0.try_collect().await.unwrap();
        bytes.into_iter().flatten().collect()
    }
}

impl std::fmt::Debug for PlayResponseReady {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        write!(f, "dyn body")
    }
}

impl PartialEq for PlayResponseReady {
    fn eq(&self, _other: &Self) -> bool {
        todo!();
    }
}

struct Session {
    start_time: UnixH264,
    first_request: bool,
    next_frame_id: u64,
}

struct Frame {
    id: u64,
    sample: VideoSample,
    boxes_size: usize,
}

impl Frame {
    fn new(
        id: u64,
        data: H264Data,
        mut duration: DurationH264,
        muxer_start_time: UnixH264,
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
        let boxes_size = generate_moof_and_empty_mdat(muxer_start_time, &[sample])?.len();

        Ok(Frame {
            id,
            sample: VideoSample {
                pts: data.pts,
                dts_offset: data.dts_offset,
                duration,
                random_access_present: data.random_access_present,
                avcc: data.avcc,
            },
            boxes_size,
        })
    }

    /*fn muxed_size(&self) -> u64 {
        u64::try_from(self.mp4_boxes.len() + self.sample.avcc.len()).expect("fit u64")
    }*/

    fn muxed(&self, start_time: UnixH264) -> impl Iterator<Item = std::io::Result<Bytes>> {
        let mp4_boxes = generate_moof_and_empty_mdat(start_time, &[self.sample.clone()])
            .map_err(|e| std::io::Error::new(std::io::ErrorKind::InvalidData, e));
        if let Ok(boxes) = &mp4_boxes {
            assert_eq!(self.boxes_size, boxes.len());
        }
        let avcc = Bytes::from_owner(ArcPaddedBytes(self.sample.avcc.clone()));
        [mp4_boxes, Ok(avcc)].into_iter()
    }
}

struct ArcPaddedBytes(Arc<PaddedBytes>);

impl AsRef<[u8]> for ArcPaddedBytes {
    fn as_ref(&self) -> &[u8] {
        &self.0
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

    #[cfg(test)]
    #[allow(clippy::unwrap_used)]
    pub async fn test_write(&mut self, pts: i64, avcc: Vec<u8>, random_access: bool) {
        use common::time::DtsOffset;

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

#[async_trait]
impl H264WriterImpl for H264Writer {
    async fn write_h264(&mut self, data: H264Data) -> Result<(), DynError> {
        let mut state = self.state.lock().await;
        if state.cancelled {
            return Ok(());
        }
        Ok(state.write_frame(data)?)
    }
}

#[cfg(test)]
pub struct DebugState {
    pub num_sessions: usize,
    pub gop_count: usize,
    pub next_seg_on_hold_count: usize,
    pub frame_on_hold_count: usize,
}
