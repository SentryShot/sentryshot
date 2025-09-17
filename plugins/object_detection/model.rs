// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{DynFetcher, FetchError, Fetcher};
use common::{ArcMsgLogger, LogLevel, write_file_atomic};
use serde::Deserialize;
use sha2::{Digest, Sha256};
use std::{
    collections::HashMap,
    ffi::OsString,
    fmt::{Debug, Display},
    num::ParseIntError,
    ops::Deref,
    path::PathBuf,
    str::FromStr,
};
use thiserror::Error;
use url::Url;

pub(crate) struct ModelCache {
    logger: ArcMsgLogger,
    fetcher: Box<dyn Fetcher>,
    path: PathBuf,
    models: HashMap<ModelChecksum, PathBuf>,
}

#[derive(Debug, Error)]
pub(crate) enum CreateModelCacheError {
    #[error("read directory: {0}")]
    ReadDir(std::io::Error),

    #[error("read entry: {0}")]
    ReadEntry(std::io::Error),

    #[error("parse file name: {0:?}")]
    ParseFileName(OsString),

    #[error("parse checksum: {0}")]
    ParseChechsum(#[from] ParseChecksumError),
}

#[derive(Debug, Error)]
pub(crate) enum ModelCacheError {
    #[error("fetch: {0}")]
    Fetch(#[from] FetchError),

    #[error("bad checksum: want: {0} got: {1}")]
    BadChecksum(ModelChecksum, ModelChecksum),

    #[error("write file: {0}")]
    WriteFile(std::io::Error),

    #[error("remove temp file: {0}")]
    RemoveTempFile(std::io::Error),
}

impl ModelCache {
    pub(crate) fn new(
        logger: ArcMsgLogger,
        fetcher: DynFetcher,
        path: PathBuf,
    ) -> Result<Self, CreateModelCacheError> {
        use CreateModelCacheError::*;
        let mut models = HashMap::new();
        for entry in std::fs::read_dir(&path).map_err(ReadDir)? {
            let entry = entry.map_err(ReadEntry)?;
            let file_name = entry
                .file_name()
                .to_str()
                .ok_or_else(|| ParseFileName(entry.file_name()))?
                .to_owned();
            let checksum: ModelChecksum = match file_name.parse() {
                Ok(v) => v,
                Err(e) => {
                    logger.log(
                        LogLevel::Warning,
                        &format!("failed to parse filename checksum: {e} {file_name}"),
                    );
                    continue;
                }
            };
            models.insert(checksum, entry.path());
        }
        Ok(Self {
            logger,
            fetcher,
            path,
            models,
        })
    }

    pub(crate) async fn fetch(
        &self,
        url: &Url,
        checksum: &ModelChecksum,
    ) -> Result<PathBuf, ModelCacheError> {
        use ModelCacheError::*;
        if let Some(model_path) = self.models.get(checksum) {
            return Ok(model_path.to_owned());
        }
        self.logger
            .log(LogLevel::Info, &format!("downloading model '{url}'"));
        let raw_model = self.fetcher.fetch(url).await?;

        let checksum_got = calculate_checksum(&raw_model);
        if &checksum_got != checksum {
            return Err(ModelCacheError::BadChecksum(checksum.clone(), checksum_got));
        }

        let file_name = checksum.as_string();
        let file_path = self.path.join(file_name);

        let mut temp_path = file_path.clone();
        temp_path.set_extension("tmp");
        if temp_path.exists() {
            std::fs::remove_file(&temp_path).map_err(RemoveTempFile)?;
        }

        write_file_atomic(&file_path, &temp_path, &raw_model).map_err(WriteFile)?;
        Ok(file_path)
    }
}

#[derive(Clone, Hash, PartialEq, Eq)]
pub(crate) struct ModelChecksum([u8; 32]);

impl ModelChecksum {
    pub(crate) fn new(v: [u8; 32]) -> Self {
        Self(v)
    }

    pub(crate) fn as_string(&self) -> String {
        use std::fmt::Write;
        let mut s = String::with_capacity(self.0.len() * 2);
        for &b in &self.0 {
            write!(&mut s, "{b:02x}").unwrap();
        }
        s
    }
}

impl Debug for ModelChecksum {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.as_string())
    }
}

impl Display for ModelChecksum {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{self:?}")
    }
}

#[derive(Debug, Error)]
pub(crate) enum ParseChecksumError {
    #[error("{0}")]
    ParseInt(#[from] ParseIntError),

    #[error("bad size: {0}")]
    BadSize(usize),
}

impl FromStr for ModelChecksum {
    type Err = ParseChecksumError;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        use ParseChecksumError::*;
        let decoded = (0..s.len())
            .step_by(2)
            .map(|i| {
                let src = &s.get(i..i + 2).ok_or(BadSize((i + 2) / 2))?;
                Ok(u8::from_str_radix(src, 16)?)
            })
            .collect::<Result<Vec<u8>, ParseChecksumError>>()?;

        Ok(Self(
            decoded.try_into().map_err(|e: Vec<u8>| BadSize(e.len()))?,
        ))
    }
}

impl<'de> Deserialize<'de> for ModelChecksum {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        let s = String::deserialize(deserializer)?;
        FromStr::from_str(&s).map_err(serde::de::Error::custom)
    }
}

impl Deref for ModelChecksum {
    type Target = [u8];

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

fn calculate_checksum(data: &[u8]) -> ModelChecksum {
    let mut hasher = Sha256::new();
    hasher.update(data);
    let checksum = hasher.finalize();

    ModelChecksum::new(
        checksum
            .as_slice()
            .try_into()
            .expect("generic bullshit to be size 32"),
    )
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_calculate_checksum() {
        assert_eq!(
            calculate_checksum(&[1, 2, 3]),
            "039058c6f2c0cb492c533b0a4d14ef77cc0f78abccced5287d84a1a2011cfb81"
                .parse()
                .unwrap()
        );
    }
}
