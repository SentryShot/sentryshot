// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{
    RecDbQuery, RecordingActive, RecordingFinalized, RecordingIncomplete, RecordingResponse,
};
use fs::{DynFs, Entry, FsError, Open};
use recording::{RecordingData, RecordingId};
use std::{
    collections::{HashMap, HashSet},
    path::PathBuf,
    str::FromStr,
    vec::IntoIter,
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
        active_recordings: HashSet<RecordingId>,
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
    active_recordings: &HashSet<RecordingId>,
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
        let is_active = active_recordings.contains(&id);
        if is_active {
            recordings.push(RecordingResponse::Active(RecordingActive { id }));
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
        let mut years = filter_dirs(
            list_dirs(fs, query.reverse)?,
            &query.recording_id.year(),
            query.reverse,
        );

        let current = if let Some(current) = years.pop() {
            Some(DirIterMonth::new_first(&current.1, query.clone())?)
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

    fn new_first(fs: &DynFs, query: RecDbQuery) -> Result<Self, CrawlerError> {
        let mut months = filter_dirs(
            list_dirs(fs, query.reverse)?,
            &query.recording_id.month(),
            query.reverse,
        );

        let current = if let Some(current) = months.pop() {
            Some(DirIterDay::new_first(&current.1, query.clone())?)
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

    fn new_first(fs: &DynFs, query: RecDbQuery) -> Result<Self, CrawlerError> {
        let mut days = filter_dirs(
            list_dirs(fs, query.reverse)?,
            &query.recording_id.day(),
            query.reverse,
        );

        let current = if let Some(current) = days.pop() {
            Some(DirIterRec::new_first(&current.1, &query)?)
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
    let mut years = Vec::new();
    for entry in fs.read_dir()? {
        let Entry::Dir(d) = entry else { continue };

        let Ok(name) = d.name().to_string_lossy().parse() else {
            continue;
        };
        let file_fs = fs.sub(d.name())?;
        years.push((name, file_fs));
    }
    if reverse {
        years.reverse();
    }
    Ok(years)
}

fn filter_dirs<T: Ord>(mut dirs: Vec<(T, DynFs)>, target: &T, reverse: bool) -> Vec<(T, DynFs)> {
    // Exact match.
    if let Some(index) = dirs.iter().position(|v| v.0 == *target) {
        if !dirs.is_empty() {
            let _ = dirs.split_off(index + 1);
        }
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
    };
    dirs
}

struct DirIterRec;

#[allow(clippy::new_ret_no_self)]
impl DirIterRec {
    fn new(fs: &DynFs, query: &RecDbQuery) -> Result<IntoIter<DirRec>, CrawlerError> {
        let mut recs = Self::list_recs(fs, query)?;
        recs.reverse();
        Ok(recs.into_iter())
    }

    fn new_first(fs: &DynFs, query: &RecDbQuery) -> Result<IntoIter<DirRec>, CrawlerError> {
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
                    recs.iter().position(|rec| rec.id > id)
                } else {
                    recs.iter().position(|rec| rec.id < id)
                };
                if let Some(index) = index {
                    recs.split_off(index)
                } else {
                    Vec::new()
                }
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
                let Entry::File(file) = file else {
                    return Err(CrawlerError::UnexpectedDir(PathBuf::from("todo!")));
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
            .recordings_by_query(query, HashSet::new())
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
            .recordings_by_query(query, HashSet::new())
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
        let recordings = c.recordings_by_query(query, HashSet::new()).await.unwrap();

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
        let rec = c.recordings_by_query(query, HashSet::new()).await.unwrap();
        assert_eq!(1, rec.len());
        let RecordingResponse::Finalized(rec) = &rec[0] else {
            panic!("expected active")
        };
        assert_eq!("2003-01-01_01-01-11_m1", rec.id.as_str());
    }

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
        let rec = c.recordings_by_query(query, HashSet::new()).await.unwrap();
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
        let rec = c.recordings_by_query(query, HashSet::new()).await.unwrap();
        let RecordingResponse::Finalized(rec) = &rec[0] else {
            panic!("expected active")
        };
        assert!(rec.data.is_none());
    }
}
