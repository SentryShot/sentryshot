mod error;
mod init;
mod muxer;
mod part;
mod playlist;
mod segment;
mod segmenter;
mod types;

pub use crate::error::SegmenterWriteH264Error;
pub use error::ParseParamsError;
pub use muxer::{HlsMuxer, NextSegmentGetter};
pub use segmenter::H264Writer;
pub use types::{track_params_from_video_params, VIDEO_TRACK_ID};

use crate::error::PartHlsQueryError;
use common::{
    time::{DurationH264, H264_MILLISECOND},
    Cancelled, DynLogger, TrackParameters,
};
use serde::Deserialize;
use std::{collections::HashMap, sync::Arc};
use tokio::sync::{mpsc, oneshot};
use tokio_util::sync::CancellationToken;

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
            loop {
                tokio::select! {
                    _ = token.cancelled() => return,

                    req = new_muxer_rx.recv() =>  {
                        let Some(req) = req else {
                            return
                        };

                        if let Some(old_muxer) = muxers.remove(&req.name) {
                            old_muxer.cancel();
                        }
                        let (muxer, writer) = HlsMuxer::new(
                            req.token,
                            logger.clone(),
                            HLS_SEGMENT_COUNT,
                            HLS_SEGMENT_DURATION,
                            HLS_PART_DURATION,
                            HLS_SEGMENT_MAX_SIZE,
                            req.params,
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

const MB: u64 = 1000000;
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
