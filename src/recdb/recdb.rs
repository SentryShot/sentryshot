// SPDX-License-Identifier: GPL-2.0-or-later

mod crawler;
mod disk;

pub use crawler::CrawlerError;
pub use disk::DiskImpl;

use bytesize::ByteSize;
use common::{
    ArcDisk, ArcLogger, DiskUsageError, LogEntry, LogLevel, MonitorId,
    recording::{RecordingData, RecordingId, RecordingIdError},
    time::{Duration, UnixH264},
};
use common::{FILE_MODE, time::UnixNano};
use crawler::Crawler;
use csv::deserialize_csv_option;
use fs::dir_fs;
use jiff::Timestamp;
use serde::{Deserialize, Serialize};
use std::{
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
    pub end: Option<RecordingId>,

    pub limit: NonZeroUsize,
    // True=forwards, False=backwards
    pub reverse: bool,

    #[serde(default)]
    #[serde(deserialize_with = "deserialize_csv_option")]
    pub monitors: Vec<String>,

    // If event data should be read from file and included.
    #[serde(rename = "include-data")]
    pub include_data: bool,
}

// Contains identifier and optionally data.
// `.mp4`, `.jpeg` or `.json` can be appended to the
// path to get the video, thumbnail or data file.
#[derive(Debug, Serialize)]
#[serde(tag = "state")]
pub enum RecordingResponse {
    // Recording in progress.
    #[serde(rename = "active")]
    Active(RecordingActive),

    // Recording finished successfully
    #[serde(rename = "finalized")]
    Finalized(RecordingFinalized),

    // Something happend before the json file was written.
    #[serde(rename = "incomplete")]
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
            RecordingResponse::Finalized(v) => v,
            _ => panic!("expected finalized"),
        }
    }
}

#[derive(Debug, Serialize)]
pub struct RecordingActive {
    id: RecordingId,
    pub data: Option<RecordingData>,
}

#[derive(Debug, Serialize)]
pub struct RecordingFinalized {
    pub id: RecordingId,
    pub data: Option<RecordingData>,
}

#[derive(Debug, Serialize)]
pub struct RecordingIncomplete {
    id: RecordingId,
}

pub struct RecDb {
    logger: RecDbLogger,
    recordings_dir: PathBuf,
    crawler: Crawler,
    disk: ArcDisk,

    // There should only be one active recording per monitor.
    active_recordings: Arc<std::sync::Mutex<HashMap<RecordingId, StartAndEnd>>>,
}

#[derive(Clone, Debug)]
struct StartAndEnd {
    start_time: UnixH264,
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

    #[error("recording doesn't exist")]
    NotExist,

    #[error("read dir: {0}")]
    ReadDir(std::io::Error),

    #[error("dir entry: {0}")]
    DirEntry(std::io::Error),

    #[error("delete file: {0}")]
    Delete(std::io::Error),

    #[error("directory doesn't have a parent: {0}")]
    DirNoParent(PathBuf),

    #[error("remove empty directory: {0}")]
    RemoveEmptyDir(std::io::Error),
}

impl RecDb {
    #[must_use]
    pub fn new(logger: ArcLogger, recording_dir: PathBuf, disk: ArcDisk) -> Self {
        Self {
            logger: RecDbLogger(logger),
            recordings_dir: recording_dir.clone(),
            crawler: Crawler::new(dir_fs(recording_dir)),
            disk,
            active_recordings: Arc::new(std::sync::Mutex::new(HashMap::new())),
        }
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

    // Returns the full path of file tied to recording id by file extension.
    pub async fn recording_file_by_ext(&self, rec_id: &RecordingId, ext: &str) -> Option<PathBuf> {
        let full_relative_path = rec_id.as_full_path();
        let mut path = self.recordings_dir.join(full_relative_path);
        path.set_extension(ext);
        let path = tokio::fs::canonicalize(path).await.ok()?;

        let is_path_safe = path.starts_with(&self.recordings_dir);
        if !is_path_safe {
            return None;
        };
        Some(path)
    }

    // Returns full path to the thumbnail file for specified recording id.
    pub async fn thumbnail_path(&self, rec_id: &RecordingId) -> Option<PathBuf> {
        self.recording_file_by_ext(rec_id, "jpeg").await
    }

    pub async fn new_recording(
        &self,
        monitor_id: MonitorId,
        start_time: UnixH264,
    ) -> Result<RecordingHandle, NewRecordingError> {
        use NewRecordingError::*;
        let start_time2: Timestamp = start_time.into();
        let ymd = start_time2.strftime("%Y/%m/%d").to_string();
        let file_dir = self.recordings_dir.join(ymd).join(&*monitor_id);

        let ymd_hms_id = start_time2
            .strftime(&format!("%Y-%m-%d_%H-%M-%S_{monitor_id}"))
            .to_string();
        let recording_id: RecordingId = ymd_hms_id.try_into()?;
        let path = file_dir.join(recording_id.as_str());

        let mut meta_path = path.clone();
        meta_path.set_extension("meta");
        if meta_path.exists() {
            return Err(AlreadyExist);
        }

        common::create_dir_all2(Handle::current(), file_dir.clone())
            .await
            .map_err(CreateDir)?;

        {
            let mut active_recordings = self.active_recordings.lock().expect("not poisoned");
            if active_recordings.contains_key(&recording_id) {
                return Err(AlreadyActive);
            }
            // Function must be infallible after id has been added.
            active_recordings.insert(
                recording_id.clone(),
                StartAndEnd {
                    start_time,
                    end_time: start_time,
                },
            );
        }

        Ok(RecordingHandle {
            active_recordings: self.active_recordings.clone(),
            id: recording_id,
            path: path.clone(),
            open_files: Arc::new(std::sync::Mutex::new(HashSet::new())),
        })
    }

    // Returns total size of delete files and maybe one error.
    pub async fn delete_recording(
        &self,
        rec_id: RecordingId,
    ) -> (ByteSize, Option<DeleteRecordingError>) {
        if self
            .active_recordings
            .lock()
            .expect("not poisoned")
            .contains_key(&rec_id)
        {
            return (ByteSize(0), Some(DeleteRecordingError::Active));
        }

        let Some(path) = self.recording_file_by_ext(&rec_id, "meta").await else {
            return (ByteSize(0), Some(DeleteRecordingError::NotExist));
        };
        let mut dir = path
            .parent()
            .expect("path should have a parent")
            .to_path_buf();

        tokio::task::spawn_blocking(move || {
            let mut num_deleted_bytes = ByteSize(0);
            let mut err = None;
            let recording_files = match dir.read_dir() {
                Ok(v) => v,
                Err(e) => return (ByteSize(0), Some(DeleteRecordingError::ReadDir(e))),
            };
            for file in recording_files {
                let file = match file {
                    Ok(v) => v,
                    Err(e) => {
                        err = Some(DeleteRecordingError::DirEntry(e));
                        continue;
                    }
                };
                let path = file.path();
                let Some(file_name) = path.file_name() else {
                    continue;
                };
                let Some(file_name) = file_name.to_str() else {
                    continue;
                };
                if file_name.starts_with(rec_id.as_str()) {
                    let file_size = path.metadata().map(|m| m.len()).unwrap_or(0);
                    if let Err(e) = std::fs::remove_file(path) {
                        err = Some(DeleteRecordingError::Delete(e));
                    };
                    num_deleted_bytes.0 += file_size;
                }
            }
            // Remove empty directories up to the recordings directory.
            for _ in 0..4 {
                let num_files = match dir.read_dir() {
                    Ok(v) => v.count(),
                    Err(e) => {
                        err = Some(DeleteRecordingError::ReadDir(e));
                        break;
                    }
                };

                if num_files == 0 {
                    if let Err(e) = std::fs::remove_dir(&dir) {
                        err = Some(DeleteRecordingError::RemoveEmptyDir(e));
                        break;
                    }
                } else {
                    break;
                }

                if let Some(v) = dir.parent() {
                    dir = v.to_path_buf();
                } else {
                    err = Some(DeleteRecordingError::DirNoParent(dir));
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
            reverse: false,
            monitors: Vec::new(),
            include_data: false,
        })
        .await
        .unwrap()
        .len()
    }

    // Checks if disk usage is above 99% and deletes recordings until disk usage is below 98%.
    // Returns timestamp of oldest recording.
    pub async fn prune(&self) -> (Option<UnixNano>, Option<PruneError>) {
        let (usage, err) = self.disk.usage(Duration::from_minutes(9)).await;
        if let Some(e) = err {
            self.logger
                .log2(LogLevel::Error, &format!("calculate disk usage: {e}"));
        }
        let usage = match usage {
            Ok(v) => v,
            Err(e) => return (None, Some(PruneError::Usage(e))),
        };
        self.logger.log2(
            LogLevel::Debug,
            &format!("{:.1}% disk usage", usage.percent),
        );
        if usage.percent < 99.0 {
            return (None, None);
        }
        let max_usage = self.disk.max_usage().0;
        let target_disk_usage = (max_usage * 98) / 100; // 98% of max.
        let bytes_to_delete = ByteSize(
            usage
                .used
                .checked_sub(target_disk_usage)
                .expect("disk usage must be greater than target usage"),
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
            reverse: true, // Oldest first.
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
            oldest_recording = Some(rec.id().nanos_inexact());
            self.logger.log(
                LogLevel::Info,
                rec.id().monitor_id(),
                &format!("deleting recording {}", rec.id()),
            );
            let (deleted, e) = self.delete_recording(rec.id().clone()).await;
            num_deleted_bytes += deleted;
            if let Some(e) = e {
                err = Some(PruneError::DeleteRecording(e));
            }
        }
        (oldest_recording, err)
    }
}

#[derive(Debug, Error)]
pub enum PruneError {
    #[error("usage: {0}")]
    Usage(#[from] DiskUsageError),

    #[error("query recordings: {0}")]
    QueryRecordings(#[from] CrawlerError),

    #[error("delete recording: {0}")]
    DeleteRecording(DeleteRecordingError),
}

pub struct RecordingHandle {
    active_recordings: Arc<std::sync::Mutex<HashMap<RecordingId, StartAndEnd>>>,
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

    pub async fn new_file(&self, ext: &str) -> Result<FileHandle, OpenFileError> {
        let mut options = OpenOptions::new();
        let options = options.create_new(true).mode(FILE_MODE).write(true);
        self.open_file_with_opts(ext, options).await
    }

    pub async fn open_file(&self, ext: &str) -> Result<FileHandle, OpenFileError> {
        let mut options = OpenOptions::new();
        let options = options.write(true).read(true);
        self.open_file_with_opts(ext, options).await
    }

    pub async fn open_file_with_opts(
        &self,
        ext: &str,
        options: &OpenOptions,
    ) -> Result<FileHandle, OpenFileError> {
        use OpenFileError::*;
        let ext = ext.to_lowercase();

        let mut path = self.path.clone();
        path.set_extension(&ext);
        let file = options
            .open(&path)
            .await
            .map_err(|e| OpenFile(path.clone(), e))?;

        {
            let mut open_files = self.open_files.lock().expect("not poisoned");
            if open_files.contains(&ext) {
                return Err(AlreadyOpen);
            }
            // Function must be infallible after file has been added.
            open_files.insert(ext.clone());
        }

        Ok(FileHandle {
            open_files: self.open_files.clone(),
            ext,
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
    ext: String,
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
                .remove(&self.ext),
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
    use crate::disk::StubDiskUsageBytes;

    use super::*;
    use bytesize::ByteSize;
    use common::{DummyDisk, DummyLogger};
    use pretty_assertions::assert_eq;
    use tempfile::TempDir;
    use test_case::test_case;

    fn new_test_recdb(recordings_dir: &Path) -> RecDb {
        RecDb::new(
            DummyLogger::new(),
            recordings_dir.to_path_buf(),
            DummyDisk::new(),
        )
    }

    #[tokio::test]
    async fn test_new_recording() {
        let temp_dir = TempDir::new().unwrap();

        let rec_db = new_test_recdb(&temp_dir.path().join("test"));
        let recording = rec_db
            .new_recording("test".to_owned().try_into().unwrap(), UnixH264::new(1))
            .await
            .unwrap();
        recording.new_file("meta").await.unwrap();
        recording.new_file("mdat").await.unwrap();
        recording.new_file("jpeg").await.unwrap();
        recording.new_file("json").await.unwrap();

        assert_eq!(rec_db.count_recordings().await, 1);
    }

    #[tokio::test]
    async fn test_new_recording_already_active() {
        let temp_dir = TempDir::new().unwrap();

        let rec_db = new_test_recdb(temp_dir.path());
        let m_id = MonitorId::try_from("test".to_owned()).unwrap();
        let recording = rec_db.test_recording().await;

        recording.new_file("meta").await.unwrap();

        assert!(
            rec_db
                .new_recording(m_id.clone(), UnixH264::new(1))
                .await
                .is_err()
        );

        drop(recording);

        assert!(
            rec_db
                .new_recording(m_id.clone(), UnixH264::new(1))
                .await
                .is_err()
        );
    }

    #[tokio::test]
    async fn test_new_recording_already_open() {
        let temp_dir = TempDir::new().unwrap();

        let rec_db = new_test_recdb(temp_dir.path());
        let recording = rec_db.test_recording().await;

        let file = recording.new_file("json").await.unwrap();
        assert!(recording.new_file("json").await.is_err());
        drop(file);
        recording.open_file("json").await.unwrap();
    }

    #[tokio::test]
    async fn test_delete_recording() {
        let recordings_dir = TempDir::new().unwrap();
        let rec_dir = recordings_dir
            .path()
            .join("2000")
            .join("01")
            .join("01")
            .join("m1");
        let rec_id = "2000-01-01_02-02-02_m1";
        let files = vec![
            rec_id.to_owned() + ".jpeg",
            rec_id.to_owned() + ".json",
            rec_id.to_owned() + ".mdat",
            rec_id.to_owned() + ".meta",
            rec_id.to_owned() + ".x",
            "2000-01-01_02-02-02_x1.mp4".to_owned(),
        ];
        std::fs::create_dir_all(&rec_dir).unwrap();
        create_files(&rec_dir, &files);
        assert_eq!(files, list_directory(&rec_dir));

        let rec_db = new_test_recdb(recordings_dir.path());
        assert_eq!(rec_db.count_recordings().await, 1);
        let (deleted, err) = rec_db
            .delete_recording(rec_id.to_owned().try_into().unwrap())
            .await;
        assert_eq!(ByteSize(0), deleted);
        assert!(err.is_none());
        assert_eq!(
            vec!["2000-01-01_02-02-02_x1.mp4".to_owned()],
            list_directory(&rec_dir)
        );
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
        &["recordings/2000/01/01/x/2000-01-01_00-00-00_x.meta"],
        &["recordings"];
        "one day"
    )]
    #[test_case(
        &["recordings/2000/01/01/x/2000-01-01_00-00-00_x.meta", "recordings/2000/01/02/x/2000-01-02_00-00-00_x.meta"],
        &["recordings/2000/01/02/x/2000-01-02_00-00-00_x.meta"];
        "two days"
    )]
    #[test_case(
        &["recordings/2000/01/01/x/2000-01-01_00-00-00_x.meta", "recordings/2000/02/01/x/2000-02-01_00-00-00_x.meta"],
        &["recordings/2000/02/01/x/2000-02-01_00-00-00_x.meta"];
        "two months"
    )]
    #[test_case(
        &["recordings/2000/01/01/x/2000-01-01_01-01-01_x.meta", "recordings/2001/01/01/x/2001-01-01_00-00-00_x.meta"],
        &["recordings/2001/01/01/x/2001-01-01_00-00-00_x.meta"];
        "two years"
    )]
    #[test_case(
        &["recordings/2000/01", "recordings/2001/01/01/x/2001-01-01_00-00-00_x.meta", "recordings/2002/01/01/x/2002-01-01_00-00-00_x.meta"],
        &["recordings/2000/01", "recordings/2002/01/01/x/2002-01-01_00-00-00_x.meta"];
        "remove empty dirs"
    )]
    #[tokio::test]
    async fn test_prune(before: &[&str], after: &[&str]) {
        let temp_dir = TempDir::new().unwrap();
        let recordings_dir = temp_dir.path().join("recordings");

        let disk = DiskImpl::with_disk_usage(
            recordings_dir.clone(),
            ByteSize(100),
            Box::new(StubDiskUsageBytes(99)),
        );
        let recdb = RecDb::new(DummyLogger::new(), recordings_dir, disk);

        write_empty_files(temp_dir.path(), before);
        assert_eq!(before, list_empty_files(temp_dir.path()));
        assert!(recdb.prune().await.1.is_none());

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
