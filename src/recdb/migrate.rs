// SPDX-License-Identifier: GPL-2.0-or-later

use common::{
    ILogger, MonitorId, ParseMonitorIdError,
    recording::{RecordingId, RecordingIdError},
    time::UnixNano,
};
use jiff::{
    Timestamp,
    civil::{Date, DateTime, Time},
    tz,
};
use recording::{HeaderFromReaderError, MetaHeader};
use serde::{Deserialize, Serialize};
use std::{
    fs::DirEntry,
    io::Write,
    num::ParseIntError,
    path::{Path, PathBuf},
};
use thiserror::Error;

#[allow(clippy::unwrap_used)]
pub async fn migrate(storage_dir: &Path) {
    let rec_dir1 = storage_dir.join("recordings");
    let rec_dir2 = storage_dir.join("recordings2").join("main");
    if !rec_dir1.exists() {
        // Nothing to migrate.
        return;
    }

    let mut log = Logger::new(&storage_dir.join("recdb_migration_log.txt"));
    log.println("Migrating 'storage_dir/recordings' to 'storage_dir/recordings2'");

    // Find recordings.
    let mut recordings = Vec::new();
    for year in rec_dir1.read_dir().unwrap() {
        for month in year.unwrap().path().read_dir().unwrap() {
            for day in month.unwrap().path().read_dir().unwrap() {
                for m_id in day.unwrap().path().read_dir().unwrap() {
                    for file in m_id.unwrap().path().read_dir().unwrap() {
                        let file = file.unwrap();
                        if !file.path().is_file() {
                            continue;
                        }
                        if let Some(ext) = file.path().extension() {
                            if ext == "meta" {
                                let mut path = file.path();
                                path.set_extension("");
                                recordings.push(path);
                            }
                        };
                    }
                }
            }
        }
    }
    log.println(&format!("Migrating {0} recordings", recordings.len()));

    let num_recordings = recordings.len();
    let mut i = 0;
    for rec in recordings {
        i += 1;
        match migrate_recording(&mut log, &rec_dir2, rec.clone()).await {
            Ok(()) => {
                let name = rec.file_name().unwrap().to_str().unwrap();
                log.println(&format!("[{i}/{num_recordings}][OK] {name}",));
            }
            Err(e) => log.println(&format!("[{i}/{num_recordings}][ERR] {e}")),
        }
    }

    // Delete empty directories.
    log.debug("deleting empty directories");
    for _ in 0..5 {
        if !rec_dir1.exists() {
            break;
        }
        let mut items = vec![rec_dir1.clone()];
        while let Some(item) = items.pop() {
            let entries = list_dir(&item).unwrap();
            if entries.is_empty() {
                std::fs::remove_dir(item).unwrap();
            }
            for entry in entries {
                if entry.path().is_dir() {
                    items.push(entry.path());
                }
            }
        }
    }

    if rec_dir1.exists() {
        log.println("Migration incomplete. Everything in 'storage_dir/recording' should have been migrated. Logs written to 'storage_dir/recdb_migration_log.txt'");
    } else {
        log.println("Migration successful!");
    }
}

fn list_dir(path: &Path) -> Result<Vec<DirEntry>, std::io::Error> {
    let mut output = Vec::new();
    for entry in path.read_dir()? {
        output.push(entry?);
    }
    Ok(output)
}

#[derive(Debug, Error)]
enum MigrateRecordingError {
    #[error("get file name: {0:?}")]
    GetFileName(PathBuf),

    #[error("parse file name: {0:?}")]
    ParseFileName(PathBuf),

    #[error("parse recording id: {0}")]
    ParseRecordingId(#[from] RecordingIdError1),

    #[error("convert recording id: {0}")]
    ConvertRecordingId(#[from] RecordingIdError),

    #[error("read data: {0}")]
    ReadDataFile(std::io::Error),

    #[error("parse data: {0}")]
    ParseData(#[from] serde_json::Error),

    #[error("create new recording dir: {0}")]
    CreateNewRecDir(std::io::Error),

    #[error("create end file: {0}")]
    CreateEndFile(std::io::Error),

    #[error("get image size: {0}")]
    GetImageSize(#[from] imagesize::ImageError),

    #[error("rename thumbnail file: {0}")]
    RenameThumbnail(std::io::Error),

    #[error("open meta file: {0}")]
    OpenMetaFile(std::io::Error),

    #[error("parse meta header: {0}")]
    ParseMetaHeader(#[from] HeaderFromReaderError),

    #[error("rename meta file: {0}")]
    RenameMetaFile(std::io::Error),

    #[error("rename mdat file: {0}")]
    RenameMdatFile(std::io::Error),

    #[error("delete json file: {0}")]
    RemoveJsonFile(std::io::Error),
}

async fn migrate_recording(
    log: &mut Logger,
    rec_dir2: &Path,
    mut path: PathBuf,
) -> Result<(), MigrateRecordingError> {
    use MigrateRecordingError::*;

    path.set_extension("");
    let Some(file_name) = path.file_name() else {
        return Err(GetFileName(path));
    };
    let Some(file_name) = file_name.to_str() else {
        return Err(ParseFileName(path));
    };
    let recid = RecordingId1::from_string(file_name)?;

    let (start_time, end_time) = {
        path.set_extension("json");
        if path.exists() {
            let data_json = std::fs::read_to_string(&path).map_err(ReadDataFile)?;
            let data: RecordingData = serde_json::from_str(&data_json)?;
            (data.start, Some(data.end))
        } else {
            (recid.nanos_inexact(), None)
        }
    };

    let recid2 = RecordingId::from_nanos(start_time, recid.monitor_id().to_owned())?;
    let new_path = rec_dir2.join(recid2.full_path());
    std::fs::create_dir_all(&new_path).map_err(CreateNewRecDir)?;

    // Write end file.
    if let Some(end_time) = end_time {
        let end_file_path = new_path.join(format!("{end_time}.end"));
        log.debug(&format!("touch {end_file_path:?}"));
        touch_file(&end_file_path).map_err(CreateEndFile)?;
    }

    // Move thumbnail.
    path.set_extension("jpeg");
    let size = imagesize::size(&path)?;
    let new_thumbnail_path = new_path.join(format!("thumb_{}x{}.jpeg", size.width, size.height));
    log.debug(&format!("mv {path:?} {new_thumbnail_path:?}"));
    std::fs::rename(&path, new_thumbnail_path).map_err(RenameThumbnail)?;

    // Move video.
    path.set_extension("meta");
    let mut meta_file = tokio::fs::OpenOptions::new()
        .read(true)
        .open(&path)
        .await
        .map_err(OpenMetaFile)?;
    let header = MetaHeader::from_reader(&mut meta_file).await?;
    let name = format!("video_{}x{}", header.width, header.height);
    let new_meta_path = new_path.join(format!("{name}.meta"));
    log.debug(&format!("mv {:?} {:?}", &path, new_meta_path));
    std::fs::rename(&path, new_meta_path).map_err(RenameMetaFile)?;

    let new_mdat_path = new_path.join(format!("{name}.mdat"));
    path.set_extension("mdat");
    log.debug(&format!("mv {:?} {:?}", &path, new_mdat_path));
    std::fs::rename(&path, new_mdat_path).map_err(RenameMdatFile)?;

    // Delete json file.
    path.set_extension("json");
    std::fs::remove_file(&path).map_err(RemoveJsonFile)?;

    Ok(())
}

fn touch_file(path: &Path) -> Result<(), std::io::Error> {
    std::fs::OpenOptions::new()
        .create(true)
        .truncate(false)
        .write(true)
        .open(path)
        .map(|_| ())
}

struct Logger(std::fs::File);

#[allow(clippy::unwrap_used)]
impl Logger {
    fn new(file: &Path) -> Self {
        let file = std::fs::OpenOptions::new()
            .create(true)
            .append(true)
            .open(file)
            .unwrap();
        Logger(file)
    }
    fn println(&mut self, s: &str) {
        println!("{s}");
        writeln!(self.0, "{s}").unwrap();
    }
    fn debug(&mut self, s: &str) {
        writeln!(self.0, "{s}").unwrap();
    }
}

impl ILogger for Logger {
    fn log(&self, entry: common::LogEntry) {
        panic!("{entry:?}");
    }
}

#[derive(Debug, PartialEq, Serialize, Deserialize)]
struct RecordingData {
    start: UnixNano,
    end: UnixNano,
    //events: Vec<Event>,
}

struct RecordingId1 {
    nanos: UnixNano,
    monitor_id: MonitorId,
}

impl RecordingId1 {
    fn from_string(s: &str) -> Result<Self, RecordingIdError1> {
        use RecordingIdError1::*;
        let b = s.as_bytes();
        if b.len() < 20 {
            return Err(InvalidString(s.to_owned()));
        }

        // "xxxx-xx-xx_xx-xx-xx_x"
        if b[4] != b'-'
            || b[7] != b'-'
            || b[10] != b'_'
            || b[13] != b'-'
            || b[16] != b'-'
            || b[19] != b'_'
        {
            return Err(InvalidString(s.to_owned()));
        }

        let year: u16 = s[..4].parse().map_err(InvalidYear)?;
        let month: u8 = s[5..7].parse().map_err(InvalidMonth)?;
        let day: u8 = s[8..10].parse().map_err(InvalidDay)?;
        let hour: u8 = s[11..13].parse().map_err(InvalidHour)?;
        let minute: u8 = s[14..16].parse().map_err(InvalidMinute)?;
        let second: u8 = s[17..19].parse().map_err(InvalidSecond)?;
        let monitor_id = MonitorId::try_from(s[20..].to_owned())?;

        let Ok(year2) = i16::try_from(year) else {
            return Err(BadYear(year));
        };
        if month > 12 {
            return Err(BadMonth(month));
        }
        let Ok(month2) = i8::try_from(month) else {
            return Err(BadMonth(month));
        };
        if day > 31 {
            return Err(BadDay(day));
        }
        let Ok(day2) = i8::try_from(day) else {
            return Err(BadDay(day));
        };
        if hour > 24 {
            return Err(BadHour(hour));
        }
        let Ok(hour2) = i8::try_from(hour) else {
            return Err(BadHour(hour));
        };
        if minute > 60 {
            return Err(BadMinute(minute));
        }
        let Ok(minute2) = i8::try_from(minute) else {
            return Err(BadMinute(minute));
        };
        if second > 60 {
            return Err(BadSecond(second));
        }
        let Ok(second2) = i8::try_from(second) else {
            return Err(BadSecond(second));
        };

        let date = DateTime::from_parts(
            Date::new(year2, month2, day2).map_err(ParseDate)?,
            Time::new(hour2, minute2, second2, 0).map_err(ParseTime)?,
        );
        let zoned = date.to_zoned(tz::TimeZone::UTC).map_err(Zoned)?;
        let time = zoned.timestamp();
        let nanos = UnixNano::new(
            time.as_nanosecond()
                .try_into()
                .map_err(|_| ConvertToNanos(time))?,
        );

        if nanos.is_negative() {
            return Err(NegativeTime(nanos));
        }

        Ok(Self { nanos, monitor_id })
    }

    #[must_use]
    fn nanos_inexact(&self) -> UnixNano {
        self.nanos
    }

    #[must_use]
    fn monitor_id(&self) -> &MonitorId {
        &self.monitor_id
    }
}

#[derive(Debug, thiserror::Error)]
#[allow(clippy::module_name_repetitions)]
enum RecordingIdError1 {
    #[error("invalid string: {0}")]
    InvalidString(String),

    #[error("invalid year: {0}")]
    InvalidYear(ParseIntError),
    #[error("invalid month: {0}")]
    InvalidMonth(ParseIntError),
    #[error("invalid day: {0}")]
    InvalidDay(ParseIntError),

    #[error("invalid hour: {0}")]
    InvalidHour(ParseIntError),
    #[error("invalid minute: {0}")]
    InvalidMinute(ParseIntError),
    #[error("invalid second: {0}")]
    InvalidSecond(ParseIntError),

    #[error("invalid monitor id: {0}")]
    InvalidMonitorId(#[from] ParseMonitorIdError),

    #[error("bad year: {0}")]
    BadYear(u16),
    #[error("bad month: {0}")]
    BadMonth(u8),
    #[error("bad day: {0}")]
    BadDay(u8),
    #[error("bad hour: {0}")]
    BadHour(u8),
    #[error("bad minute: {0}")]
    BadMinute(u8),
    #[error("bad second: {0}")]
    BadSecond(u8),

    #[error("parse date: {0}")]
    ParseDate(jiff::Error),

    #[error("parse time: {0}")]
    ParseTime(jiff::Error),

    #[error("zoned: {0}")]
    Zoned(jiff::Error),

    #[error("can't convert to nanos: {0}")]
    ConvertToNanos(Timestamp),

    #[error("time is negative: {0:?}")]
    NegativeTime(UnixNano),
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {
    use super::*;

    use tempfile::TempDir;

    /*
    fn m_id(v: &str) -> MonitorId {
        v.to_owned().try_into().unwrap()
    }

    fn time(s: &str) -> UnixH264 {
        let ts: jiff::Timestamp = s.parse().unwrap();
        UnixNano::new((ts.as_nanosecond()).try_into().unwrap()).into()
    }*/

    /*
    #[tokio::test]
    async fn generate_test_data() {
        let v1_dir = std::env::current_dir()
            .unwrap()
            .join("testdata")
            .join("migrate")
            .join("v1")
            .join("recordings");

        let recdb = RecDb::new(DummyLogger::new(), v1_dir, DummyStorage::new()).await;

        let time = time("2000-01-02T03:04:05Z");
        let start_time = UnixNano::from(time);
        let rec = recdb.new_recording(m_id("m1"), time).await.unwrap();
        let mut meta = rec.new_file("meta").await.unwrap();
        meta.write_all(&[
            1, // Version.
            0, 0, 0, 0, 0x3b, 0x9a, 0xca, 0, // Start time.
            0, 0x50, // Width.
            0, 0x40, // Height.
            0, 2, // Extra data size.
            0, 1, // Extra data.
        ])
        .await
        .unwrap();
        rec.new_file("mdat").await.unwrap();
        rec.new_file("jpeg").await.unwrap();
        let mut json_file = rec.new_file("json").await.unwrap();
        let data = serde_json::to_vec_pretty(&RecordingData {
            start: start_time,
            end: start_time.checked_add(UnixNano::new(100 * SECOND)).unwrap(),
            events: Vec::new(),
        })
        .unwrap();
        json_file.write_all(&data).await.unwrap();

        panic!("");
    }
    */

    /*
    #[tokio::test]
    async fn generate_test_data2() {
        let v2_dir = std::env::current_dir()
            .unwrap()
            .join("testdata")
            .join("migrate")
            .join("v2")
            .join("main");

        let recdb = RecDb2::new(DummyLogger::new(), v2_dir, DummyStorage::new())
            .await
            .unwrap();

        let time = time("2000-01-02T03:04:05Z");
        let start_time = UnixNano::from(time);
        let rec = recdb.new_recording(m_id("m1"), time).await.unwrap();
        let meta = rec.new_file("video_80x64.meta").await.unwrap();
        meta.write_all(&[
            1, // Version.
            0, 0, 0, 0, 0x3b, 0x9a, 0xca, 0, // Start time.
            0, 0x50, // Width.
            0, 0x40, // Height.
            0, 2, // Extra data size.
            0, 1, // Extra data.
        ])
        .await
        .unwrap();
        rec.new_file("video_80x64.mdat").await.unwrap();
        rec.new_file("946782345000000000.end").await.unwrap();

        panic!("AAAA");
    }
    */

    #[tokio::test]
    async fn test_migrate() {
        let testdata = std::env::current_dir()
            .unwrap()
            .join("testdata")
            .join("migrate");
        let v1_dir = testdata.join("v1");
        let v2_dir = testdata.join("v2");

        let temp_dir = TempDir::new().unwrap();
        let path = temp_dir.path();

        copy_dir(&v1_dir, path);
        assert_paths(&v1_dir, path);

        migrate(path).await;
        std::fs::remove_file(path.join("recdb_migration_log.txt")).unwrap();
        assert_paths(&v2_dir, path);
    }

    #[track_caller]
    fn copy_dir(src: &Path, dst: &Path) {
        let mut items = list_dir(src);
        while let Some(item) = items.pop() {
            let src_path = src.join(&item);
            let dst_path = dst.join(&item);
            if src_path.is_dir() {
                std::fs::create_dir(dst_path).unwrap();
                items.extend(list_dir(&src_path).into_iter().map(|v| item.join(v)));
            } else {
                std::fs::copy(src_path, dst_path).unwrap();
            }
        }
    }

    fn list_dir(path: &Path) -> Vec<PathBuf> {
        let mut items: Vec<_> = path
            .read_dir()
            .unwrap()
            .map(|v| PathBuf::from(v.unwrap().file_name().to_str().unwrap().to_owned()))
            .collect();
        items.sort();
        items
    }

    #[track_caller]
    fn assert_paths(left: &Path, right: &Path) {
        assert_eq!(list_dir(left), list_dir(right));

        let mut items = list_dir(left);
        while let Some(item) = items.pop() {
            let left_path = left.join(&item);
            let right_path = right.join(&item);
            if left_path.is_dir() {
                assert_eq!(list_dir(&left_path), list_dir(&right_path));
                items.extend(list_dir(&left_path).into_iter().map(|v| item.join(v)));
            } else {
                let left_data = std::fs::read(left_path).unwrap();
                let right_data = std::fs::read(right_path).unwrap();
                assert!((left_data == right_data), "{item:?} file contents differ");
            }
        }
    }
}
