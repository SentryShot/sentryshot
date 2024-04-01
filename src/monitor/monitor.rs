// SPDX-License-Identifier: GPL-2.0-or-later

mod recorder;
mod source;

use recdb::RecDb;
pub use source::{DecoderError, Source, SubscribeDecodedError};

use crate::{recorder::new_recorder, source::SourceRtsp};
use async_trait::async_trait;
use common::{
    monitor::{MonitorConfig, MonitorConfigs, SourceConfig},
    time::{Duration, UnixNano},
    DynLogger, Event, LogEntry, LogLevel, MonitorId, NonEmptyString, StreamType,
};
use hls::HlsServer;
use sentryshot_convert::Frame;
use serde::Serialize;
use std::{
    collections::HashMap,
    path::{Path, PathBuf},
    sync::Arc,
};
use thiserror::Error;
use tokio::{
    io::AsyncWriteExt,
    sync::{mpsc, oneshot, Mutex},
};
use tokio_util::sync::CancellationToken;

type Monitors = HashMap<MonitorId, Arc<Monitor>>;
pub struct Monitor {
    token: CancellationToken,
    config: MonitorConfig,
    shutdown_complete: Mutex<mpsc::Receiver<()>>,
    source_main_tx: mpsc::Sender<oneshot::Sender<Arc<Source>>>,
    source_sub_tx: mpsc::Sender<oneshot::Sender<Option<Arc<Source>>>>,
    send_event_tx: mpsc::Sender<Event>,
}

impl Monitor {
    pub fn config(&self) -> &MonitorConfig {
        &self.config
    }

    // SendEvent sends event to recorder.
    /*fn SendEvent(&self, event: Event) {
        _ = self.send_event_tx.send(event)
        //m.recorder.sendEvent(m.ctx, event)
    }*/

    async fn stop(&self) {
        self.token.cancel();
        self.shutdown_complete.lock().await.recv().await;
    }

    // Return sub stream if it exists otherwise returns main stream.
    // Returns None if cancelled
    pub async fn get_smallest_source(&self) -> Option<Arc<Source>> {
        match self.source_sub().await {
            // Sub stream exists.
            Some(Some(sub_stream)) => Some(sub_stream),
            // Sub stream doesn't exist, use main.
            Some(None) => self.source_main().await,
            // Sub stream is cancelled.
            None => None,
        }
    }

    // Returns None if cancelled.
    pub async fn source_main(&self) -> Option<Arc<Source>> {
        let (res_tx, res_rx) = oneshot::channel();
        tokio::select! {
            () = self.token.cancelled() => return None,
            _ = self.source_main_tx.send(res_tx) => {}
        }
        tokio::select! {
            () = self.token.cancelled() => None,
            res = res_rx => {
                res.ok()
            }
        }
    }

    // Returns None if cancelled and Some(None) if sub stream doesn't exist.
    pub async fn source_sub(&self) -> Option<Option<Arc<Source>>> {
        let (res_tx, res_rx) = oneshot::channel();
        tokio::select! {
            () = self.token.cancelled() => return None,
            _ = self.source_sub_tx.send(res_tx) => {}
        }
        tokio::select! {
            () = self.token.cancelled() => None,
            res = res_rx => {
                res.ok()
            }
        }
    }

    pub async fn send_event(&self, event: Event) {
        tokio::select! {
            () = self.token.cancelled() => {},
            _ = self.send_event_tx.send(event) => {},
        }
    }
}

impl Monitor {
    #[cfg(test)]
    #[allow(clippy::unwrap_used)]
    fn empty() -> Arc<Monitor> {
        use common::monitor::{Config, Protocol, SourceRtspConfig};
        use serde_json::Value;

        let (_, shutdown_complete) = mpsc::channel(1);
        let (source_main_tx, _) = mpsc::channel(1);
        let (source_sub_tx, _) = mpsc::channel(1);
        let (send_event_tx, _) = mpsc::channel(1);
        //let (send_event_tx, _) = mpsc::channel(1);
        Arc::new(Monitor {
            config: MonitorConfig {
                config: Config {
                    id: "a".to_owned().try_into().unwrap(),
                    name: "b".to_owned().try_into().unwrap(),
                    enable: false,
                    source: common::monitor::SelectedSource::Rtsp,
                    always_record: false,
                    video_length: 0.0,
                },
                source: SourceConfig::Rtsp(SourceRtspConfig {
                    protocol: Protocol::Tcp,
                    main_stream: "rtsp::/c".parse().unwrap(),
                    sub_stream: None,
                }),
                raw: Value::Null,
            },
            token: CancellationToken::new(),
            shutdown_complete: Mutex::new(shutdown_complete),
            source_main_tx,
            source_sub_tx,
            send_event_tx,
        })
    }
}

pub fn log_monitor(logger: &DynLogger, level: LogLevel, id: &MonitorId, msg: &str) {
    logger.log(LogEntry::new(
        level,
        "monitor",
        Some(id.to_owned()),
        msg.to_owned(),
    ));
}

#[derive(Debug, Error)]
pub enum NewMonitorManagerError {
    #[error("create directory: {0}")]
    CreateDir(std::io::Error),

    #[error("read directory: {0}")]
    ReadDir(std::io::Error),

    #[error("stat file:")]
    StatFile(std::io::Error),

    #[error("get file metadata: {0}")]
    GetFileMetadata(std::io::Error),

    #[error("read file: {0}")]
    ReadFile(std::io::Error),

    #[error("deserialize config '{0}': {1}")]
    Deserialize(String, serde_json::Error),

    #[error("config missing Id: {0}")]
    MissingId(String),
}

#[derive(Debug, Error)]
pub enum MonitorSetError {
    #[error("open file: {0}")]
    OpenFile(std::io::Error),

    #[error("serialize config: {0}")]
    Serialize(#[from] serde_json::Error),

    #[error("write config to file: {0}")]
    WriteToFile(std::io::Error),

    #[error("rename tempoary file: {0}")]
    RenameTempFile(std::io::Error),
}

#[derive(Debug, Error)]
pub enum MonitorDeleteError {
    #[error("monitor does not exist '{0}'")]
    NotExist(String),

    #[error("remove file: {0}")]
    RemoveFile(#[from] std::io::Error),
}

#[derive(Debug, Error)]
pub enum MonitorRestartError {
    #[error("monitor does not exist '{0}'")]
    NotExist(String),
}

// Manager for the monitors.
pub struct MonitorManager {
    token: CancellationToken,

    configs: MonitorConfigs,
    started_monitors: Monitors,

    rec_db: Arc<RecDb>,
    logger: DynLogger,
    hls_server: Arc<HlsServer>,
    path: PathBuf,

    hooks: Option<DynMonitorHooks>,
}

impl MonitorManager {
    pub fn new(
        config_path: PathBuf,
        rec_db: Arc<RecDb>,
        logger: DynLogger,
        hls_server: Arc<HlsServer>,
        //hooks *Hooks,
    ) -> Result<Self, NewMonitorManagerError> {
        use NewMonitorManagerError::*;
        std::fs::create_dir_all(&config_path).map_err(CreateDir)?;

        let mut configs = HashMap::new();
        for entry in std::fs::read_dir(&config_path).map_err(ReadDir)? {
            let entry = entry.map_err(StatFile)?;

            if entry.metadata().map_err(GetFileMetadata)?.is_dir() {
                continue;
            }

            let name = entry.file_name().to_string_lossy().to_string();
            let is_json_file = Path::new(&name)
                .extension()
                .map_or(false, |ext| ext.eq_ignore_ascii_case("json"));
            if !is_json_file {
                continue;
            }

            let json = std::fs::read(entry.path()).map_err(ReadFile)?;
            let config: MonitorConfig =
                serde_json::from_slice(&json).map_err(|e| Deserialize(name, e))?;

            configs.insert(config.id().to_owned(), config);
        }

        Ok(MonitorManager {
            token: CancellationToken::new(),
            configs,
            started_monitors: HashMap::new(),

            rec_db,
            logger,
            hls_server,
            path: config_path,
            hooks: None,
        })
    }

    pub async fn start_monitors(&mut self, hooks: DynMonitorHooks) {
        self.hooks = Some(hooks);
        for (id, config) in &self.configs {
            if let Some(monitor) = self.start_monitor(config.to_owned()).await {
                self.started_monitors.insert(id.to_owned(), monitor);
            }
        }
    }

    // Stops monitor (if running) and starts it again.
    pub async fn monitor_restart(&mut self, id: &MonitorId) -> Result<(), MonitorRestartError> {
        use MonitorRestartError::*;
        let Some(raw_config) = self.configs.get(id) else {
            return Err(NotExist(id.to_string()));
        };

        // Stop monitor if running.
        if let Some(monitor) = self.started_monitors.remove(id) {
            log_monitor(&self.logger, LogLevel::Info, id, "stopping");
            monitor.stop().await;
            log_monitor(&self.logger, LogLevel::Debug, id, "stopped");
        }

        // Restart monitor.
        if let Some(monitor) = self.start_monitor(raw_config.to_owned()).await {
            self.started_monitors.insert(id.to_owned(), monitor);
        }

        Ok(())
    }

    // Sets config for specified monitor.
    // Changes are not applied until the montior restarts.
    pub async fn monitor_set(&mut self, config: MonitorConfig) -> Result<bool, MonitorSetError> {
        use MonitorSetError::*;

        let id = config.id();

        // Write config to file.
        let path = self.config_path(id);

        let mut temp_path = path.clone();
        temp_path.set_file_name(id.to_string() + ".json.tmp");

        let mut file = tokio::fs::OpenOptions::new()
            .create(true)
            .write(true)
            .truncate(true)
            .open(&temp_path)
            .await
            .map_err(OpenFile)?;

        let json = serde_json::to_vec_pretty(&config)?;
        file.write_all(&json).await.map_err(WriteToFile)?;

        tokio::fs::rename(temp_path, path)
            .await
            .map_err(RenameTempFile)?;

        let created = !self.configs.contains_key(id);
        if created {
            log_monitor(&self.logger, LogLevel::Info, id, "created");
        } else {
            log_monitor(&self.logger, LogLevel::Info, id, "saved");
        }

        self.configs.insert(id.to_owned(), config);
        Ok(created)
    }

    // MonitorDelete deletes monitor by id.
    pub async fn monitor_delete(&mut self, id: &MonitorId) -> Result<(), MonitorDeleteError> {
        use MonitorDeleteError::*;

        if let Some(monitor) = self.started_monitors.remove(id) {
            log_monitor(&self.logger, LogLevel::Info, id, "stopping");
            monitor.stop().await;
            log_monitor(&self.logger, LogLevel::Debug, id, "stopped");
            self.started_monitors.remove(id);
        };

        if self.configs.remove(id).is_none() {
            return Err(NotExist(id.to_string()));
        }

        tokio::fs::remove_file(self.config_path(id)).await?;
        log_monitor(&self.logger, LogLevel::Debug, id, "deleted");
        Ok(())
    }

    // Returns common information about the monitors.
    // This will be accessesable by normal users.
    #[must_use]
    pub fn monitors_info(&self) -> HashMap<MonitorId, MonitorInfo> {
        let mut configs = HashMap::new();
        for raw_conf in self.configs.values() {
            let c = raw_conf;

            configs.insert(
                c.id().to_owned(),
                MonitorInfo {
                    id: c.id().to_owned(),
                    name: c.name().to_owned(),
                    enable: c.enabled(),
                    has_sub_stream: c.has_sub_stream(),
                },
            );
        }
        configs
    }

    fn config_path(&self, id: &MonitorId) -> PathBuf {
        fn monitor_config_path(path: &Path, id: String) -> PathBuf {
            path.join(id + ".json")
        }
        monitor_config_path(&self.path, id.to_string())
    }

    // Configurations for all monitors.
    #[must_use]
    pub fn monitor_configs(&self) -> MonitorConfigs {
        let mut configs = MonitorConfigs::new();
        for (id, raw_conf) in &self.configs {
            configs.insert(id.to_owned(), raw_conf.to_owned());
        }
        configs
    }

    async fn start_monitor(&self, config: MonitorConfig) -> Option<Arc<Monitor>> {
        let hooks = self.hooks.clone().expect("hooks to be set");

        if !config.enabled() {
            log_monitor(&self.logger, LogLevel::Info, config.id(), "disabled");
            return None;
        }
        log_monitor(&self.logger, LogLevel::Info, config.id(), "starting");

        let monitor_token = self.token.child_token();
        let (shutdown_complete_tx, shutdown_complete_rx) = mpsc::channel(1);

        let (source_main, source_sub) = match &config.source {
            SourceConfig::Rtsp(conf) => {
                let source_main = SourceRtsp::new(
                    monitor_token.child_token(),
                    shutdown_complete_tx.clone(),
                    self.logger.clone(),
                    self.hls_server.clone(),
                    config.id().to_owned(),
                    conf.to_owned(),
                    StreamType::Main,
                )
                .expect("source main should never be None");

                let source_sub = SourceRtsp::new(
                    monitor_token.child_token(),
                    shutdown_complete_tx.clone(),
                    self.logger.clone(),
                    self.hls_server.clone(),
                    config.id().to_owned(),
                    conf.to_owned(),
                    StreamType::Sub,
                );

                (Arc::new(source_main), source_sub.map(Arc::new))
            }
        };

        let send_event_tx = new_recorder(
            monitor_token.clone(),
            shutdown_complete_tx.clone(),
            hooks.clone(),
            self.logger.clone(),
            config.id().to_owned(),
            source_main.clone(),
            config.clone(),
            self.rec_db.clone(),
        );

        if config.always_record() {
            _ = send_event_tx
                .send(Event {
                    time: UnixNano::now(),
                    duration: Duration::from_secs(0),
                    rec_duration: Duration::from(1_000_000_000_000_000),
                    detections: Vec::new(),
                })
                .await;
        }

        let (source_main_tx, mut source_main_rx) = mpsc::channel(1);
        let (source_sub_tx, mut source_sub_rx) = mpsc::channel(1);

        let monitor = Arc::new(Monitor {
            token: monitor_token.clone(),
            config: config.clone(),
            shutdown_complete: Mutex::new(shutdown_complete_rx),
            source_main_tx,
            source_sub_tx,
            send_event_tx,
        });

        // Monitor actor.
        let monitor_token2 = monitor_token.clone();
        tokio::spawn(async move {
            let _shutdown_complete = shutdown_complete_tx;
            loop {
                tokio::select! {
                    () = monitor_token2.cancelled() => return,
                    res = source_main_rx.recv() => {
                        let Some(res) = res else {
                            return
                        };
                        _ = res.send(source_main.clone());
                    },
                    res = source_sub_rx.recv() => {
                        let Some(res) = res else {
                            return
                        };
                        _ = res.send(source_sub.clone());
                    },
                };
            }
        });

        hooks.on_monitor_start(monitor_token, monitor.clone()).await;

        Some(monitor)
    }

    pub async fn stop(&mut self) {
        // Cancel token.
        self.token.cancel();

        // Stop monitors.
        for (_, monitor) in self.started_monitors.drain() {
            monitor.stop().await;
        }

        // Break circular reference.
        self.hooks = None;
    }

    #[cfg(test)]
    fn monitor_is_running(&self, id: &MonitorId) -> bool {
        self.started_monitors.get(id).is_some()
    }
}

#[derive(Debug, Serialize, PartialEq, Eq)]
pub struct MonitorInfo {
    id: MonitorId,
    name: NonEmptyString,
    enable: bool,

    #[serde(rename = "hasSubStream")]
    has_sub_stream: bool,
}

pub type DynMonitorHooks = Arc<dyn MonitorHooks + Send + Sync>;

#[async_trait]
pub trait MonitorHooks {
    async fn on_monitor_start(&self, token: CancellationToken, monitor: Arc<Monitor>);
    // Blocking.
    fn on_thumb_save(&self, config: &MonitorConfig, frame: Frame) -> Frame;
}

#[allow(clippy::needless_pass_by_value, clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use bytesize::ByteSize;
    use common::{
        monitor::{Config, Protocol, SelectedSource, SourceConfig, SourceRtspConfig},
        DummyLogger, ParseMonitorIdError,
    };
    use pretty_assertions::assert_eq;
    use recdb::Disk;
    use serde_json::json;
    use std::{fs, path::PathBuf};
    use tempfile::TempDir;
    use test_case::test_case;

    #[test_case("", ParseMonitorIdError::Empty; "empty")]
    #[test_case("@", ParseMonitorIdError::InvalidChars("@".to_owned()); "invalid_chars")]
    fn source_parse(input: &str, want: ParseMonitorIdError) {
        assert_eq!(
            want,
            MonitorId::try_from(input.to_owned()).expect_err("expected error")
        );
    }

    fn prepare_dir() -> (TempDir, PathBuf) {
        let temp_dir = TempDir::new().unwrap();

        let test_config_dir = temp_dir.path().join("monitors");
        fs::create_dir_all(&test_config_dir).unwrap();

        std::fs::write(
            test_config_dir.join("1.json"),
            "{
                \"id\": \"1\",
                \"name\": \"one\",
                \"enable\": false,
                \"source\": \"rtsp\",
                \"sourcertsp\": {
                    \"protocol\": \"tcp\",
                    \"mainStream\": \"rtsp://x1\"
                },
                \"alwaysRecord\": false,
                \"videoLength\": 0.0
            }",
        )
        .unwrap();

        std::fs::write(
            test_config_dir.join("2.json"),
            "{
                \"id\": \"2\",
                \"name\": \"two\",
                \"enable\": false,
                \"source\": \"rtsp\",
                \"sourcertsp\": {
                    \"protocol\": \"udp\",
                    \"mainStream\": \"rtsp://x1\",
                    \"subStream\": \"rtsp://x2\"
                },
                \"alwaysRecord\": false,
                \"videoLength\": 0.0
            }",
        )
        .unwrap();

        (temp_dir, test_config_dir)
    }

    fn new_test_recdb(recordings_dir: &Path) -> RecDb {
        let disk = Disk::new(recordings_dir.to_path_buf(), ByteSize(0));
        RecDb::new(DummyLogger::new(), recordings_dir.to_path_buf(), disk)
    }

    fn new_test_manager() -> (TempDir, PathBuf, MonitorManager) {
        let (temp_dir, config_dir) = prepare_dir();

        let token = CancellationToken::new();
        let manager = MonitorManager::new(
            config_dir.clone(),
            Arc::new(new_test_recdb(temp_dir.path())),
            DummyLogger::new(),
            Arc::new(HlsServer::new(token, DummyLogger::new())),
        )
        .unwrap();

        (temp_dir, config_dir, manager)
    }

    fn read_config(path: PathBuf) -> MonitorConfig {
        let json = fs::read(path).unwrap();
        serde_json::from_slice(&json).unwrap()
    }

    fn m_id(s: &str) -> MonitorId {
        s.to_owned().try_into().unwrap()
    }
    fn name(s: &str) -> NonEmptyString {
        s.to_owned().try_into().unwrap()
    }

    #[tokio::test]
    async fn test_new_manager_ok() {
        let (temp_dir, config_dir, _) = new_test_manager();

        let token = CancellationToken::new();
        let manager = MonitorManager::new(
            config_dir.clone(),
            Arc::new(new_test_recdb(temp_dir.path())),
            DummyLogger::new(),
            Arc::new(HlsServer::new(token, DummyLogger::new())),
        )
        .unwrap();

        let want = manager.configs[&m_id("1")].clone();
        let got = read_config(config_dir.join("1.json"));
        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_new_manager_unmarshal_error() {
        let (temp_dir, config_dir) = prepare_dir();

        std::fs::write(config_dir.join("1.json"), "{").unwrap();

        let token = CancellationToken::new();
        assert!(matches!(
            MonitorManager::new(
                config_dir,
                Arc::new(new_test_recdb(temp_dir.path())),
                DummyLogger::new(),
                //&video.Server{},
                //&Hooks{Migrate: func(RawConfig) error { return nil }},
                Arc::new(HlsServer::new(token, DummyLogger::new())),
            ),
            Err(NewMonitorManagerError::Deserialize(..))
        ));
    }

    #[tokio::test]
    async fn test_monitor_set_create_new() {
        let (_temp_dir, config_dir, mut manager) = new_test_manager();

        let config = MonitorConfig {
            config: Config {
                id: m_id("new"),
                name: name("new"),
                enable: false,
                source: SelectedSource::Rtsp,
                always_record: false,
                video_length: 0.0,
            },
            source: SourceConfig::Rtsp(SourceRtspConfig {
                protocol: Protocol::Tcp,
                main_stream: "rtsp://x1".parse().unwrap(),
                sub_stream: None,
            }),
            raw: json!({
                "id": "new",
                "name": "new",
                "enable": false,
                "unused": "x",
                "source": "rtsp",
                "sourcertsp": {
                    "protocol": "tcp",
                    "mainStream": "rtsp://x1",
                },
                "alwaysRecord": false,
                "videoLength": 0.0,
            }),
        };

        let created = manager.monitor_set(config).await.unwrap();
        assert!(created);

        let new = &m_id("new");
        let new_name = manager.configs[new].name();
        assert_eq!(&name("new"), new_name);

        // Check if changes were saved to file.
        let saved_config = read_config(config_dir.join("new.json"));
        assert_eq!(manager.configs[new], saved_config);
    }

    #[tokio::test]
    async fn test_monitor_set_update() {
        let (_temp_dir, config_dir, mut manager) = new_test_manager();

        let one = m_id("1");
        let old_monitor = &manager.configs[&one];

        let old_name = old_monitor.name();
        assert_eq!(&name("one"), old_name);

        let config = MonitorConfig {
            config: Config {
                id: m_id("1"),
                name: name("two"),
                enable: false,
                source: SelectedSource::Rtsp,
                always_record: false,
                video_length: 0.0,
            },
            source: SourceConfig::Rtsp(SourceRtspConfig {
                protocol: Protocol::Tcp,
                main_stream: "rtsp://x1".parse().unwrap(),
                sub_stream: None,
            }),
            raw: json!({
                "id": "1",
                "name": "two",
                "enable": false,
                "unused": "x",
                "source": "rtsp",
                "sourcertsp": {
                    "protocol": "tcp",
                    "mainStream": "rtsp://x1",
                },
                "alwaysRecord": false,
                "videoLength": 0,
            }),
        };

        let created = manager.monitor_set(config).await.unwrap();
        assert!(!created);

        let new_name = manager.configs[&one].name();
        assert_eq!(&name("two"), new_name);

        // Check if changes were saved to file.
        let saved_config = read_config(config_dir.join("1.json"));
        assert_eq!(manager.configs[&one], saved_config);
    }

    #[tokio::test]
    async fn test_monitor_delete_ok() {
        let (_temp_dir, config_dir, mut manager) = new_test_manager();

        let one = m_id("1");
        manager
            .started_monitors
            .insert(one.clone(), Monitor::empty());

        manager.monitor_delete(&one).await.unwrap();

        assert!(!manager.monitor_is_running(&one));
        assert!(!Path::new(&config_dir.join("1.json")).exists());
    }

    #[tokio::test]
    async fn test_monitor_delete_exist_error() {
        let (_, _, mut manager) = new_test_manager();
        assert!(matches!(
            manager.monitor_delete(&m_id("nil")).await,
            Err(MonitorDeleteError::NotExist(_))
        ));
    }

    /*
    #[tokio::test]
    async fn test_monitors_info() {
        let token = CancellationToken::new();
        let manager = MonitorManager {
            token: token.clone(),
            configs: MonitorConfigs::from([
                (
                    "1".parse().unwrap(),
                    MonitorConfig {
                        config: Config {
                            id: "1".parse().unwrap(),
                            name: "2".parse().unwrap(),
                            enable: false,
                            source: SelectedSource::Rtsp,
                        },
                        source: Source::Rtsp(SourceRtspConfig {
                            protocol: Protocol::Tcp,
                            main_stream: "rtsp://x".parse().unwrap(),
                            sub_stream: None,
                        }),
                        raw: serde_json::Value::Null,
                    },
                ),
                (
                    "2".parse().unwrap(),
                    MonitorConfig {
                        config: Config {
                            id: "3".parse().unwrap(),
                            name: "4".parse().unwrap(),
                            enable: true,
                            source: SelectedSource::Rtsp,
                        },
                        source: Source::Rtsp(SourceRtspConfig {
                            protocol: Protocol::Udp,
                            main_stream: "rtsp://x".parse().unwrap(),
                            sub_stream: None,
                        }),
                        raw: serde_json::Value::Null,
                    },
                ),
            ]),
            started_monitors: HashMap::new(),
            //env: env::Config::empty(),
            logger: new_dummy_logger(),
            path: PathBuf::new(),
            hls_server: Arc::new(HlsServer::new(token, new_dummy_logger(), 0)),
        };

        let want: HashMap<MonitorId, MonitorInfo> = HashMap::from([
            (
                "1".parse().unwrap(),
                MonitorInfo {
                    id: "1".parse().unwrap(),
                    name: "2".parse().unwrap(),
                    enable: false,
                    sub_input_enabled: false,
                },
            ),
            (
                "3".parse().unwrap(),
                MonitorInfo {
                    id: "3".parse().unwrap(),
                    name: "4".parse().unwrap(),
                    enable: true,
                    sub_input_enabled: false,
                },
            ),
        ]);
        let got = manager.monitors_info();
        assert_eq!(want, got);
    }*/

    #[tokio::test]
    async fn test_monitor_configs() {
        let (_, _, manager) = new_test_manager();

        let got = manager.monitor_configs();
        let want: HashMap<MonitorId, MonitorConfig> = HashMap::from([
            (
                m_id("1"),
                MonitorConfig {
                    config: Config {
                        id: m_id("1"),
                        name: name("one"),
                        enable: false,
                        source: SelectedSource::Rtsp,
                        always_record: false,
                        video_length: 0.0,
                    },
                    source: SourceConfig::Rtsp(SourceRtspConfig {
                        protocol: Protocol::Tcp,
                        main_stream: "rtsp://x1".parse().unwrap(),
                        sub_stream: None,
                    }),
                    raw: json!({
                        "id": "1",
                        "name": "one",
                        "enable": false,
                        "source": "rtsp",
                        "sourcertsp": {
                            "protocol": "tcp",
                            "mainStream": "rtsp://x1",
                        },
                        "alwaysRecord": false,
                        "videoLength": 0.0,
                    }),
                },
            ),
            (
                m_id("2"),
                MonitorConfig {
                    config: Config {
                        id: m_id("2"),
                        name: name("two"),
                        enable: false,
                        source: SelectedSource::Rtsp,
                        always_record: false,
                        video_length: 0.0,
                    },
                    source: SourceConfig::Rtsp(SourceRtspConfig {
                        protocol: Protocol::Udp,
                        main_stream: "rtsp://x1".parse().unwrap(),
                        sub_stream: Some("rtsp://x2".parse().unwrap()),
                    }),
                    raw: json!({
                        "id": "2",
                        "name": "two",
                        "enable": false,
                        "source": "rtsp",
                        "sourcertsp": {
                            "protocol": "udp",
                            "mainStream": "rtsp://x1",
                            "subStream": "rtsp://x2",
                        },
                        "alwaysRecord": false,
                        "videoLength": 0.0,
                    }),
                },
            ),
        ]);

        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_manager_drop() {
        let (_, _, mut manager) = new_test_manager();
        manager.started_monitors =
            Monitors::from([(m_id("1"), Monitor::empty()), (m_id("2"), Monitor::empty())]);
        manager.stop().await;
    }

    #[tokio::test]
    async fn test_restart_monitor_not_exist_error() {
        let (_, _, mut manager) = new_test_manager();
        assert!(matches!(
            manager.monitor_restart(&m_id("x")).await,
            Err(MonitorRestartError::NotExist(_))
        ));
    }
}
