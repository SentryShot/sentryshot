// SPDX-License-Identifier: GPL-2.0-or-later

mod crawler;
mod disk;

pub use crawler::CrawlerError;
pub use disk::Disk;

use common::recording::{RecordingData, RecordingId, RecordingIdError};
use common::{
    time::{Duration, UnixH264},
    ArcLogger, LogEntry, LogLevel, MonitorId,
};
use crawler::Crawler;
use csv::deserialize_csv_option;
use disk::UsageError;
use fs::dir_fs;
use serde::{Deserialize, Serialize};
use std::{
    collections::HashSet,
    num::NonZeroUsize,
    ops::{Deref, DerefMut},
    path::{Path, PathBuf},
    sync::Arc,
};
use thiserror::Error;
use tokio::fs::{File, OpenOptions};
use tokio_util::sync::CancellationToken;

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
}

#[derive(Debug, Serialize)]
pub struct RecordingActive {
    id: RecordingId,
}

#[derive(Debug, Serialize)]
pub struct RecordingFinalized {
    pub id: RecordingId,
    data: Option<RecordingData>,
}

#[derive(Debug, Serialize)]
pub struct RecordingIncomplete {
    id: RecordingId,
}

pub struct RecDb {
    logger: ArcLogger,
    recordings_dir: PathBuf,
    crawler: Crawler,
    disk: Disk,

    // There should only be one active recording per monitor.
    active_recordings: Arc<std::sync::Mutex<HashSet<RecordingId>>>,
}

#[derive(Debug, Error)]
pub enum NewRecordingError {
    #[error("recording is already active")]
    AlreadyActive,

    #[error("chrono")]
    Chrono,

    #[error("recording already exists")]
    AlreadyExist,

    #[error("create directory for recording: {0}")]
    CreateDir(std::io::Error),

    #[error("parse recording id: {0}")]
    RecordingId(#[from] RecordingIdError),
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
}

impl RecDb {
    #[must_use]
    pub fn new(logger: ArcLogger, recording_dir: PathBuf, disk: Disk) -> Self {
        Self {
            logger,
            recordings_dir: recording_dir.clone(),
            crawler: Crawler::new(dir_fs(recording_dir)),
            disk,
            active_recordings: Arc::new(std::sync::Mutex::new(HashSet::new())),
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
        let start_time_chrono = start_time.as_chrono().ok_or(Chrono)?;
        let ymd = start_time_chrono.format("%Y/%m/%d").to_string();

        let file_dir = self.recordings_dir.join(ymd).join(&*monitor_id);

        let ymd_hms_id = start_time_chrono
            .format(&format!("%Y-%m-%d_%H-%M-%S_{monitor_id}"))
            .to_string();

        let path = file_dir.join(&ymd_hms_id);
        let recording_id = ymd_hms_id.try_into()?;

        let mut path2 = path.clone();
        path2.set_extension("meta");
        if path2.exists() {
            return Err(AlreadyExist);
        }

        tokio::fs::create_dir_all(&file_dir)
            .await
            .map_err(CreateDir)?;

        {
            let mut active_recordings = self.active_recordings.lock().expect("not poisoned");
            if active_recordings.contains(&recording_id) {
                return Err(AlreadyActive);
            }
            // Function must be infallible after id has been added.
            active_recordings.insert(recording_id.clone());
        }

        Ok(RecordingHandle {
            active_recordings: self.active_recordings.clone(),
            id: recording_id,
            path: path.clone(),
            open_files: Arc::new(std::sync::Mutex::new(HashSet::new())),
        })
    }

    pub async fn delete_recording(&self, rec_id: RecordingId) -> Result<(), DeleteRecordingError> {
        use DeleteRecordingError::*;
        if self
            .active_recordings
            .lock()
            .expect("not poisoned")
            .contains(&rec_id)
        {
            return Err(Active);
        }

        let Some(path) = self.recording_file_by_ext(&rec_id, "meta").await else {
            return Err(NotExist);
        };
        let dir = path
            .parent()
            .expect("path should have a parent")
            .to_path_buf();

        tokio::task::spawn_blocking(move || {
            let mut res = Ok(());
            for file in dir.read_dir().map_err(ReadDir)? {
                let file = match file {
                    Ok(v) => v,
                    Err(e) => {
                        res = Err(DirEntry(e));
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
                    if let Err(e) = std::fs::remove_file(path) {
                        res = Err(Delete(e));
                    };
                }
            }
            res
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
            recording_id: "9999-01-01_01-01-01_x".to_owned().try_into().unwrap(),
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

    // Runs `prune()` on an interval until the token is canceled.
    pub async fn prune_loop(&self, token: CancellationToken, interval: std::time::Duration) {
        loop {
            tokio::select! {
                () = token.cancelled() => return,
                () = tokio::time::sleep(interval) => {
                    if let Err(e) = self.prune().await {
                        self.logger.log(LogEntry::new(
                            LogLevel::Error,
                            "app",
                            None,
                            format!("failed to prune storage: {e}"),
                        ));
                    }
                }
            }
        }
    }

    // Checks if disk usage is above 99% and if true deletes all files from the oldest day.
    #[allow(clippy::items_after_statements)]
    pub(crate) async fn prune(&self) -> Result<(), PruneError> {
        use PruneError::*;
        let usage = self.disk.usage(Duration::from_minutes(10)).await?;

        if usage.percent < 99.0 {
            return Ok(());
        }

        const DAY_DEPTH: u8 = 3;

        // Find the oldest day.
        let mut path = self.recordings_dir.clone();

        let mut depth = 1;
        while depth <= DAY_DEPTH {
            let path2 = path.clone();
            let entries = tokio::task::spawn_blocking(move || std::fs::read_dir(path2))
                .await
                .expect("join")
                .map_err(ReadDir)?;

            let mut list = Vec::new();
            for entry in entries {
                list.push(entry.map_err(DirEntry)?);
            }

            if list.is_empty() {
                // Don't delete the recordings directory.
                if depth == 1 {
                    return Ok(());
                }

                // Remove empty directory.
                tokio::fs::remove_dir_all(&path)
                    .await
                    .map_err(RemoveDirAll)?;

                path = self.recordings_dir.clone();
                depth = 1;
                continue;
            }

            list.sort_by_key(std::fs::DirEntry::path);
            let first_file = list[0].file_name();
            path = path.join(first_file);

            depth += 1;
        }

        self.logger.log(LogEntry::new(
            LogLevel::Info,
            "app",
            None,
            format!("pruning storage: deleting {path:?}"),
        ));

        // Delete all files from that day
        tokio::fs::remove_dir_all(&path)
            .await
            .map_err(RemoveDirAll)?;

        Ok(())
    }
}

#[derive(Debug, Error)]
pub(crate) enum PruneError {
    #[error("usage: {0}")]
    Usage(#[from] UsageError),

    #[error("read dir: {0}")]
    ReadDir(std::io::Error),

    #[error("dir entry: {0}")]
    DirEntry(std::io::Error),

    #[error("remove dir all: {0}")]
    RemoveDirAll(std::io::Error),
}

pub struct RecordingHandle {
    active_recordings: Arc<std::sync::Mutex<HashSet<RecordingId>>>,
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
        let options = options.create_new(true).write(true);
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
}

impl Drop for RecordingHandle {
    fn drop(&mut self) {
        assert!(
            self.active_recordings
                .lock()
                .expect("not poisoned")
                .remove(&self.id),
            "recording should be in hashset"
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

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {
    use super::*;
    use crate::disk::StubDiskUsageBytes;
    use bytesize::{ByteSize, GB};
    use common::DummyLogger;
    use pretty_assertions::assert_eq;
    use tempfile::TempDir;
    use test_case::test_case;

    fn new_test_recdb(recordings_dir: &Path) -> RecDb {
        let disk = Disk::new(recordings_dir.to_path_buf(), ByteSize(0));
        RecDb::new(DummyLogger::new(), recordings_dir.to_path_buf(), disk)
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

        assert!(rec_db
            .new_recording(m_id.clone(), UnixH264::new(1))
            .await
            .is_err());

        drop(recording);

        assert!(rec_db
            .new_recording(m_id.clone(), UnixH264::new(1))
            .await
            .is_err());
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
        rec_db
            .delete_recording(rec_id.to_owned().try_into().unwrap())
            .await
            .unwrap();
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

    fn list_directory(path: &Path) -> Vec<String> {
        let mut entries: Vec<String> = std::fs::read_dir(path)
            .unwrap()
            .map(|entry| entry.unwrap().file_name().into_string().unwrap())
            .collect();
        entries.sort();
        entries
    }

    #[test_case(&["recordings/2000/01"], &["recordings"]; "no days"  )]
    #[test_case(&["recordings/2000"],    &["recordings"]; "no months")]
    #[test_case(&["recordings"],         &["recordings"]; "no years" )]
    #[test_case(
        &["recordings/2000/01/01/x/x/x"],
        &["recordings/2000/01"];
        "one day"
    )]
    #[test_case(
        &["recordings/2000/01/01/x/x/x", "recordings/2000/01/02/x/x/x"],
        &["recordings/2000/01/02/x/x/x"];
        "two days"
    )]
    #[test_case(
        &["recordings/2000/01/01/x/x/x", "recordings/2000/02/01/x/x/x"],
        &["recordings/2000/01", "recordings/2000/02/01/x/x/x"];
        "two months"
    )]
    #[test_case(
        &["recordings/2000/01/01/x/x/x", "recordings/2001/01/01/x/x/x"],
        &["recordings/2000/01", "recordings/2001/01/01/x/x/x"];
        "two years"
    )]
    #[test_case(
        &["recordings/2000/01", "recordings/2001/01/01/x/x/x", "recordings/2002/01/01/x/x/x"],
        &["recordings/2001/01", "recordings/2002/01/01/x/x/x"];
        "remove empty dirs"
    )]
    #[tokio::test]
    async fn test_prune(before: &[&str], after: &[&str]) {
        let temp_dir = TempDir::new().unwrap();
        let recordings_dir = temp_dir.path().join("recordings");

        let disk = Disk::with_disk_usage(
            recordings_dir.clone(),
            ByteSize(GB),
            Box::new(StubDiskUsageBytes(1_000_000_000)),
        );
        let recdb = RecDb::new(DummyLogger::new(), recordings_dir, disk);

        write_empty_dirs(temp_dir.path(), before);
        assert_eq!(before, list_empty_dirs(temp_dir.path()));
        recdb.prune().await.unwrap();

        assert_eq!(after, list_empty_dirs(temp_dir.path()));
    }

    fn write_empty_dirs(base: &Path, paths: &[&str]) {
        for path in paths {
            std::fs::create_dir_all(base.join(path)).unwrap();
        }
    }

    fn list_empty_dirs(path: &Path) -> Vec<String> {
        let mut list = Vec::new();

        let mut dirs = vec![path.to_owned()];
        while let Some(dir) = dirs.pop() {
            let entries: Vec<_> = std::fs::read_dir(&dir).unwrap().collect();
            if entries.is_empty() {
                list.push(
                    dir.strip_prefix(path)
                        .unwrap()
                        .to_string_lossy()
                        .to_string(),
                );
                continue;
            }

            for entry in entries {
                let entry = entry.unwrap();
                if entry.metadata().unwrap().is_dir() {
                    dirs.push(dir.join(entry.file_name()));
                }
            }
        }

        list.sort();
        list
    }
}
