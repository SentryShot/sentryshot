// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{
    config::Percent,
    label::{CreateLabelCacheError, LabelCache, LabelCacheError, LabelMap},
    model::{CreateModelCacheError, ModelCache, ModelCacheError, ModelChecksum},
    Fetcher,
};
use common::{
    Cancelled, Detection, Detections, DynMsgLogger, Label, Labels, LogLevel, RectangleNormalized,
    Region,
};
use serde::{Deserialize, Serialize};
use std::{
    collections::HashMap,
    fmt::Debug,
    num::{NonZeroU16, NonZeroU32, NonZeroU8},
    ops::Deref,
    path::{Path, PathBuf},
    str::FromStr,
    sync::Arc,
    time::Duration,
};
use tflite_lib::NewDetectorError;
use thiserror::Error;
use tokio::{
    runtime::Handle,
    sync::{mpsc, oneshot},
};
use url::Url;

#[derive(Debug, Default, Deserialize, PartialEq, Eq)]
#[serde(default)]
struct RawDetectorConfigs {
    detector_cpu: Vec<RawDetectorConfigCpu>,
    detector_edgetpu: Vec<RawDetectorConfigEdgeTpu>,
}

#[derive(Debug, Deserialize, PartialEq, Eq)]
struct RawDetectorConfigCpu {
    enable: bool,
    name: DetectorName,
    width: NonZeroU16,
    height: NonZeroU16,
    model: Url,
    sha256sum: ModelChecksum,
    label_map: Url,
    threads: NonZeroU8,
}

#[derive(Debug, Deserialize, PartialEq, Eq)]
struct RawDetectorConfigEdgeTpu {
    enable: bool,
    name: DetectorName,
    width: NonZeroU16,
    height: NonZeroU16,
    model: Url,
    sha256sum: ModelChecksum,
    label_map: Url,
    device: PathBuf,
}

type DetectorConfigs = HashMap<DetectorName, DetectorConfig>;

#[derive(Debug, Serialize)]
pub(crate) struct DetectorConfig {
    width: NonZeroU16,
    height: NonZeroU16,
    labels: Labels,
}

#[derive(Clone, Debug, Hash, PartialEq, Eq, Serialize)]
pub(crate) struct DetectorName(String);

impl<'de> Deserialize<'de> for DetectorName {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        let s = String::deserialize(deserializer)?;
        FromStr::from_str(&s).map_err(serde::de::Error::custom)
    }
}

impl std::fmt::Display for DetectorName {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.0)
    }
}

#[derive(Debug, Error, PartialEq, Eq)]
pub enum ParseDetectorNameError {
    #[error("empty string")]
    Empty,

    #[error("bad char: {0}")]
    BadChar(char),

    #[error("white space not allowed")]
    WhiteSpace,
}

impl FromStr for DetectorName {
    type Err = ParseDetectorNameError;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        if s.is_empty() {
            return Err(Self::Err::Empty);
        }
        for c in s.chars() {
            if c.is_whitespace() {
                return Err(Self::Err::WhiteSpace);
            }
            if !c.is_alphanumeric() && c != '-' && c != '_' {
                return Err(Self::Err::BadChar(c));
            }
        }
        Ok(Self(s.to_owned()))
    }
}

impl Deref for DetectorName {
    type Target = str;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

fn parse_raw_detector_configs(raw: &str) -> Result<RawDetectorConfigs, toml::de::Error> {
    toml::from_str::<RawDetectorConfigs>(raw)
}

type Detectors = HashMap<DetectorName, Arc<Detector>>;

pub(crate) struct Detector {
    rt_handle: Handle,
    detect_tx: async_channel::Sender<DetectRequest>,
    width: NonZeroU16,
    height: NonZeroU16,
}

#[derive(Debug, Error)]
pub(crate) enum DetectError {
    #[error["{0}"]]
    Detect(#[from] tflite_lib::DetectError),

    #[error("detector did not repond in 1 second")]
    DetectorTimeout,

    #[error("detection took longer than 1 second")]
    DetectionTimeout,

    #[error("{0}")]
    Cancelled(Cancelled),
}

impl Detector {
    pub(crate) async fn detect(&self, data: Vec<u8>) -> Result<Detections, DetectError> {
        use DetectError::*;
        let (res_tx, res_rx) = oneshot::channel();
        let req = DetectRequest { data, res: res_tx };

        let _enter = self.rt_handle.enter();
        tokio::select!(
            _ = self.detect_tx.send(req) => {},
            _ = tokio::time::sleep(Duration::from_secs(1)) => return Err(DetectorTimeout),
        );

        let res = tokio::select!(
            _ = tokio::time::sleep(Duration::from_secs(1)) => return Err(DetectionTimeout),
            v = res_rx => v,
        );
        if let Ok(res) = res {
            Ok(res?)
        } else {
            // Detector went out of scope.
            Err(Cancelled(common::Cancelled))
        }
    }
    pub(crate) fn width(&self) -> NonZeroU16 {
        self.width
    }
    pub(crate) fn height(&self) -> NonZeroU16 {
        self.height
    }
}

#[derive(Debug)]
struct DetectRequest {
    data: Vec<u8>,
    res: oneshot::Sender<Result<Detections, tflite_lib::DetectError>>,
}

pub(crate) type Thresholds = HashMap<Label, Percent>;

pub(crate) struct DetectorManager {
    detectors: Detectors,
    configs: DetectorConfigs,
}

#[derive(Debug, Error)]
pub(crate) enum DetectorManagerError {
    #[error("write config: {0}")]
    WriteConfig(std::io::Error),

    #[error("create tflite dir: {0}")]
    CreateDir(std::io::Error),

    #[error("create model cache: {0}")]
    CreateModelCache(#[from] CreateModelCacheError),

    #[error("create label cache: {0}")]
    CreateLabelCache(#[from] CreateLabelCacheError),

    #[error("read config: {0}")]
    ReadConfig(std::io::Error),

    #[error("deserialize config: {0}")]
    DeserializeConfig(#[from] toml::de::Error),

    #[error("found multiple detectors with the name '{0}")]
    Duplicate(DetectorName),

    #[error("get model: {0}")]
    GetModell(#[from] ModelCacheError),

    #[error("get label: {0}")]
    GetLabel(#[from] LabelCacheError),

    #[error("create detector: {0}")]
    CreateDetector(#[from] NewDetectorError),
}

impl DetectorManager {
    pub(crate) async fn new(
        rt_handle: Handle,
        shutdown_complete_tx: mpsc::Sender<()>,
        logger: DynMsgLogger,
        fetcher: &'static dyn Fetcher,
        config_dir: &Path,
    ) -> Result<Self, DetectorManagerError> {
        use DetectorManagerError::*;
        let config_path = config_dir.join("tflite.toml");
        if !config_path.exists() {
            logger.log(
                LogLevel::Info,
                &format!("generating {}", config_path.to_string_lossy()),
            );
            write_detector_config(&config_path).map_err(WriteConfig)?;
        }

        let tflite_dir = config_dir.join(".tflite");
        if !tflite_dir.exists() {
            std::fs::create_dir(&tflite_dir).map_err(CreateDir)?;
        }

        let labels_path = tflite_dir.join("labels.json");
        let mut label_cache = LabelCache::new(logger.clone(), fetcher, labels_path)?;

        let models_dir = tflite_dir.join("models");
        if !models_dir.exists() {
            std::fs::create_dir(&models_dir).map_err(CreateDir)?;
        }

        let mut model_cache = ModelCache::new(logger.clone(), fetcher, models_dir)?;

        let raw_config = std::fs::read_to_string(config_path).map_err(ReadConfig)?;
        let detector_configs = parse_raw_detector_configs(&raw_config)?;

        parse_detector_configs(
            &rt_handle,
            shutdown_complete_tx,
            logger,
            &mut model_cache,
            &mut label_cache,
            detector_configs,
        )
        .await
    }

    pub(crate) fn detectors(&self) -> &DetectorConfigs {
        &self.configs
    }

    #[allow(unused)]
    pub(crate) async fn get_detector(&self, name: &DetectorName) -> Option<Arc<Detector>> {
        self.detectors.get(name).cloned()
    }
}

const DEFAULT_CONFIG: &str = include_str!("./default_config.toml");

pub(crate) fn write_detector_config(path: &Path) -> Result<(), std::io::Error> {
    std::fs::write(path, DEFAULT_CONFIG)
}

async fn parse_detector_configs(
    rt_handle: &Handle,
    shutdown_complete_tx: mpsc::Sender<()>,
    logger: DynMsgLogger,
    model_cache: &mut ModelCache,
    label_cache: &mut LabelCache,
    configs: RawDetectorConfigs,
) -> Result<DetectorManager, DetectorManagerError> {
    use DetectorManagerError::*;
    let mut detectors = HashMap::new();
    let mut detector_configs = HashMap::new();

    for cpu in configs.detector_cpu {
        if !cpu.enable {
            logger.log(
                LogLevel::Debug,
                &format!("detector '{}' disabled", cpu.name),
            );
            continue;
        }
        let model_path = model_cache.get(&cpu.model, &cpu.sha256sum).await?;
        let label_map = label_cache.get(&cpu.label_map).await?;
        if detector_configs.contains_key(&cpu.name) {
            return Err(Duplicate(cpu.name));
        };
        let config = DetectorConfig {
            width: cpu.width,
            height: cpu.height,
            labels: label_map.values().cloned().collect(),
        };
        detector_configs.insert(cpu.name.clone(), config);
        let detector = new_cpu_detector(
            rt_handle.clone(),
            shutdown_complete_tx.clone(),
            logger.clone(),
            cpu.name.clone(),
            cpu.width,
            cpu.height,
            &model_path,
            cpu.threads,
            label_map,
        )?;
        detectors.insert(cpu.name, Arc::new(detector));
    }
    Ok(DetectorManager {
        detectors,
        configs: detector_configs,
    })
}

#[allow(clippy::too_many_arguments)]
fn new_cpu_detector(
    rt_handle: Handle,
    _shutdown_complete_tx: mpsc::Sender<()>,
    logger: DynMsgLogger,
    name: DetectorName,
    width: NonZeroU16,
    height: NonZeroU16,
    model_path: &Path,
    threads: NonZeroU8,
    label_map: LabelMap,
) -> Result<Detector, NewDetectorError> {
    let (detect_tx, detect_rx) = async_channel::bounded::<DetectRequest>(1);
    for i in 0..threads.get() {
        logger.log(
            LogLevel::Info,
            &format!("starting detector '{}' T{}", name, i),
        );
        let _shutdown_complete_tx = _shutdown_complete_tx.clone();
        let rt_handle2 = rt_handle.clone();
        let detect_rx = detect_rx.clone();
        let mut detector = tflite_lib::Detector::new(model_path)?;
        let label_map = label_map.clone();

        rt_handle.spawn(async move {
            let _shutdown_complete_tx = _shutdown_complete_tx;
            while let Ok(req) = detect_rx.recv().await {
                let result;
                (detector, result) = rt_handle2
                    .spawn_blocking(move || {
                        let result = detector.detect(&req.data);
                        (detector, result)
                    })
                    .await
                    .unwrap();
                let result = result.map(|v| parse_detections(&label_map, v));
                _ = req.res.send(result);
            }
        });
    }
    Ok(Detector {
        rt_handle,
        detect_tx,
        width,
        height,
    })
}

fn parse_detections(label_map: &LabelMap, input: Vec<tflite_lib::Detection>) -> Detections {
    let get_label = |class| {
        if let Some(label) = label_map.get(&class) {
            label.to_owned()
        } else {
            format!("unknown{}", class).parse().unwrap()
        }
    };
    input
        .into_iter()
        .filter_map(|d| {
            let rect = parse_rect(d.top, d.left, d.bottom, d.right)?;
            Some(Detection {
                label: get_label(d.class),
                score: d.score * 100.0,
                region: Region {
                    rectangle: Some(rect),
                    polygon: None,
                },
            })
        })
        .collect()
}

fn parse_rect(top: f32, left: f32, bottom: f32, right: f32) -> Option<RectangleNormalized> {
    fn scale(v: f32) -> u32 {
        (v * 1_000_000.0) as u32
    }
    let top = scale(top);
    let left = scale(left);
    let bottom = scale(bottom);
    let right = scale(right);
    if top > bottom || left > right {
        return None;
    }
    Some(RectangleNormalized {
        x: left,
        y: top,
        width: NonZeroU32::new(right - left)?,
        height: NonZeroU32::new(bottom - top)?,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use pretty_assertions::assert_eq;

    #[test]
    fn test_parse_detector_config() {
        let raw = "
            [[detector_cpu]]
            enable = false
            name = \"1\"
            width = 2
            height = 3
            model = \"file:///4\"
            sha256sum = \"5555555555555555555555555555555555555555555555555555555555555555\"
            label_map = \"file:///6\"
            threads = 7

            [[detector_edgetpu]]
            enable = true
            name = \"8\"
            width = 9
            height = 10
            model = \"file:///11\"
            sha256sum = \"1212121212121212121212121212121212121212121212121212121212121212\"
            label_map = \"file:///13\"
            device = \"14\"
        ";
        let got = parse_raw_detector_configs(&raw).unwrap();
        let want = RawDetectorConfigs {
            detector_cpu: vec![RawDetectorConfigCpu {
                enable: false,
                name: "1".parse().unwrap(),
                width: NonZeroU16::new(2).unwrap(),
                height: NonZeroU16::new(3).unwrap(),
                model: "file:///4".parse().unwrap(),
                sha256sum: "5555555555555555555555555555555555555555555555555555555555555555"
                    .parse()
                    .unwrap(),
                label_map: "file:///6".parse().unwrap(),
                threads: NonZeroU8::new(7).unwrap(),
            }],
            detector_edgetpu: vec![RawDetectorConfigEdgeTpu {
                enable: true,
                name: "8".parse().unwrap(),
                width: NonZeroU16::new(9).unwrap(),
                height: NonZeroU16::new(10).unwrap(),
                model: "file:///11".parse().unwrap(),
                sha256sum: "1212121212121212121212121212121212121212121212121212121212121212"
                    .parse()
                    .unwrap(),
                label_map: "file:///13".parse().unwrap(),
                device: "14".parse().unwrap(),
            }],
        };
        assert_eq!(want, got);
    }
    #[test]
    fn test_parse_detector_config_empty() {
        assert_eq!(
            RawDetectorConfigs::default(),
            parse_raw_detector_configs("").unwrap()
        );
    }
}
