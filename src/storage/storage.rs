// SPDX-License-Identifier: GPL-2.0-or-later

use async_trait::async_trait;
use bytesize::{ByteSize, GB};
use common::{
    time::{Duration, UnixNano},
    DynLogger, LogEntry, LogLevel,
};
use recording::RecordingId;
use std::{
    path::{Path, PathBuf},
    sync::Arc,
};
use thiserror::Error;
use tokio::sync::Mutex;
use tokio_util::sync::CancellationToken;

pub struct StoragePruner<'a> {
    recordings_dir: PathBuf,
    disk: Arc<Disk<'a>>,
    logger: DynLogger,
}

#[derive(Debug, Error)]
enum PruneError {
    #[error("usage: {0}")]
    Usage(#[from] UsageError),

    #[error("read dir: {0}")]
    ReadDir(std::io::Error),

    #[error("dir entry: {0}")]
    DirEntry(std::io::Error),

    #[error("remove dir all: {0}")]
    RemoveDirAll(std::io::Error),
}

impl<'a> StoragePruner<'a> {
    pub fn new(recordings_dir: PathBuf, disk: Arc<Disk<'a>>, logger: DynLogger) -> Self {
        StoragePruner {
            recordings_dir,
            disk,
            logger,
        }
    }

    // Checks if disk usage is above 99% and if true deletes all files from the oldest day.
    async fn prune(&self) -> Result<(), PruneError> {
        use PruneError::*;
        let usage = self.disk.usage(Duration::from_minutes(10)).await?;

        if usage.percent < 99.0 {
            return Ok(());
        }

        const DAY_DEPTH: u8 = 3;

        // Find the oldest day.
        let mut path = self.recordings_dir.to_owned();

        let mut depth = 1;
        while depth <= DAY_DEPTH {
            let path2 = path.to_owned();
            let entries = tokio::task::spawn_blocking(move || std::fs::read_dir(path2))
                .await
                .unwrap()
                .map_err(ReadDir)?;

            let mut list = Vec::new();
            for entry in entries {
                list.push(entry.map_err(DirEntry)?)
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

                path = self.recordings_dir.to_owned();
                depth = 1;
                continue;
            }

            list.sort_by_key(|v| v.path());
            let first_file = list[0].file_name();
            path = path.join(first_file);

            depth += 1;
        }

        self.logger.log(LogEntry {
            level: LogLevel::Info,
            source: "app".parse().unwrap(),
            monitor_id: None,
            message: format!("pruning storage: deleting {:?}", path)
                .parse()
                .unwrap(),
        });

        // Delete all files from that day
        tokio::fs::remove_dir_all(&path)
            .await
            .map_err(RemoveDirAll)?;

        Ok(())
    }

    // Runs `prune()` on an interval until the token is canceled.
    pub async fn prune_loop(&self, token: CancellationToken, interval: std::time::Duration) {
        loop {
            tokio::select! {
                _ = token.cancelled() => return,
                _ = tokio::time::sleep(interval) => {
                    if let Err(e) = self.prune().await {
                        self.logger.log(LogEntry{
                            level: LogLevel::Error,
                            source: "app".parse().unwrap(),
                            monitor_id: None,
                            message: format!("could not prune storage: {}", e).parse().unwrap(),
                        })
                    }
                }
            }
        }
    }
}

#[async_trait]
trait DiskUsager {
    async fn bytes(&self, path: PathBuf) -> Result<u64, UsageBytesError>;
}

// Only used to calculate and cache disk usage.
pub struct Disk<'a> {
    recordings_dir: PathBuf,
    max_disk_usage: ByteSize,
    //storageDirFS   fs.FS
    //diskUsageBytes func(fs.FS) int64
    disk_usage: &'a (dyn DiskUsager + Sync + Send),
    //
    cache: Mutex<Option<DiskCache>>,
    //last_update: Mutex<UnixNano>,
    //cacheLock  sync.Mutex
    update_lock: Mutex<()>,
    //updateLock sync.Mutex
}

#[derive(Clone, Copy)]
struct DiskCache {
    usage: DiskUsage,
    last_update: UnixNano,
}

#[derive(Debug, Error)]
pub enum UsageError {
    #[error("sub")]
    Sub,

    #[error("calculate disk usage: {0}")]
    CalculateDiskUsage(#[from] UsageBytesError),
}

impl<'a> Disk<'a> {
    pub fn new(recordings_dir: PathBuf, max_disk_usage: ByteSize) -> Self {
        Self {
            recordings_dir,
            max_disk_usage,
            cache: Mutex::new(None),
            disk_usage: &DiskUsageBytes,
            //storageDirFS:   storageDirFS,
            update_lock: Mutex::new(()),
        }
    }

    // Returns cached value and age if available.
    pub async fn usage_cached(&self) -> Option<(DiskUsage, Duration)> {
        let cache = self.cache.lock().await.to_owned()?;
        let age = UnixNano::now().sub(cache.last_update)?;
        Some((cache.usage, age))
    }

    // Returns cached value if witin maxAge.
    // Will update and return new value if the cached value is too old.
    pub async fn usage(&self, max_age: Duration) -> Result<DiskUsage, UsageError> {
        use UsageError::*;
        let max_time = UnixNano::now().sub_duration(max_age).ok_or(Sub)?;

        if let Some(cache) = &*self.cache.lock().await {
            if cache.last_update.after(max_time) {
                return Ok(cache.usage.to_owned());
            }
        }

        // Cache is too old, acquire update lock and update it.
        let _update_guard = self.update_lock.lock().await;

        // Check if it was updated while we were waiting for the update lock.
        if let Some(cache) = &*self.cache.lock().await {
            if cache.last_update.after(max_time) {
                return Ok(cache.usage.to_owned());
            }
        }
        // Still outdated.

        let updated_usage = self.calculate_disk_usage().await?;

        *self.cache.lock().await = Some(DiskCache {
            usage: updated_usage.to_owned(),
            last_update: UnixNano::now(),
        });

        Ok(updated_usage)
    }

    async fn calculate_disk_usage(&self) -> Result<DiskUsage, UsageBytesError> {
        let used = self
            .disk_usage
            .bytes(self.recordings_dir.to_owned())
            .await?;
        let percent = (((used * 100) as f64) / (self.max_disk_usage.as_u64() as f64)) as f32;
        let max = self.max_disk_usage.as_u64() / GB;
        Ok(DiskUsage {
            used,
            percent,
            max,
            //formatted: format_disk_usage(used),
        })
    }
}

// DiskUsage in Bytes.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct DiskUsage {
    used: u64,
    percent: f32,
    max: u64,
    //formatted: String,
}

/*fn format_disk_usage(used: u64) -> String {
    if used < 1000 * MB {
        format!("{:.0}", used / MB)
    } else if used < 10 * GB {
        format!("{:.2}", used / GB)
    } else if used < 100 * GB {
        format!("{:.1}", used / GB)
    } else if used < 1000 * GB {
        format!("{:.0}", used / GB)
    } else if used < 10 * TB {
        format!("{:.2}", used / TB)
    } else if used < 100 * TB {
        format!("{:.1}", used / TB)
    } else {
        format!("{:.0}", used / TB)
    }
    /*switch {
    case used < 1000*megabyte:
        return fmt.Sprintf("%.0fMB", used/megabyte)
    case used < 10*gigabyte:
        return fmt.Sprintf("%.2fGB", used/gigabyte)
    case used < 100*gigabyte:
        return fmt.Sprintf("%.1fGB", used/gigabyte)
    case used < 1000*gigabyte:
        return fmt.Sprintf("%.0fGB", used/gigabyte)
    case used < 10*terabyte:
        return fmt.Sprintf("%.2fTB", used/terabyte)
    case used < 100*terabyte:
        return fmt.Sprintf("%.1fTB", used/terabyte)
    default:
        return fmt.Sprintf("%.0fTB", used/terabyte)
    }*/
}*/

struct DiskUsageBytes;

#[derive(Debug, Error)]
pub enum UsageBytesError {
    #[error("read dir: {0} {1}")]
    ReadDir(std::io::Error, PathBuf),

    #[error("dir entry: {0}")]
    DirEntry(std::io::Error),

    #[error("metadata: {0}")]
    Metadata(std::io::Error),
}

#[async_trait]
impl DiskUsager for DiskUsageBytes {
    async fn bytes(&self, path: PathBuf) -> Result<u64, UsageBytesError> {
        tokio::task::spawn_blocking(move || -> Result<u64, UsageBytesError> {
            use UsageBytesError::*;
            let mut total = 0;

            let mut dirs = vec![path];
            while let Some(dir) = dirs.pop() {
                for entry in std::fs::read_dir(&dir).map_err(|e| ReadDir(e, dir.to_owned()))? {
                    let entry = entry.map_err(DirEntry)?;
                    let metadata = entry.metadata().map_err(Metadata)?;

                    total += metadata.len();

                    if metadata.is_dir() {
                        dirs.push(dir.join(entry.file_name()))
                    }
                }
            }
            Ok(total)
        })
        .await
        .unwrap()
    }
}

// PrepareEnvironment prepares directories.
/*func (env ConfigEnv) PrepareEnvironment() error {
    err := os.MkdirAll(env.RecordingsDir(), 0o700)
    if err != nil && !errors.Is(err, os.ErrExist) {
        return fmt.Errorf("create recordings directory: %v: %w", env.StorageDir, err)
    }

    // Make sure env.TempDir isn't set to "/".
    if len(env.TempDir) <= 4 {
        panic(fmt.Sprintf("tempDir sanity check: %v", env.TempDir))
    }
    err = os.RemoveAll(env.TempDir)
    if err != nil {
        return fmt.Errorf("clear tempDir: %v: %w", env.TempDir, err)
    }

    err = os.MkdirAll(env.TempDir, 0o700)
    if err != nil {
        return fmt.Errorf("create tempDir: %v: %w", env.StorageDir, err)
    }

    return nil
}

// CensorLog replaces sensitive env config values.
func (env ConfigEnv) CensorLog(msg string) string {
    if env.StorageDir != "" {
        msg = strings.ReplaceAll(msg, env.StorageDir, "$StorageDir")
    }
    return msg
}*/

#[derive(Debug, Error)]
enum DeleteRecordingError {
    #[error("read dir: {0}")]
    ReadDir(std::io::Error),

    #[error("dir entry: {0}")]
    DirEntry(std::io::Error),

    #[error("remove file: {0}")]
    RemoveFile(std::io::Error),

    #[error("recording does not exist")]
    NotExist,
}

// Delete a recording by ID.
#[allow(unused)]
async fn delete_recording(
    recordings_dir: &Path,
    rec_id: &RecordingId,
) -> Result<(), DeleteRecordingError> {
    use DeleteRecordingError::*;
    let rec_path = rec_id.as_full_path();
    let full_rec_path = recordings_dir.join(rec_path);
    let rec_dir = full_rec_path.parent().expect("path to have parent");

    let mut recording_exists = false;

    let rec_dir2 = rec_dir.to_owned();
    let entries = tokio::task::spawn_blocking(move || std::fs::read_dir(rec_dir2))
        .await
        .unwrap()
        .map_err(ReadDir)?;

    for entry in entries {
        let entry = entry.map_err(DirEntry)?;

        let Ok(name) = entry.file_name().into_string() else {
            continue
        };
        if !name.starts_with(rec_id.as_str()) {
            continue;
        }

        let path = rec_dir.join(entry.file_name());
        tokio::fs::remove_file(path).await.map_err(RemoveFile)?;
        recording_exists = true;
    }

    if !recording_exists {
        return Err(NotExist);
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use bytesize::{ByteSize, MB, TB};
    use common::{new_dummy_logger, time::SECOND};
    use pretty_assertions::assert_eq;
    use tempfile::TempDir;
    use test_case::test_case;
    use tokio::sync::oneshot;

    fn du(used: u64, percent: f32, max: u64) -> DiskUsage {
        DiskUsage { used, percent, max }
    }

    #[test_case(  11*MB,  100*MB, du(       11000000, 11.0,       0); "MB")]
    #[test_case(2345*MB,   10*GB, du(     2345000000, 23.45,     10); "GB2")]
    #[test_case(  22*GB,  100*GB, du(    22000000000, 22.0,     100); "GB1")]
    #[test_case( 234*GB, 1000*GB, du(   234000000000, 23.4,    1000); "GB0")]
    #[test_case(2345*GB,   10*TB, du(  2345000000000, 23.45,  10000); "TB2")]
    #[test_case(  22*TB,  100*TB, du( 22000000000000, 22.0,  100000); "TB1")]
    #[test_case( 234*TB, 1000*TB, du(234000000000000, 23.4, 1000000); "default")]
    #[tokio::test]
    async fn test_disk(used: u64, space: u64, want: DiskUsage) {
        let d = Disk {
            recordings_dir: PathBuf::new(),
            max_disk_usage: ByteSize(space),
            disk_usage: &StubDiskUsage(used),
            cache: Mutex::new(None),
            update_lock: Mutex::new(()),
        };

        let got = d.usage(Duration::from(0)).await.unwrap();
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_disk_cached() {
        let usage = DiskUsage {
            used: 1,
            percent: 0.0,
            max: 0,
        };
        let d = Disk {
            cache: Mutex::new(Some(DiskCache {
                usage,
                last_update: UnixNano::now(),
            })),
            recordings_dir: PathBuf::new(),
            max_disk_usage: ByteSize(0),
            disk_usage: &StubDiskUsage(0),
            update_lock: Mutex::new(()),
        };
        let (got, age) = d.usage_cached().await.unwrap();
        assert_eq!(usage, got);
        assert!(*age < SECOND);
    }

    #[tokio::test]
    async fn test_update_during_lock() {
        let d = Arc::new(Disk {
            recordings_dir: PathBuf::new(),
            max_disk_usage: ByteSize(0),
            disk_usage: &StubDiskUsage(0),
            cache: Mutex::new(None),
            update_lock: Mutex::new(()),
        });
        let update_lock = d.update_lock.lock().await;

        let (result_tx, result_rx) = oneshot::channel();
        let d2 = d.to_owned();
        tokio::spawn(async move {
            let usage = d2.usage(Duration::from_hours(1)).await.unwrap();
            result_tx.send(usage).unwrap();
        });
        tokio::time::sleep(std::time::Duration::from_millis(10)).await;

        let usage = DiskUsage {
            used: 1,
            percent: 0.0,
            max: 0,
        };

        *d.cache.lock().await = Some(DiskCache {
            usage,
            last_update: UnixNano::now(),
        });

        drop(update_lock);
        assert_eq!(usage, result_rx.await.unwrap());
    }

    #[tokio::test]
    async fn test_disk_space_zero() {
        let d = Arc::new(Disk {
            recordings_dir: PathBuf::new(),
            max_disk_usage: ByteSize(0),
            disk_usage: &StubDiskUsage(1000),
            cache: Mutex::new(None),
            update_lock: Mutex::new(()),
        });

        let got = d.usage(Duration::from(0)).await.unwrap();
        let want = DiskUsage {
            used: 1000,
            percent: f32::INFINITY,
            max: 0,
        };
        assert_eq!(want, got);
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
    async fn test_purge(before: &[&str], after: &[&str]) {
        let temp_dir = TempDir::new().unwrap();
        let recordings_dir = temp_dir.path().join("recordings");
        //tempDir := t.TempDir()

        let m = StoragePruner {
            recordings_dir: recordings_dir.to_owned(),
            disk: Arc::new(Disk {
                recordings_dir,
                max_disk_usage: ByteSize(GB),
                disk_usage: &StubDiskUsage(1000000000),
                cache: Mutex::new(None),
                update_lock: Mutex::new(()),
            }),
            logger: new_dummy_logger(),
        };

        write_empty_dirs(temp_dir.path(), before);
        assert_eq!(before, list_empty_dirs(temp_dir.path()));
        m.prune().await.unwrap();

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
                    dirs.push(dir.join(entry.file_name()))
                }
            }
        }

        list.sort();
        list
    }

    /*t.Run("CensorLog", func(t *testing.T) {
        cases := map[string]struct {
            env      ConfigEnv
            input    string
            expected string
        }{
            "emptyConfig": {
                ConfigEnv{},
                "a b c",
                "a b c",
            },
            "storageDir": {
                ConfigEnv{
                    StorageDir: "a",
                },
                "a b c",
                "$StorageDir b c",
            },
        }
        for name, tc := range cases {
            t.Run(name, func(t *testing.T) {
                actual := tc.env.CensorLog(tc.input)
                require.Equal(t, tc.expected, actual)
            })
        }
    }*/

    /*func TestPrepareEnvironment(t *testing.T) {
        t.Run("ok", func(t *testing.T) {
            tempDir, err := os.MkdirTemp("", "")
            require.NoError(t, err)
            defer os.RemoveAll(tempDir)

            env := &ConfigEnv{
                StorageDir: filepath.Join(tempDir, "configs"),
                TempDir:    filepath.Join(tempDir, "temp"),
            }

            // Create test file.
            err = os.Mkdir(env.TempDir, 0o700)
            require.NoError(t, err)
            testFile := filepath.Join(env.TempDir, "test")
            file, err := os.Create(testFile)
            require.NoError(t, err)
            file.Close()

            err = env.PrepareEnvironment()
            require.NoError(t, err)
            require.DirExists(t, env.RecordingsDir())
            require.NoFileExists(t, testFile)
        })
    }*/

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
            rec_id.to_owned() + ".mp4",
            rec_id.to_owned() + ".x",
            "2000-01-01_02-02-02_x1.mp4".to_owned(),
        ];
        std::fs::create_dir_all(&rec_dir).unwrap();
        create_files(&rec_dir, &files);
        assert_eq!(files, list_directory(&rec_dir));

        delete_recording(recordings_dir.path(), &rec_id.parse().unwrap())
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

    struct StubDiskUsage(u64);

    #[async_trait]
    impl DiskUsager for StubDiskUsage {
        async fn bytes(&self, _: PathBuf) -> Result<u64, UsageBytesError> {
            Ok(self.0)
        }
    }
}
