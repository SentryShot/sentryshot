#![allow(clippy::unwrap_used)]

use common::{
    DummyLogger, Event,
    time::{Duration, UnixNano},
};
use criterion::{BenchmarkId, Criterion, criterion_group, criterion_main};
use eventdb::{Database, EventQuery};
use std::{num::NonZeroUsize, path::Path};
use tempfile::tempdir;
use tokio::{
    runtime::{Handle, Runtime},
    sync::RwLock,
};
use tokio_util::{sync::CancellationToken, task::TaskTracker};

pub fn eventdb_insert(c: &mut Criterion) {
    let rt = Runtime::new().unwrap();

    let temp_dir = tempdir().unwrap();
    let h = RwLock::new(rt.block_on(Helper::new(rt.handle(), temp_dir.path())));
    c.bench_with_input(BenchmarkId::new("insert", ""), &h, |b, h| {
        b.to_async(&rt).iter(|| async {
            let event = h.write().await.random_event();
            h.read().await.db.write_event(event).await;
        });
    });
}

pub fn eventdb_query(c: &mut Criterion) {
    let rt = Runtime::new().unwrap();

    let temp_dir = tempdir().unwrap();
    let mut h = rt.block_on(Helper::new(rt.handle(), temp_dir.path()));

    let h = rt.block_on(async {
        for _ in 0..100_000 {
            let event = h.random_event();
            h.db.write_event(event).await;
        }
        h
    });

    let mut group = c.benchmark_group("eventdb_query");
    group.bench_with_input(BenchmarkId::new("query_20", ""), &h, |b, h| {
        b.to_async(&rt).iter(|| async {
            let entries =
                h.db.query(EventQuery {
                    start: UnixNano::new(0),
                    end: UnixNano::new(i64::MAX),
                    limit: NonZeroUsize::new(20).unwrap(),
                })
                .await
                .unwrap()
                .unwrap();
            assert_eq!(entries.len(), 20);
        });
    });
    group.sample_size(10);
    group.bench_with_input(BenchmarkId::new("read_all", ""), &h, |b, h| {
        b.to_async(&rt).iter(|| async {
            h.db.query(EventQuery {
                start: UnixNano::new(0),
                end: UnixNano::new(i64::MAX),
                limit: NonZeroUsize::MAX,
            })
            .await
            .unwrap();
        });
    });
}

criterion_group!(benches, eventdb_insert, eventdb_query);
criterion_main!(benches);

struct Helper {
    _token: CancellationToken,
    db: Database,
    count: i64,
}

impl Helper {
    async fn new(rt_handle: &Handle, log_dir: &Path) -> Self {
        let token = CancellationToken::new();
        let _enter = rt_handle.enter();
        let db = Database::new(
            TaskTracker::new().token(),
            DummyLogger::new(),
            log_dir.to_owned(),
            0,
            0,
            true,
        )
        .await
        .unwrap();

        Self {
            _token: token,
            db,
            count: 0,
        }
    }

    fn random_event(&mut self) -> Event {
        self.count += 1;
        Event {
            time: UnixNano::new(self.count),
            duration: Duration::new(4),
            detections: Vec::new(),
            source: Some("test".to_owned().try_into().unwrap()),
        }
    }
}
