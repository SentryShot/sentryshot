// SPDX-License-Identifier: GPL-2.0-or-later

mod crawler;

pub use crawler::CrawlerError;

use common::{time::UnixH264, MonitorId};
use crawler::Crawler;
use csv::deserialize_csv_option;
use fs::dir_fs;
use recording::{RecordingData, RecordingId, RecordingIdError};
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

// Query of recordings for crawler to find.
#[derive(Deserialize)]
pub struct RecDbQuery {
    #[serde(rename = "recording-id")]
    recording_id: RecordingId,

    limit: NonZeroUsize,
    reverse: bool,

    #[serde(default)]
    #[serde(deserialize_with = "deserialize_csv_option")]
    monitors: Vec<String>,

    // If event data should be read from file and included.
    #[serde(rename = "include-data")]
    include_data: bool,
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

#[derive(Debug, Serialize)]
pub struct RecordingActive {
    id: RecordingId,
}

#[derive(Debug, Serialize)]
pub struct RecordingFinalized {
    id: RecordingId,
    data: Option<RecordingData>,
}

#[derive(Debug, Serialize)]
pub struct RecordingIncomplete {
    id: RecordingId,
}

pub struct RecDb {
    recordings_dir: PathBuf,
    crawler: Crawler,
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
    pub fn new(recording_dir: PathBuf) -> Self {
        Self {
            recordings_dir: recording_dir.clone(),
            crawler: Crawler::new(dir_fs(recording_dir).into()),
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
            .recordings_by_query(query, &active_recordings)
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
        let start_time_chrono = start_time.as_nanos().as_chrono().ok_or(Chrono)?;
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
        self.new_recording("test".to_owned().try_into().unwrap(), UnixH264::from(1))
            .await
            .unwrap()
    }

    #[cfg(test)]
    async fn count_recordings(&self) -> usize {
        #[allow(clippy::unwrap_used)]
        self.recordings_by_query(&RecDbQuery {
            recording_id: "9999-01-01_01-01-01_x".to_owned().try_into().unwrap(),
            limit: NonZeroUsize::new(1000).unwrap(),
            reverse: false,
            monitors: Vec::new(),
            include_data: false,
        })
        .await
        .unwrap()
        .len()
    }
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
    use pretty_assertions::assert_eq;
    use tempfile::TempDir;

    #[tokio::test]
    async fn test_new_recording() {
        let temp_dir = TempDir::new().unwrap();

        let rec_db = RecDb::new(temp_dir.path().join("test").clone());
        let recording = rec_db
            .new_recording("test".to_owned().try_into().unwrap(), UnixH264::from(1))
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

        let rec_db = RecDb::new(temp_dir.path().to_path_buf());
        let m_id = MonitorId::try_from("test".to_owned()).unwrap();
        let recording = rec_db.test_recording().await;

        recording.new_file("meta").await.unwrap();

        assert!(rec_db
            .new_recording(m_id.clone(), UnixH264::from(1))
            .await
            .is_err());

        drop(recording);

        assert!(rec_db
            .new_recording(m_id.clone(), UnixH264::from(1))
            .await
            .is_err());
    }

    #[tokio::test]
    async fn test_new_recording_already_open() {
        let temp_dir = TempDir::new().unwrap();

        let rec_db = RecDb::new(temp_dir.path().to_path_buf());
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

        let rec_db = RecDb::new(recordings_dir.path().to_path_buf());
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
}
