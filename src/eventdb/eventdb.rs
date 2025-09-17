// SPDX-License-Identifier: GPL-2.0-or-later

mod buf_seek_reader;
mod database;
pub mod slow_poller;

pub use database::{Database, EventQuery};

use common::{
    ArcLogger, Event, LogEntry, LogLevel, MonitorId, MsgLogger, monitor::CreateEventDbError,
    time::UnixNano,
};
use database::QueryEventsError;
use database::TimeToIdError;
use database::list_chunks;
use database::time_to_id;
use std::{collections::HashMap, path::PathBuf, sync::Arc};
use thiserror::Error;
use tokio::sync::Mutex;
use tokio_util::sync::CancellationToken;
use tokio_util::task::task_tracker::TaskTrackerToken;

#[derive(Clone)]
pub struct EventDb {
    logger: ArcLogger,
    path: PathBuf,

    // Each monitor has its own database.
    databases: Arc<Mutex<Option<HashMap<MonitorId, Database>>>>,

    task_token: TaskTrackerToken,
}

impl EventDb {
    #[must_use]
    pub fn new(
        token: CancellationToken,
        task_token: TaskTrackerToken,
        logger: ArcLogger,
        path: PathBuf,
    ) -> Self {
        let databases = Arc::new(Mutex::new(Some(HashMap::new())));

        let databases2 = databases.clone();
        tokio::spawn(async move {
            token.cancelled().await;
            *databases2.lock().await = None;
        });

        Self {
            logger,
            path,
            databases,
            task_token,
        }
    }

    #[cfg(test)]
    #[allow(clippy::unwrap_used)]
    async fn write_test(&self, monitor_id: &str, event: Event) {
        self.get_or_create_db(&monitor_id.to_owned().try_into().unwrap())
            .await
            .unwrap()
            .unwrap()
            .write_event(event)
            .await;
    }

    pub async fn write_event_deduplicate_time(
        &self,
        monitor_id: &MonitorId,
        event: Event,
    ) -> Result<(), CreateEventDbError> {
        let Some(db) = self.get_or_create_db(monitor_id).await? else {
            // Cancelled.
            return Ok(());
        };
        db.write_event(event).await;
        Ok(())
    }

    pub async fn query(
        &self,
        monitor_id: MonitorId,
        q: EventQuery,
    ) -> Result<Option<Vec<Event>>, QueryError> {
        let Some(db) = self.get_or_create_db(&monitor_id).await? else {
            // Cancelled.
            return Ok(None);
        };
        Ok(db.query(q).await?)
    }

    async fn get_or_create_db(
        &self,
        monitor_id: &MonitorId,
    ) -> Result<Option<Database>, CreateEventDbError> {
        let mut databases = self.databases.lock().await;
        let Some(databases) = databases.as_mut() else {
            // Cancelled.
            return Ok(None);
        };
        if let Some(db) = databases.get(monitor_id).cloned() {
            return Ok(Some(db));
        }

        let logger = EventDbLogger::new(self.logger.clone(), monitor_id.clone());
        let db = Database::new(
            self.task_token.clone(),
            logger,
            self.path.join(monitor_id.to_string()),
            128,
            64,
            false,
        )
        .await?;

        databases.insert(monitor_id.clone(), db.clone());
        Ok(Some(db))
    }

    pub async fn prune(&self, older_than: UnixNano) -> Result<(), PruneError> {
        use PruneError::*;
        let path = self.path.clone();
        let dirs = tokio::spawn(async move {
            let mut dirs = Vec::new();
            for entry in std::fs::read_dir(path).map_err(ReadDir)? {
                let entry = entry.map_err(DirEntry)?;
                if !entry.path().is_dir() {
                    continue;
                }
                dirs.push(entry.path().clone());
            }
            Ok::<Vec<PathBuf>, PruneError>(dirs)
        })
        .await
        .expect("join")?;

        let older_than_id = time_to_id(older_than)?;
        for dir in dirs {
            let chunks = list_chunks(dir.clone()).await.map_err(ListChunks)?;
            let chunks_to_delete = chunks.into_iter().filter(|c| c < &older_than_id);
            for chunk in chunks_to_delete {
                let data_path = dir.join(format!("{chunk}.data"));
                let payload_path = dir.join(format!("{chunk}.payload"));
                tokio::fs::remove_file(data_path)
                    .await
                    .map_err(DeleteFile)?;
                tokio::fs::remove_file(payload_path)
                    .await
                    .map_err(DeleteFile)?;
            }
        }
        Ok(())
    }
}

#[derive(Debug, Error)]
pub enum QueryError {
    #[error("create database: {0}")]
    CreateDb(#[from] CreateEventDbError),

    #[error(transparent)]
    Query(#[from] QueryEventsError),
}

#[derive(Debug, Error)]
pub enum PruneError {
    #[error("time to id: {0}")]
    TimeToId(#[from] TimeToIdError),

    #[error("read directory: {0}")]
    ReadDir(std::io::Error),

    #[error("list chunks: {0}")]
    ListChunks(std::io::Error),

    #[error("dir entry: {0}")]
    DirEntry(std::io::Error),

    #[error("delete file: {0}")]
    DeleteFile(std::io::Error),
}

struct EventDbLogger {
    logger: ArcLogger,
    monitor_id: MonitorId,
}

impl EventDbLogger {
    fn new(logger: ArcLogger, monitor_id: MonitorId) -> Arc<Self> {
        Arc::new(Self { logger, monitor_id })
    }
}

impl MsgLogger for EventDbLogger {
    fn log(&self, level: LogLevel, msg: &str) {
        self.logger
            .log(LogEntry::new(level, "eventdb", &self.monitor_id, msg));
    }
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {
    use super::*;
    use common::{DummyLogger, time::DAY};
    use pretty_assertions::assert_eq;
    use std::path::Path;
    use tempfile::tempdir;
    use tokio_util::task::TaskTracker;

    fn test_event(time: i64) -> Event {
        Event {
            time: UnixNano::new(time),
            duration: common::time::Duration::new(0),
            detections: Vec::new(),
            source: Some("test".to_owned().try_into().unwrap()),
        }
    }

    #[tokio::test]
    async fn test_prune() {
        let temp_dir = tempdir().unwrap();
        let token = CancellationToken::new();
        let tracker = TaskTracker::new();
        let eventdb = EventDb::new(
            token.clone(),
            tracker.token(),
            DummyLogger::new(),
            temp_dir.path().to_path_buf(),
        );
        tracker.close();

        assert!(list_files(temp_dir.path()).is_empty());
        eventdb.write_test("db1", test_event(1)).await;
        eventdb.write_test("db2", test_event(1)).await;
        eventdb.write_test("db2", test_event(2 * DAY)).await;
        eventdb.write_test("db2", test_event(3 * DAY)).await;
        eventdb.write_test("db3", test_event(2 * DAY)).await;
        eventdb.write_test("db3", test_event(3 * DAY)).await;
        eventdb.write_test("db3", test_event(4 * DAY)).await;
        assert_eq!(
            vec![
                "db1/00000.data",
                "db1/00000.payload",
                //
                "db2/00000.data",
                "db2/00000.payload",
                "db2/00001.data",
                "db2/00001.payload",
                "db2/00002.data",
                "db2/00002.payload",
                //
                "db3/00001.data",
                "db3/00001.payload",
                "db3/00002.data",
                "db3/00002.payload",
                "db3/00003.data",
                "db3/00003.payload",
            ],
            list_files(temp_dir.path())
        );

        eventdb.prune(UnixNano::new(3 * DAY)).await.unwrap();
        assert_eq!(
            vec![
                "db1",
                //
                "db2/00002.data",
                "db2/00002.payload",
                //
                "db3/00002.data",
                "db3/00002.payload",
                "db3/00003.data",
                "db3/00003.payload",
            ],
            list_files(temp_dir.path())
        );

        token.cancel();
        drop(eventdb);
        tracker.wait().await;
    }

    fn list_files(path: &Path) -> Vec<String> {
        let mut list = Vec::new();

        let relative_path_str =
            |p: &Path| p.strip_prefix(path).unwrap().to_string_lossy().to_string();

        let mut dirs = vec![path.to_owned()];
        while let Some(dir) = dirs.pop() {
            let entries: Vec<_> = std::fs::read_dir(&dir).unwrap().collect();
            if entries.is_empty() {
                let s = relative_path_str(&dir);
                if !s.is_empty() {
                    list.push(s);
                }
                continue;
            }

            for entry in entries {
                let entry = entry.unwrap();
                if entry.metadata().unwrap().is_dir() {
                    dirs.push(dir.join(entry.file_name()));
                } else {
                    list.push(relative_path_str(&entry.path()));
                }
            }
        }

        list.sort();
        list
    }
}
