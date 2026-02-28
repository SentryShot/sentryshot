// SPDX-License-Identifier: GPL-2.0-or-later

// Recordings are stored in the following format.
// The recording ID is the start time of the recording.
// Plugins can attach arbitrary data to recordings.
//
// <Unix 1M seconds>      // Unix timestamp with 12 day precision.
// └── <Unix 10K seconds> // Unix timestamp with 3 hour precision.
//     ├── <UnixNano>_<MonitorID>
//     └── <UnixNano>_<MonitorID>
//         ├── thumb_<Width>x<Height>.jpeg  // Thumbnail.
//         ├── video_<Width>x<Height>.meta  // Video metadata.
//         ├── video_<Width>x<Height>.mdat  // Raw video data.
//         ├── <UnixNano>.end               // Empty file, end timestamp.
//         └── my_plugin_data               // File created by plugin.
//
// The presence of the end timestamp file indicates that the recording was finalized.

mod crawler;
mod migrate;
mod storage;

pub use crawler::CrawlerError;
pub use migrate::migrate;
pub use storage::StorageImpl;

use bytesize::ByteSize;
use common::{
    ArcLogger, ArcStorage, FILE_MODE, LogEntry, LogLevel, MonitorId, StorageUsageError,
    recording::{RecordingId, RecordingIdError},
    time::{Duration, UnixH264, UnixNano},
};
use crawler::Crawler;
use csv::deserialize_csv_option;
use fs::dir_fs;
use serde::Deserialize;
use std::{
    borrow::ToOwned,
    collections::{HashMap, HashSet},
    num::NonZeroUsize,
    ops::{Deref, DerefMut},
    path::{Path, PathBuf},
    sync::Arc,
};
use thiserror::Error;
use tokio::{
    fs::{File, OpenOptions},
    runtime::Handle,
};

// Query of recordings for crawler to find.
#[derive(Clone, Debug, Deserialize)]
pub struct RecDbQuery {
    #[serde(rename = "recording-id")]
    pub recording_id: RecordingId,

    // This is not part of the public API.
    #[serde(default)]
    pub end: Option<UnixNano>,

    pub limit: NonZeroUsize,

    #[serde(rename = "oldest-first")]
    pub oldest_first: bool,

    #[serde(default)]
    #[serde(deserialize_with = "deserialize_csv_option")]
    pub monitors: Vec<String>,

    // If event data should be read from file and included.
    #[serde(rename = "include-data")]
    pub include_data: bool,
}

#[derive(Debug)]
pub enum RecordingResponse {
    // Recording in progress.
    Active(RecordingActive),

    // Recording finished successfully
    Finalized(RecordingFinalized),

    // Something happend before the end file was written.
    Incomplete(RecordingIncomplete),
}

impl RecordingResponse {
    #[must_use]
    pub fn id(&self) -> &RecordingId {
        match self {
            RecordingResponse::Active(v) => &v.id,
            RecordingResponse::Finalized(v) => &v.id,
            RecordingResponse::Incomplete(v) => &v.id,
        }
    }

    #[cfg(test)]
    #[track_caller]
    pub(crate) fn assert_finalized(&self) -> &RecordingFinalized {
        match self {
            RecordingResponse::Active(v) => {
                panic!("expected finalized, got: {v:?}")
            }
            RecordingResponse::Finalized(v) => v,
            RecordingResponse::Incomplete(v) => {
                panic!("expected finalized, got: {v:?}")
            }
        }
    }
}

#[derive(Debug)]
pub struct RecordingActive {
    pub id: RecordingId,
    pub end: UnixNano,
}

#[derive(Debug)]
pub struct RecordingFinalized {
    pub id: RecordingId,
    pub end: UnixNano,
}

#[derive(Debug)]
pub struct RecordingIncomplete {
    pub id: RecordingId,
}

pub struct RecDb {
    logger: RecDbLogger,
    recordings_dir: PathBuf,
    crawler: Crawler,
    storage: ArcStorage,

    // There should only be one active recording per monitor.
    active_recordings: Arc<std::sync::Mutex<HashMap<RecordingId, ActiveRec>>>,
    time_of_oldest_recording: tokio::sync::Mutex<Option<UnixNano>>,
}

#[derive(Clone, Debug)]
struct ActiveRec {
    end_time: UnixH264,
}

#[derive(Debug, Error)]
pub enum NewRecordingError {
    #[error("recording is already active")]
    AlreadyActive,

    #[error("recording already exists")]
    AlreadyExist,

    #[error("create directory for recording: {0}")]
    CreateDir(std::io::Error),

    #[error("parse recording id: {0}")]
    RecId(#[from] RecordingIdError),
}

#[derive(Debug, Error)]
pub enum DeleteRecordingError {
    #[error("deleting active recordings is not implemented")]
    Active,

    #[error("recording doesn't exist: {0}")]
    NotExist(PathBuf),

    #[error("read dir: {0}")]
    ReadDir(std::io::Error),

    #[error("dir entry: {0}")]
    DirEntry(std::io::Error),

    #[error("read metadata: {0}")]
    ReadMetadata(std::io::Error),

    #[error("delete file: {0}")]
    Delete(std::io::Error),

    #[error("directory doesn't have a parent: {0}")]
    DirNoParent(PathBuf),

    #[error("remove empty directory: {0}")]
    RemoveEmptyDir(std::io::Error),
}

#[derive(Debug, Error)]
#[error("create directory: {0}")]
pub struct CreateRecDbError(#[from] std::io::Error);

impl RecDb {
    pub async fn new(
        logger: ArcLogger,
        recordings_dir: PathBuf,
        storage: ArcStorage,
    ) -> Result<Self, CreateRecDbError> {
        common::create_dir_all2(Handle::current(), recordings_dir.clone()).await?;

        let crawler = Crawler::new(dir_fs(recordings_dir.clone()));
        let query = RecDbQuery {
            recording_id: RecordingId::zero(),
            end: None,
            limit: NonZeroUsize::new(1).expect("not zero"),
            oldest_first: true, // Oldest first.
            monitors: Vec::new(),
            include_data: false,
        };
        let time_of_oldest_recording = crawler
            .recordings_by_query(query, HashMap::new())
            .await
            .ok()
            .and_then(|recs| recs.first().map(|rec| rec.id().nanos()));

        Ok(Self {
            logger: RecDbLogger(logger),
            recordings_dir,
            crawler,
            storage,
            active_recordings: Arc::new(std::sync::Mutex::new(HashMap::new())),
            time_of_oldest_recording: tokio::sync::Mutex::new(time_of_oldest_recording),
        })
    }

    // finds the best matching recording and
    // returns limit number of subsequent recorings.
    pub async fn recordings_by_query(
        &self,
        query: &RecDbQuery,
    ) -> Result<Vec<RecordingResponse>, CrawlerError> {
        // Do not hold onto the lock.
        let active_recordings = self.active_recordings.lock().expect("not poisoned").clone();
        self.crawler
            .recordings_by_query(query.clone(), active_recordings)
            .await
    }

    pub async fn recording_path(&self, rec_id: &RecordingId) -> Option<PathBuf> {
        let path = self.recordings_dir.join(rec_id.full_path());
        let path = tokio::fs::canonicalize(path).await.ok()?;

        let is_path_safe = path.starts_with(&self.recordings_dir);
        if !is_path_safe {
            return None;
        };
        Some(path)
    }

    // Returns the full path of file tied to recording id by file name.
    pub async fn recording_file(&self, rec_id: &RecordingId, name: &str) -> Option<PathBuf> {
        let path = self.recordings_dir.join(rec_id.full_path()).join(name);
        let path = tokio::fs::canonicalize(path).await.ok()?;

        let is_path_safe = path.starts_with(&self.recordings_dir);
        if !is_path_safe {
            return None;
        };
        Some(path)
    }

    async fn recording_files(&self, rec_id: &RecordingId) -> Vec<String> {
        let dir = self.recordings_dir.join(rec_id.full_path());
        tokio::task::spawn_blocking(|| {
            std::fs::read_dir(dir)
                .ok()
                .into_iter()
                .flatten()
                .flatten()
                .filter_map(|entry| entry.file_name().to_str().map(ToOwned::to_owned))
                .collect()
        })
        .await
        .expect("join")
    }

    // Returns full path to the thumbnail file for specified recording id.
    pub async fn recording_thumbnail_path(&self, rec_id: &RecordingId) -> Option<PathBuf> {
        for name in self.recording_files(rec_id).await {
            if name.starts_with("thumb_") {
                return self.recording_file(rec_id, &name).await;
            }
        }
        None
    }

    pub async fn recording_video_paths(&self, rec_id: &RecordingId) -> Option<(PathBuf, PathBuf)> {
        let mut meta_path = None;
        let mut mdat_path = None;
        for file in self.recording_files(rec_id).await {
            if !file.starts_with("video") {
                continue;
            }
            if file.ends_with("meta") {
                meta_path = self.recording_file(rec_id, &file).await;
            }
            if file.ends_with("mdat") {
                mdat_path = self.recording_file(rec_id, &file).await;
            }
        }
        if let (Some(meta_path), Some(mdat_path)) = (meta_path, mdat_path) {
            Some((meta_path, mdat_path))
        } else {
            None
        }
    }

    pub async fn new_recording(
        &self,
        monitor_id: MonitorId,
        start_time: UnixH264,
    ) -> Result<RecordingHandle, NewRecordingError> {
        use NewRecordingError::*;

        let rec_id = RecordingId::from_nanos(start_time.into(), monitor_id)?;
        let path = self.recordings_dir.join(rec_id.full_path());
        if path.exists() {
            return Err(AlreadyExist);
        }

        common::create_dir_all2(Handle::current(), path.clone())
            .await
            .map_err(CreateDir)?;

        {
            let mut active_recordings = self.active_recordings.lock().expect("not poisoned");
            if active_recordings.contains_key(&rec_id) {
                return Err(AlreadyActive);
            }
            // Function must be infallible after id has been added.
            active_recordings.insert(
                rec_id.clone(),
                ActiveRec {
                    end_time: start_time,
                },
            );
            drop(active_recordings);
        }

        Ok(RecordingHandle {
            active_recordings: self.active_recordings.clone(),
            id: rec_id,
            path,
            open_files: Arc::new(std::sync::Mutex::new(HashSet::new())),
        })
    }

    fn recording_is_active(&self, rec_id: &RecordingId) -> bool {
        self.active_recordings
            .lock()
            .expect("not poisoned")
            .contains_key(rec_id)
    }

    // Returns total size of delete files and maybe one error.
    pub async fn recording_delete(
        &self,
        rec_id: RecordingId,
    ) -> (ByteSize, Option<DeleteRecordingError>) {
        if self.recording_is_active(&rec_id) {
            return (ByteSize(0), Some(DeleteRecordingError::Active));
        }

        let Some(mut rec_dir) = self.recording_path(&rec_id).await else {
            return (
                ByteSize(0),
                Some(DeleteRecordingError::NotExist(
                    self.recordings_dir.join(rec_id.full_path()),
                )),
            );
        };

        tokio::task::spawn_blocking(move || {
            let mut num_deleted_bytes = ByteSize(0);
            let mut err = None;
            let recording_files = match rec_dir.read_dir() {
                Ok(v) => v,
                Err(e) => return (ByteSize(0), Some(DeleteRecordingError::ReadDir(e))),
            };
            for file in recording_files {
                let path = match file {
                    Ok(v) => v.path(),
                    Err(e) => {
                        err = Some(DeleteRecordingError::DirEntry(e));
                        continue;
                    }
                };
                let file_size = match path.metadata() {
                    Ok(v) => v.len(),
                    Err(e) => {
                        err = Some(DeleteRecordingError::ReadMetadata(e));
                        continue;
                    }
                };
                if let Err(e) = std::fs::remove_file(path) {
                    err = Some(DeleteRecordingError::Delete(e));
                };
                num_deleted_bytes.0 += file_size;
            }
            // Remove empty directories up to the recordings directory.
            for _ in 0..3 {
                let num_files = match rec_dir.read_dir() {
                    Ok(v) => v.count(),
                    Err(e) => {
                        err = Some(DeleteRecordingError::ReadDir(e));
                        break;
                    }
                };

                if num_files == 0 {
                    if let Err(e) = std::fs::remove_dir(&rec_dir) {
                        err = Some(DeleteRecordingError::RemoveEmptyDir(e));
                        break;
                    }
                } else {
                    break;
                }

                if let Some(v) = rec_dir.parent() {
                    rec_dir = v.to_path_buf();
                } else {
                    err = Some(DeleteRecordingError::DirNoParent(rec_dir));
                    break;
                };
            }

            (num_deleted_bytes, err)
        })
        .await
        .expect("join")
    }

    pub async fn test_recording(&self) -> RecordingHandle {
        #[allow(clippy::unwrap_used)]
        self.new_recording("test".to_owned().try_into().unwrap(), UnixH264::new(1))
            .await
            .unwrap()
    }

    #[cfg(test)]
    async fn count_recordings(&self) -> usize {
        #[allow(clippy::unwrap_used)]
        self.recordings_by_query(&RecDbQuery {
            recording_id: RecordingId::max(),
            end: None,
            limit: NonZeroUsize::new(1000).unwrap(),
            oldest_first: false,
            monitors: Vec::new(),
            include_data: false,
        })
        .await
        .unwrap()
        .len()
    }

    // Checks if storage usage is above 99% and deletes recordings until storage usage is below 98%.
    // Returns timestamp of oldest recording.
    pub async fn prune(&self) -> (Option<UnixNano>, Option<PruneError>) {
        let (usage, err) = self.storage.usage(Duration::from_minutes(9)).await;
        if let Some(e) = err {
            self.logger
                .log2(LogLevel::Error, &format!("calculate storage usage: {e}"));
        }
        let usage = match usage {
            Ok(v) => v,
            Err(e) => return (None, Some(PruneError::Usage(e))),
        };
        self.logger.log2(
            LogLevel::Debug,
            &format!("{:.1}% storage usage", usage.percent),
        );
        if usage.percent < 99.0 {
            return (None, None);
        }
        let max_usage = self.storage.max_usage().0;
        let target_storage_usage = (max_usage * 98) / 100; // 98% of max.
        let bytes_to_delete = ByteSize(
            usage
                .used
                .checked_sub(target_storage_usage)
                .expect("storage usage must be greater than target usage"),
        );
        self.logger.log2(
            LogLevel::Info,
            &format!("deleting {bytes_to_delete} of recordings"),
        );

        let query = RecDbQuery {
            recording_id: RecordingId::zero(),
            end: None,
            // Delete max 200 recordings every 10 minutes.
            limit: NonZeroUsize::new(200).expect("not zero"),
            oldest_first: true, // Oldest first.
            monitors: Vec::new(),
            include_data: false,
        };
        let recordings = match self.recordings_by_query(&query).await {
            Ok(v) => v,
            Err(e) => return (None, Some(PruneError::QueryRecordings(e))),
        };

        let mut num_deleted_bytes = ByteSize(0);
        let mut oldest_recording = None;
        let mut err = None;
        for rec in recordings {
            if num_deleted_bytes >= bytes_to_delete {
                break;
            }
            oldest_recording = Some(rec.id().nanos());
            self.logger.log(
                LogLevel::Info,
                rec.id().monitor_id(),
                &format!("deleting recording {}", rec.id()),
            );
            let (deleted, e) = self.recording_delete(rec.id().clone()).await;
            num_deleted_bytes += deleted;
            if let Some(e) = e {
                err = Some(PruneError::DeleteRecording(e));
            }
        }
        if oldest_recording.is_some() {
            *self.time_of_oldest_recording.lock().await = oldest_recording;
        }
        (oldest_recording, err)
    }

    pub async fn time_of_oldest_recording(&self) -> Option<UnixNano> {
        *self.time_of_oldest_recording.lock().await
    }
}

#[derive(Debug, Error)]
pub enum PruneError {
    #[error("usage: {0}")]
    Usage(#[from] StorageUsageError),

    #[error("query recordings: {0}")]
    QueryRecordings(#[from] CrawlerError),

    #[error("delete recording: {0}")]
    DeleteRecording(DeleteRecordingError),
}

#[derive(Debug)]
pub struct RecordingHandle {
    active_recordings: Arc<std::sync::Mutex<HashMap<RecordingId, ActiveRec>>>,
    id: RecordingId,

    path: PathBuf,
    open_files: Arc<std::sync::Mutex<HashSet<String>>>,
}

#[derive(Debug, Error)]
pub enum OpenFileError {
    #[error("a file with this extension is already open")]
    AlreadyOpen,

    #[error("open file: {0} {1}")]
    OpenFile(PathBuf, std::io::Error),
}

impl RecordingHandle {
    #[must_use]
    pub fn id(&self) -> &RecordingId {
        &self.id
    }

    pub async fn new_file(&self, name: &str) -> Result<FileHandle, OpenFileError> {
        let mut options = OpenOptions::new();
        let options = options.create_new(true).mode(FILE_MODE).write(true);
        self.open_file_with_opts(name.to_owned(), options).await
    }

    pub async fn open_file(&self, name: &str) -> Result<FileHandle, OpenFileError> {
        let mut options = OpenOptions::new();
        let options = options.write(true).read(true);
        self.open_file_with_opts(name.to_owned(), options).await
    }

    pub async fn open_file_with_opts(
        &self,
        name: String,
        options: &OpenOptions,
    ) -> Result<FileHandle, OpenFileError> {
        use OpenFileError::*;

        let path = self.path.join(Path::new(&name));
        let file = options
            .open(&path)
            .await
            .map_err(|e| OpenFile(path.clone(), e))?;

        {
            let mut open_files = self.open_files.lock().expect("not poisoned");
            if open_files.contains(&name) {
                return Err(AlreadyOpen);
            }
            // Function must be infallible after file has been added.
            open_files.insert(name.clone());
        }

        Ok(FileHandle {
            open_files: self.open_files.clone(),
            name,
            path,
            file,
        })
    }

    pub fn set_end_time(&self, end_time: UnixH264) {
        self.active_recordings
            .lock()
            .expect("not poisoned")
            .get_mut(&self.id)
            .expect("should exist")
            .end_time = end_time;
    }
}

impl Drop for RecordingHandle {
    fn drop(&mut self) {
        assert!(
            self.active_recordings
                .lock()
                .expect("not poisoned")
                .remove(&self.id)
                .is_some(),
            "recording id should have been in hashmap"
        );
    }
}

pub struct FileHandle {
    open_files: Arc<std::sync::Mutex<HashSet<String>>>,
    name: String,
    path: PathBuf,
    file: File,
}

impl FileHandle {
    pub fn path(&self) -> &Path {
        &self.path
    }
}

impl Drop for FileHandle {
    fn drop(&mut self) {
        assert!(
            self.open_files
                .lock()
                .expect("not poisoned")
                .remove(&self.name),
            "extension should be in hashset"
        );
    }
}

impl Deref for FileHandle {
    type Target = File;

    fn deref(&self) -> &Self::Target {
        &self.file
    }
}

impl DerefMut for FileHandle {
    fn deref_mut(&mut self) -> &mut Self::Target {
        &mut self.file
    }
}

struct RecDbLogger(ArcLogger);

impl RecDbLogger {
    fn log(&self, level: LogLevel, monitor_id: &MonitorId, msg: &str) {
        self.0.log(LogEntry::new(level, "recdb", monitor_id, msg));
    }
    fn log2(&self, level: LogLevel, msg: &str) {
        self.0.log(LogEntry::new2(level, "recdb", msg));
    }
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {
    use crate::{StorageImpl, storage::StubStorageUsageBytes};

    use super::*;
    use bytesize::ByteSize;
    use common::{DummyLogger, DummyStorage};
    use pretty_assertions::assert_eq;
    use tempfile::TempDir;
    use test_case::test_case;

    async fn new_test_recdb(recordings_dir: &Path) -> RecDb {
        RecDb::new(
            DummyLogger::new(),
            recordings_dir.to_path_buf(),
            DummyStorage::new(),
        )
        .await
        .unwrap()
    }

    #[tokio::test]
    async fn test_new_recording() {
        let temp_dir = TempDir::new().unwrap();

        let rec_db = new_test_recdb(&temp_dir.path().join("test")).await;
        let recording = rec_db
            .new_recording("test".to_owned().try_into().unwrap(), UnixH264::new(1))
            .await
            .unwrap();

        recording.new_file("video.meta").await.unwrap();
        recording.new_file("video.mdat").await.unwrap();
        recording.new_file("thumb_64x64.jpeg").await.unwrap();
        recording.new_file("1000.end").await.unwrap();

        assert_eq!(rec_db.count_recordings().await, 1);
    }

    #[tokio::test]
    async fn test_new_recording_already_active() {
        let temp_dir = TempDir::new().unwrap();

        let rec_db = new_test_recdb(temp_dir.path()).await;
        let m_id = MonitorId::try_from("test".to_owned()).unwrap();
        let recording = rec_db.test_recording().await;

        recording.new_file("video.meta").await.unwrap();

        rec_db
            .new_recording(m_id.clone(), UnixH264::new(1))
            .await
            .unwrap_err();

        drop(recording);

        rec_db
            .new_recording(m_id.clone(), UnixH264::new(1))
            .await
            .unwrap_err();
    }

    #[tokio::test]
    async fn test_new_recording_already_open() {
        let temp_dir = TempDir::new().unwrap();

        let rec_db = new_test_recdb(temp_dir.path()).await;
        let recording = rec_db.test_recording().await;

        let file = recording.new_file("1000.end").await.unwrap();
        assert!(recording.new_file("1000.end").await.is_err());
        drop(file);
        recording.open_file("1000.end").await.unwrap();
    }

    #[tokio::test]
    async fn test_delete_recording() {
        let recordings_dir = TempDir::new().unwrap();
        let rec_id = "946692122000000000_m1";
        let rec_dir = recordings_dir.path().join("946").join("94669").join(rec_id);
        let rec_id: RecordingId = rec_id.parse().unwrap();
        let files: Vec<String> = ["thumb_64x64.jpeg", "video.mdat", "video.meta", "x.txt"]
            .into_iter()
            .map(std::borrow::ToOwned::to_owned)
            .collect();
        std::fs::create_dir_all(&rec_dir).unwrap();
        create_files(&rec_dir, &files);

        assert_eq!(files, list_directory(&rec_dir));

        let rec_db = new_test_recdb(recordings_dir.path()).await;
        assert_eq!(rec_db.count_recordings().await, 1);
        let (deleted, err) = rec_db.recording_delete(rec_id).await;
        assert_eq!(ByteSize(0), deleted);
        if let Some(e) = err {
            panic!("{e}");
        };
    }

    fn create_files(dir: &Path, files: &[String]) {
        for file in files {
            std::fs::OpenOptions::new()
                .create_new(true)
                .write(true)
                .open(dir.join(file))
                .unwrap();
        }
    }

    #[track_caller]
    fn list_directory(path: &Path) -> Vec<String> {
        let mut entries: Vec<String> = std::fs::read_dir(path)
            .unwrap()
            .map(|entry| entry.unwrap().file_name().into_string().unwrap())
            .collect();
        entries.sort();
        entries
    }

    #[test_case(&["recordings"], &["recordings"]; "no years" )]
    #[test_case(
        &[
            "recordings/946/94668/946684800000000000_x/video.meta",
        ],
        &[
            "recordings",
        ];
        "one day"
    )]
    #[test_case(
        &[
            "recordings/946/94668/946684800000000000_x/video.meta",
            "recordings/946/94677/946771200000000000_x/video.meta",
        ],
        &[
            "recordings/946/94677/946771200000000000_x/video.meta",
        ];
        "two days"
    )]
    #[test_case(
        &[
            "recordings/946/94668/946684800000000000_x/video.meta",
            "recordings/949/94936/949363200000000000_x/video.meta",
        ],
        &[
            "recordings/949/94936/949363200000000000_x/video.meta",
        ];
        "two months"
    )]
    #[test_case(
        &[
            "recordings/946/94668/946688461000000000_x/video.meta",
            "recordings/978/97830/978307200000000000_x/video.meta",
        ],
        &[
            "recordings/978/97830/978307200000000000_x/video.meta",
        ];
        "two years"
    )]
    #[test_case(
        &[
            "recordings/1009/100984/1009843200000000000_x/video.meta",
            "recordings/900",
            "recordings/978/97830/978307200000000000_x/video.meta",
        ],
        &[
            "recordings/1009/100984/1009843200000000000_x/video.meta",
            "recordings/900",
        ];
        "remove empty dirs"
    )]
    #[tokio::test]
    async fn test_prune(before: &[&str], after: &[&str]) {
        let temp_dir = TempDir::new().unwrap();
        let recordings_dir = temp_dir.path().join("recordings");

        let storage = StorageImpl::with_storage_usage(
            recordings_dir.clone(),
            ByteSize(100),
            Box::new(StubStorageUsageBytes(99)),
        );
        let recdb = RecDb::new(DummyLogger::new(), recordings_dir, storage)
            .await
            .unwrap();

        write_empty_files(temp_dir.path(), before);
        assert_eq!(before, list_empty_files(temp_dir.path()));
        if let Some(err) = recdb.prune().await.1 {
            panic!("{err}");
        };

        assert_eq!(after, list_empty_files(temp_dir.path()));
    }

    fn write_empty_files(base: &Path, paths: &[&str]) {
        for path in paths.iter().map(|p| base.join(p)) {
            if path.extension().is_some() {
                std::fs::create_dir_all(path.parent().unwrap()).unwrap();
                std::fs::write(path, [0]).unwrap();
            } else {
                std::fs::create_dir_all(path).unwrap();
            }
        }
    }

    fn list_empty_files(path: &Path) -> Vec<String> {
        let mut list = Vec::new();

        let relative_path_str =
            |p: &Path| p.strip_prefix(path).unwrap().to_string_lossy().to_string();

        let mut dirs = vec![path.to_owned()];
        while let Some(dir) = dirs.pop() {
            let entries: Vec<_> = std::fs::read_dir(&dir).unwrap().collect();
            if entries.is_empty() {
                list.push(relative_path_str(&dir));
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
