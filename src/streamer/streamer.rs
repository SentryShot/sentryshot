// SPDX-License-Identifier: GPL-2.0-or-later

mod boxes;
mod muxer;

#[cfg(test)]
mod test;

use async_trait::async_trait;
pub use muxer::{
    CreateMuxerError, H264Writer, Muxer, PlayReponse, StartSessionReponse, WriteFrameError,
};

use common::{
    monitor::{DynH264Writer, StreamerImpl},
    time::UnixH264,
    ArcLogger, ArcStreamerMuxer, DynError, H264Data, MonitorId, TrackParameters,
};
use std::{collections::HashMap, sync::Arc};
use tokio::sync::{oneshot, Mutex, MutexGuard};
use tokio_util::sync::CancellationToken;

use crate::muxer::PlayRequest;

#[derive(Clone)]
pub struct Streamer(Arc<Mutex<StreamerState>>);

impl Streamer {
    pub fn new(token: CancellationToken, logger: ArcLogger) -> Self {
        let state = Arc::new(Mutex::new(StreamerState {
            logger,
            muxers: HashMap::new(),
            muxer_id_count: 0,
            cancelled: false,
        }));

        let state2 = state.clone();
        tokio::spawn(async move {
            token.cancelled().await;
            let mut state = state2.lock().await;
            state.cancelled = true;
            for (_, muxer) in state.muxers.drain() {
                muxer.cancel().await;
            }
        });

        Self(state)
    }

    async fn get_state_lock(&self) -> Option<MutexGuard<StreamerState>> {
        let state = self.0.lock().await;
        if state.cancelled {
            return None;
        }
        Some(state)
    }

    pub async fn new_muxer2(
        &self,
        token: CancellationToken,
        monitor_id: MonitorId,
        sub_stream: bool,
        params: TrackParameters,
        start_time: UnixH264,
        first_frame: H264Data,
    ) -> Result<Option<(Arc<Muxer>, H264Writer)>, CreateMuxerError> {
        let Some(mut state) = self.get_state_lock().await else {
            return Ok(None);
        };
        if let Some(old_muxer) = state.muxers.remove(&(monitor_id.clone(), sub_stream)) {
            old_muxer.cancel().await;
        }

        state.muxer_id_count += 1;
        let (muxer, writer) = Muxer::new(
            &token,
            &state.logger.clone(),
            params,
            state.muxer_id_count,
            start_time,
            first_frame,
        )?;
        let muxer = Arc::new(muxer);
        state.muxers.insert((monitor_id, sub_stream), muxer.clone());
        Ok(Some((muxer, writer)))
    }

    // Returns None if cancelled and Some(None) if the muxer doesn't exist.
    pub async fn muxer(
        &self,
        monitor_id: MonitorId,
        sub_stream: bool,
    ) -> Option<Option<Arc<Muxer>>> {
        let Some(state) = self.get_state_lock().await else {
            return None;
        };
        Some(state.muxers.get(&(monitor_id, sub_stream)).cloned())
    }

    // Starts a session and returns the date time of the first frame.
    pub async fn start_session(
        &self,
        monitor_id: MonitorId,
        sub_stream: bool,
        session_id: u32,
    ) -> StartSessionReponse {
        use StartSessionReponse::*;
        let Some(state) = self.get_state_lock().await else {
            return StreamerCancelled;
        };
        let Some(muxer) = state.muxers.get(&(monitor_id, sub_stream)).cloned() else {
            return MuxerNotExist;
        };
        drop(state); // Release lock.
        muxer.start_session(session_id).await
    }

    pub async fn play(
        &self,
        monitor_id: MonitorId,
        sub_stream: bool,
        session_id: u32,
    ) -> PlayReponse {
        use PlayReponse::*;
        let Some(state) = self.get_state_lock().await else {
            return StreamerCancelled;
        };
        let Some(muxer) = state.muxers.get(&(monitor_id, sub_stream)).cloned() else {
            return MuxerNotExist;
        };
        drop(state); // Release lock.

        let (tx, rx) = oneshot::channel();
        let req = PlayRequest {
            session_id,
            res_tx: tx,
        };
        muxer.play(req).await;
        drop(muxer);

        match rx.await {
            Ok(v) => v,
            Err(_) => MuxerCancelled,
        }
    }
}

#[async_trait]
impl StreamerImpl for Streamer {
    async fn new_muxer(
        &self,
        token: CancellationToken,
        monitor_id: MonitorId,
        sub_stream: bool,
        params: TrackParameters,
        start_time: UnixH264,
        first_frame: H264Data,
    ) -> Result<Option<(ArcStreamerMuxer, DynH264Writer)>, DynError> {
        match self
            .new_muxer2(
                token,
                monitor_id,
                sub_stream,
                params,
                start_time,
                first_frame,
            )
            .await
        {
            Ok(Some(v)) => Ok(Some((v.0, Box::new(v.1)))),
            Ok(None) => Ok(None),
            Err(e) => Err(Box::new(e)),
        }
    }
}

struct StreamerState {
    logger: ArcLogger,

    muxers: HashMap<(MonitorId, bool), Arc<Muxer>>,
    muxer_id_count: u16,

    cancelled: bool,
}
