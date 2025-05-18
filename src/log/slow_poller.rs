// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{LogEntryWithTime, UnixMicro, log_db::entry_matches_query};
use common::{LogLevel, LogSource, MonitorId};
use csv::{deserialize_csv_option, deserialize_csv_option2};
use serde::Deserialize;
use std::collections::VecDeque;
use tokio::sync::{broadcast, mpsc, oneshot};
use tokio_util::sync::CancellationToken;

enum Request {
    SlowPoll(SlowPollRequest),
    #[cfg(test)]
    Debug(oneshot::Sender<DebugResponse>),
}

struct SlowPollRequest {
    query: PollQuery,
    res: oneshot::Sender<Response>,
}

#[cfg(test)]
#[derive(Debug)]
struct DebugResponse {
    buf_count: usize,
    on_hold_count: usize,
}

#[derive(Debug, PartialEq)]
pub enum Response {
    Ok(Vec<LogEntryWithTime>),
    TooManyConncetions,
    Cancelled,
}

#[derive(Clone)]
pub struct SlowPoller(mpsc::Sender<Request>);

impl SlowPoller {
    const ENTRY_MAX_AGE_SEC: u16 = 30;
    const MAX_CONNECTIONS: usize = 16;

    #[must_use]
    #[allow(clippy::manual_let_else)]
    pub fn new(token: CancellationToken, mut feed: broadcast::Receiver<LogEntryWithTime>) -> Self {
        let (tx, mut rx) = mpsc::channel::<Request>(4);

        tokio::spawn(async move {
            // Ordered by time.
            let mut buffer: VecDeque<LogEntryWithTime> = VecDeque::new();
            let mut on_hold: VecDeque<SlowPollRequest> =
                VecDeque::with_capacity(SlowPoller::MAX_CONNECTIONS);
            loop {
                tokio::select! {
                    () = token.cancelled() => return,
                    entry = feed.recv() => {
                        let mut entry = match entry {
                            Ok(v) => v,
                            // Feed was closed.
                            Err(broadcast::error::RecvError::Closed) => return,
                            Err(broadcast::error::RecvError::Lagged(_)) => continue,
                        };

                        // Maintain buffer order.
                        if let Some(back_entry) = buffer.back() {
                            if entry.time <= back_entry.time {
                                entry.time = match back_entry.time.checked_add(UnixMicro::new(1)) {
                                    Some(v) => v,
                                    None => continue,
                                }
                            }
                        }

                        // Prune entries older than max age from buffer.
                        let entry_time_minus_max_age = match entry.time.
                                checked_sub(UnixMicro::from_secs(SlowPoller::ENTRY_MAX_AGE_SEC)) {
                            Some(v) => v,
                            None => continue,
                        };
                        while let Some(e) = buffer.front() {
                            if e.time < entry_time_minus_max_age {
                                buffer.pop_front();
                            } else {
                                break;
                            }
                        }

                        // Send entry to waiting clients.
                        on_hold = on_hold.into_iter().filter_map(|e| {
                            if entry.time > e.query.time && entry_matches_query(&entry, &e.query) {
                                // Client may have timed out.
                                _ = e.res.send(Response::Ok(vec![entry.clone()]));
                                None
                            } else {
                                Some(e)
                            }
                        }).collect();

                        buffer.push_back(entry);
                    },
                    req = rx.recv() => {
                        let req = match req {
                            Some(Request::SlowPoll(req)) => req,
                            #[cfg(test)]
                            #[allow(clippy::unwrap_used)]
                            Some(Request::Debug(res_tx)) => {
                                res_tx.send(DebugResponse{
                                    buf_count: buffer.len(),
                                    on_hold_count: on_hold.len(),
                                }).unwrap();
                                continue
                            },
                            // Poller was dropped.
                            None => return,
                        };

                        let mut entries = Vec::new();
                        for e in &buffer {
                            if e.time <= req.query.time {
                                continue
                            }
                            if !entry_matches_query(e, &req.query) {
                                continue
                            }
                            entries.push(e.clone());
                        }

                        if entries.is_empty() {
                            if on_hold.len() == SlowPoller::MAX_CONNECTIONS {
                                if let Some(oldest_req) = on_hold.pop_front() {
                                    // Client may have timed out.
                                    _ = oldest_req.res.send(Response::TooManyConncetions);
                                }
                            }
                            on_hold.push_back(req);
                        } else {
                            // Client may have timed out.
                            _ = req.res.send(Response::Ok(entries));
                        }
                    },
                }
            }
        });

        Self(tx)
    }

    pub async fn slow_poll(&self, query: PollQuery) -> Response {
        let (tx, rx) = oneshot::channel();
        let req = SlowPollRequest { query, res: tx };
        if self.0.send(Request::SlowPoll(req)).await.is_err() {
            return Response::Cancelled;
        }
        rx.await.unwrap_or(Response::Cancelled)
    }

    #[cfg(test)]
    async fn debug(&self) -> Option<DebugResponse> {
        let (tx, rx) = oneshot::channel();
        self.0.send(Request::Debug(tx)).await.ok()?;
        rx.await.ok()
    }
}

#[derive(Default, Deserialize)]
pub struct PollQuery {
    pub time: UnixMicro,

    #[serde(default)]
    #[serde(deserialize_with = "deserialize_csv_option2")]
    pub levels: Vec<LogLevel>,

    #[serde(default)]
    #[serde(deserialize_with = "deserialize_csv_option")]
    pub sources: Vec<LogSource>,

    #[serde(default)]
    #[serde(deserialize_with = "deserialize_csv_option")]
    pub monitors: Vec<MonitorId>,
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {
    use crate::UnixMicro;

    use super::*;
    use common::{LogLevel, LogMessage, LogSource, MonitorId};
    use pretty_assertions::assert_eq;

    const SECOND: u64 = 1_000_000;
    const MINUTE: u64 = SECOND * 60;

    fn src(s: &'static str) -> LogSource {
        s.try_into().unwrap()
    }
    fn m_id(s: &str) -> MonitorId {
        s.to_owned().try_into().unwrap()
    }
    fn msg(s: &str) -> LogMessage {
        s.to_owned().try_into().unwrap()
    }

    fn new_test_entry(time: u64) -> LogEntryWithTime {
        LogEntryWithTime {
            level: LogLevel::Info,
            source: src("s1"),
            monitor_id: Some(m_id("m1")),
            message: msg("msg1"),
            time: UnixMicro::new(time),
        }
    }
    fn new_test_entry2(time: u64, level: LogLevel) -> LogEntryWithTime {
        LogEntryWithTime {
            level,
            source: src("s1"),
            monitor_id: Some(m_id("m1")),
            message: msg("msg1"),
            time: UnixMicro::new(time),
        }
    }

    #[tokio::test]
    async fn test_simple() {
        let token = CancellationToken::new();
        let (tx, rx) = broadcast::channel(3);
        let poller = SlowPoller::new(token.child_token(), rx);

        let msg1 = new_test_entry(MINUTE + 1);
        let msg2 = new_test_entry(MINUTE + 2);
        let msg3 = new_test_entry(MINUTE + 3);
        tx.send(msg1.clone()).unwrap();
        tx.send(msg2.clone()).unwrap();
        tx.send(msg3.clone()).unwrap();

        while poller.debug().await.unwrap().buf_count != 3 {}

        let want = Response::Ok(vec![msg1, msg2, msg3]);
        let got = poller.slow_poll(PollQuery::default()).await;
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_prune_old() {
        let token = CancellationToken::new();
        let (tx, rx) = broadcast::channel(3);
        let poller = SlowPoller::new(token.child_token(), rx);

        let msg1 = new_test_entry(MINUTE + 1);
        let msg2 = new_test_entry(MINUTE + 2);
        let msg3 = new_test_entry(MINUTE + 2 + u64::from(SlowPoller::ENTRY_MAX_AGE_SEC) * SECOND);
        tx.send(msg1.clone()).unwrap();
        tx.send(msg2.clone()).unwrap();
        tx.send(msg3.clone()).unwrap();

        while poller.debug().await.unwrap().buf_count != 2 {}

        let want = Response::Ok(vec![msg2, msg3]);
        let got = poller.slow_poll(PollQuery::default()).await;
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_order() {
        let token = CancellationToken::new();
        let (tx, rx) = broadcast::channel(3);
        let poller = SlowPoller::new(token.child_token(), rx);

        let msg1 = new_test_entry(MINUTE + 1);
        let msg2 = new_test_entry(MINUTE + 3);
        let msg3 = new_test_entry(MINUTE + 2);
        tx.send(msg1.clone()).unwrap();
        tx.send(msg2.clone()).unwrap();
        tx.send(msg3.clone()).unwrap();

        while poller.debug().await.unwrap().buf_count != 3 {}

        let want = Response::Ok(vec![msg1, msg2, new_test_entry(MINUTE + 4)]);
        let got = poller.slow_poll(PollQuery::default()).await;
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_cancelled() {
        let token = CancellationToken::new();
        let (tx, rx) = broadcast::channel(1);
        let poller = SlowPoller::new(token.child_token(), rx);

        drop(tx);
        while poller.debug().await.is_some() {}

        assert_eq!(
            Response::Cancelled,
            poller.slow_poll(PollQuery::default()).await
        );
    }

    #[tokio::test]
    async fn test_time_zero() {
        let token = CancellationToken::new();
        let (tx, rx) = broadcast::channel(3);
        let poller = SlowPoller::new(token.child_token(), rx);

        let msg1 = new_test_entry(0);
        let msg2 = new_test_entry(MINUTE);
        tx.send(msg1.clone()).unwrap();
        tx.send(msg2.clone()).unwrap();

        while poller.debug().await.unwrap().buf_count != 1 {}

        let want = Response::Ok(vec![msg2]);
        let got = poller.slow_poll(PollQuery::default()).await;
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_query_time() {
        let token = CancellationToken::new();
        let (tx, rx) = broadcast::channel(3);
        let poller = SlowPoller::new(token.child_token(), rx);

        let msg1 = new_test_entry(MINUTE + 1);
        let msg2 = new_test_entry(MINUTE + 2);
        let msg3 = new_test_entry(MINUTE + 3);
        tx.send(msg1.clone()).unwrap();
        tx.send(msg2.clone()).unwrap();
        tx.send(msg3.clone()).unwrap();

        while poller.debug().await.unwrap().buf_count != 3 {}

        let query = PollQuery {
            time: UnixMicro::new(MINUTE + 2),
            ..Default::default()
        };
        let want = Response::Ok(vec![msg3]);
        let got = poller.slow_poll(query).await;
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_filter() {
        let token = CancellationToken::new();
        let (tx, rx) = broadcast::channel(3);
        let poller = SlowPoller::new(token.child_token(), rx);

        let msg1 = new_test_entry2(MINUTE + 1, LogLevel::Error);
        let msg2 = new_test_entry2(MINUTE + 2, LogLevel::Info);
        let msg3 = new_test_entry2(MINUTE + 3, LogLevel::Warning);
        tx.send(msg1.clone()).unwrap();
        tx.send(msg2.clone()).unwrap();
        tx.send(msg3.clone()).unwrap();

        while poller.debug().await.unwrap().buf_count != 3 {}

        let query = PollQuery {
            levels: vec![LogLevel::Info],
            ..Default::default()
        };
        let want = Response::Ok(vec![msg2]);
        let got = poller.slow_poll(query).await;
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_on_hold() {
        let token = CancellationToken::new();
        let (tx, rx) = broadcast::channel(3);
        let poller = SlowPoller::new(token.child_token(), rx);

        let msg1 = new_test_entry(MINUTE + 1);
        let msg2 = new_test_entry(MINUTE + 2);
        let msg3 = new_test_entry(MINUTE + 3);

        tx.send(msg1.clone()).unwrap();
        while poller.debug().await.unwrap().buf_count != 1 {}

        let poller2 = poller.clone();
        let request = tokio::spawn(async move {
            let query = PollQuery {
                time: UnixMicro::new(MINUTE + 2),
                ..Default::default()
            };
            poller2.slow_poll(query).await
        });
        while poller.debug().await.unwrap().on_hold_count != 1 {}

        tx.send(msg2.clone()).unwrap();
        tx.send(msg3.clone()).unwrap();

        let want = Response::Ok(vec![msg3]);
        let got = request.await.unwrap();
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_max_connections() {
        let token = CancellationToken::new();
        let (tx, rx) = broadcast::channel(3);
        let poller = SlowPoller::new(token.child_token(), rx);

        let msg1 = new_test_entry(MINUTE + 1);
        let msg2 = new_test_entry(MINUTE + 2);

        tx.send(msg1.clone()).unwrap();
        while poller.debug().await.unwrap().buf_count != 1 {}

        let poll = || {
            let poller2 = poller.clone();
            tokio::spawn(async move {
                poller2
                    .slow_poll(PollQuery {
                        time: UnixMicro::new(MINUTE + 1),
                        ..Default::default()
                    })
                    .await
            })
        };

        while poller.debug().await.unwrap().on_hold_count != 0 {}
        let request1 = poll();
        while poller.debug().await.unwrap().on_hold_count != 1 {}

        let request2 = poll();
        while poller.debug().await.unwrap().on_hold_count != 2 {}

        for _ in 0..SlowPoller::MAX_CONNECTIONS - 1 {
            poll();
        }

        assert_eq!(Response::TooManyConncetions, request1.await.unwrap());

        tx.send(msg2.clone()).unwrap();
        assert_eq!(Response::Ok(vec![msg2]), request2.await.unwrap());
    }
}
