// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{
    ActiveRec, RecDbQuery, RecordingActive, RecordingFinalized, RecordingIncomplete,
    RecordingResponse,
};
use common::{
    recording::{RecordingId, RecordingIdError},
    time::UnixNano,
};
use fs::{DynFs, Entry, FsError};
use std::{
    collections::HashMap,
    path::{Path, PathBuf},
    str::FromStr,
    vec::IntoIter,
};
use thiserror::Error;

#[derive(Debug, Error)]
#[allow(clippy::module_name_repetitions)]
pub enum CrawlerError {
    #[error("fs: {0}")]
    Fs(#[from] FsError),

    #[error("parse name: {0} {1:?}")]
    ParseName(std::num::ParseIntError, PathBuf),

    #[error("parse end file name: {0} {1:?}")]
    ParseEndFileName(std::num::ParseIntError, PathBuf),

    #[error("parse recording id: {0}")]
    ParseRecordingId(RecordingIdError),

    #[error("{0:?}: unexpected directory")]
    UnexpectedDir(PathBuf),

    #[error("{0:?}: unexpected file")]
    UnexpectedFile(PathBuf),

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

    // Finds the best matching recording and
    // returns limit number of subsequent recorings.
    pub(crate) async fn recordings_by_query(
        &self,
        query: RecDbQuery,
        active_recordings: HashMap<RecordingId, ActiveRec>,
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
    active_recordings: &HashMap<RecordingId, ActiveRec>,
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
            if query.oldest_first && &id.nanos() > end || !query.oldest_first && &id.nanos() < end {
                break;
            }
        }

        if let Some(active_rec) = active_recordings.get(&id) {
            recordings.push(RecordingResponse::Active(RecordingActive {
                id,
                end: active_rec.end_time.into(),
            }));
            continue;
        }

        if let Some(end) = rec.end.take() {
            recordings.push(RecordingResponse::Finalized(RecordingFinalized { id, end }));
        } else {
            recordings.push(RecordingResponse::Incomplete(RecordingIncomplete { id }));
        }
    }
    Ok(recordings)
}

struct DirIterYear {
    query: RecDbQuery,
    years: Vec<DynFs>,
    current: Option<DirIterLevel2>,
}

impl DirIterYear {
    fn new(fs: &DynFs, query: RecDbQuery) -> Result<Self, CrawlerError> {
        let dirs = list_dirs::<u16>(fs, query.oldest_first)?;
        let (mut years, found_exact) =
            filter_dirs(dirs, &query.recording_id.level1(), query.oldest_first);

        let current = if let Some(current) = years.pop() {
            if found_exact {
                Some(DirIterLevel2::new_exact(&current.1, query.clone())?)
            } else {
                Some(DirIterLevel2::new(&current.1, query.clone())?)
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
                match DirIterLevel2::new(&next_year, self.query.clone()) {
                    Ok(v) => self.current = Some(v),
                    Err(e) => return Some(Err(e)),
                };
                continue;
            }
            return None;
        }
    }
}

struct DirIterLevel2 {
    query: RecDbQuery,
    months: Vec<DynFs>,
    current: Option<IntoIter<DirRec>>,
}

impl DirIterLevel2 {
    fn new(fs: &DynFs, query: RecDbQuery) -> Result<Self, CrawlerError> {
        let months = list_dirs::<u32>(fs, query.oldest_first)?
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
        let dirs = list_dirs(fs, query.oldest_first)?;
        let (mut months, found_exact) =
            filter_dirs(dirs, &query.recording_id.level2(), query.oldest_first);

        let current = if let Some(current) = months.pop() {
            if found_exact {
                Some(DirIterLevel3::new_exact(&current.1, &query)?)
            } else {
                Some(DirIterLevel3::new(&current.1, &query)?)
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

impl Iterator for DirIterLevel2 {
    type Item = Result<DirRec, CrawlerError>;

    fn next(&mut self) -> Option<Self::Item> {
        loop {
            if let Some(current) = &mut self.current {
                if let Some(rec) = current.next() {
                    return Some(Ok(rec));
                }
                self.current = None;
            };
            if let Some(next_month) = self.months.pop() {
                match DirIterLevel3::new(&next_month, &self.query) {
                    Ok(v) => self.current = Some(v),
                    Err(e) => return Some(Err(e)),
                };
                continue;
            }
            return None;
        }
    }
}

fn list_dirs<S: FromStr<Err = std::num::ParseIntError> + Ord + Copy>(
    fs: &DynFs,
    reverse: bool,
) -> Result<Vec<(S, DynFs)>, CrawlerError> {
    let mut dirs = Vec::new();
    for entry in fs.read_dir()? {
        let Entry::Dir(d) = entry else { continue };

        let name = d
            .name()
            .to_string_lossy()
            .parse()
            .map_err(|e| CrawlerError::ParseName(e, d.name().to_path_buf()))?;
        let file_fs = fs.sub(d.name())?;
        dirs.push((name, file_fs));
    }
    dirs.sort_by_key(|v| v.0);
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
    if let Some(index) = dirs.iter().position(|v| v.0 == *target) {
        // Exact match.
        if !dirs.is_empty() {
            let _ = dirs.split_off(index + 1);
        }
        (dirs, true)
    } else {
        // Inexact match.
        let index = if reverse {
            dirs.iter().position(|dir| dir.0 < *target)
        } else {
            dirs.iter().position(|dir| dir.0 > *target)
        };
        if let Some(index) = index {
            _ = dirs.split_off(index);
        }
        (dirs, false)
    }
}

struct DirIterLevel3;

#[allow(clippy::new_ret_no_self)]
impl DirIterLevel3 {
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
                let index = if query.oldest_first {
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
        let mut recs = Vec::new();

        let files = fs.read_dir()?;
        for file in files {
            let dir = match file {
                Entry::Dir(dir) => dir,
                Entry::File(f) => {
                    return Err(CrawlerError::UnexpectedFile(f.name().to_path_buf()));
                }
                Entry::Symlink(s) => {
                    return Err(CrawlerError::UnexpectedSymlink(s.name().to_path_buf()));
                }
            };
            let Some(name) = dir.name().to_str() else {
                continue;
            };
            let rec_id: RecordingId = name.parse().map_err(CrawlerError::ParseRecordingId)?;
            if !monitor_selected(&query.monitors, rec_id.monitor_id()) {
                continue;
            }
            recs.push(DirRec {
                id: rec_id,
                end: find_end_file(fs, dir.name())?,
            });
        }

        recs.sort_by_key(|v| v.id.clone());
        if query.oldest_first {
            recs.reverse();
        }
        Ok(recs)
    }
}

fn find_end_file(fs: &DynFs, path: &Path) -> Result<Option<UnixNano>, CrawlerError> {
    for file in fs.sub(path)?.read_dir()? {
        match file {
            Entry::Dir(d) => {
                return Err(CrawlerError::UnexpectedDir(d.name().to_path_buf()));
            }
            Entry::File(file) => {
                let Some(name) = file.name().to_str() else {
                    continue;
                };
                let Some(ext) = file.name().extension() else {
                    continue;
                };
                if ext == "end" {
                    let end = name.trim_end_matches(".end").parse().map_err(|e| {
                        CrawlerError::ParseEndFileName(e, file.name().to_path_buf())
                    })?;
                    return Ok(Some(UnixNano::new(end)));
                }
            }
            Entry::Symlink(s) => {
                return Err(CrawlerError::UnexpectedFile(s.name().to_path_buf()));
            }
        }
    }
    Ok(None)
}

struct DirRec {
    id: RecordingId,
    end: Option<UnixNano>,
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
    use common::recording::RecordingId;
    use fs::{MapEntry, MapFs};
    use pretty_assertions::assert_eq;
    use std::num::NonZeroUsize;
    use test_case::test_case;

    fn map_fs_item(path: &str) -> Vec<(PathBuf, MapEntry)> {
        map_fs_item_with_end(path, "1000")
    }

    fn map_fs_item_with_end(rec_id2: &str, end: &str) -> Vec<(PathBuf, MapEntry)> {
        let rec_id: RecordingId = rec_id2.parse().unwrap();
        vec![(
            rec_id.full_path().join(format!("{end}.end")),
            MapEntry {
                is_file: true,
                ..Default::default()
            },
        )]
    }

    fn crawler_test_fs() -> DynFs {
        Box::new(MapFs(
            [
                map_fs_item("1000000000000011_m1"),
                map_fs_item("1000000000000022_m1"),
                map_fs_item("1010000000000011_m1"),
                map_fs_item("2010000000000011_m1"),
                map_fs_item("3000000000000011_m1"),
                map_fs_item("4000000000000011_m1"),
                map_fs_item("4000000000000011_m2"),
                map_fs_item("5000000000000011_m1"),
                map_fs_item("5000000000000022_m1"),
                map_fs_item_with_end("9900000000000011_m1", "4073680924000000000"),
            ]
            .into_iter()
            .flatten()
            .collect(),
        ))
    }

    #[track_caller]
    fn r_id(s: &str) -> RecordingId {
        s.parse().unwrap()
    }

    #[test_case("0_m1",                "";                    "EOF")]
    #[test_case("9999999999999999_m1", "9900000000000011_m1"; "newest")]
    #[test_case("1000000000000022_m1", "1000000000000011_m1"; "prev hour")]
    #[test_case("1000000000000023_m1", "1000000000000022_m1"; "prev hour inexact")]
    #[test_case("1010000000000011_m1", "1000000000000022_m1"; "prev month")]
    #[test_case("2010000000000011_m1", "1010000000000011_m1"; "prev year")]
    #[test_case("3900000000000001_m1", "3000000000000011_m1"; "empty prev day")]
    #[test_case("5000000000000022_m1", "5000000000000011_m1"; "same day")]
    #[tokio::test]
    async fn test_recording_by_query(input: &str, want: &str) {
        let query = RecDbQuery {
            recording_id: r_id(input),
            end: None,
            limit: NonZeroUsize::new(1).unwrap(),
            oldest_first: false,
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
            assert!(rec.is_empty(), "{rec:?}");
        } else {
            let got = rec[0].assert_finalized().id.as_string();
            assert_eq!(want, got);
        }
    }

    #[test_case("0_m1",                "1000000000000011_m1"; "oldest")]
    #[test_case("1000000000000011_m1", "1000000000000022_m1"; "next hour")]
    #[test_case("1000000000000010_m1", "1000000000000011_m1"; "next hour inexact")]
    #[test_case("1000000000000022_m1", "1010000000000011_m1"; "next month")]
    #[test_case("1000000000000030_m1", "1010000000000011_m1"; "next month inexact")]
    #[test_case("1010000000000011_m1", "2010000000000011_m1"; "next year")]
    #[test_case("2900000000000001_m1", "3000000000000011_m1"; "empty next day")]
    #[tokio::test]
    async fn test_recording_by_query_reverse(input: &str, want: &str) {
        let query = RecDbQuery {
            recording_id: r_id(input),
            end: None,
            limit: NonZeroUsize::new(1).unwrap(),
            oldest_first: true,
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
        let got = rec[0].assert_finalized().id.as_string();
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_recording_by_query_multiple() {
        let c = Crawler::new(crawler_test_fs());
        let query = RecDbQuery {
            recording_id: RecordingId::max(),
            end: None,
            limit: NonZeroUsize::new(5).unwrap(),
            oldest_first: false,
            monitors: Vec::new(),
            include_data: false,
        };
        let recordings = c.recordings_by_query(query, HashMap::new()).await.unwrap();

        let mut ids = Vec::new();
        for rec in recordings {
            ids.push(rec.assert_finalized().id.as_string());
        }

        let want = vec![
            "9900000000000011_m1",
            "5000000000000022_m1",
            "5000000000000011_m1",
            "4000000000000011_m2",
            "4000000000000011_m1",
        ];
        assert_eq!(want, ids);
    }

    #[tokio::test]
    async fn test_recording_by_query_monitors() {
        let c = Crawler::new(crawler_test_fs());
        let query = RecDbQuery {
            recording_id: r_id("4010000000000011_m1"),
            end: None,
            limit: NonZeroUsize::new(1).unwrap(),
            oldest_first: false,
            monitors: vec!["m1".to_owned()],
            include_data: false,
        };
        let rec = c.recordings_by_query(query, HashMap::new()).await.unwrap();
        assert_eq!(1, rec.len());
        let got = rec[0].assert_finalized().id.as_string();
        assert_eq!("4000000000000011_m1", got);
    }

    #[tokio::test]
    async fn test_recording_by_query_data() {
        let c = Crawler::new(crawler_test_fs());
        let query = RecDbQuery {
            recording_id: RecordingId::max(),
            end: None,
            limit: NonZeroUsize::new(1).unwrap(),
            oldest_first: false,
            monitors: Vec::new(),
            include_data: true,
        };
        let rec = c.recordings_by_query(query, HashMap::new()).await.unwrap();

        let got = rec[0].assert_finalized().end;
        assert_eq!(UnixNano::new(4_073_680_924_000_000_000), got);
    }

    #[tokio::test]
    async fn test_inexact_propagation() {
        let rec_id = "1717030871000000000_m1";
        let dirs = Box::new(MapFs([map_fs_item(rec_id)].into_iter().flatten().collect()));

        let query = RecDbQuery {
            recording_id: RecordingId::max(),
            end: None,
            limit: NonZeroUsize::new(1).unwrap(),
            oldest_first: false,
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
        assert_eq!(rec_id, rec[0].assert_finalized().id.as_string());
    }

    #[tokio::test]
    async fn test_recording_by_query_end() {
        let c = Crawler::new(crawler_test_fs());
        let query = RecDbQuery {
            recording_id: RecordingId::max(),
            end: Some(UnixNano::new(4_000_000_000_000_011)),
            limit: NonZeroUsize::new(100).unwrap(),
            oldest_first: false,
            monitors: Vec::new(),
            include_data: false,
        };
        let recordings = c.recordings_by_query(query, HashMap::new()).await.unwrap();

        let mut ids = Vec::new();
        for rec in recordings {
            ids.push(rec.assert_finalized().id.as_string());
        }

        let want = vec![
            "9900000000000011_m1",
            "5000000000000022_m1",
            "5000000000000011_m1",
            "4000000000000011_m2",
            "4000000000000011_m1",
        ];
        assert_eq!(want, ids);
    }
    #[tokio::test]
    async fn test_recording_by_query_end_reverse() {
        let c = Crawler::new(crawler_test_fs());
        let query = RecDbQuery {
            recording_id: RecordingId::zero(),
            end: Some(UnixNano::new(4_000_000_000_000_011)),
            limit: NonZeroUsize::new(100).unwrap(),
            oldest_first: true,
            monitors: Vec::new(),
            include_data: false,
        };
        let recordings = c.recordings_by_query(query, HashMap::new()).await.unwrap();

        let mut ids = Vec::new();
        for rec in recordings {
            ids.push(rec.assert_finalized().id.as_string());
        }

        let want = vec![
            "1000000000000011_m1",
            "1000000000000022_m1",
            "1010000000000011_m1",
            "2010000000000011_m1",
            "3000000000000011_m1",
            "4000000000000011_m1",
            "4000000000000011_m2",
        ];
        assert_eq!(want, ids);
    }

    #[test_case("3_m1", "2_m1", false)]
    #[test_case("4_m1", "2_m1", false)]
    #[test_case("3_m1", "4_m1", true)]
    #[test_case("2_m1", "4_m1", true)]
    #[tokio::test]
    async fn test_file_direction(input: &str, want: &str, oldest_first: bool) {
        let dirs = Box::new(MapFs(
            [
                map_fs_item("1_m1"),
                map_fs_item("2_m1"),
                map_fs_item("4_m1"),
                map_fs_item("5_m1"),
            ]
            .into_iter()
            .flatten()
            .collect(),
        ));

        let query = RecDbQuery {
            recording_id: r_id(input),
            end: None,
            limit: NonZeroUsize::new(1).unwrap(),
            oldest_first,
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
        assert_eq!(want, rec[0].assert_finalized().id.as_string());
    }

    #[test_case("893_m1", "890_m2", false)]
    #[test_case("896_m1", "890_m2", false)]
    #[test_case("893_m1", "896_m1", true)]
    #[test_case("890_m2", "896_m1", true)]
    #[tokio::test]
    async fn test_day_direction(input: &str, want: &str, oldest_first: bool) {
        let dirs = Box::new(MapFs(
            [
                map_fs_item("884_m1"),
                map_fs_item("890_m2"),
                map_fs_item("896_m1"),
                map_fs_item("902_m2"),
                map_fs_item("908_m1"),
            ]
            .into_iter()
            .flatten()
            .collect(),
        ));

        let query = RecDbQuery {
            recording_id: r_id(input),
            end: None,
            limit: NonZeroUsize::new(1).unwrap(),
            oldest_first,
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
        assert_eq!(want, rec[0].assert_finalized().id.as_string());
    }
}
