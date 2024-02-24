mod error;
mod init;
mod muxer;
mod part;
mod playlist;
mod segment;
mod segmenter;
mod types;

use crate::error::PartHlsQueryError;
pub use crate::error::SegmenterWriteH264Error;
use common::{
    time::{DurationH264, H264_MILLISECOND},
    Cancelled, DynLogger, TrackParameters,
};
pub use error::ParseParamsError;
pub use muxer::{HlsMuxer, NextSegmentGetter};
pub use segmenter::H264Writer;
use serde::Deserialize;
use std::{collections::HashMap, sync::Arc};
use tokio::sync::{mpsc, oneshot};
use tokio_util::sync::CancellationToken;
use types::MuxerIdCounter;
pub use types::{track_params_from_video_params, VIDEO_TRACK_ID};

pub struct HlsServer {
    new_muxer_tx: mpsc::Sender<NewMuxerRequest>,
    muxer_by_name_tx: mpsc::Sender<MuxerByNameRequest>,
}

impl HlsServer {
    pub fn new(token: CancellationToken, logger: DynLogger) -> Self {
        let (new_muxer_tx, mut new_muxer_rx) = mpsc::channel::<NewMuxerRequest>(1);
        let (muxer_by_name_tx, mut muxer_by_name_rx) = mpsc::channel::<MuxerByNameRequest>(1);

        tokio::spawn(async move {
            let mut muxers: HashMap<String, Arc<HlsMuxer>> = HashMap::new();
            let mut muxer_id_counter = MuxerIdCounter::new();
            loop {
                tokio::select! {
                    () = token.cancelled() => return,

                    req = new_muxer_rx.recv() =>  {
                        let Some(req) = req else {
                            return
                        };

                        if let Some(old_muxer) = muxers.remove(&req.name) {
                            old_muxer.cancel();
                        }
                        let (muxer, writer) = HlsMuxer::new(
                            &req.token,
                            &logger,
                            HLS_SEGMENT_COUNT,
                            HLS_SEGMENT_DURATION,
                            HLS_PART_DURATION,
                            HLS_SEGMENT_MAX_SIZE,
                            req.params,
                            muxer_id_counter.next_id(),
                        );

                        let muxer = Arc::new(muxer);
                        muxers.insert(req.name, muxer.clone());
                        _ = req.res_tx.send((muxer,writer));
                    }

                    req = muxer_by_name_rx.recv() => {
                        let Some(req) = req else {
                            return
                        };
                        _ = req.res_tx.send(muxers.get(&req.name).cloned());
                    }
                }
            }
        });

        Self {
            new_muxer_tx,
            muxer_by_name_tx,
        }
    }

    // Creates muxer and returns a H264Writer to it.
    // Stops and replaces existing muxer if present.
    #[allow(clippy::similar_names)]
    pub async fn new_muxer(
        &self,
        token: CancellationToken,
        name: String,
        params: TrackParameters,
    ) -> Result<(Arc<HlsMuxer>, H264Writer), Cancelled> {
        let (res_tx, res_rx) = oneshot::channel();
        let req = NewMuxerRequest {
            token,
            name,
            params,
            res_tx,
        };
        if self.new_muxer_tx.send(req).await.is_err() {
            return Err(Cancelled);
        }

        let Ok(res) = res_rx.await else {
            return Err(Cancelled);
        };
        Ok(res)
    }

    #[allow(clippy::similar_names)]
    pub async fn muxer_by_name(&self, name: String) -> Result<Option<Arc<HlsMuxer>>, Cancelled> {
        let (res_tx, res_rx) = oneshot::channel();
        let req = MuxerByNameRequest { name, res_tx };

        if self.muxer_by_name_tx.send(req).await.is_err() {
            return Err(Cancelled);
        };

        let Ok(res) = res_rx.await else {
            return Err(Cancelled);
        };
        Ok(res)
    }
}

const HLS_SEGMENT_COUNT: usize = 3;
const HLS_SEGMENT_DURATION: DurationH264 = DurationH264::new(900 * H264_MILLISECOND);
const HLS_PART_DURATION: DurationH264 = DurationH264::new(300 * H264_MILLISECOND);

const MB: u64 = 1_000_000;
const HLS_SEGMENT_MAX_SIZE: u64 = 50 * MB;

struct NewMuxerRequest {
    token: CancellationToken,
    name: String,
    params: TrackParameters,
    res_tx: oneshot::Sender<(Arc<HlsMuxer>, H264Writer)>,
}

struct MuxerByNameRequest {
    name: String,
    res_tx: oneshot::Sender<Option<Arc<HlsMuxer>>>,
}

#[derive(Debug)]
pub struct HlsQuery {
    msn_and_part: Option<(u64, u64)>,
    is_delta_update: bool,
}

impl<'de> Deserialize<'de> for HlsQuery {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        use serde::de::Error;

        #[derive(Deserialize)]
        struct Temp {
            #[serde(rename = "_HLS_msn")]
            msn: Option<String>,

            #[serde(rename = "_HLS_part")]
            part: Option<String>,

            #[serde(rename = "_HLS_skip")]
            skip: Option<String>,
        }
        let temp = Temp::deserialize(deserializer).map_err(Error::custom)?;

        let msn = if let Some(msn) = temp.msn {
            Some(msn.parse().map_err(Error::custom)?)
        } else {
            None
        };

        let part = if let Some(part) = temp.part {
            Some(part.parse().map_err(Error::custom)?)
        } else {
            None
        };

        let msn_and_part = match (msn, part) {
            (Some(msn), Some(part)) => Some((msn, part)),
            (Some(_), None) | (None, Some(_)) => {
                return Err(Error::custom(PartHlsQueryError::BothOrNeitherMsnAndPart));
            }
            (None, None) => None,
        };

        let is_delta_update = if let Some(skip) = &temp.skip {
            skip == "YES" || skip == "v2"
        } else {
            false
        };

        Ok(Self {
            msn_and_part,
            is_delta_update,
        })
    }

    fn deserialize_in_place<D>(deserializer: D, place: &mut Self) -> Result<(), D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        // Default implementation just delegates to `deserialize` impl.
        *place = Deserialize::deserialize(deserializer)?;
        Ok(())
    }
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use common::{DummyLogger, HlsMuxer};
    use pretty_assertions::assert_eq;

    #[tokio::test]
    async fn test_part_duration() {
        let token = CancellationToken::new();
        let server = HlsServer::new(token.clone(), DummyLogger::new());

        let params = TrackParameters {
            width: 64,
            height: 64,
            codec: "test_codec".to_owned(),
            extra_data: Vec::new(),
        };

        let (_, mut writer) = server
            .new_muxer(token, "test".to_owned(), params)
            .await
            .unwrap();
        let muxer = server
            .muxer_by_name("test".to_owned())
            .await
            .unwrap()
            .unwrap();

        #[rustfmt::skip]
        assert_eq!(
"404 Not Found
None", get_playlist(&muxer, None).await
        );

        // 1 second = 90000.
        writer.test_write(0, Vec::new(), true).await;
        writer.test_write(100_000, Vec::new(), true).await;

        #[rustfmt::skip]
        assert_eq!(
"200 OK
Some({\"content-type\": \"application/x-mpegURL\"})
#EXTM3U
#EXT-X-VERSION:9
#EXT-X-TARGETDURATION:2
#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=2.77778,CAN-SKIP-UNTIL=12
#EXT-X-PART-INF:PART-TARGET=1.111111111
#EXT-X-MEDIA-SEQUENCE:1
#EXT-X-MAP:URI=\"init.mp4\"
#EXT-X-GAP
#EXTINF:1.11111,
gap.mp4
#EXT-X-GAP
#EXTINF:1.11111,
gap.mp4
#EXT-X-GAP
#EXTINF:1.11111,
gap.mp4
#EXT-X-GAP
#EXTINF:1.11111,
gap.mp4
#EXT-X-GAP
#EXTINF:1.11111,
gap.mp4
#EXT-X-GAP
#EXTINF:1.11111,
gap.mp4
#EXT-X-PART:DURATION=1.11111,URI=\"part0.mp4\",INDEPENDENT=YES
#EXTINF:1.11111,
seg7.mp4
#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part1.mp4\"
", get_playlist(&muxer, None).await
        );
    }

    #[tokio::test]
    async fn test_multiple_blocking_playlists() {
        let token = CancellationToken::new();
        let server = HlsServer::new(token.clone(), DummyLogger::new());

        let params = TrackParameters {
            width: 64,
            height: 64,
            codec: "test_codec".to_owned(),
            extra_data: Vec::new(),
        };

        let (_, mut writer) = server
            .new_muxer(token, "test".to_owned(), params)
            .await
            .unwrap();
        let muxer = server
            .muxer_by_name("test".to_owned())
            .await
            .unwrap()
            .unwrap();

        writer.test_write(0, Vec::new(), true).await;
        writer.test_write(100_000, Vec::new(), true).await;

        #[rustfmt::skip]
            assert_eq!(
"200 OK
Some({\"content-type\": \"application/x-mpegURL\"})
#EXTM3U
#EXT-X-VERSION:9
#EXT-X-TARGETDURATION:2
#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=2.77778,CAN-SKIP-UNTIL=12
#EXT-X-PART-INF:PART-TARGET=1.111111111
#EXT-X-MEDIA-SEQUENCE:1
#EXT-X-MAP:URI=\"init.mp4\"
#EXT-X-GAP
#EXTINF:1.11111,
gap.mp4
#EXT-X-GAP
#EXTINF:1.11111,
gap.mp4
#EXT-X-GAP
#EXTINF:1.11111,
gap.mp4
#EXT-X-GAP
#EXTINF:1.11111,
gap.mp4
#EXT-X-GAP
#EXTINF:1.11111,
gap.mp4
#EXT-X-GAP
#EXTINF:1.11111,
gap.mp4
#EXT-X-PART:DURATION=1.11111,URI=\"part0.mp4\",INDEPENDENT=YES
#EXTINF:1.11111,
seg7.mp4
#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part1.mp4\"
", get_playlist(&muxer, None).await
            );

        let muxer2 = muxer.clone();
        let handle = tokio::spawn(async move {
            get_playlist(&muxer2, Some((7, 1, false))).await;
        });

        let muxer2 = muxer.clone();
        let handle2 = tokio::spawn(async move {
            get_playlist(&muxer2, Some((7, 1, false))).await;
        });

        while muxer.playlist_state().await.num_playlists_on_hold != 2 {
            tokio::task::yield_now().await;
        }

        writer.test_write(100_000_000, Vec::new(), false).await;

        handle.await.unwrap();
        handle2.await.unwrap();

        assert_eq!(muxer.playlist_state().await.num_playlists_on_hold, 0);
    }

    #[tokio::test]
    async fn test_next_segment() {
        let token = CancellationToken::new();
        let server = HlsServer::new(token.clone(), DummyLogger::new());

        let params = TrackParameters {
            width: 64,
            height: 64,
            codec: "test_codec".to_owned(),
            extra_data: Vec::new(),
        };

        let (muxer, mut writer) = server
            .new_muxer(token.clone(), "test".to_owned(), params.clone())
            .await
            .unwrap();

        assert_eq!(muxer.playlist_state().await.num_segments, 0);

        writer.test_write(0, Vec::new(), true).await;
        writer.test_write(1_000_000, Vec::new(), true).await;
        writer.test_write(2_000_000, Vec::new(), true).await;
        writer.test_write(3_000_000, Vec::new(), true).await;

        // 7, 8, 9
        assert_eq!(muxer.playlist_state().await.num_segments, 3);

        let seg7 = muxer.next_segment(None).await.unwrap();
        assert_eq!(seg7.id(), 7);
        let seg8 = muxer.next_segment(Some(&seg7)).await.unwrap();
        assert_eq!(seg8.id(), 8);
        let seg9 = muxer.next_segment(Some(&seg8)).await.unwrap();
        assert_eq!(seg9.id(), 9);

        // Attempt to use segments from a different muxer.
        let (muxer2, mut writer2) = server
            .new_muxer(token, "test".to_owned(), params)
            .await
            .unwrap();

        let muxer3 = muxer2.clone();
        let pending = tokio::spawn(async move { muxer3.next_segment(Some(&seg9)).await.unwrap() });

        while muxer2.playlist_state().await.num_segments_on_hold != 1 {
            tokio::task::yield_now().await;
        }

        writer2.test_write(0, Vec::new(), true).await;
        writer2.test_write(1_000_000, Vec::new(), true).await;
        writer2.test_write(2_000_000, Vec::new(), true).await;
        writer2.test_write(3_000_000, Vec::new(), true).await;
        assert_eq!(muxer2.playlist_state().await.num_segments, 3);

        assert_eq!(pending.await.unwrap().id(), 7);
        assert_eq!(muxer2.next_segment(Some(&seg8)).await.unwrap().id(), 7);
    }

    async fn get_playlist(muxer: &muxer::HlsMuxer, opts: Option<(u64, u64, bool)>) -> String {
        let query = {
            if let Some((msn, part, is_delta_update)) = opts {
                HlsQuery {
                    msn_and_part: Some((msn, part)),
                    is_delta_update,
                }
            } else {
                HlsQuery {
                    msn_and_part: None,
                    is_delta_update: false,
                }
            }
        };
        muxer.file("stream.m3u8", &query).await.print().await
    }
}
