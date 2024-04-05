// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{
    RecDbQuery, RecordingActive, RecordingFinalized, RecordingIncomplete, RecordingResponse,
};
use fs::{ArcFs, Entry, FsError, Open};
use recording::{RecordingData, RecordingId};
use std::{
    collections::{HashMap, HashSet},
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
// Event data is only generated if video was saved successfully.
// The job of these functions are to on-request find and return recording IDs.

#[derive(Debug, Error)]
#[allow(clippy::module_name_repetitions)]
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

struct Cache {
    root: Option<Vec<DirYear>>,
    year: HashMap<PathBuf, Vec<DirMonth>>,
    month: HashMap<PathBuf, Vec<DirDay>>,
    day: HashMap<PathBuf, Vec<DirRec>>,
}

impl Cache {
    fn new() -> Self {
        Self {
            root: None,
            year: HashMap::new(),
            month: HashMap::new(),
            day: HashMap::new(),
        }
    }
}

// Crawls through storage looking for recordings.
pub struct Crawler {
    pub(crate) fs: ArcFs,
}

impl Crawler {
    #[must_use]
    pub(crate) fn new(fs: ArcFs) -> Self {
        Self { fs }
    }

    // finds the best matching recording and
    // returns limit number of subsequent recorings.
    pub(crate) async fn recordings_by_query(
        &self,
        query: &RecDbQuery,
        active_recordings: &HashSet<RecordingId>,
    ) -> Result<Vec<RecordingResponse>, CrawlerError> {
        let cache = &mut Cache::new();

        let mut recordings = Vec::new();
        let mut current: Option<DirNotRoot> = None;
        while recordings.len() < query.limit.get() {
            let rec = if let Some(rec) = current {
                match rec.sibling(query, cache)? {
                    Some(sibling) => match sibling.is_rec() {
                        DirYmdOrRec::Ymd(ymd) => {
                            if let Some(dir) = ymd.find_file_deep(query, cache)? {
                                dir
                            } else {
                                current = Some(ymd.into());
                                continue;
                            }
                        }
                        DirYmdOrRec::Rec(rec) => DirNotRoot::Rec(rec),
                    },
                    None => return Ok(recordings), // Last recording.
                }
            } else {
                match self.find_first_recording(query, cache)? {
                    Some(rec) => rec,
                    None => return Ok(recordings), // No recordings.
                }
            };
            current = Some(rec.clone());

            let DirNotRoot::Rec(rec) = rec else {
                // Continue searching.
                continue;
            };

            let id = rec.id;
            let is_active = active_recordings.contains(&id);
            if is_active {
                recordings.push(RecordingResponse::Active(RecordingActive { id }));
                continue;
            }

            let Some(json_file) = rec.json_file.clone() else {
                recordings.push(RecordingResponse::Incomplete(RecordingIncomplete { id }));
                continue;
            };

            let data = if query.include_data {
                tokio::task::spawn_blocking(move || read_data_file(&json_file))
                    .await
                    .expect("join")
            } else {
                None
            };
            recordings.push(RecordingResponse::Finalized(RecordingFinalized {
                id,
                data,
            }));
        }
        Ok(recordings)
    }

    fn find_first_recording(
        &self,
        query: &RecDbQuery,
        cache: &mut Cache,
    ) -> Result<Option<DirNotRoot>, CrawlerError> {
        if query.recording_id.len() < 10 {
            return Err(CrawlerError::InvalidValue(query.recording_id.clone()));
        }

        // Try to find exact day.
        let [year, month, day] = query.recording_id.year_month_day();
        let root = DirRoot {
            fs: self.fs.clone(),
        };

        let Some(exact_year) = root.child_by_exact_name(cache, &year)? else {
            return match root.child_by_name(query, cache, &year)? {
                Some(inexact) => inexact.find_file_deep(query, cache),
                None => Ok(None),
            };
        };

        let Some(exact_month) = exact_year.child_by_exact_name(cache, &month)? else {
            return match exact_year.child_by_name(query, cache, &month)? {
                Some(inexact) => inexact.find_file_deep(query, cache),
                None => Ok(Some(DirNotRoot::Year(exact_year))),
            };
        };

        let Some(exact_day) = exact_month.child_by_exact_name(cache, &day)? else {
            return match exact_month.child_by_name(query, cache, &day)? {
                Some(inexact) => inexact.find_file_deep(query, cache),
                None => Ok(Some(DirNotRoot::Month(exact_month))),
            };
        };

        // If exact match found, return sibling of match.
        if let Some(exact) =
            exact_day.child_by_exact_name(query, cache, query.recording_id.as_path())?
        {
            return sibling_search(&exact.into(), query, cache);
        }

        // If inexact file found, return match.
        if let Some(inexact) =
            exact_day.child_by_name(query, cache, query.recording_id.as_path())?
        {
            return Ok(Some(inexact.into()));
        }

        sibling_search(&DirNotRoot::Day(exact_day), query, cache)
    }
}

fn sibling_search(
    dir: &DirNotRoot,
    query: &RecDbQuery,
    cache: &mut Cache,
) -> Result<Option<DirNotRoot>, CrawlerError> {
    Ok(match dir.sibling(query, cache)? {
        Some(sibling) => match sibling.is_rec() {
            DirYmdOrRec::Ymd(ymd) => match ymd.find_file_deep(query, cache)? {
                Some(dir) => Some(dir),
                None => Some(ymd.into()),
            },
            DirYmdOrRec::Rec(rec) => Some(rec.into()),
        },
        None => None, // Last recording.
    })
}

fn read_data_file(fs: &ArcFs) -> Option<RecordingData> {
    let Ok(Open::File(mut file)) = fs.open(&PathBuf::from(".")) else {
        return None;
    };
    let raw_data = file.read().ok()?;
    serde_json::from_slice::<RecordingData>(&raw_data).ok()
}

#[derive(Clone)]
enum DirNotRoot {
    Year(DirYear),
    Month(DirMonth),
    Day(DirDay),
    Rec(DirRec),
}

impl DirNotRoot {
    fn is_rec(&self) -> DirYmdOrRec {
        match self.clone() {
            DirNotRoot::Year(year) => DirYmdOrRec::Ymd(DirYearMonthDay::Year(year)),
            DirNotRoot::Month(month) => DirYmdOrRec::Ymd(DirYearMonthDay::Month(month)),
            DirNotRoot::Day(day) => DirYmdOrRec::Ymd(DirYearMonthDay::Day(day)),
            DirNotRoot::Rec(rec) => DirYmdOrRec::Rec(rec),
        }
    }

    // Returns next or previous sibling.
    // Will climb to parent directories.
    fn sibling(
        &self,
        query: &RecDbQuery,
        cache: &mut Cache,
    ) -> Result<Option<DirNotRoot>, CrawlerError> {
        match self {
            DirNotRoot::Year(year) => year.sibling(query, cache).map(|v| v.map(DirNotRoot::Year)),
            DirNotRoot::Month(month) => month.sibling(query, cache).map(|v| v.map(Into::into)),
            DirNotRoot::Day(day) => day.sibling(query, cache).map(|v| v.map(Into::into)),
            DirNotRoot::Rec(rec) => rec.sibling(query, cache),
        }
    }
}

impl From<DirRec> for DirNotRoot {
    fn from(value: DirRec) -> Self {
        DirNotRoot::Rec(value)
    }
}

impl From<DirYearMonth> for DirNotRoot {
    fn from(value: DirYearMonth) -> Self {
        match value {
            DirYearMonth::Year(year) => DirNotRoot::Year(year),
            DirYearMonth::Month(month) => DirNotRoot::Month(month),
        }
    }
}

impl From<DirYearMonthDay> for DirNotRoot {
    fn from(value: DirYearMonthDay) -> Self {
        match value {
            DirYearMonthDay::Year(year) => DirNotRoot::Year(year),
            DirYearMonthDay::Month(month) => DirNotRoot::Month(month),
            DirYearMonthDay::Day(day) => DirNotRoot::Day(day),
        }
    }
}

enum DirYmdOrRec {
    Ymd(DirYearMonthDay),
    Rec(DirRec),
}

#[derive(Clone)]
struct DirRoot {
    fs: ArcFs,
}

impl DirRoot {
    // Children of current directory.
    fn children(&self, cache: &mut Cache) -> Result<Vec<DirYear>, CrawlerError> {
        if let Some(cached) = &cache.root {
            return Ok(cached.clone());
        }

        let entries = self.fs.read_dir()?;

        let mut children = Vec::new();
        for entry in entries {
            let Entry::Dir(d) = entry else { continue };

            let path = d.name().to_path_buf();
            let file_fs = self.fs.sub(d.name())?;
            children.push(DirYear {
                fs: file_fs.into(),
                name: d.name().to_path_buf(),
                path,
                parent: self.to_owned(),
            });
        }

        cache.root = Some(children.clone());
        Ok(children)
    }

    // Returns child of current directory by exact name.
    fn child_by_exact_name(
        &self,
        cache: &mut Cache,
        name: &Path,
    ) -> Result<Option<DirYear>, CrawlerError> {
        Ok(self.children(cache)?.into_iter().find(|v| v.name == name))
    }

    // Returns next or previous child.
    fn child_by_name(
        &self,
        query: &RecDbQuery,
        cache: &mut Cache,
        name: &Path,
    ) -> Result<Option<DirYear>, CrawlerError> {
        let children = self.children(cache)?;
        if query.reverse {
            Ok(children.into_iter().find(|child| child.name > name))
        } else {
            Ok(children.into_iter().rev().find(|child| child.name < name))
        }
    }
}

#[derive(Clone)]
struct DirYear {
    fs: ArcFs,
    name: PathBuf,
    path: PathBuf,
    parent: DirRoot,
}

impl DirYear {
    // Children of current directory.
    fn children(&self, cache: &mut Cache) -> Result<Vec<DirMonth>, CrawlerError> {
        if let Some(cached) = cache.year.get(&self.path) {
            return Ok(cached.clone());
        }

        let entries = self.fs.read_dir()?;

        let mut children = Vec::new();
        for entry in entries {
            let Entry::Dir(d) = entry else { continue };

            let path = self.path.join(d.name());
            let file_fs = self.fs.sub(d.name())?;
            children.push(DirMonth {
                fs: file_fs.into(),
                name: d.name().to_path_buf(),
                path,
                parent: self.to_owned(),
            });
        }

        cache.year.insert(self.path.clone(), children.clone());
        Ok(children)
    }

    // Returns next or previous sibling. Will climb to parent directories.
    fn sibling(
        &self,
        query: &RecDbQuery,
        cache: &mut Cache,
    ) -> Result<Option<DirYear>, CrawlerError> {
        let siblings = self.parent.children(cache)?;
        let self_index = siblings
            .iter()
            .position(|s| s.path == self.path)
            .ok_or(CrawlerError::NoSibling)?;
        if query.reverse {
            if let Some(next) = siblings.get(self_index + 1) {
                return Ok(Some(next.clone()));
            }
        } else if self_index > 0 {
            if let Some(prev) = siblings.get(self_index - 1) {
                return Ok(Some(prev.clone()));
            }
        }
        Ok(None)
    }

    // Returns next or previous child.
    fn child_by_name(
        &self,
        query: &RecDbQuery,
        cache: &mut Cache,
        name: &Path,
    ) -> Result<Option<DirMonth>, CrawlerError> {
        let children = self.children(cache)?;
        if query.reverse {
            Ok(children.into_iter().find(|child| child.name > name))
        } else {
            Ok(children.into_iter().rev().find(|child| child.name < name))
        }
    }

    // Returns child of current directory by exact name.
    fn child_by_exact_name(
        &self,
        cache: &mut Cache,
        name: &Path,
    ) -> Result<Option<DirMonth>, CrawlerError> {
        Ok(self.children(cache)?.into_iter().find(|v| v.name == name))
    }

    // Returns the newest or oldest file in all decending directories.
    fn find_file_deep(
        &self,
        query: &RecDbQuery,
        cache: &mut Cache,
    ) -> Result<Option<DirNotRoot>, CrawlerError> {
        let children = self.children(cache)?;

        let (Some(first_child), Some(last_child)) = (children.first(), children.last()) else {
            let Some(sibling) = self.sibling(query, cache)? else {
                return Ok(None);
            };
            return sibling.find_file_deep(query, cache);
        };
        if query.reverse {
            first_child.find_file_deep(query, cache)
        } else {
            last_child.find_file_deep(query, cache)
        }
    }
}

#[derive(Clone)]
struct DirMonth {
    fs: ArcFs,
    name: PathBuf,
    path: PathBuf,
    parent: DirYear,
}

impl DirMonth {
    // Children of current directory.
    fn children(&self, cache: &mut Cache) -> Result<Vec<DirDay>, CrawlerError> {
        if let Some(cached) = cache.month.get(&self.path) {
            return Ok(cached.clone());
        }

        let entries = self.fs.read_dir()?;

        let mut children = Vec::new();
        for entry in entries {
            let Entry::Dir(d) = entry else { continue };

            let path = self.path.join(d.name());
            let file_fs = self.fs.sub(d.name())?;
            children.push(DirDay {
                fs: file_fs.into(),
                name: d.name().to_path_buf(),
                path,
                parent: self.to_owned(),
            });
        }

        cache.month.insert(self.path.clone(), children.clone());
        Ok(children)
    }

    // Returns next or previous sibling. Will climb to parent directories.
    fn sibling(
        &self,
        query: &RecDbQuery,
        cache: &mut Cache,
    ) -> Result<Option<DirYearMonth>, CrawlerError> {
        let siblings = self.parent.children(cache)?;
        let self_index = siblings
            .iter()
            .position(|s| s.path == self.path)
            .ok_or(CrawlerError::NoSibling)?;
        if query.reverse {
            if let Some(next) = siblings.get(self_index + 1) {
                return Ok(Some(DirYearMonth::Month(next.clone())));
            }
        } else if self_index > 0 {
            if let Some(prev) = siblings.get(self_index - 1) {
                return Ok(Some(DirYearMonth::Month(prev.clone())));
            }
        }
        self.parent
            .sibling(query, cache)
            .map(|v| v.map(DirYearMonth::Year))
    }

    // Returns next or previous child.
    fn child_by_name(
        &self,
        query: &RecDbQuery,
        cache: &mut Cache,
        name: &Path,
    ) -> Result<Option<DirDay>, CrawlerError> {
        let children = self.children(cache)?;
        if query.reverse {
            Ok(children.into_iter().find(|child| child.name > name))
        } else {
            Ok(children.into_iter().rev().find(|child| child.name < name))
        }
    }

    // Returns child of current directory by exact name.
    fn child_by_exact_name(
        &self,
        cache: &mut Cache,
        name: &Path,
    ) -> Result<Option<DirDay>, CrawlerError> {
        Ok(self.children(cache)?.into_iter().find(|v| v.name == name))
    }

    // Returns the newest or oldest file in all decending directories.
    fn find_file_deep(
        &self,
        query: &RecDbQuery,
        cache: &mut Cache,
    ) -> Result<Option<DirNotRoot>, CrawlerError> {
        let children = self.children(cache)?;

        let (Some(first_child), Some(last_child)) = (children.first(), children.last()) else {
            let Some(sibling) = self.sibling(query, cache)? else {
                return Ok(None);
            };
            return match sibling {
                DirYearMonth::Year(sibling) => sibling.find_file_deep(query, cache),
                DirYearMonth::Month(sibling) => sibling.find_file_deep(query, cache),
            };
        };
        if query.reverse {
            first_child.find_file_deep(query, cache)
        } else {
            last_child.find_file_deep(query, cache)
        }
    }
}

#[derive(Clone)]
struct DirDay {
    fs: ArcFs,
    name: PathBuf,
    path: PathBuf,
    parent: DirMonth,
}

impl DirDay {
    // Returns recordings from this day.
    fn children(&self, query: &RecDbQuery, cache: &mut Cache) -> Result<Vec<DirRec>, CrawlerError> {
        if let Some(cached) = cache.day.get(&self.path) {
            return Ok(cached.clone());
        }

        let mut children = self.find_all_files(query)?;
        children.sort_by_key(|v| v.name.clone());

        cache.day.insert(self.path.clone(), children.clone());
        Ok(children)
    }

    // Returns next or previous sibling. Will climb to parent directories.
    fn sibling(
        &self,
        query: &RecDbQuery,
        cache: &mut Cache,
    ) -> Result<Option<DirYearMonthDay>, CrawlerError> {
        let siblings = self.parent.children(cache)?;
        let self_index = siblings
            .iter()
            .position(|s| s.path == self.path)
            .ok_or(CrawlerError::NoSibling)?;
        if query.reverse {
            if let Some(next) = siblings.get(self_index + 1) {
                return Ok(Some(DirYearMonthDay::Day(next.clone())));
            }
        } else if self_index > 0 {
            if let Some(prev) = siblings.get(self_index - 1) {
                return Ok(Some(DirYearMonthDay::Day(prev.clone())));
            }
        }
        self.parent.sibling(query, cache).map(|v| v.map(Into::into))
    }

    // Returns next or previous child.
    fn child_by_name(
        &self,
        query: &RecDbQuery,
        cache: &mut Cache,
        name: &Path,
    ) -> Result<Option<DirRec>, CrawlerError> {
        let children = self.children(query, cache)?;
        if query.reverse {
            Ok(children.into_iter().find(|child| child.name > name))
        } else {
            Ok(children.into_iter().rev().find(|child| child.name < name))
        }
    }

    // Returns child of current directory by exact name.
    fn child_by_exact_name(
        &self,
        query: &RecDbQuery,
        cache: &mut Cache,
        name: &Path,
    ) -> Result<Option<DirRec>, CrawlerError> {
        Ok(self
            .children(query, cache)?
            .into_iter()
            .find(|v| v.name == name))
    }

    // Returns the newest or oldest file in all decending directories.
    fn find_file_deep(
        &self,
        query: &RecDbQuery,
        cache: &mut Cache,
    ) -> Result<Option<DirNotRoot>, CrawlerError> {
        let children = self.children(query, cache)?;
        let (Some(first_child), Some(last_child)) = (children.first(), children.last()) else {
            let Some(sibling) = self.sibling(query, cache)? else {
                return Ok(None);
            };
            return match sibling {
                DirYearMonthDay::Year(year) => year.find_file_deep(query, cache),
                DirYearMonthDay::Month(month) => month.find_file_deep(query, cache),
                DirYearMonthDay::Day(day) => day.find_file_deep(query, cache),
            };
        };
        if query.reverse {
            Ok(Some(first_child.to_owned().into()))
        } else {
            Ok(Some(last_child.to_owned().into()))
        }
    }

    // Finds all recordings belong to
    // selected monitors in decending directories.
    // Only called by `children()`.
    fn find_all_files(&self, query: &RecDbQuery) -> Result<Vec<DirRec>, CrawlerError> {
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

            let mut meta_files = Vec::new();
            let mut json_files = HashMap::new();

            let files = montor_fs.read_dir()?;
            for file in &files {
                let Entry::File(file) = file else {
                    return Err(CrawlerError::UnexpectedDir(monitor_path));
                };
                let Some(name) = file.name().to_str() else {
                    continue;
                };
                let Some(ext) = file.name().extension() else {
                    continue;
                };
                if ext == "meta" {
                    let name = name.trim_end_matches(".meta");
                    meta_files.push((name, file.name()));
                } else if ext == "json" {
                    let name = name.trim_end_matches(".json");
                    let file_fs = montor_fs.sub(file.name())?;
                    json_files.insert(name, file_fs.into());
                }
            }

            for (name, file_name) in meta_files {
                let mut path = monitor_path.join(file_name);
                path.set_extension("");

                let Ok(id) = RecordingId::try_from(name.to_owned()) else {
                    continue;
                };
                all_files.push(DirRec {
                    id,
                    name: PathBuf::from(name),
                    path,
                    parent: self.to_owned(),
                    json_file: json_files.remove(name),
                });
            }
        }
        Ok(all_files)
    }
}

#[derive(Clone)]
struct DirRec {
    id: RecordingId,
    name: PathBuf,
    path: PathBuf,
    parent: DirDay,
    json_file: Option<ArcFs>,
}

impl DirRec {
    // Returns next or previous sibling. Will climb to parent directories.
    fn sibling(
        &self,
        query: &RecDbQuery,
        cache: &mut Cache,
    ) -> Result<Option<DirNotRoot>, CrawlerError> {
        let siblings = self.parent.children(query, cache)?;
        let self_index = siblings
            .iter()
            .position(|s| s.path == self.path)
            .ok_or(CrawlerError::NoSibling)?;
        if query.reverse {
            if let Some(next) = siblings.get(self_index + 1) {
                return Ok(Some(next.clone().into()));
            }
        } else if self_index > 0 {
            if let Some(prev) = siblings.get(self_index - 1) {
                return Ok(Some(prev.clone().into()));
            }
        }
        self.parent.sibling(query, cache).map(|v| v.map(Into::into))
    }
}

enum DirYearMonth {
    Year(DirYear),
    Month(DirMonth),
}

#[derive(Clone)]
enum DirYearMonthDay {
    Year(DirYear),
    Month(DirMonth),
    Day(DirDay),
}

impl DirYearMonthDay {
    // Returns the newest or oldest file in all decending directories.
    fn find_file_deep(
        &self,
        query: &RecDbQuery,
        cache: &mut Cache,
    ) -> Result<Option<DirNotRoot>, CrawlerError> {
        match self {
            DirYearMonthDay::Year(year) => year.find_file_deep(query, cache),
            DirYearMonthDay::Month(month) => month.find_file_deep(query, cache),
            DirYearMonthDay::Day(day) => day.find_file_deep(query, cache),
        }
    }
}

impl From<DirYearMonth> for DirYearMonthDay {
    fn from(value: DirYearMonth) -> Self {
        match value {
            DirYearMonth::Year(year) => DirYearMonthDay::Year(year),
            DirYearMonth::Month(month) => DirYearMonthDay::Month(month),
        }
    }
}

fn monitor_selected(monitors: &[String], monitor: &str) -> bool {
    if monitors.is_empty() {
        return true;
    }
    monitors.iter().any(|m| m == monitor)
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {
    use std::{num::NonZeroUsize, sync::Arc};

    use crate::RecDbQuery;

    use super::*;
    use fs::{MapEntry, MapFs};
    use test_case::test_case;

    fn map_fs_item(path: &str) -> [(PathBuf, MapEntry); 2] {
        map_fs_item_with_data(path, Vec::new())
    }

    fn map_fs_item_with_data(path: &str, data: Vec<u8>) -> [(PathBuf, MapEntry); 2] {
        let mut path1 = PathBuf::from(path);
        let mut path2 = path1.clone();
        path1.set_extension("meta");
        path2.set_extension("json");
        [
            (
                path1,
                MapEntry {
                    is_file: true,
                    ..Default::default()
                },
            ),
            (
                path2,
                MapEntry {
                    data,
                    is_file: true,
                    ..Default::default()
                },
            ),
        ]
    }

    fn crawler_test_fs() -> Arc<MapFs> {
        Arc::new(MapFs(
            [
                map_fs_item("2000/01/01/m1/2000-01-01_01-01-11_m1"),
                map_fs_item("2000/01/01/m1/2000-01-01_01-01-22_m1"),
                map_fs_item("2000/01/02/m1/2000-01-02_01-01-11_m1"),
                map_fs_item("2000/02/01/m1/2000-02-01_01-01-11_m1"),
                map_fs_item("2001/02/01/m1/2001-02-01_01-01-11_m1"),
                map_fs_item("2002/01/01/m1/2002-01-01_01-01-11_m1"),
                map_fs_item("2003/01/01/m1/2003-01-01_01-01-11_m1"),
                map_fs_item("2003/01/01/m2/2003-01-01_01-01-11_m2"),
                map_fs_item("2004/01/01/m1/2004-01-01_01-01-11_m1"),
                map_fs_item("2004/01/01/m1/2004-01-01_01-01-22_m1"),
                map_fs_item_with_data(
                    "2099/01/01/m1/2099-01-01_01-01-11_m1",
                    CRAWLER_TEST_DATA.as_bytes().to_owned(),
                ),
            ]
            .into_iter()
            .flatten()
            .collect(),
        ))
    }

    fn r_id(s: &str) -> RecordingId {
        s.to_owned().try_into().unwrap()
    }

    const CRAWLER_TEST_DATA: &str = "
    {
        \"start\": 4073680922000000000,
        \"end\": 4073680924000000000,
        \"events\": [
            {
                \"time\": 4073680922000000000,
                \"detections\": [
                    {
                        \"label\": \"a\",
                        \"score\": 1,
                        \"region\": {
                            \"rect\": [2, 3, 4, 5]
                        }
                    }
                ],
                \"duration\": 6
            }
        ]
    }";

    #[test_case("0000-01-01_01-01-01_m1", "";                       "no files")]
    #[test_case("1999-01-01_01-01-01_m1", "";                       "EOF")]
    #[test_case("9999-01-01_01-01-01_m1", "2099-01-01_01-01-11_m1"; "latest")]
    #[test_case("2000-01-01_01-01-22_m1", "2000-01-01_01-01-11_m1"; "prev")]
    #[test_case("2000-01-02_01-01-11_m1", "2000-01-01_01-01-22_m1"; "prev day")]
    #[test_case("2000-02-01_01-01-11_m1", "2000-01-02_01-01-11_m1"; "prev month")]
    #[test_case("2001-01-01_01-01-11_m1", "2000-02-01_01-01-11_m1"; "prev year")]
    #[test_case("2002-12-01_01-01-01_m1", "2002-01-01_01-01-11_m1"; "empty prev day")]
    #[test_case("2004-01-01_01-01-22_m1", "2004-01-01_01-01-11_m1"; "same day")]
    #[tokio::test]
    async fn test_recording_by_query(input: &str, want: &str) {
        let query = RecDbQuery {
            recording_id: r_id(input),
            limit: NonZeroUsize::new(1).unwrap(),
            reverse: false,
            monitors: Vec::new(),
            include_data: false,
        };
        let rec = match Crawler::new(crawler_test_fs())
            .recordings_by_query(&query, &HashSet::new())
            .await
        {
            Ok(v) => v,
            Err(e) => {
                println!("{e}");
                panic!("{e}");
            }
        };

        if want.is_empty() {
            assert!(rec.is_empty());
        } else {
            let RecordingResponse::Finalized(rec) = &rec[0] else {
                panic!("expected active")
            };
            let got = rec.id.as_str();
            assert_eq!(want, got);
        }
    }

    #[test_case("1111-01-01_01-01-01_m1", "2000-01-01_01-01-11_m1"; "latest")]
    #[test_case("2000-01-01_01-01-11_m1", "2000-01-01_01-01-22_m1"; "next")]
    #[test_case("2000-01-01_01-01-22_m1", "2000-01-02_01-01-11_m1"; "next day")]
    #[test_case("2000-01-02_01-01-11_m1", "2000-02-01_01-01-11_m1"; "next month")]
    #[test_case("2000-01-02_01-01-12_m1", "2000-02-01_01-01-11_m1"; "next month2")]
    #[test_case("2000-02-01_01-01-11_m1", "2001-02-01_01-01-11_m1"; "next year")]
    #[test_case("2001-12-01_01-01-01_m1", "2002-01-01_01-01-11_m1"; "empty next day")]
    #[tokio::test]
    async fn test_recording_by_query_reverse(input: &str, want: &str) {
        let query = RecDbQuery {
            recording_id: r_id(input),
            limit: NonZeroUsize::new(1).unwrap(),
            reverse: true,
            monitors: Vec::new(),
            include_data: false,
        };
        let rec = match Crawler::new(crawler_test_fs())
            .recordings_by_query(&query, &HashSet::new())
            .await
        {
            Ok(v) => v,
            Err(e) => {
                println!("{e}");
                panic!("{e}");
            }
        };
        let RecordingResponse::Finalized(rec) = &rec[0] else {
            panic!("expected active")
        };

        let got = rec.id.as_str();
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_recording_by_query_multiple() {
        let c = Crawler::new(crawler_test_fs());
        let query = RecDbQuery {
            recording_id: r_id("9999-01-01_01-01-01_x"),
            limit: NonZeroUsize::new(5).unwrap(),
            reverse: false,
            monitors: Vec::new(),
            include_data: false,
        };
        let recordings = c
            .recordings_by_query(&query, &HashSet::new())
            .await
            .unwrap();

        let mut ids = Vec::new();
        for rec in recordings {
            let RecordingResponse::Finalized(rec) = rec else {
                panic!("expected active")
            };
            ids.push(rec.id.as_str().to_owned());
        }

        let want = vec![
            "2099-01-01_01-01-11_m1",
            "2004-01-01_01-01-22_m1",
            "2004-01-01_01-01-11_m1",
            "2003-01-01_01-01-11_m2",
            "2003-01-01_01-01-11_m1",
        ];
        assert_eq!(want, ids);
    }

    #[tokio::test]
    async fn test_recording_by_query_monitors() {
        let c = Crawler::new(crawler_test_fs());
        let query = RecDbQuery {
            recording_id: r_id("2003-02-01_01-01-11_m1"),
            limit: NonZeroUsize::new(1).unwrap(),
            reverse: false,
            monitors: vec!["m1".to_owned()],
            include_data: false,
        };
        let rec = c
            .recordings_by_query(&query, &HashSet::new())
            .await
            .unwrap();
        assert_eq!(1, rec.len());
        let RecordingResponse::Finalized(rec) = &rec[0] else {
            panic!("expected active")
        };
        assert_eq!("2003-01-01_01-01-11_m1", rec.id.as_str());
    }

    /*
    t.Run("emptyMonitorsNoPanic", func(t *testing.T) {
        c := NewCrawler(crawlerTestFS)
        c.RecordingByQuery(
            &CrawlerQuery{
                Time:     "2003-02-01_1_m1",
                Limit:    1,
                Monitors: []string{""},
            },
        )
    })
    t.Run("invalidTimeErr", func(t *testing.T) {
        c := NewCrawler(crawlerTestFS)
        _, err := c.RecordingByQuery(
            &CrawlerQuery{Time: "", Limit: 1},
        )
        require.Error(t, err)
    })*/

    #[tokio::test]
    async fn test_recording_by_query_data() {
        let c = Crawler::new(crawler_test_fs());
        let query = RecDbQuery {
            recording_id: r_id("9999-01-01_01-01-01_m1"),
            limit: NonZeroUsize::new(1).unwrap(),
            reverse: false,
            monitors: Vec::new(),
            include_data: true,
        };
        let rec = c
            .recordings_by_query(&query, &HashSet::new())
            .await
            .unwrap();
        let RecordingResponse::Finalized(rec) = &rec[0] else {
            panic!("expected active")
        };

        let want: RecordingData = serde_json::from_str(CRAWLER_TEST_DATA).unwrap();
        println!("{rec:?}");
        let got = rec.data.as_ref().unwrap();
        assert_eq!(&want, got);
    }

    #[tokio::test]
    async fn test_recording_by_query_missing_data() {
        let c = Crawler::new(crawler_test_fs());
        let query = RecDbQuery {
            recording_id: r_id("2002-01-01_01-01-01_m1"),
            limit: NonZeroUsize::new(1).unwrap(),
            reverse: true,
            monitors: Vec::new(),
            include_data: true,
        };
        let rec = c
            .recordings_by_query(&query, &HashSet::new())
            .await
            .unwrap();
        let RecordingResponse::Finalized(rec) = &rec[0] else {
            panic!("expected active")
        };
        assert!(rec.data.is_none());
    }
}
