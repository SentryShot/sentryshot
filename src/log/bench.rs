use bytesize::ByteSize;
use common::{LogLevel, LogSource};
use criterion::{criterion_group, criterion_main, BenchmarkId, Criterion};
use log::{
    log_db::{LogDb, LogDbHandle, LogQuery},
    LogEntryWithTime, UnixMicro,
};
use rand::{
    distributions::{Alphanumeric, DistString},
    Rng, SeedableRng,
};
use rand_chacha::ChaCha8Rng;
use std::{num::NonZeroUsize, path::Path, sync::RwLock};
use tempfile::tempdir;
use tokio::{runtime::Runtime, sync::mpsc};

pub fn logdb_insert(c: &mut Criterion) {
    let rt = Runtime::new().unwrap();

    let temp_dir = tempdir().unwrap();
    let h = RwLock::new(Helper::new(temp_dir.path()));
    c.bench_with_input(BenchmarkId::new("insert", ""), &h, |b, h| {
        b.to_async(&rt).iter(|| async {
            let entry = h.write().unwrap().random_entry();
            h.read().unwrap().db.save_log_testing(entry).await
        });
    });
}

pub fn logdb_query(c: &mut Criterion) {
    let rt = Runtime::new().unwrap();

    let temp_dir = tempdir().unwrap();
    let mut h = Helper::new(temp_dir.path());

    let h = rt.block_on(async {
        for _ in 0..100_000 {
            let entry = h.random_entry();
            h.db.save_log_testing(entry).await
        }
        h
    });

    let mut group = c.benchmark_group("logdb_query");
    group.bench_with_input(BenchmarkId::new("query_20", ""), &h, |b, h| {
        b.to_async(&rt).iter(|| async {
            let entries =
                h.db.query(LogQuery {
                    limit: Some(NonZeroUsize::new(20).unwrap()),
                    ..Default::default()
                })
                .await
                .unwrap();
            assert_eq!(entries.len(), 20);
        });
    });
    group.sample_size(10);
    group.bench_with_input(BenchmarkId::new("scan_all", ""), &h, |b, h| {
        b.to_async(&rt).iter(|| async {
            let entries =
                h.db.query(LogQuery {
                    monitors: vec!["x".parse().unwrap()],
                    limit: Some(NonZeroUsize::new(1).unwrap()),
                    ..Default::default()
                })
                .await
                .unwrap();
            assert!(entries.is_empty());
        });
    });
}

criterion_group!(benches, logdb_insert, logdb_query);
criterion_main!(benches);

struct Helper {
    db: LogDbHandle,
    rng: ChaCha8Rng,
    count: u64,
    identical_msg: String,
}

impl Helper {
    fn new(log_dir: &Path) -> Self {
        let (shutdown_complete_tx, _) = mpsc::channel::<()>(1);
        let db = LogDb::new(
            shutdown_complete_tx,
            log_dir.to_owned(),
            ByteSize(0),
            ByteSize(0),
        )
        .unwrap();

        let mut rng = ChaCha8Rng::seed_from_u64(2);
        let identical_msg = Alphanumeric.sample_string(&mut rng, 26);

        Self {
            db,
            rng,
            count: 0,
            identical_msg,
        }
    }

    fn random_entry(&mut self) -> LogEntryWithTime {
        let levels = [
            LogLevel::Debug,
            LogLevel::Info,
            LogLevel::Warning,
            LogLevel::Error,
        ];
        let sources: &[LogSource] = &[
            "app".parse().unwrap(),
            "auth".parse().unwrap(),
            "monitor".parse().unwrap(),
            "recorder".parse().unwrap(),
            "motion".parse().unwrap(),
            "tflite".parse().unwrap(),
        ];

        self.count += 1;
        let monitor_id_len = self.rng.gen_range(6..=8);
        let monitor_id = Alphanumeric.sample_string(&mut self.rng, monitor_id_len);

        // 10% of messages are identical.
        let message = if self.count % 100 <= 10 {
            self.identical_msg.clone()
        } else {
            // Get average length.
            // cat *.msg | awk '{print length}' | awk '{if(min==""){min=max=$1}; if($1>max) {max=$1}; if($1<min) {min=$1}; total+=$1; count+=1} END {print total/count, max, min}'
            let message_len = self.rng.gen_range(18..=42);
            Alphanumeric.sample_string(&mut self.rng, message_len)
        };

        LogEntryWithTime {
            level: levels[self.rng.gen_range(0..levels.len())],
            source: sources[self.rng.gen_range(0..sources.len())].clone(),
            monitor_id: Some(monitor_id.parse().unwrap()),
            message: message.parse().unwrap(),
            time: UnixMicro::from(self.count),
        }
    }
}
