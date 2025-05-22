use common::{MonitorId, write_file_atomic2};
use serde::{Deserialize, Serialize};
use std::{
    collections::HashMap,
    fmt,
    ops::Deref,
    path::{Path, PathBuf},
    sync::Arc,
};
use thiserror::Error;
use tokio::{runtime::Handle, sync::Mutex};

pub type ArcMonitorGroups = Arc<MonitorGroups>;

pub struct MonitorGroups {
    file_path: PathBuf,
    temp_file_path: PathBuf,

    groups: Mutex<Groups>,
}

#[derive(Debug, Error)]
pub enum CreateMonitorGroupsError {
    #[error("read file: {0}")]
    ReadFile(std::io::Error),

    #[error("deserialize: {0}")]
    Deserialize(#[from] serde_json::Error),

    #[error("create file: {0}")]
    CreateFile(std::io::Error),
}

#[derive(Debug, Error)]
pub enum SetMonitorGroupsError {
    #[error("write file: {0}")]
    WriteFile(std::io::Error),

    #[error("rename file: {0}")]
    RenameFile(std::io::Error),
}

impl MonitorGroups {
    pub async fn new(storage_dir: &Path) -> Result<Self, CreateMonitorGroupsError> {
        use CreateMonitorGroupsError::*;
        let file_path = storage_dir.join("monitorGroups.json");
        let temp_file_path = storage_dir.join("monitorGroups.json.tmp");
        let groups = {
            if file_path.exists() {
                let json = tokio::fs::read(&file_path).await.map_err(ReadFile)?;
                tokio::task::spawn_blocking(move || serde_json::from_slice(&json))
                    .await
                    .expect("join")?
            } else {
                HashMap::new()
            }
        };

        Ok(Self {
            file_path,
            temp_file_path,
            groups: Mutex::new(groups),
        })
    }

    pub async fn get(&self) -> Groups {
        self.groups.lock().await.clone()
    }

    pub async fn set(&self, groups: Groups) -> Result<(), SetMonitorGroupsError> {
        let (json, groups) = tokio::task::spawn_blocking(move || {
            let json = serde_json::to_vec_pretty(&groups).expect("should be infallible");
            (json, groups)
        })
        .await
        .expect("join");

        // Hold lock until file is written.
        let mut g = self.groups.lock().await;

        let file_path = self.file_path.clone();
        let temp_file_path = self.temp_file_path.clone();
        write_file_atomic2(Handle::current(), file_path, temp_file_path, json)
            .await
            .map_err(SetMonitorGroupsError::WriteFile)?;

        *g = groups;
        Ok(())
    }
}

pub type Groups = HashMap<GroupId, Group>;

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct Group {
    id: GroupId,
    name: GroupName,
    monitors: Vec<MonitorId>,
}

pub const GROUP_ID_MAX_LENGTH: usize = 24;

#[derive(Clone, Debug, Hash, PartialEq, Eq, Serialize)]
pub struct GroupId(String);

impl fmt::Display for GroupId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.0)
    }
}

#[derive(Debug, Error, PartialEq, Eq)]
pub enum ParseGroupIdError {
    #[error("empty string")]
    Empty,

    #[error("invalid character: '{0}'")]
    InvalidChar(char),

    #[error("max length is {GROUP_ID_MAX_LENGTH}")]
    MaxLength,
}

impl TryFrom<String> for GroupId {
    type Error = ParseGroupIdError;

    fn try_from(s: String) -> Result<Self, Self::Error> {
        use ParseGroupIdError::*;
        if s.is_empty() {
            return Err(Empty);
        }
        for c in s.chars() {
            if !c.is_ascii_alphanumeric() {
                return Err(InvalidChar(c));
            }
        }
        if s.len() > GROUP_ID_MAX_LENGTH {
            return Err(MaxLength);
        }
        Ok(Self(s))
    }
}

impl<'de> Deserialize<'de> for GroupId {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        let s = String::deserialize(deserializer)?;
        TryFrom::try_from(s).map_err(serde::de::Error::custom)
    }
}

impl Deref for GroupId {
    type Target = str;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

pub const GROUP_NAME_MAX_LENGTH: usize = 24;

#[derive(Clone, Debug, Hash, PartialEq, Eq, Serialize)]
pub struct GroupName(String);

impl fmt::Display for GroupName {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.0)
    }
}

#[derive(Debug, Error, PartialEq, Eq)]
pub enum ParseGroupNameError {
    #[error("empty string")]
    Empty,

    #[error("invalid character: '{0}'")]
    InvalidChar(char),

    #[error("max length is {GROUP_NAME_MAX_LENGTH}")]
    MaxLength,
}

const ALLOWED_GROUP_NAME_CHARS: [char; 2] = ['-', '_'];

impl TryFrom<String> for GroupName {
    type Error = ParseGroupIdError;

    fn try_from(s: String) -> Result<Self, Self::Error> {
        use ParseGroupIdError::*;
        if s.is_empty() {
            return Err(Empty);
        }
        for c in s.chars() {
            if !c.is_ascii_alphanumeric() && !ALLOWED_GROUP_NAME_CHARS.contains(&c) {
                return Err(InvalidChar(c));
            }
        }
        if s.len() > GROUP_ID_MAX_LENGTH {
            return Err(MaxLength);
        }
        Ok(Self(s))
    }
}

impl<'de> Deserialize<'de> for GroupName {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        let s = String::deserialize(deserializer)?;
        TryFrom::try_from(s).map_err(serde::de::Error::custom)
    }
}

impl Deref for GroupName {
    type Target = str;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_monitor_groups() {
        let temp_dir = tempfile::tempdir().unwrap();
        let monitor_groups = MonitorGroups::new(temp_dir.path()).await.unwrap();

        assert!(monitor_groups.get().await.is_empty());

        let id1: GroupId = "id1".to_owned().try_into().unwrap();
        let group1 = Group {
            id: id1.clone(),
            name: "name1".to_owned().try_into().unwrap(),
            monitors: vec!["monitor1".to_owned().try_into().unwrap()],
        };
        let map1 = HashMap::from([(id1.clone(), group1)]);

        monitor_groups.set(map1.clone()).await.unwrap();
        assert_eq!(map1, monitor_groups.get().await);

        drop(monitor_groups);
        let monitor_groups = MonitorGroups::new(temp_dir.path()).await.unwrap();
        assert_eq!(map1, monitor_groups.get().await);
    }

    #[test]
    fn test_parse_group_name() {
        GroupName::try_from("a".to_owned()).unwrap();
        GroupName::try_from("1-1".to_owned()).unwrap();
        GroupName::try_from("1_1".to_owned()).unwrap();

        GroupName::try_from(String::new()).unwrap_err();
        GroupName::try_from("a a".to_owned()).unwrap_err();
        GroupName::try_from("{".to_owned()).unwrap_err();
        GroupName::try_from("(".to_owned()).unwrap_err();
        GroupName::try_from("<".to_owned()).unwrap_err();
    }
}
