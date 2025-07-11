use crate::{
    DurationH264, HlsQuery,
    error::FullPlaylistError,
    muxer::{
        MUXER_FILE_RESPONSE_BAD_REQUEST, MUXER_FILE_RESPONSE_CANCELLED, MUXER_FILE_RESPONSE_ERROR,
        MUXER_FILE_RESPONSE_NOT_FOUND, MuxerFileResponse,
    },
    part::{PartFinalized, part_name},
    segment::SegmentFinalized,
};
use common::{ArcLogger, LogEntry, LogLevel, Segment, SegmentImpl, time::SECOND};
use http::{HeaderName, HeaderValue, StatusCode};
use std::{
    collections::{HashMap, VecDeque},
    io::Cursor,
    sync::Arc,
};
use tokio::sync::{Mutex, MutexGuard, oneshot};
use tokio_util::sync::CancellationToken;

struct Gap(DurationH264);

enum SegmentOrGap {
    Segment(Arc<SegmentFinalized>),
    Gap(Gap),
}

impl SegmentOrGap {
    fn duration(&self) -> DurationH264 {
        match self {
            SegmentOrGap::Segment(seg) => seg.duration(),
            SegmentOrGap::Gap(gap) => gap.0,
        }
    }
}

fn target_duration(segments: &VecDeque<SegmentOrGap>) -> i64 {
    let mut ret: i64 = 0;

    // EXTINF, when rounded to the nearest integer, must be <= EXT-X-TARGETDURATION
    for sog in segments {
        let v = div_up(sog.duration().as_nanos(), SECOND);
        if v > ret {
            ret = v;
        }
    }

    ret
}

fn div_up(a: i64, b: i64) -> i64 {
    (a + (b - 1)) / b
}

fn part_target_duration(
    segments: &VecDeque<SegmentOrGap>,
    next_segment_parts: &Vec<Arc<PartFinalized>>,
) -> DurationH264 {
    let mut ret = DurationH264::new(0);

    for sog in segments {
        let SegmentOrGap::Segment(seg) = sog else {
            continue;
        };

        for part in seg.parts() {
            if part.rendered_duration > ret {
                ret = part.rendered_duration;
            }
        }
    }

    for part in next_segment_parts {
        if part.rendered_duration > ret {
            ret = part.rendered_duration;
        }
    }

    ret
}

#[allow(clippy::struct_field_names)]
pub struct Playlist {
    muxer_id: u16,
    state: Arc<Mutex<PlaylistState>>,
}

impl Playlist {
    #[allow(clippy::too_many_lines)]
    pub fn new(
        token: CancellationToken,
        logger: ArcLogger,
        segment_count: usize,
        muxer_id: u16,
    ) -> Self {
        let state = Arc::new(Mutex::new(PlaylistState {
            is_cancelled: false,
            logger,
            segment_count,
            segments: VecDeque::new(),
            segment_delete_count: 0,
            parts_by_name: HashMap::new(),
            next_segment_id: 0,
            next_segment_parts: Vec::new(),
            next_part_id: 0,

            playlists_on_hold: Vec::new(),
            parts_on_hold: Vec::new(),
            seg_final_on_hold: Vec::new(),
            next_segments_on_hold: Vec::new(),
        }));

        // Cancellation and cleanup.
        let state2 = state.clone();
        tokio::spawn(async move {
            token.cancelled().await;
            let mut state = state2.lock().await;

            state.is_cancelled = true;

            // Drop pending request channels.
            state.next_segments_on_hold.clear();
            state.parts_on_hold.clear();
            state.playlists_on_hold.clear();
        });

        Self { muxer_id, state }
    }

    async fn get_state_lock(&self) -> Option<MutexGuard<PlaylistState>> {
        let state = self.state.lock().await;
        // State cannot be used after being cancelled.
        if state.is_cancelled {
            return None;
        }
        Some(state)
    }

    pub async fn on_segment_finalized(&self, segment: SegmentFinalized) {
        let Some(mut state) = self.get_state_lock().await else {
            // Cancelled.
            return;
        };
        state.segment_finalized(&Arc::new(segment));
    }

    #[allow(clippy::case_sensitive_file_extension_comparisons)]
    pub async fn file(&self, name: &str, query: &HlsQuery) -> MuxerFileResponse {
        if name == "stream.m3u8" {
            return self.playlist_reader(query).await;
        }

        if name.ends_with(".mp4") {
            return self.segment_reader(name).await;
        }

        // Apple bug?
        if name.ends_with(".mp") {
            return self.segment_reader(&[name, "4"].join("")).await;
        }

        MUXER_FILE_RESPONSE_NOT_FOUND
    }

    async fn blocking_playlist(
        &self,
        is_delta_update: bool,
        msn: u64,
        part: u64,
    ) -> MuxerFileResponse {
        let res_rx: oneshot::Receiver<MuxerFileResponse>;
        {
            let Some(mut state) = self.get_state_lock().await else {
                // Cancelled.
                return MUXER_FILE_RESPONSE_CANCELLED;
            };
            // If the _HLS_msn is greater than the Media Sequence Number of the last
            // Media Segment in the current Playlist plus two, or if the _HLS_part
            // exceeds the last Partial Segment in the current Playlist by the
            // Advance Part Limit, then the server SHOULD immediately return Bad
            // Request, such as HTTP 400.
            if msn > (state.next_segment_id + 1) {
                return MUXER_FILE_RESPONSE_BAD_REQUEST;
            }

            if state.has_content() && state.has_part(msn, part) {
                let body = match state.full_playlist(is_delta_update) {
                    Ok(v) => v,
                    Err(e) => {
                        state.log(LogLevel::Error, &format!("full playlist: {e}"));
                        return MUXER_FILE_RESPONSE_ERROR;
                    }
                };
                return MuxerFileResponse {
                    status: StatusCode::OK,
                    headers: Some(HashMap::from([(
                        #[allow(clippy::unwrap_used)]
                        HeaderName::from_bytes(b"Content-Type").unwrap(),
                        #[allow(clippy::unwrap_used)]
                        HeaderValue::from_str("application/x-mpegURL").unwrap(),
                    )])),
                    body: Some(Box::new(Cursor::new(body))),
                };
            }

            let res_tx: oneshot::Sender<MuxerFileResponse>;
            (res_tx, res_rx) = oneshot::channel();
            state.playlists_on_hold.push(BlockingPlaylistRequest {
                is_delta_update,
                msn,
                part,
                res_tx,
            });
        }

        // Mutex must be released at this point.
        let Ok(res) = res_rx.await else {
            return MUXER_FILE_RESPONSE_CANCELLED;
        };
        res
    }

    #[allow(clippy::similar_names)]
    async fn playlist_reader(&self, query: &HlsQuery) -> MuxerFileResponse {
        if let Some((msn, part)) = query.msn_and_part {
            return self
                .blocking_playlist(query.is_delta_update, msn, part)
                .await;
        }

        let Some(state) = self.get_state_lock().await else {
            // Cancelled.
            return MUXER_FILE_RESPONSE_CANCELLED;
        };
        if !state.has_content() {
            return MuxerFileResponse {
                status: StatusCode::NOT_FOUND,
                headers: None,
                body: None,
            };
        }

        let body = match state.full_playlist(query.is_delta_update) {
            Ok(v) => v,
            Err(e) => {
                state.log(LogLevel::Error, &format!("full playlist: {e}"));
                return MUXER_FILE_RESPONSE_ERROR;
            }
        };

        MuxerFileResponse {
            status: StatusCode::OK,
            #[allow(clippy::unwrap_used)]
            headers: Some(HashMap::from([(
                HeaderName::from_bytes(b"Content-Type").unwrap(),
                HeaderValue::from_str("application/x-mpegURL").unwrap(),
            )])),
            body: Some(Box::new(Cursor::new(body))),
        }
    }

    async fn blocking_part(&self, file_name: &str) -> MuxerFileResponse {
        let res_rx: oneshot::Receiver<MuxerFileResponse>;
        {
            let Some(mut state) = self.get_state_lock().await else {
                // Cancelled.
                return MUXER_FILE_RESPONSE_CANCELLED;
            };

            let base = file_name
                .strip_suffix(".mp4")
                .expect("part_name to have suffix");
            #[allow(clippy::unwrap_used)]
            if let Some(part) = state.parts_by_name.get(base) {
                return MuxerFileResponse {
                    status: StatusCode::OK,
                    headers: Some(HashMap::from([(
                        HeaderName::from_bytes(b"Content-Type").unwrap(),
                        HeaderValue::from_str("video/mp4").unwrap(),
                    )])),
                    body: Some(part.reader()),
                };
            }

            if file_name != part_name(state.next_part_id) {
                return MUXER_FILE_RESPONSE_NOT_FOUND;
            }

            let res_tx: oneshot::Sender<MuxerFileResponse>;
            (res_tx, res_rx) = oneshot::channel();
            let req = BlockingPartRequest {
                part_name: file_name.to_owned(),
                part_id: state.next_part_id,
                res_tx,
            };
            state.parts_on_hold.push(req);
        }

        // Lock must be released at this point.
        let Ok(res) = res_rx.await else {
            return MUXER_FILE_RESPONSE_CANCELLED;
        };
        res
    }

    #[allow(clippy::similar_names)]
    async fn segment_reader(&self, file_name: &str) -> MuxerFileResponse {
        if file_name.starts_with("seg") {
            let Some(state) = self.get_state_lock().await else {
                // Cancelled.
                return MUXER_FILE_RESPONSE_CANCELLED;
            };

            let base = file_name
                .strip_suffix(".mp4")
                .expect("file_name to have suffix");

            let Some(segment) = state.segment_by_name(base) else {
                return MUXER_FILE_RESPONSE_NOT_FOUND;
            };

            return MuxerFileResponse {
                status: StatusCode::OK,
                headers: Some(HashMap::from([(
                    #[allow(clippy::unwrap_used)]
                    HeaderName::from_bytes(b"Content-Type").unwrap(),
                    #[allow(clippy::unwrap_used)]
                    HeaderValue::from_str("video/mp4").unwrap(),
                )])),
                body: Some(segment.reader()),
            };
        }
        if file_name.starts_with("part") {
            return self.blocking_part(file_name).await;
        }
        MUXER_FILE_RESPONSE_NOT_FOUND
    }

    pub async fn part_finalized(&self, part: Arc<PartFinalized>) -> Option<()> {
        let mut state = self.get_state_lock().await?;

        state.next_part_id = part.id + 1;
        state.parts_by_name.insert(part.name(), part.clone());
        state.next_segment_parts.push(part);

        state.check_pending();
        Some(())
    }

    #[allow(clippy::similar_names)]
    pub async fn next_segment(&self, prev_seg: Option<Segment>) -> Option<Segment> {
        let res_rx: oneshot::Receiver<Arc<SegmentFinalized>>;
        {
            let mut state = self.get_state_lock().await?;

            let prev_id = match prev_seg {
                Some(seg)
                    if seg.muxer_id() == self.muxer_id && seg.id() < state.next_segment_id =>
                {
                    seg.id()
                }
                Some(_) | None => 0,
            };

            let seg = || {
                for sog in &state.segments {
                    let SegmentOrGap::Segment(seg) = sog else {
                        continue;
                    };
                    if prev_id < seg.id() {
                        return Some(seg.clone());
                    }
                }
                None
            };
            if let Some(seg) = seg() {
                return Some(seg);
            }

            let res_tx: oneshot::Sender<Arc<SegmentFinalized>>;
            (res_tx, res_rx) = oneshot::channel();
            state
                .next_segments_on_hold
                .push(NextSegmentRequest { prev_id, res_tx });
        }

        // Lock must be released at this point.
        res_rx
            .await
            .map(|v| {
                let seg: Segment = v;
                seg
            })
            .ok()
    }

    #[cfg(test)]
    #[allow(clippy::unwrap_used)]
    pub async fn debug_state(&self) -> PlaylistDebugState {
        #[allow(clippy::unwrap_used)]
        let state = self.get_state_lock().await.unwrap();
        let num_segments = state
            .segments
            .iter()
            .filter(|sog| match sog {
                SegmentOrGap::Segment(_) => true,
                SegmentOrGap::Gap(_) => false,
            })
            .count();
        PlaylistDebugState {
            num_segments,
            num_segments_on_hold: state.next_segments_on_hold.len(),
            num_playlists_on_hold: state.playlists_on_hold.len(),
        }
    }
}

#[derive(Debug)]
struct BlockingPlaylistRequest {
    is_delta_update: bool,
    msn: u64,
    part: u64,
    res_tx: oneshot::Sender<MuxerFileResponse>,
}

#[derive(Debug)]
struct BlockingPartRequest {
    part_name: String,
    part_id: u64,
    res_tx: oneshot::Sender<MuxerFileResponse>,
}

#[derive(Debug)]
struct NextSegmentRequest {
    prev_id: u64,
    res_tx: oneshot::Sender<Arc<SegmentFinalized>>,
}

#[derive(Debug)]
#[allow(unused, clippy::module_name_repetitions, clippy::struct_field_names)]
pub struct PlaylistDebugState {
    pub num_segments: usize,
    pub num_segments_on_hold: usize,
    pub num_playlists_on_hold: usize,
}

struct PlaylistState {
    is_cancelled: bool,
    logger: ArcLogger,
    segment_count: usize,
    segments: VecDeque<SegmentOrGap>,
    segment_delete_count: usize,
    parts_by_name: HashMap<String, Arc<PartFinalized>>,
    next_segment_id: u64,
    next_segment_parts: Vec<Arc<PartFinalized>>,
    next_part_id: u64,

    playlists_on_hold: Vec<BlockingPlaylistRequest>,
    parts_on_hold: Vec<BlockingPartRequest>,
    seg_final_on_hold: Vec<oneshot::Sender<()>>,
    next_segments_on_hold: Vec<NextSegmentRequest>,
}

impl PlaylistState {
    fn log(&self, level: LogLevel, msg: &str) {
        self.logger.log(LogEntry::new2(
            level,
            "app",
            &format!("hls playlist: {msg}"),
        ));
    }

    fn check_pending(&mut self) {
        // RUSTC: extract_if
        if self.has_content() {
            let mut i = 0;
            #[allow(clippy::unwrap_used)]
            while i < self.playlists_on_hold.len() {
                if self.has_part(
                    self.playlists_on_hold[i].msn,
                    self.playlists_on_hold[i].part,
                ) {
                    let req = self.playlists_on_hold.swap_remove(i);

                    let body = match self.full_playlist(req.is_delta_update) {
                        Ok(v) => v,
                        Err(e) => {
                            self.log(LogLevel::Error, &format!("full playlist: {e}"));
                            _ = req.res_tx.send(MUXER_FILE_RESPONSE_ERROR);
                            continue;
                        }
                    };
                    _ = req.res_tx.send(MuxerFileResponse {
                        status: StatusCode::OK,
                        headers: Some(HashMap::from([(
                            HeaderName::from_bytes(b"Content-Type").unwrap(),
                            HeaderValue::from_str("application/x-mpegURL").unwrap(),
                        )])),
                        body: Some(Box::new(Cursor::new(body))),
                    });
                } else {
                    i += 1;
                }
            }
        }

        let mut i = 0;
        #[allow(clippy::unwrap_used)]
        while i < self.parts_on_hold.len() {
            if self.next_part_id <= self.parts_on_hold[i].part_id {
                i += 1;
            } else {
                let req = self.parts_on_hold.swap_remove(i);
                let Some(part) = self.parts_by_name.get(&req.part_name) else {
                    // TODO: LOG
                    _ = req.res_tx.send(MUXER_FILE_RESPONSE_ERROR);
                    continue;
                };

                _ = req.res_tx.send(MuxerFileResponse {
                    status: StatusCode::OK,
                    headers: Some(HashMap::from([(
                        HeaderName::from_bytes(b"Content-Type").unwrap(),
                        HeaderValue::from_str("video/mp4").unwrap(),
                    )])),
                    body: Some(part.reader()),
                });
            }
        }
    }

    fn has_content(&self) -> bool {
        !self.segments.is_empty()
    }

    fn has_part(&self, mut segment_id: u64, mut part_id: u64) -> bool {
        if !self.has_content() {
            return false;
        }

        for sop in &self.segments {
            let SegmentOrGap::Segment(seg) = sop else {
                continue;
            };

            if segment_id != seg.id() {
                continue;
            }

            // If the Client requests a Part Index greater than that of the final
            // Partial Segment of the Parent Segment, the Server MUST treat the
            // request as one for Part Index 0 of the following Parent Segment.
            if part_id >= u64::try_from(seg.parts().len()).expect("usize to fit u64") {
                segment_id += 1;
                part_id = 0;
                continue;
            }

            return true;
        }

        if segment_id != self.next_segment_id {
            return false;
        }

        if part_id >= u64::try_from(self.next_segment_parts.len()).expect("usize to fit u64") {
            return false;
        }

        true
    }

    fn segment_by_name(&self, name: &str) -> Option<&SegmentFinalized> {
        for sog in &self.segments {
            if let SegmentOrGap::Segment(seg) = sog {
                if seg.name() == name {
                    return Some(seg);
                }
            }
        }
        None
    }

    fn segment_finalized(&mut self, segment: &Arc<SegmentFinalized>) {
        // add initial gaps, required by iOS.
        if self.segments.is_empty() {
            for _ in 0..7 {
                self.segments
                    .push_back(SegmentOrGap::Gap(Gap(segment.duration())));
            }
        }

        self.segments
            .push_back(SegmentOrGap::Segment(segment.clone()));

        self.next_segment_id = segment.id() + 1;

        self.next_segment_parts.clear();

        if self.segments.len() > self.segment_count {
            let to_delete = self.segments.pop_front().expect("len > 0");

            if let SegmentOrGap::Segment(to_delete_seg) = to_delete {
                for part in to_delete_seg.parts() {
                    self.parts_by_name
                        .remove(&part.name())
                        .expect("part should exist in lookup table");
                }
            }

            self.segment_delete_count += 1;
        }

        for done in self.seg_final_on_hold.drain(..) {
            done.send(()).expect("sender should still be alive");
        }

        /*
        self.nextSegmentsOnHold
        .drain_filter(|req| segment.id > req.prevID)
        .for_each(|req| {
            req.res.send(segment.clone());
        });
        */
        let mut i = 0;
        while i < self.next_segments_on_hold.len() {
            if segment.id() > self.next_segments_on_hold[i].prev_id {
                let req = self.next_segments_on_hold.swap_remove(i);
                req.res_tx
                    .send(segment.clone())
                    .expect("sender should still be alive");
            } else {
                i += 1;
            }
        }

        self.check_pending();
    }

    fn full_playlist(&self, is_delta_update: bool) -> Result<Vec<u8>, FullPlaylistError> {
        let mut cnt = "#EXTM3U\n".to_owned();
        cnt += "#EXT-X-VERSION:9\n";

        let target_duration = target_duration(&self.segments);
        cnt += &format!("#EXT-X-TARGETDURATION:{target_duration}\n");

        let skip_boundary = f64::from(u32::try_from(target_duration)?) * 6.0;

        let part_target_duration = part_target_duration(&self.segments, &self.next_segment_parts);

        // The value is an enumerated-string whose value is YES if the server
        // supports Blocking Playlist Reload
        cnt += "#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES";

        // The value is a decimal-floating-point number of seconds that
        // indicates the server-recommended minimum distance from the end of
        // the Playlist at which clients should begin to play or to which
        // they should seek when playing in Low-Latency Mode.  Its value MUST
        // be at least twice the Part Target Duration.  Its value SHOULD be
        // at least three times the Part Target Duration.
        cnt += &format!(
            ",PART-HOLD-BACK={:.5}",
            part_target_duration.as_secs_f64() * 2.5
        );
        //cnt += ",PART-HOLD-BACK=" + strconv.FormatFloat((partTargetDuration*SECOND)*2.5, 'f', 5, 64)

        // Indicates that the Server can produce Playlist Delta Updates in
        // response to the _HLS_skip Delivery Directive.  Its value is the
        // Skip Boundary, a decimal-floating-point number of seconds.  The
        // Skip Boundary MUST be at least six times the Target Duration.
        cnt += &format!(",CAN-SKIP-UNTIL={skip_boundary}");

        cnt += "\n";

        cnt += &format!(
            "#EXT-X-PART-INF:PART-TARGET={}\n",
            part_target_duration.as_secs_f64(),
        );

        cnt += &format!("#EXT-X-MEDIA-SEQUENCE:{}\n", self.segment_delete_count);

        let mut skipped = 0;
        if is_delta_update {
            let mut cur_duration = DurationH264::new(0);
            let mut shown = 0;
            for sog in &self.segments {
                cur_duration = cur_duration
                    .checked_add(sog.duration())
                    .ok_or(FullPlaylistError::DurationOverflowing)?;
                if cur_duration.as_secs_f64() >= skip_boundary {
                    break;
                }
                shown += 1;
            }
            skipped = (self.segments.len()) - shown;
            cnt += &format!("#EXT-X-SKIP:SKIPPED-SEGMENTS={skipped}\n");
        } else {
            cnt += "#EXT-X-MAP:URI=\"init.mp4\"\n";
        }

        for (i, sog) in self.segments.iter().enumerate() {
            if i < skipped {
                continue;
            }

            match sog {
                SegmentOrGap::Segment(seg) => {
                    /*if (self.segments.len() - i) <= 2 {
                        cnt += "#EXT-X-PROGRAM-DATE-TIME:"
                            + seg.StartTime.Format("2006-01-02T15:04:05.999Z07:00")
                            + "\n"
                    }*/

                    if (self.segments.len() - i) <= 2 {
                        for part in seg.parts() {
                            cnt += &format!(
                                "#EXT-X-PART:DURATION={0:.5},URI=\"{1}.mp4\"",
                                part.rendered_duration.as_secs_f64(),
                                part.name(),
                            );
                            if part.is_independent {
                                cnt += ",INDEPENDENT=YES";
                            }
                            cnt += "\n";
                        }
                    }

                    cnt += &format!("#EXTINF:{0:.5},\n", seg.duration().as_secs_f64());
                    cnt += &format!("{}.mp4\n", seg.name());
                }
                SegmentOrGap::Gap(gap) => {
                    cnt += "#EXT-X-GAP\n";
                    cnt += &format!("#EXTINF:{0:.5},\n", gap.0.as_secs_f64());
                    cnt += "gap.mp4\n";
                }
            }
        }

        for part in &self.next_segment_parts {
            cnt += &format!(
                "#EXT-X-PART:DURATION={0:.5},URI=\"{1}.mp4\"",
                part.rendered_duration.as_secs_f64(),
                part.name(),
            );
            if part.is_independent {
                cnt += ",INDEPENDENT=YES";
            }
            cnt += "\n";
        }

        // preload hint must always be present
        // otherwise hls.js goes into a loop
        cnt += &format!(
            "#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"{}.mp4\"\n",
            &part_name(self.next_part_id),
        );

        Ok(cnt.into())
    }
}

#[allow(clippy::module_name_repetitions)]
pub fn primary_playlist(codec: &str) -> MuxerFileResponse {
    let body = [
        "#EXTM3U\n",
        "#EXT-X-VERSION:9\n",
        "#EXT-X-INDEPENDENT-SEGMENTS\n",
        "\n",
        &format!("#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"{codec}\"\n"),
        "stream.m3u8\n",
    ]
    .join("")
    .into_bytes();

    #[allow(clippy::unwrap_used)]
    MuxerFileResponse {
        status: StatusCode::OK,
        headers: Some(HashMap::from([(
            HeaderName::from_bytes(b"Content-Type").unwrap(),
            HeaderValue::from_str("application/x-mpegURL").unwrap(),
        )])),
        body: Some(Box::new(Cursor::new(body))),
    }
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use common::DummyLogger;
    use pretty_assertions::assert_eq;
    use tokio::io::AsyncReadExt;

    #[tokio::test]
    async fn test_primary_playlist() {
        let got = primary_playlist("avc1.640016");
        assert_eq!(StatusCode::OK, got.status);

        let mut got_body = Vec::with_capacity(200);
        got.body.unwrap().read_buf(&mut got_body).await.unwrap();

        let want_body = "#EXTM3U
#EXT-X-VERSION:9
#EXT-X-INDEPENDENT-SEGMENTS

#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"avc1.640016\"
stream.m3u8
";
        assert_eq!(want_body, String::from_utf8(got_body).unwrap());
    }

    fn new_empty_playlist_state() -> PlaylistState {
        PlaylistState {
            is_cancelled: false,
            logger: DummyLogger::new(),
            segment_count: 0,
            segments: VecDeque::new(),
            segment_delete_count: 0,
            parts_by_name: HashMap::new(),
            next_segment_id: 0,
            next_segment_parts: Vec::new(),
            next_part_id: 0,
            playlists_on_hold: Vec::new(),
            parts_on_hold: Vec::new(),
            seg_final_on_hold: Vec::new(),
            next_segments_on_hold: Vec::new(),
        }
    }

    #[test]
    fn test_full_playlist_delta_update_true() {
        let playlist = new_empty_playlist_state();
        let got = playlist.full_playlist(true).unwrap();
        let want = "#EXTM3U
#EXT-X-VERSION:9
#EXT-X-TARGETDURATION:0
#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=0.00000,CAN-SKIP-UNTIL=0
#EXT-X-PART-INF:PART-TARGET=0
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-SKIP:SKIPPED-SEGMENTS=0
#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part0.mp4\"
";
        assert_eq!(want, String::from_utf8(got).unwrap());
    }

    #[test]
    fn test_full_playlist_delta_update_false() {
        let playlist = new_empty_playlist_state();
        let got = playlist.full_playlist(false).unwrap();
        let want = "#EXTM3U
#EXT-X-VERSION:9
#EXT-X-TARGETDURATION:0
#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=0.00000,CAN-SKIP-UNTIL=0
#EXT-X-PART-INF:PART-TARGET=0
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-MAP:URI=\"init.mp4\"
#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part0.mp4\"
";
        assert_eq!(want, String::from_utf8(got).unwrap());
    }
}
