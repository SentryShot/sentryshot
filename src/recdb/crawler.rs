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

type Cache = HashMap<PathBuf, Vec<Dir>>;

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
        let mut file = self.find_recording(query, cache)?;
        while recordings.len() < query.limit.get() {
            let Some(mut prev_file) = file.clone() else {
                // Last recording is reached.
                return Ok(recordings);
            };

            let first_file = recordings.is_empty();
            file = if first_file && prev_file.name != query.recording_id.as_path() {
                self.find_recording(query, cache)?
            } else {
                match prev_file.sibling(query, cache) {
                    Ok(Some(v)) => Some(v),
                    Ok(None) => return Ok(recordings), // Last recording.
                    Err(e) => return Err(e),
                }
            };
            let Some(file) = &file else {
                // Last recording is reached.
                return Ok(recordings);
            };

            let id = file
                .path
                .file_name()
                .expect("file should have a name")
                .to_string_lossy()
                .to_string();

            let Ok(id) = RecordingId::try_from(id) else {
                continue;
            };

            let is_active = active_recordings.contains(&id);
            if is_active {
                recordings.push(RecordingResponse::Active(RecordingActive { id }));
                continue;
            }

            let Some(json_file) = file.json_file.clone() else {
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

    fn find_recording(
        &self,
        query: &RecDbQuery,
        cache: &mut Cache,
    ) -> Result<Option<Dir>, CrawlerError> {
        if query.recording_id.len() < 10 {
            return Err(CrawlerError::InvalidValue(query.recording_id.clone()));
        }

        let root = Dir {
            fs: self.fs.clone(),
            name: PathBuf::from(""),
            path: PathBuf::from(""),
            depth: 0,
            parent: None,
            json_file: None,
        };

        // Try to find exact file.
        let mut current = root;
        for val in query.recording_id.year_month_day() {
            let mut parent = current.clone();
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

fn read_data_file(fs: &ArcFs) -> Option<RecordingData> {
    let Ok(Open::File(mut file)) = fs.open(&PathBuf::from(".")) else {
        return None;
    };
    let raw_data = file.read().ok()?;
    serde_json::from_slice::<RecordingData>(&raw_data).ok()
}

#[derive(Clone)]
struct Dir {
    fs: ArcFs,
    name: PathBuf,
    path: PathBuf,
    depth: u8,
    parent: Option<Box<Dir>>,
    json_file: Option<ArcFs>,
}

impl std::fmt::Debug for Dir {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}: {:?}", self.depth, self.path)
    }
}

const MONITOR_DEPTH: u8 = 3;
const REC_DEPTH: u8 = 5;

impl Dir {
    // children of current directory. Special case if depth == monitorDepth.
    fn children(
        &mut self,
        query: &RecDbQuery,
        cache: &mut Cache,
    ) -> Result<Vec<Dir>, CrawlerError> {
        if let Some(cached) = cache.get(&self.path) {
            return Ok(cached.to_owned());
        }

        if self.depth == MONITOR_DEPTH {
            let mut children = self.find_all_files(query)?;
            children.sort_by_key(|v| v.name.clone());
            cache.insert(self.path.clone(), children.clone());
            return Ok(children);
        }

        let entries = self.fs.read_dir()?;

        let mut children = Vec::new();

        for entry in entries {
            let Entry::Dir(dir) = entry else { continue };

            let path = self.path.join(dir.name());
            let file_fs = self.fs.sub(dir.name())?;
            children.push(Dir {
                fs: file_fs.into(),
                name: dir.name().to_path_buf(),
                path,
                parent: Some(Box::new(self.to_owned())),
                depth: self.depth + 1,
                json_file: None,
            });
        }

        cache.insert(self.path.clone(), children.clone());

        Ok(children)
    }

    // Finds all json files beloning to
    // selected monitors in decending directories.
    // Only called by `children()`.
    fn find_all_files(&mut self, query: &RecDbQuery) -> Result<Vec<Dir>, CrawlerError> {
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

                let file_fs = montor_fs.sub(file_name)?;

                all_files.push(Dir {
                    fs: file_fs.into(),
                    name: PathBuf::from(name),
                    path,
                    parent: Some(Box::new(self.to_owned())),
                    depth: self.depth + 2,
                    json_file: json_files.remove(name),
                });
            }
        }
        Ok(all_files)
    }

    // Returns next or previous child.
    fn child_by_name(
        &mut self,
        query: &RecDbQuery,
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
        query: &RecDbQuery,
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
        query: &RecDbQuery,
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
                current = first_child.to_owned();
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
        query: &RecDbQuery,
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
