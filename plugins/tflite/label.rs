// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{FetchError, Fetcher};
use common::{DynMsgLogger, Label, LogLevel, ParseLabelError};
use std::{collections::HashMap, num::ParseIntError, path::PathBuf, string::FromUtf8Error};
use thiserror::Error;
use url::Url;

pub(crate) type LabelMap = HashMap<u8, Label>;
type LabelMaps = HashMap<Url, LabelMap>;

pub(crate) struct LabelCache {
    logger: DynMsgLogger,
    fetcher: &'static dyn Fetcher,
    path: PathBuf,
    label_maps: LabelMaps,
}

#[derive(Debug, Error)]
pub(crate) enum CreateLabelCacheError {
    #[error("read file: {0}")]
    ReadFile(std::io::Error),

    #[error("deserialize labels sets: {0}")]
    DeserializeLabelsSets(serde_json::Error),
}

#[derive(Debug, Error)]
pub(crate) enum LabelCacheError {
    #[error("{0}")]
    ParseLabels(#[from] ParseLabelsError),

    #[error("serialize labels sets: {0}")]
    SerializeLabelsSets(serde_json::Error),

    #[error("write file: {0}")]
    WriteFile(std::io::Error),

    #[error("rename file: {0}")]
    RenameFile(std::io::Error),

    #[error("fetch: {0}")]
    Fetch(#[from] FetchError),

    #[error("parse utf8: {0}")]
    Utf8(#[from] FromUtf8Error),
}

impl LabelCache {
    pub(crate) fn new(
        logger: DynMsgLogger,
        fetcher: &'static dyn Fetcher,
        path: PathBuf,
    ) -> Result<Self, CreateLabelCacheError> {
        use CreateLabelCacheError::*;
        let labels_sets = if path.exists() {
            let raw = std::fs::read(&path).map_err(ReadFile)?;
            serde_json::from_slice(&raw).map_err(DeserializeLabelsSets)?
        } else {
            HashMap::new()
        };
        Ok(Self {
            logger,
            fetcher,
            path,
            label_maps: labels_sets,
        })
    }

    pub(crate) async fn get(&mut self, url: &Url) -> Result<LabelMap, LabelCacheError> {
        if let Some(labels) = self.label_maps.get(url) {
            return Ok(labels.to_owned());
        }
        self.logger
            .log(LogLevel::Info, &format!("downloading labelmap '{}'", url));
        let raw = self.fetcher.fetch(url).await?;
        let raw_string = String::from_utf8(raw)?;
        let labels = parse_labels(&raw_string)?;
        self.label_maps.insert(url.to_owned(), labels.to_owned());
        self.save_to_disk()?;
        Ok(labels)
    }

    fn save_to_disk(&self) -> Result<(), LabelCacheError> {
        use LabelCacheError::*;
        let raw = serde_json::to_vec_pretty(&self.label_maps).map_err(SerializeLabelsSets)?;

        let mut temp_path = self.path.to_owned();
        temp_path.set_extension("tmp");

        std::fs::write(&temp_path, raw).map_err(WriteFile)?;
        std::fs::rename(&temp_path, &self.path).map_err(RenameFile)?;
        Ok(())
    }
}

#[derive(Debug, Error, PartialEq, Eq)]
#[error("parse labels: line={0}: '{1}': {2}")]
pub(crate) struct ParseLabelsError(usize, String, ParseLabelsErrorInner);

#[derive(Debug, Error, PartialEq, Eq)]
pub(crate) enum ParseLabelsErrorInner {
    #[error("split key and label")]
    SplitLine,

    #[error("parse key: {0}")]
    ParseKey(ParseIntError),

    #[error("parse label: {0}")]
    ParseLabel(ParseLabelError),
}

pub(crate) fn parse_labels(raw: &str) -> Result<LabelMap, ParseLabelsError> {
    use ParseLabelsErrorInner::*;
    let mut labels = HashMap::new();
    for (i, line) in raw.trim().lines().enumerate() {
        let (key, label) = line
            .split_once(' ')
            .ok_or_else(|| ParseLabelsError(i, line.to_owned(), SplitLine))?;
        let key: u8 = key
            .parse()
            .map_err(|e| ParseLabelsError(i, line.to_owned(), ParseKey(e)))?;
        let label = label
            .trim()
            .parse()
            .map_err(|e| ParseLabelsError(i, line.to_owned(), ParseLabel(e)))?;
        labels.insert(key, label);
    }
    Ok(labels)
}

#[cfg(test)]
mod tests {
    use super::*;
    use pretty_assertions::assert_eq;

    #[test]
    fn test_parse_labels() {
        let labels = "
0  person
1  bicycle
2  car
3  motorcycle
4  airplane
5  bus
6  train
7  car
8  boat
9  traffic light";
        let got = parse_labels(labels).unwrap();
        let want = HashMap::from([
            (0, "person".parse().unwrap()),
            (1, "bicycle".parse().unwrap()),
            (2, "car".parse().unwrap()),
            (3, "motorcycle".parse().unwrap()),
            (4, "airplane".parse().unwrap()),
            (5, "bus".parse().unwrap()),
            (6, "train".parse().unwrap()),
            (7, "car".parse().unwrap()),
            (8, "boat".parse().unwrap()),
            (9, "traffic light".parse().unwrap()),
        ]);
        assert_eq!(want, got)
    }
}
