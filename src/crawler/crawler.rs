// SPDX-License-Identifier: GPL-2.0-or-later

#[cfg(test)]
mod test;

use csv::deserialize_csv_option;
use fs::{DynFs, Entry, Fs, FsError, Open};
use recording::{RecordingData, RecordingId};
use serde::{Deserialize, Serialize};
use std::{
    collections::HashMap,
    path::{Path, PathBuf},
};
use thiserror::Error;

// Recordings are stored in the following format.
//
// <Year>
// └── <Month>
//     └── <Day>
//         ├── Monitor1
//         └── Monitor2
//             ├── YYYY-MM-DD_hh-mm-ss_monitor2.jpeg  // Thumbnail.
//             ├── YYYY-MM-DD_hh-mm-ss_monitor2.mp4   // Video.
//             └── YYYY-MM-DD_hh-mm-ss_monitor2.json  // Event data.
//
// Event data is only generated If video was saved successfully.
// The job of these functions are to on-request find and return recording IDs.

#[derive(Debug, Error)]
pub enum CrawlerError {
    #[error("fs: {0}")]
    Fs(#[from] FsError),

    #[error("{0:?}: unexpected directory")]
    UnexpectedDir(PathBuf),

    #[error("could not find sibling")]
    NoSibling,

    #[error("invalid value: {0:?}")]
    InvalidValue(RecordingId),
}

// Query of recordings for crawler to find.
#[derive(Deserialize)]
pub struct CrawlerQuery {
    #[serde(rename = "recording-id")]
    recording_id: RecordingId,

    limit: usize,
    reverse: bool,

    #[serde(default)]
    #[serde(deserialize_with = "deserialize_csv_option")]
    monitors: Vec<String>,

    // If event data should be read from file and included.
    #[serde(rename = "include-data")]
    include_data: bool,
}

type Cache = HashMap<PathBuf, Vec<Dir>>;

// Contains identifier and optionally data.
// `.mp4`, `.jpeg` or `.json` can be appended to the
// path to get the video, thumbnail or data file.
#[derive(Debug, Serialize)]
pub struct CrawlerRecording {
    id: String,
    data: Option<RecordingData>,
}

// Crawls through storage looking for recordings.
pub struct Crawler {
    fs: DynFs,
}

impl Crawler {
    pub fn new(fs: DynFs) -> Self {
        Self { fs }
    }

    // Finds the best matching recording and
    // returns limit number of subsequent recorings.
    pub async fn recordings_by_query(
        &self,
        query: &CrawlerQuery,
    ) -> Result<Vec<CrawlerRecording>, CrawlerError> {
        let cache = &mut Cache::new();

        let mut recordings = Vec::new();
        let mut file = self.find_recording(query, cache)?;
        while recordings.len() < query.limit {
            let Some(mut prev_file) = file.to_owned() else {
                // Last recording is reached.
                return Ok(recordings);
            };

            let first = recordings.is_empty();
            file = if first && prev_file.name != query.recording_id.as_path() {
                self.find_recording(query, cache)?
            } else {
                match prev_file.sibling(query, cache) {
                    Ok(Some(v)) => Some(v),
                    Ok(None) => return Ok(recordings), // Last recording.
                    Err(e) => return Err(e),
                }
            };

            let data = if query.include_data {
                let file_fs = file.as_ref().unwrap().fs.clone();
                tokio::task::spawn_blocking(|| read_data_file(file_fs))
                    .await
                    .unwrap()
            } else {
                None
            };

            recordings.push(CrawlerRecording {
                id: file
                    .as_ref()
                    .unwrap()
                    .path
                    .file_name()
                    .unwrap()
                    .to_str()
                    .unwrap()
                    .to_owned(),
                data,
            });
        }
        Ok(recordings)
    }

    fn find_recording(
        &self,
        query: &CrawlerQuery,
        cache: &mut Cache,
    ) -> Result<Option<Dir>, CrawlerError> {
        if query.recording_id.len() < 10 {
            return Err(CrawlerError::InvalidValue(query.recording_id.to_owned()));
        }

        let root = Dir {
            fs: self.fs.clone(),
            name: PathBuf::from(""),
            path: PathBuf::from(""),
            depth: 0,
            parent: None,
        };

        // Try to find exact file.
        let mut current = root;
        for val in query.recording_id.year_month_day() {
            let mut parent = current.to_owned();
            match current.child_by_exact_name(query, cache, &val)? {
                Some(exact) => current = exact,
                None => {
                    // Exact match could not be found.
                    return match parent.child_by_name(query, cache, &val)? {
                        Some(inexact) => inexact.find_file_deep(query, cache),
                        None => parent.sibling(query, cache),
                    };
                }
            };
        }

        // If exact match found, return sibling of match.
        if let Some(mut file) =
            current.child_by_exact_name(query, cache, query.recording_id.as_path())?
        {
            return file.sibling(query, cache);
        }

        // If inexact file found, return match.
        if let Some(file) = current.child_by_name(query, cache, query.recording_id.as_path())? {
            return Ok(Some(file));
        }

        current.sibling(query, cache)
    }
}

fn read_data_file(fs: Box<dyn Fs>) -> Option<RecordingData> {
    let Ok(Open::File(mut file)) = fs.open(&PathBuf::from(".")) else {
        return None;
    };

    let Ok(raw_data) = file.read() else {
        return None;
    };

    let Ok(data) = serde_json::from_slice::<RecordingData>(&raw_data) else {
        return None;
    };

    Some(data)
}

struct Dir {
    fs: DynFs,
    name: PathBuf,
    path: PathBuf,
    depth: usize,
    parent: Option<Box<Dir>>,
}

impl Clone for Dir {
    fn clone(&self) -> Self {
        Self {
            fs: self.fs.clone(),
            name: self.name.clone(),
            path: self.path.clone(),
            depth: self.depth,
            parent: self.parent.clone(),
        }
    }
}

const MONITOR_DEPTH: usize = 3;
const REC_DEPTH: usize = 5;

impl Dir {
    // children of current directory. Special case if depth == monitorDepth.
    fn children(
        &mut self,
        query: &CrawlerQuery,
        cache: &mut Cache,
    ) -> Result<Vec<Dir>, CrawlerError> {
        if let Some(cached) = cache.get(&self.path) {
            return Ok(cached.to_owned());
        }

        if self.depth == MONITOR_DEPTH {
            let mut children = self.find_all_files(query)?;
            children.sort_by_key(|v| v.name.to_owned());
            cache.insert(self.path.to_owned(), children.to_owned());
            return Ok(children);
        }

        let entries = self.fs.read_dir()?;

        let mut children = Vec::new();

        for entry in entries {
            let dir = match entry {
                Entry::Dir(v) => v,
                Entry::File(_) => continue,
                Entry::Symlink(_) => continue,
            };
            let path = self.path.join(dir.name());
            let file_fs = self.fs.sub(dir.name())?;
            children.push(Dir {
                fs: file_fs,
                name: dir.name().to_path_buf(),
                path,
                parent: Some(Box::new(self.to_owned())),
                depth: self.depth + 1,
            })
        }

        cache.insert(self.path.to_owned(), children.to_owned());

        Ok(children)
    }

    // Finds all json files beloning to
    // selected monitors in decending directories.
    // Only called by `children()`.
    fn find_all_files(&mut self, query: &CrawlerQuery) -> Result<Vec<Dir>, CrawlerError> {
        let monitor_dirs = self.fs.read_dir()?;

        let mut all_files = Vec::new();
        for entry in monitor_dirs {
            let Some(name) = entry.name().to_str() else {
                continue;
            };
            if !monitor_selected(&query.monitors, name) {
                continue;
            }

            let monitor_path = self.path.join(entry.name());
            let montor_fs = self.fs.sub(entry.name())?;

            let files = montor_fs.read_dir()?;

            for file in files {
                let Entry::File(file) = file else {
                    return Err(CrawlerError::UnexpectedDir(monitor_path));
                };

                if file.name().extension().unwrap() != "json" {
                    continue;
                }

                let json_path = monitor_path.join(file.name());

                let mut path = json_path.to_owned();
                path.set_extension("");

                let file_fs = montor_fs.sub(file.name())?;

                all_files.push(Dir {
                    fs: file_fs,
                    name: PathBuf::from(file.name().to_string_lossy().trim_end_matches(".json")),
                    path,
                    parent: Some(Box::new(self.to_owned())),
                    depth: self.depth + 2,
                })
            }
        }
        Ok(all_files)
    }

    // Returns next or previous child.
    fn child_by_name(
        &mut self,
        query: &CrawlerQuery,
        cache: &mut Cache,
        name: &Path,
    ) -> Result<Option<Dir>, CrawlerError> {
        let children = self.children(query, cache)?;
        if query.reverse {
            Ok(children.into_iter().find(|child| child.name > name))
        } else {
            Ok(children.into_iter().rev().find(|child| child.name < name))
        }
    }

    // Returns child of current directory by exact name.
    fn child_by_exact_name(
        &mut self,
        query: &CrawlerQuery,
        cache: &mut Cache,
        name: &Path,
    ) -> Result<Option<Dir>, CrawlerError> {
        Ok(self
            .children(query, cache)?
            .into_iter()
            .find(|v| v.name == name))
    }

    // Returns the newest or oldest file in all decending directories.
    fn find_file_deep(
        &self,
        query: &CrawlerQuery,
        cache: &mut Cache,
    ) -> Result<Option<Dir>, CrawlerError> {
        let mut current = self.to_owned();
        while current.depth < REC_DEPTH {
            let children = current.children(query, cache)?;
            let (Some(first_child), Some(last_child)) = (children.first(), children.last()) else {
                // children.is_empty()
                if self.depth == 0 {
                    return Ok(None);
                }
                let Some(sibling) = current.sibling(query, cache)? else {
                    return Ok(None);
                };
                return sibling.find_file_deep(query, cache);
            };
            if query.reverse {
                current = first_child.to_owned()
            } else {
                current = last_child.to_owned();
            }
        }
        Ok(Some(current))
    }

    // Returns next or previous sibling.
    // Will climb to parent directories.
    fn sibling(
        &mut self,
        query: &CrawlerQuery,
        cache: &mut Cache,
    ) -> Result<Option<Dir>, CrawlerError> {
        if self.depth == 0 {
            return Ok(None);
        }

        let Some(parent) = &mut self.parent else {
            return Ok(None);
        };

        let siblings = parent.children(query, cache)?;
        for (i, sibling) in siblings.iter().enumerate() {
            if sibling.path != self.path {
                continue;
            }
            if query.reverse {
                if let Some(next) = siblings.get(i + 1) {
                    return next.find_file_deep(query, cache);
                }
            } else if i > 0 {
                if let Some(prev) = siblings.get(i - 1) {
                    return prev.find_file_deep(query, cache);
                }
            }
            return parent.sibling(query, cache);
        }

        Err(CrawlerError::NoSibling)
    }
}

fn monitor_selected(monitors: &[String], monitor: &str) -> bool {
    if monitors.is_empty() {
        return true;
    }
    monitors.iter().any(|m| m == monitor)
}
