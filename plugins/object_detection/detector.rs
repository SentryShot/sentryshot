// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{
    DynFetcher,
    backend::{BackendLoader, LoadTfliteBackendError},
    config::Percent,
    label::{CreateLabelCacheError, LabelCache, LabelCacheError},
    model::{CreateModelCacheError, ModelCache, ModelCacheError, ModelChecksum},
};
use common::{ArcMsgLogger, DynError, Label, Labels, LogLevel};
use plugin::object_detection::{ArcTfliteDetector, DetectorName, TfliteFormat};
use serde::{Deserialize, Serialize};
use std::{
    collections::HashMap,
    fmt::Debug,
    num::{NonZeroU8, NonZeroU16},
    path::{Path, PathBuf},
};
use thiserror::Error;
use tokio::runtime::Handle;
use tokio_util::task::task_tracker::TaskTrackerToken;
use url::Url;

#[derive(Debug, Default, Deserialize, PartialEq, Eq)]
#[serde(default)]
struct RawDetectorConfigs {
    detector_tflite: Vec<RawDetectorConfigTflite>,
    detector_edgetpu: Vec<RawDetectorConfigEdgeTpu>,
}

#[derive(Debug, Deserialize, PartialEq, Eq)]
struct RawDetectorConfigTflite {
    enable: bool,
    name: DetectorName,
    width: NonZeroU16,
    height: NonZeroU16,
    model: Url,
    sha256sum: ModelChecksum,
    label_map: Url,

    #[serde(default)]
    format: TfliteFormat,
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

    #[serde(default)]
    format: TfliteFormat,
    device: String,
}

type TfliteDetectorConfigs = HashMap<DetectorName, TfliteDetectorConfig>;

#[derive(Debug, Serialize)]
pub(crate) struct TfliteDetectorConfig {
    width: NonZeroU16,
    height: NonZeroU16,
    labels: Labels,
}

fn parse_raw_detector_configs(raw: &str) -> Result<RawDetectorConfigs, toml::de::Error> {
    toml::from_str::<RawDetectorConfigs>(raw)
}

pub(crate) type Thresholds = HashMap<Label, Percent>;

pub(crate) struct DetectorManager {
    detectors: HashMap<DetectorName, ArcTfliteDetector>,
    configs: TfliteDetectorConfigs,
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
    GetModel(#[from] ModelCacheError),

    #[error("get label: {0}")]
    GetLabel(#[from] LabelCacheError),

    #[error("create detector: {0}")]
    CreateDetector(DynError),

    #[error("load tflite backend: {0}")]
    LoadTfliteBackend(#[from] LoadTfliteBackendError),
}

impl DetectorManager {
    pub(crate) async fn new(
        rt_handle: Handle,
        task_token: TaskTrackerToken,
        logger: ArcMsgLogger,
        fetcher: DynFetcher,
        config_dir: &Path,
        plugin_dir: PathBuf,
    ) -> Result<Self, DetectorManagerError> {
        use DetectorManagerError::*;
        let config_path = config_dir.join("object_detection.toml");
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
        let mut label_cache = LabelCache::new(logger.clone(), fetcher.clone(), labels_path)?;

        let models_dir = tflite_dir.join("models");
        if !models_dir.exists() {
            std::fs::create_dir(&models_dir).map_err(CreateDir)?;
        }

        let mut model_cache = ModelCache::new(logger.clone(), fetcher, models_dir)?;

        let raw_config = std::fs::read_to_string(config_path).map_err(ReadConfig)?;
        let detector_configs = parse_raw_detector_configs(&raw_config)?;

        let mut backend_loader = BackendLoader::new(rt_handle, plugin_dir);

        parse_detector_configs(
            &mut backend_loader,
            task_token,
            logger,
            &mut model_cache,
            &mut label_cache,
            detector_configs,
        )
        .await
    }

    pub(crate) fn detectors(&self) -> &TfliteDetectorConfigs {
        &self.configs
    }

    #[allow(unused)]
    pub(crate) fn get_detector(&self, name: &DetectorName) -> Option<ArcTfliteDetector> {
        self.detectors.get(name).cloned()
    }
}

const DEFAULT_CONFIG: &str = include_str!("./default_config.toml");

pub(crate) fn write_detector_config(path: &Path) -> Result<(), std::io::Error> {
    common::write_file(path, DEFAULT_CONFIG.as_bytes())
}

async fn parse_detector_configs(
    backend_loader: &mut BackendLoader,
    task_token: TaskTrackerToken,
    logger: ArcMsgLogger,
    model_cache: &mut ModelCache,
    label_cache: &mut LabelCache,
    configs: RawDetectorConfigs,
) -> Result<DetectorManager, DetectorManagerError> {
    use DetectorManagerError::*;
    let mut detectors = HashMap::new();
    let mut detector_configs = HashMap::new();

    for cpu in configs.detector_tflite {
        if !cpu.enable {
            logger.log(
                LogLevel::Debug,
                &format!("detector '{}' disabled", cpu.name),
            );
            continue;
        }
        let model_path = model_cache.fetch(&cpu.model, &cpu.sha256sum).await?;
        let label_map = label_cache.get(&cpu.label_map).await?;
        if detector_configs.contains_key(&cpu.name) {
            return Err(Duplicate(cpu.name));
        };
        let config = TfliteDetectorConfig {
            width: cpu.width,
            height: cpu.height,
            labels: label_map.values().cloned().collect(),
        };
        detector_configs.insert(cpu.name.clone(), config);
        let detector = backend_loader
            .tflite_backend()?
            .new_tflite_detector(
                &task_token,
                &logger,
                &cpu.name,
                cpu.width,
                cpu.height,
                &model_path,
                cpu.format,
                &label_map,
                cpu.threads,
            )
            .map_err(CreateDetector)?;
        detectors.insert(cpu.name, detector);
    }

    for edgetpu in configs.detector_edgetpu {
        if !edgetpu.enable {
            logger.log(
                LogLevel::Debug,
                &format!("detector '{}' disabled", edgetpu.name),
            );
            continue;
        }
        let model_path = model_cache
            .fetch(&edgetpu.model, &edgetpu.sha256sum)
            .await?;
        let label_map = label_cache.get(&edgetpu.label_map).await?;
        if detector_configs.contains_key(&edgetpu.name) {
            return Err(Duplicate(edgetpu.name));
        };
        let config = TfliteDetectorConfig {
            width: edgetpu.width,
            height: edgetpu.height,
            labels: label_map.values().cloned().collect(),
        };
        detector_configs.insert(edgetpu.name.clone(), config);
        let detector = backend_loader
            .tflite_backend()?
            .new_edgetpu_detector(
                task_token.clone(),
                &logger,
                &edgetpu.name,
                edgetpu.width,
                edgetpu.height,
                &model_path,
                edgetpu.format,
                label_map,
                edgetpu.device,
            )
            .map_err(CreateDetector)?;
        detectors.insert(edgetpu.name, detector);
    }

    Ok(DetectorManager {
        detectors,
        configs: detector_configs,
    })
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use crate::StubFetcher;
    use common::DummyLogger;
    use pretty_assertions::assert_eq;
    use tempfile::tempdir;
    use tokio::runtime::Handle;
    use tokio_util::task::TaskTracker;

    #[tokio::test]
    async fn test_detector_manager() {
        let fetcher = StubFetcher::new(HashMap::from([
            ("https://test.com/model.tflite".parse().unwrap(), vec![0]),
            (
                "https://test.com/labels.txt".parse().unwrap(),
                "0 person".as_bytes().to_owned(),
            ),
        ]));

        let temp_dir = tempdir().unwrap();
        let config_dir = temp_dir.path();
        std::fs::write(
            config_dir.join("object_detection.toml"),
            "
[[detector_tflite]]
enable = true
name = \"test\"
width = 340
height = 340
model = \"https://test.com/model.tflite\"
sha256sum = \"6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d\"
label_map = \"https://test.com/labels.txt\"
threads = 1
            ",
        )
        .unwrap();

        let result = DetectorManager::new(
            Handle::current(),
            TaskTracker::new().token(),
            DummyLogger::new(),
            Box::new(fetcher),
            config_dir,
            temp_dir.path().to_path_buf(),
        )
        .await;

        match result {
            Err(DetectorManagerError::LoadTfliteBackend(_)) => {}
            Err(e) => panic!("wrong error {e:?}"),
            _ => panic!("expected error"),
        };
    }

    #[tokio::test]
    async fn test_detector_manager_bad_files() {
        // Check if bad files are ignored.
        let fetcher = StubFetcher::new(HashMap::new());

        let temp_dir = tempdir().unwrap();
        let config_dir = temp_dir.path();
        std::fs::write(config_dir.join("object_detection.toml"), "").unwrap();

        let models_dir = config_dir.join(".tflite").join("models");
        std::fs::create_dir_all(&models_dir).unwrap();
        std::fs::write(models_dir.join("bad"), [0]).unwrap();

        DetectorManager::new(
            Handle::current(),
            TaskTracker::new().token(),
            DummyLogger::new(),
            Box::new(fetcher),
            config_dir,
            temp_dir.path().to_path_buf(),
        )
        .await
        .unwrap();
    }

    #[tokio::test]
    async fn test_detector_manager_tmp_file_already_exist() {
        let fetcher = StubFetcher::new(HashMap::from([
            ("https://test.com/model.tflite".parse().unwrap(), vec![0]),
            (
                "https://test.com/labels.txt".parse().unwrap(),
                "0 person".as_bytes().to_owned(),
            ),
        ]));

        let temp_dir = tempdir().unwrap();
        let config_dir = temp_dir.path();
        std::fs::write(
            config_dir.join("object_detection.toml"),
            "
[[detector_tflite]]
enable = true
name = \"test\"
width = 340
height = 340
model = \"https://test.com/model.tflite\"
sha256sum = \"6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d\"
label_map = \"https://test.com/labels.txt\"
threads = 1
            ",
        )
        .unwrap();

        let models_dir = config_dir.join(".tflite").join("models");
        std::fs::create_dir_all(&models_dir).unwrap();
        std::fs::write(
            models_dir.join("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d.tmp"),
            [0],
        )
        .unwrap();

        let result = DetectorManager::new(
            Handle::current(),
            TaskTracker::new().token(),
            DummyLogger::new(),
            Box::new(fetcher),
            config_dir,
            temp_dir.path().to_path_buf(),
        )
        .await;

        match result {
            Err(DetectorManagerError::LoadTfliteBackend(_)) => {}
            Err(e) => panic!("wrong error {e:?}"),
            _ => panic!("expected error"),
        };
    }

    #[test]
    fn test_parse_detector_config() {
        let raw = "
            [[detector_tflite]]
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
            format = \"nolo\"
            device = \"14\"
        ";
        let got = parse_raw_detector_configs(raw).unwrap();
        let want = RawDetectorConfigs {
            detector_tflite: vec![RawDetectorConfigTflite {
                enable: false,
                name: "1".to_owned().try_into().unwrap(),
                width: NonZeroU16::new(2).unwrap(),
                height: NonZeroU16::new(3).unwrap(),
                model: "file:///4".parse().unwrap(),
                sha256sum: "5555555555555555555555555555555555555555555555555555555555555555"
                    .parse()
                    .unwrap(),
                label_map: "file:///6".parse().unwrap(),
                format: TfliteFormat::OdAPi,
                threads: NonZeroU8::new(7).unwrap(),
            }],
            detector_edgetpu: vec![RawDetectorConfigEdgeTpu {
                enable: true,
                name: "8".to_owned().try_into().unwrap(),
                width: NonZeroU16::new(9).unwrap(),
                height: NonZeroU16::new(10).unwrap(),
                model: "file:///11".parse().unwrap(),
                sha256sum: "1212121212121212121212121212121212121212121212121212121212121212"
                    .parse()
                    .unwrap(),
                label_map: "file:///13".parse().unwrap(),
                format: TfliteFormat::Nolo,
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
