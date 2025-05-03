// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{
    RecDbQuery, RecordingActive, RecordingFinalized, RecordingIncomplete, RecordingResponse,
    StartAndEnd,
};
use common::recording::{RecordingData, RecordingId};
use fs::{DynFs, Entry, FsError, Open};
use std::{collections::HashMap, path::PathBuf, str::FromStr, vec::IntoIter};
use thiserror::Error;

// Recordings are stored in the following format.
//
// <Year>
// └── <Month>
//     └── <Day>
//         ├── Monitor1
//         └── Monitor2
//             ├── YYYY-MM-DD_hh-mm-ss_monitor2.jpeg  // Thumbnail.
//             ├── YYYY-MM-DD_hh-mm-ss_monitor2.meta  // Video metadata.
//             ├── YYYY-MM-DD_hh-mm-ss_monitor2.mdat  // Raw video data.
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

    #[error("{0:?}: unexpected symlink")]
    UnexpectedSymlink(PathBuf),
}

// Crawls through storage looking for recordings.
pub struct Crawler {
    fs: DynFs,
}

impl Crawler {
    #[must_use]
    pub(crate) fn new(fs: DynFs) -> Self {
        Self { fs }
    }

    // finds the best matching recording and
    // returns limit number of subsequent recorings.
    pub(crate) async fn recordings_by_query(
        &self,
        query: RecDbQuery,
        active_recordings: HashMap<RecordingId, StartAndEnd>,
    ) -> Result<Vec<RecordingResponse>, CrawlerError> {
        let fs = self.fs.clone();
        tokio::task::spawn_blocking(move || recordings_by_query(&fs, &query, &active_recordings))
            .await
            .expect("join")
    }
}

fn recordings_by_query(
    fs: &DynFs,
    query: &RecDbQuery,
    active_recordings: &HashMap<RecordingId, StartAndEnd>,
) -> Result<Vec<RecordingResponse>, CrawlerError> {
    let mut recordings = Vec::new();
    let mut year_iter = DirIterYear::new(fs, query.clone())?;

    while recordings.len() < query.limit.get() {
        let mut rec = match year_iter.next() {
            Some(rec) => rec?,
            // Last recording.
            None => return Ok(recordings),
        };

        let id = rec.id;
        if let Some(end) = &query.end {
            if query.reverse && &id > end || !query.reverse && &id < end {
                break;
            }
        }

        if let Some(active_rec) = active_recordings.get(&id) {
            let data = query.include_data.then(|| RecordingData {
                start: active_rec.start_time.into(),
                end: active_rec.end_time.into(),
                events: Vec::new(),
            });
            recordings.push(RecordingResponse::Active(RecordingActive { id, data }));
            continue;
        }

        let Some(json_file) = rec.json_file.take() else {
            recordings.push(RecordingResponse::Incomplete(RecordingIncomplete { id }));
            continue;
        };

        let data = if query.include_data {
            read_data_file(&json_file)
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

fn read_data_file(fs: &DynFs) -> Option<RecordingData> {
    let Ok(Open::File(mut file)) = fs.open(&PathBuf::from(".")) else {
        return None;
    };
    let raw_data = file.read().ok()?;
    serde_json::from_slice::<RecordingData>(&raw_data).ok()
}

struct DirIterYear {
    query: RecDbQuery,
    years: Vec<DynFs>,
    current: Option<DirIterMonth>,
}

impl DirIterYear {
    fn new(fs: &DynFs, query: RecDbQuery) -> Result<Self, CrawlerError> {
        let (mut years, found_exact) = filter_dirs(
            list_dirs(fs, query.reverse)?,
            &query.recording_id.year(),
            query.reverse,
        );

        let current = if let Some(current) = years.pop() {
            if found_exact {
                Some(DirIterMonth::new_exact(&current.1, query.clone())?)
            } else {
                Some(DirIterMonth::new(&current.1, query.clone())?)
            }
        } else {
            None
        };

        Ok(Self {
            query,
            years: years.into_iter().map(|v| v.1).collect(),
            current,
        })
    }
}

impl Iterator for DirIterYear {
    type Item = Result<DirRec, CrawlerError>;

    fn next(&mut self) -> Option<Self::Item> {
        loop {
            if let Some(current) = &mut self.current {
                if let Some(rec) = current.next() {
                    return Some(rec);
                }
                self.current = None;
            };
            if let Some(next_year) = self.years.pop() {
                match DirIterMonth::new(&next_year, self.query.clone()) {
                    Ok(v) => self.current = Some(v),
                    Err(e) => return Some(Err(e)),
                };
                continue;
            }
            return None;
        }
    }
}

struct DirIterMonth {
    query: RecDbQuery,
    months: Vec<DynFs>,
    current: Option<DirIterDay>,
}

impl DirIterMonth {
    fn new(fs: &DynFs, query: RecDbQuery) -> Result<Self, CrawlerError> {
        let months = list_dirs::<u8>(fs, query.reverse)?
            .into_iter()
            .map(|v| v.1)
            .collect();
        Ok(Self {
            query,
            months,
            current: None,
        })
    }

    fn new_exact(fs: &DynFs, query: RecDbQuery) -> Result<Self, CrawlerError> {
        let (mut months, found_exact) = filter_dirs(
            list_dirs(fs, query.reverse)?,
            &query.recording_id.month(),
            query.reverse,
        );

        let current = if let Some(current) = months.pop() {
            if found_exact {
                Some(DirIterDay::new_exact(&current.1, query.clone())?)
            } else {
                Some(DirIterDay::new(&current.1, query.clone())?)
            }
        } else {
            None
        };

        Ok(Self {
            query,
            months: months.into_iter().map(|v| v.1).collect(),
            current,
        })
    }
}

impl Iterator for DirIterMonth {
    type Item = Result<DirRec, CrawlerError>;

    fn next(&mut self) -> Option<Self::Item> {
        loop {
            if let Some(current) = &mut self.current {
                if let Some(rec) = current.next() {
                    return Some(rec);
                }
                self.current = None;
            };
            if let Some(next_month) = self.months.pop() {
                match DirIterDay::new(&next_month, self.query.clone()) {
                    Ok(v) => self.current = Some(v),
                    Err(e) => return Some(Err(e)),
                };
                continue;
            }
            return None;
        }
    }
}

struct DirIterDay {
    query: RecDbQuery,
    days: Vec<DynFs>,
    current: Option<IntoIter<DirRec>>,
}

impl DirIterDay {
    fn new(fs: &DynFs, query: RecDbQuery) -> Result<Self, CrawlerError> {
        let days = list_dirs::<u8>(fs, query.reverse)?
            .into_iter()
            .map(|v| v.1)
            .collect();
        Ok(Self {
            query,
            days,
            current: None,
        })
    }

    fn new_exact(fs: &DynFs, query: RecDbQuery) -> Result<Self, CrawlerError> {
        let (mut days, found_exact) = filter_dirs(
            list_dirs(fs, query.reverse)?,
            &query.recording_id.day(),
            query.reverse,
        );

        let current = if let Some(current) = days.pop() {
            if found_exact {
                Some(DirIterRec::new_exact(&current.1, &query)?)
            } else {
                Some(DirIterRec::new(&current.1, &query)?)
            }
        } else {
            None
        };

        Ok(Self {
            query,
            days: days.into_iter().map(|v| v.1).collect(),
            current,
        })
    }
}

impl Iterator for DirIterDay {
    type Item = Result<DirRec, CrawlerError>;

    fn next(&mut self) -> Option<Self::Item> {
        loop {
            if let Some(current) = &mut self.current {
                if let Some(rec) = current.next() {
                    return Some(Ok(rec));
                }
                self.current = None;
            };
            if let Some(next_day) = self.days.pop() {
                match DirIterRec::new(&next_day, &self.query) {
                    Ok(v) => self.current = Some(v),
                    Err(e) => return Some(Err(e)),
                };
                continue;
            }
            return None;
        }
    }
}

fn list_dirs<S: FromStr>(fs: &DynFs, reverse: bool) -> Result<Vec<(S, DynFs)>, CrawlerError> {
    let mut dirs = Vec::new();
    for entry in fs.read_dir()? {
        let Entry::Dir(d) = entry else { continue };

        let Ok(name) = d.name().to_string_lossy().parse() else {
            continue;
        };
        let file_fs = fs.sub(d.name())?;
        dirs.push((name, file_fs));
    }
    if reverse {
        dirs.reverse();
    }
    Ok(dirs)
}

fn filter_dirs<T: Ord>(
    mut dirs: Vec<(T, DynFs)>,
    target: &T,
    reverse: bool,
) -> (Vec<(T, DynFs)>, bool) {
    // Exact match.
    if let Some(index) = dirs.iter().position(|v| v.0 == *target) {
        if !dirs.is_empty() {
            let _ = dirs.split_off(index + 1);
        }
        (dirs, true)
    } else {
        // Inexact match.
        let index = if reverse {
            dirs.iter().position(|dir| dir.0 > *target)
        } else {
            dirs.iter().position(|dir| dir.0 < *target)
        };
        if let Some(index) = index {
            dirs = dirs.split_off(index);
        } else {
            dirs.clear();
        }
        (dirs, false)
    }
}

struct DirIterRec;

#[allow(clippy::new_ret_no_self)]
impl DirIterRec {
    fn new(fs: &DynFs, query: &RecDbQuery) -> Result<IntoIter<DirRec>, CrawlerError> {
        let mut recs = Self::list_recs(fs, query)?;
        recs.reverse();
        Ok(recs.into_iter())
    }

    fn new_exact(fs: &DynFs, query: &RecDbQuery) -> Result<IntoIter<DirRec>, CrawlerError> {
        let id = query.recording_id.clone();
        let mut recs = Self::list_recs(fs, query)?;

        let mut recs = {
            // Exact match.
            if let Some(index) = recs.iter().position(|rec| rec.id == id) {
                // Skip exact match.
                let _ = recs.split_off(index);
                recs
            } else {
                // Inexact match.
                let index = if query.reverse {
                    recs.iter().position(|rec| rec.id < id)
                } else {
                    recs.iter().position(|rec| rec.id > id)
                };
                if let Some(index) = index {
                    _ = recs.split_off(index);
                }
                recs
            }
        };

        recs.reverse();
        Ok(recs.into_iter())
    }

    // Finds all recordings belong to selected monitors in decending directories.
    fn list_recs(fs: &DynFs, query: &RecDbQuery) -> Result<Vec<DirRec>, CrawlerError> {
        let monitor_dirs = fs.read_dir()?;

        let mut all_files = Vec::new();
        for entry in monitor_dirs {
            let Some(name) = entry.name().to_str() else {
                continue;
            };
            if !monitor_selected(&query.monitors, name) {
                continue;
            }

            let montor_fs = fs.sub(entry.name())?;

            let mut meta_files = Vec::new();
            let mut json_files = HashMap::new();

            let files = montor_fs.read_dir()?;
            for file in &files {
                let file = match file {
                    Entry::Dir(d) => {
                        return Err(CrawlerError::UnexpectedDir(d.name().to_path_buf()));
                    }
                    Entry::File(v) => v,
                    Entry::Symlink(s) => {
                        return Err(CrawlerError::UnexpectedDir(s.name().to_path_buf()));
                    }
                };
                let Some(name) = file.name().to_str() else {
                    continue;
                };
                let Some(ext) = file.name().extension() else {
                    continue;
                };
                if ext == "meta" {
                    let name = name.trim_end_matches(".meta");
                    meta_files.push(name);
                } else if ext == "json" {
                    let name = name.trim_end_matches(".json");
                    let file_fs = montor_fs.sub(file.name())?;
                    json_files.insert(name, file_fs);
                }
            }

            for name in meta_files {
                let Ok(id) = RecordingId::try_from(name.to_owned()) else {
                    continue;
                };
                all_files.push(DirRec {
                    id,
                    json_file: json_files.remove(name),
                });
            }
        }
        if query.reverse {
            all_files.reverse();
        }
        Ok(all_files)
    }
}

struct DirRec {
    id: RecordingId,
    json_file: Option<DynFs>,
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
    use super::*;
    use crate::RecDbQuery;
    use fs::{MapEntry, MapFs};
    use pretty_assertions::assert_eq;
    use std::num::NonZeroUsize;
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

    fn crawler_test_fs() -> DynFs {
        Box::new(MapFs(
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

    #[track_caller]
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

    #[test_case("1700-01-01_01-01-01_m1", "";                       "no files")]
    #[test_case("1999-01-01_01-01-01_m1", "";                       "EOF")]
    #[test_case("2200-01-01_01-01-01_m1", "2099-01-01_01-01-11_m1"; "latest")]
    #[test_case("2000-01-01_01-01-22_m1", "2000-01-01_01-01-11_m1"; "prev hour")]
    #[test_case("2000-01-01_01-01-23_m1", "2000-01-01_01-01-22_m1"; "prev hour inexact")]
    #[test_case("2000-01-02_01-01-11_m1", "2000-01-01_01-01-22_m1"; "prev day")]
    #[test_case("2000-02-01_01-01-11_m1", "2000-01-02_01-01-11_m1"; "prev month")]
    #[test_case("2001-01-01_01-01-11_m1", "2000-02-01_01-01-11_m1"; "prev year")]
    #[test_case("2002-12-01_01-01-01_m1", "2002-01-01_01-01-11_m1"; "empty prev day")]
    #[test_case("2004-01-01_01-01-22_m1", "2004-01-01_01-01-11_m1"; "same day")]
    #[tokio::test]
    async fn test_recording_by_query(input: &str, want: &str) {
        let query = RecDbQuery {
            recording_id: r_id(input),
            end: None,
            limit: NonZeroUsize::new(1).unwrap(),
            reverse: false,
            monitors: Vec::new(),
            include_data: false,
        };
        let rec = match Crawler::new(crawler_test_fs())
            .recordings_by_query(query, HashMap::new())
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
            let got = rec[0].assert_finalized().id.as_str();
            assert_eq!(want, got);
        }
    }

    #[test_case("1711-01-01_01-01-01_m1", "2000-01-01_01-01-11_m1"; "latest")]
    #[test_case("2000-01-01_01-01-11_m1", "2000-01-01_01-01-22_m1"; "next hour")]
    #[test_case("2000-01-01_01-01-10_m1", "2000-01-01_01-01-11_m1"; "next hour inexact")]
    #[test_case("2000-01-01_01-01-22_m1", "2000-01-02_01-01-11_m1"; "next day")]
    #[test_case("2000-01-02_01-01-11_m1", "2000-02-01_01-01-11_m1"; "next month")]
    #[test_case("2000-01-02_01-01-12_m1", "2000-02-01_01-01-11_m1"; "next month2")]
    #[test_case("2000-02-01_01-01-11_m1", "2001-02-01_01-01-11_m1"; "next year")]
    #[test_case("2001-12-01_01-01-01_m1", "2002-01-01_01-01-11_m1"; "empty next day")]
    #[tokio::test]
    async fn test_recording_by_query_reverse(input: &str, want: &str) {
        let query = RecDbQuery {
            recording_id: r_id(input),
            end: None,
            limit: NonZeroUsize::new(1).unwrap(),
            reverse: true,
            monitors: Vec::new(),
            include_data: false,
        };
        let rec = match Crawler::new(crawler_test_fs())
            .recordings_by_query(query, HashMap::new())
            .await
        {
            Ok(v) => v,
            Err(e) => {
                println!("{e}");
                panic!("{e}");
            }
        };
        let got = rec[0].assert_finalized().id.as_str();
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_recording_by_query_multiple() {
        let c = Crawler::new(crawler_test_fs());
        let query = RecDbQuery {
            recording_id: RecordingId::max(),
            end: None,
            limit: NonZeroUsize::new(5).unwrap(),
            reverse: false,
            monitors: Vec::new(),
            include_data: false,
        };
        let recordings = c.recordings_by_query(query, HashMap::new()).await.unwrap();

        let mut ids = Vec::new();
        for rec in recordings {
            ids.push(rec.assert_finalized().id.as_str().to_owned());
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
            end: None,
            limit: NonZeroUsize::new(1).unwrap(),
            reverse: false,
            monitors: vec!["m1".to_owned()],
            include_data: false,
        };
        let rec = c.recordings_by_query(query, HashMap::new()).await.unwrap();
        assert_eq!(1, rec.len());
        let got = rec[0].assert_finalized().id.as_str();
        assert_eq!("2003-01-01_01-01-11_m1", got);
    }

    #[tokio::test]
    async fn test_recording_by_query_data() {
        let c = Crawler::new(crawler_test_fs());
        let query = RecDbQuery {
            recording_id: RecordingId::max(),
            end: None,
            limit: NonZeroUsize::new(1).unwrap(),
            reverse: false,
            monitors: Vec::new(),
            include_data: true,
        };
        let rec = c.recordings_by_query(query, HashMap::new()).await.unwrap();

        let want: RecordingData = serde_json::from_str(CRAWLER_TEST_DATA).unwrap();
        let got = rec[0].assert_finalized().data.as_ref().unwrap();
        assert_eq!(&want, got);
    }

    #[tokio::test]
    async fn test_recording_by_query_missing_data() {
        let c = Crawler::new(crawler_test_fs());
        let query = RecDbQuery {
            recording_id: r_id("2002-01-01_01-01-01_m1"),
            end: None,
            limit: NonZeroUsize::new(1).unwrap(),
            reverse: true,
            monitors: Vec::new(),
            include_data: true,
        };
        let rec = c.recordings_by_query(query, HashMap::new()).await.unwrap();
        assert!(rec[0].assert_finalized().data.is_none());
    }

    #[tokio::test]
    async fn test_inexact_propagation() {
        let dirs = Box::new(MapFs(
            [map_fs_item("2024/05/30/m1/2024-05-30_01-01-11_m1")]
                .into_iter()
                .flatten()
                .collect(),
        ));

        let query = RecDbQuery {
            recording_id: RecordingId::max(),
            end: None,
            limit: NonZeroUsize::new(1).unwrap(),
            reverse: false,
            monitors: Vec::new(),
            include_data: false,
        };
        let rec = match Crawler::new(dirs)
            .recordings_by_query(query, HashMap::new())
            .await
        {
            Ok(v) => v,
            Err(e) => {
                println!("{e}");
                panic!("{e}");
            }
        };
        assert_eq!(r_id("2024-05-30_01-01-11_m1"), rec[0].assert_finalized().id);
    }

    #[tokio::test]
    async fn test_recording_by_query_end() {
        let c = Crawler::new(crawler_test_fs());
        let query = RecDbQuery {
            recording_id: RecordingId::max(),
            end: Some(r_id("2003-01-01_01-01-11_m1")),
            limit: NonZeroUsize::new(100).unwrap(),
            reverse: false,
            monitors: Vec::new(),
            include_data: false,
        };
        let recordings = c.recordings_by_query(query, HashMap::new()).await.unwrap();

        let mut ids = Vec::new();
        for rec in recordings {
            ids.push(rec.assert_finalized().id.as_str().to_owned());
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
    async fn test_recording_by_query_end_reverse() {
        let c = Crawler::new(crawler_test_fs());
        let query = RecDbQuery {
            recording_id: RecordingId::zero(),
            end: Some(r_id("2003-01-01_01-01-11_m1")),
            limit: NonZeroUsize::new(100).unwrap(),
            reverse: true,
            monitors: Vec::new(),
            include_data: false,
        };
        let recordings = c.recordings_by_query(query, HashMap::new()).await.unwrap();

        let mut ids = Vec::new();
        for rec in recordings {
            ids.push(rec.assert_finalized().id.as_str().to_owned());
        }

        let want = vec![
            "2000-01-01_01-01-11_m1",
            "2000-01-01_01-01-22_m1",
            "2000-01-02_01-01-11_m1",
            "2000-02-01_01-01-11_m1",
            "2001-02-01_01-01-11_m1",
            "2002-01-01_01-01-11_m1",
            "2003-01-01_01-01-11_m1",
        ];
        assert_eq!(want, ids);
    }

    #[test_case("2000-01-01_01-01-15_m1", "2000-01-01_01-01-10_m1", false)]
    #[test_case("2000-01-01_01-01-20_m1", "2000-01-01_01-01-10_m1", false)]
    #[test_case("2000-01-01_01-01-15_m1", "2000-01-01_01-01-20_m1", true)]
    #[test_case("2000-01-01_01-01-10_m1", "2000-01-01_01-01-20_m1", true)]
    #[tokio::test]
    async fn test_file_direction(input: &str, want: &str, reverse: bool) {
        let dirs = Box::new(MapFs(
            [
                map_fs_item("2000/01/01/m1/2000-01-01_01-01-00_m1"),
                map_fs_item("2000/01/01/m1/2000-01-01_01-01-10_m1"),
                map_fs_item("2000/01/01/m1/2000-01-01_01-01-20_m1"),
                map_fs_item("2000/01/01/m1/2000-01-01_01-01-30_m1"),
                map_fs_item("2000/01/01/m1/2000-01-01_01-01-40_m1"),
            ]
            .into_iter()
            .flatten()
            .collect(),
        ));

        let query = RecDbQuery {
            recording_id: r_id(input),
            end: None,
            limit: NonZeroUsize::new(1).unwrap(),
            reverse,
            monitors: Vec::new(),
            include_data: false,
        };
        let rec = match Crawler::new(dirs)
            .recordings_by_query(query, HashMap::new())
            .await
        {
            Ok(v) => v,
            Err(e) => {
                println!("{e}");
                panic!("{e}");
            }
        };
        assert_eq!(r_id(want), rec[0].assert_finalized().id);
    }

    #[test_case("2000-01-01_01-15-00_m1", "2000-01-01_01-10-00_m1", false)]
    #[test_case("2000-01-01_01-20-00_m1", "2000-01-01_01-10-00_m1", false)]
    #[test_case("2000-01-01_01-15-00_m1", "2000-01-01_01-20-00_m1", true)]
    #[test_case("2000-01-01_01-10-00_m1", "2000-01-01_01-20-00_m1", true)]
    #[tokio::test]
    async fn test_day_direction(input: &str, want: &str, reverse: bool) {
        let dirs = Box::new(MapFs(
            [
                map_fs_item("2000/01/01/m1/2000-01-01_01-00-00_m1"),
                map_fs_item("2000/01/01/m1/2000-01-01_01-10-00_m1"),
                map_fs_item("2000/01/01/m1/2000-01-01_01-20-00_m1"),
                map_fs_item("2000/01/01/m1/2000-01-01_01-30-00_m1"),
                map_fs_item("2000/01/01/m1/2000-01-01_01-40-00_m1"),
            ]
            .into_iter()
            .flatten()
            .collect(),
        ));

        let query = RecDbQuery {
            recording_id: r_id(input),
            end: None,
            limit: NonZeroUsize::new(1).unwrap(),
            reverse,
            monitors: Vec::new(),
            include_data: false,
        };
        let rec = match Crawler::new(dirs)
            .recordings_by_query(query, HashMap::new())
            .await
        {
            Ok(v) => v,
            Err(e) => {
                println!("{e}");
                panic!("{e}");
            }
        };
        assert_eq!(r_id(want), rec[0].assert_finalized().id);
    }
}
