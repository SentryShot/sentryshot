// SPDX-License-Identifier: GPL-2.0-or-later

use async_trait::async_trait;
use bytesize::{ByteSize, GB};
use common::time::{Duration, UnixNano};
use std::path::PathBuf;
use thiserror::Error;
use tokio::sync::Mutex;

#[async_trait]
pub(crate) trait DiskBytesUsed {
    async fn bytes(&self, path: PathBuf) -> Result<u64, UsageBytesError>;
}

// Only used to calculate and cache disk usage.
#[allow(clippy::struct_field_names)]
pub struct Disk {
    storage_dir: PathBuf,
    max_disk_usage: ByteSize,
    disk_usage: Box<dyn DiskBytesUsed + Send + Sync>,

    cache: Mutex<Option<DiskCache>>,
    update_lock: Mutex<()>,
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

impl Disk {
    #[must_use]
    pub fn new(storage_dir: PathBuf, max_disk_usage: ByteSize) -> Self {
        Self {
            storage_dir,
            max_disk_usage,
            cache: Mutex::new(None),
            disk_usage: Box::new(DiskUsageBytes),
            update_lock: Mutex::new(()),
        }
    }

    #[must_use]
    #[cfg(test)]
    pub(crate) fn with_disk_usage(
        storage_dir: PathBuf,
        max_disk_usage: ByteSize,
        disk_usage: Box<dyn DiskBytesUsed + Send + Sync>,
    ) -> Self {
        Self {
            storage_dir,
            max_disk_usage,
            cache: Mutex::new(None),
            disk_usage,
            update_lock: Mutex::new(()),
        }
    }

    pub(crate) async fn usage(&self, max_age: Duration) -> Result<DiskUsage, UsageError> {
        use UsageError::*;
        let max_time = UnixNano::now().sub_duration(max_age).ok_or(Sub)?;

        if let Some(cache) = &*self.cache.lock().await {
            if cache.last_update.after(max_time) {
                return Ok(cache.usage);
            }
        }

        // Cache is too old, acquire update lock and update it.
        let _update_guard = self.update_lock.lock().await;

        // Check if it was updated while we were waiting for the update lock.
        if let Some(cache) = &*self.cache.lock().await {
            if cache.last_update.after(max_time) {
                return Ok(cache.usage);
            }
        }
        // Still outdated.

        let updated_usage = self.calculate_disk_usage().await?;

        *self.cache.lock().await = Some(DiskCache {
            usage: updated_usage,
            last_update: UnixNano::now(),
        });

        Ok(updated_usage)
    }

    // Returns cached value and age if available.
    #[allow(unused)]
    async fn usage_cached(&self) -> Option<(DiskUsage, Duration)> {
        let cache = self.cache.lock().await.to_owned()?;
        let age = UnixNano::now().sub(cache.last_update)?;
        Some((cache.usage, age))
    }

    #[allow(
        clippy::cast_precision_loss,
        clippy::cast_possible_truncation,
        clippy::as_conversions
    )]
    async fn calculate_disk_usage(&self) -> Result<DiskUsage, UsageBytesError> {
        let used = self.disk_usage.bytes(self.storage_dir.clone()).await?;
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
#[allow(clippy::module_name_repetitions)]
pub struct DiskUsage {
    pub(crate) used: u64,
    pub(crate) percent: f32,
    pub(crate) max: u64,
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
impl DiskBytesUsed for DiskUsageBytes {
    async fn bytes(&self, path: PathBuf) -> Result<u64, UsageBytesError> {
        tokio::task::spawn_blocking(move || -> Result<u64, UsageBytesError> {
            use UsageBytesError::*;
            let mut total = 0;

            let mut dirs = vec![path];
            while let Some(dir) = dirs.pop() {
                for entry in std::fs::read_dir(&dir).map_err(|e| ReadDir(e, dir.clone()))? {
                    let entry = entry.map_err(DirEntry)?;
                    let metadata = entry.metadata().map_err(Metadata)?;

                    total += metadata.len();

                    if metadata.is_dir() {
                        dirs.push(dir.join(entry.file_name()));
                    }
                }
            }
            Ok(total)
        })
        .await
        .expect("join")
    }
}

/*
// CensorLog replaces sensitive env config values.
func (env ConfigEnv) CensorLog(msg string) string {
    if env.StorageDir != "" {
        msg = strings.ReplaceAll(msg, env.StorageDir, "$StorageDir")
    }
    return msg
}*/

pub(crate) struct StubDiskUsageBytes(pub u64);

#[async_trait]
impl DiskBytesUsed for StubDiskUsageBytes {
    async fn bytes(&self, _: PathBuf) -> Result<u64, UsageBytesError> {
        Ok(self.0)
    }
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use bytesize::{ByteSize, MB, TB};
    use common::time::SECOND;
    use pretty_assertions::assert_eq;
    use std::sync::Arc;
    use test_case::test_case;
    use tokio::sync::oneshot;

    fn du(used: u64, percent: f32, max: u64) -> DiskUsage {
        DiskUsage { used, percent, max }
    }

    #[test_case(  11*MB,  100*MB, du(         11_000_000, 11.0,         0); "MB")]
    #[test_case(2345*MB,   10*GB, du(      2_345_000_000, 23.45,       10); "GB2")]
    #[test_case(  22*GB,  100*GB, du(     22_000_000_000, 22.0,       100); "GB1")]
    #[test_case( 234*GB, 1000*GB, du(    234_000_000_000, 23.4,      1000); "GB0")]
    #[test_case(2345*GB,   10*TB, du(  2_345_000_000_000, 23.45,    10000); "TB2")]
    #[test_case(  22*TB,  100*TB, du( 22_000_000_000_000, 22.0,   100_000); "TB1")]
    #[test_case( 234*TB, 1000*TB, du(234_000_000_000_000, 23.4, 1_000_000); "default")]
    #[tokio::test]
    async fn test_disk(used: u64, space: u64, want: DiskUsage) {
        let d = Disk::with_disk_usage(
            PathBuf::new(),
            ByteSize(space),
            Box::new(StubDiskUsageBytes(used)),
        );
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
            storage_dir: PathBuf::new(),
            max_disk_usage: ByteSize(0),
            disk_usage: Box::new(StubDiskUsageBytes(0)),
            update_lock: Mutex::new(()),
        };
        let (got, age) = d.usage_cached().await.unwrap();
        assert_eq!(usage, got);
        assert!(*age < SECOND);
    }

    #[tokio::test]
    async fn test_update_during_lock() {
        let d = Arc::new(Disk::with_disk_usage(
            PathBuf::new(),
            ByteSize(0),
            Box::new(StubDiskUsageBytes(0)),
        ));
        let update_lock = d.update_lock.lock().await;

        let (result_tx, result_rx) = oneshot::channel();
        let d2 = d.clone();
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
        let d = Disk::with_disk_usage(
            PathBuf::new(),
            ByteSize(0),
            Box::new(StubDiskUsageBytes(1000)),
        );

        let got = d.usage(Duration::from(0)).await.unwrap();
        let want = DiskUsage {
            used: 1000,
            percent: f32::INFINITY,
            max: 0,
        };
        assert_eq!(want, got);
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
}
