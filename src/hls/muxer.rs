use crate::{
    init::generate_init,
    playlist::{primary_playlist, Playlist},
    segmenter::{H264Writer, Segmenter},
    HlsQuery,
};
use async_trait::async_trait;
use bytes::Bytes;
use common::{time::DurationH264, DynLogger, SegmentFinalized, TrackParameters};
use http::{HeaderName, HeaderValue, StatusCode};
use std::{
    collections::HashMap,
    fmt::Formatter,
    io::Cursor,
    sync::Arc,
    time::{SystemTime, UNIX_EPOCH},
};
use tokio::{io::AsyncRead, sync::Mutex};
use tokio_util::sync::CancellationToken;

impl std::fmt::Debug for MuxerFileResponse {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        write!(f, "{} {:?}", self.status, self.headers)
    }
}

#[derive(Debug)]
#[allow(clippy::module_name_repetitions)]
pub struct HlsMuxer {
    token: CancellationToken,
    playlist: Arc<Playlist>,
    params: TrackParameters,

    //videoLastSPS: []byte,
    //videoLastPPS: []byte,
    init_content: Mutex<Bytes>,
}

impl HlsMuxer {
    pub fn new(
        parent_token: &CancellationToken,
        logger: &DynLogger,
        segment_count: usize,
        segment_duration: DurationH264,
        part_duration: DurationH264,
        segment_max_size: u64,
        params: TrackParameters,
    ) -> (Self, H264Writer) {
        let token = parent_token.child_token();
        let playlist = Arc::new(Playlist::new(token.clone(), logger.clone(), segment_count));

        let now = i64::try_from(
            SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .expect("time went backwards")
                .as_nanos(),
        )
        .expect("time should fit i64");

        let segmenter = Segmenter::new(
            now,
            segment_duration,
            part_duration,
            segment_max_size,
            playlist.clone(),
        );

        let muxer = Self {
            token,
            playlist,
            params,
            init_content: Mutex::new(Bytes::new()),
        };
        (muxer, H264Writer::new(segmenter))
    }

    pub fn cancel(&self) {
        self.token.cancel();
    }

    pub async fn file(&self, name: &str, query: &HlsQuery) -> MuxerFileResponse {
        if name == "index.m3u8" {
            return primary_playlist(&self.params.codec);
        }

        if name == "init.mp4" {
            //sps := m.videoTrack.SPS

            if self.init_content.lock().await.is_empty()
            /*|| (!bytes.Equal(m.videoLastSPS, sps) ||
            !bytes.Equal(m.videoLastPPS, m.videoTrack.PPS))*/
            {
                let result = generate_init(&self.params);
                match result {
                    Ok(init_content) => *self.init_content.lock().await = init_content,
                    Err(_) => {
                        //m.logf(log.LevelError, "generate init.mp4: %w", err)
                        return MuxerFileResponse {
                            status: StatusCode::INTERNAL_SERVER_ERROR,
                            headers: None,
                            body: None,
                        };
                    }
                }
                //m.videoLastSPS = m.videoTrack.SPS
                //m.videoLastPPS = m.videoTrack.PPS
            }

            let body = Box::new(Cursor::new(self.init_content.lock().await.clone()));
            #[allow(clippy::unwrap_used)]
            return MuxerFileResponse {
                status: StatusCode::OK,
                headers: Some(HashMap::from([(
                    HeaderName::from_bytes(b"Content-Type").unwrap(),
                    HeaderValue::from_str("video/mp4").unwrap(),
                )])),
                body: Some(body),
            };
        }

        self.playlist.file(name, query).await
    }

    // Blocks until a new segment has been finalized.
    /*async fn wait_for_seg_finalized(&self) {
        self.playlist.wait_for_seg_finalized().await
    }*/
}

#[async_trait]
impl common::HlsMuxer for HlsMuxer {
    fn params(&self) -> &TrackParameters {
        &self.params
    }

    // Returns the first segment with a ID greater than prevID.
    // Will wait for new segments if the next segment isn't cached.
    async fn next_segment(&self, prev_id: u64) -> Option<Arc<SegmentFinalized>> {
        self.playlist.next_segment(prev_id).await
    }
}

#[async_trait]
pub trait NextSegmentGetter {
    async fn next_segment(&self, prev_id: u64) -> Option<Arc<SegmentFinalized>>;
}

// Response of the Muxer's File() fn.
#[allow(clippy::module_name_repetitions)]
pub struct MuxerFileResponse {
    pub status: StatusCode,
    pub headers: Option<HashMap<HeaderName, HeaderValue>>,
    pub body: Option<Box<dyn AsyncRead + Send + Unpin>>,
}

impl MuxerFileResponse {
    #[cfg(test)]
    #[allow(clippy::unwrap_used)]
    pub async fn print(mut self) -> String {
        use tokio::io::AsyncReadExt;
        let body = if let Some(body) = &mut self.body {
            let mut buf = "\n".to_owned();
            body.read_to_string(&mut buf).await.unwrap();
            buf
        } else {
            String::new()
        };
        format!("{}\n{:?}{}", self.status, self.headers, body)
    }
}

pub const MUXER_FILE_RESPONSE_CANCELLED: MuxerFileResponse = MuxerFileResponse {
    status: StatusCode::NOT_FOUND,
    headers: None,
    body: None,
};

pub const MUXER_FILE_RESPONSE_ERROR: MuxerFileResponse = MuxerFileResponse {
    status: StatusCode::INTERNAL_SERVER_ERROR,
    headers: None,
    body: None,
};

pub const MUXER_FILE_RESPONSE_BAD_REQUEST: MuxerFileResponse = MuxerFileResponse {
    status: StatusCode::BAD_REQUEST,
    headers: None,
    body: None,
};

pub const MUXER_FILE_RESPONSE_NOT_FOUND: MuxerFileResponse = MuxerFileResponse {
    status: StatusCode::NOT_FOUND,
    headers: None,
    body: None,
};
