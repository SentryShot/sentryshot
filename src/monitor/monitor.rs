// SPDX-License-Identifier: GPL-2.0-or-later

mod recorder;
mod source;

use recdb::RecDb;
pub use source::MonitorSource;
pub use source::Streamer;

use crate::{recorder::new_recorder, source::SourceRtsp};
use async_trait::async_trait;
use common::{
    monitor::{
        ArcMonitorHooks, ArcSource, IMonitor, IMonitorManager, MonitorConfig, MonitorConfigs,
        MonitorDeleteError, MonitorInfo, MonitorRestartError, MonitorSetAndRestartError,
        MonitorSetError, SourceConfig,
    },
    ArcLogger, Event, LogEntry, LogLevel, MonitorId, StreamType,
};
use std::{
    collections::HashMap,
    path::{Path, PathBuf},
    sync::Arc,
};
use thiserror::Error;
use tokio::{
    self,
    io::AsyncWriteExt,
    sync::{mpsc, oneshot, Mutex},
};
use tokio_util::sync::CancellationToken;

type Monitors = HashMap<MonitorId, Arc<Monitor>>;
pub struct Monitor {
    token: CancellationToken,
    config: MonitorConfig,
    shutdown_complete: Mutex<mpsc::Receiver<()>>,
    source_main_tx: mpsc::Sender<oneshot::Sender<ArcSource>>,
    source_sub_tx: mpsc::Sender<oneshot::Sender<Option<ArcSource>>>,
    send_event_tx: mpsc::Sender<Event>,
}

#[async_trait]
impl IMonitor for Monitor {
    fn config(&self) -> &MonitorConfig {
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
    async fn get_smallest_source(&self) -> Option<ArcSource> {
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
    async fn source_main(&self) -> Option<ArcSource> {
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
    async fn source_sub(&self) -> Option<Option<ArcSource>> {
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

    async fn send_event(&self, event: Event) {
        tokio::select! {
            () = self.token.cancelled() => {},
            _ = self.send_event_tx.send(event) => {},
        }
    }
}

pub fn log_monitor(logger: &ArcLogger, level: LogLevel, id: &MonitorId, msg: &str) {
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

#[rustfmt::skip]
enum MonitorManagerRequest {
    StartMonitors((oneshot::Sender<()>, ArcMonitorHooks)),
    MonitorRestart((oneshot::Sender<Result<(), MonitorRestartError>>, MonitorId)),
    MonitorSet((oneshot::Sender<Result<bool, MonitorSetError>>, MonitorConfig)),
    MonitorSetAndRestart((oneshot::Sender<Result<bool, MonitorSetAndRestartError>>, MonitorConfig)),
    MonitorDelete((oneshot::Sender<Result<(), MonitorDeleteError>>, MonitorId)),
    MonitorsInfo(oneshot::Sender<HashMap<MonitorId, MonitorInfo>>),
    MonitorConfig((oneshot::Sender<Option<MonitorConfig>>, MonitorId)),
    MonitorConfigs(oneshot::Sender<MonitorConfigs>),
    Stop(oneshot::Sender<()>),
    MonitorIsRunning((oneshot::Sender<bool>, MonitorId)),
}

#[derive(Clone)]
pub struct MonitorManager(mpsc::Sender<MonitorManagerRequest>);

impl MonitorManager {
    pub fn new(
        config_path: PathBuf,
        rec_db: Arc<RecDb>,
        logger: ArcLogger,
        streamer: Streamer,
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

        let (tx, rx) = mpsc::channel(1);

        // This must be an actor in order to be callable from plugins.
        tokio::spawn(async move {
            MonitorManagerState {
                token: CancellationToken::new(),
                configs,
                started_monitors: HashMap::new(),
                rec_db,
                logger,
                streamer,
                path: config_path,
                hooks: None,
            }
            .run(rx)
            .await;
        });

        Ok(Self(tx))
    }
}

#[async_trait]
impl IMonitorManager for MonitorManager {
    async fn start_monitors(&self, hooks: ArcMonitorHooks) {
        let (tx, rx) = oneshot::channel();
        self.0
            .send(MonitorManagerRequest::StartMonitors((tx, hooks)))
            .await
            .expect("actor should still be active");

        _ = rx.await;
    }

    async fn monitor_restart(&self, monitor_id: MonitorId) -> Result<(), MonitorRestartError> {
        let (tx, rx) = oneshot::channel();
        self.0
            .send(MonitorManagerRequest::MonitorRestart((tx, monitor_id)))
            .await
            .expect("actor should still be active");

        rx.await.expect("actor should respond")
    }

    async fn monitor_set(&self, config: MonitorConfig) -> Result<bool, MonitorSetError> {
        let (tx, rx) = oneshot::channel();
        self.0
            .send(MonitorManagerRequest::MonitorSet((tx, config)))
            .await
            .expect("actor should still be active");

        rx.await.expect("actor should respond")
    }

    async fn monitor_set_and_restart(
        &self,
        config: MonitorConfig,
    ) -> Result<bool, MonitorSetAndRestartError> {
        let (tx, rx) = oneshot::channel();
        self.0
            .send(MonitorManagerRequest::MonitorSetAndRestart((tx, config)))
            .await
            .expect("actor should still be active");

        rx.await.expect("actor should respond")
    }

    async fn monitor_delete(&self, id: MonitorId) -> Result<(), MonitorDeleteError> {
        let (tx, rx) = oneshot::channel();
        self.0
            .send(MonitorManagerRequest::MonitorDelete((tx, id)))
            .await
            .expect("actor should still be active");

        rx.await.expect("actor should respond")
    }

    async fn monitors_info(&self) -> HashMap<MonitorId, MonitorInfo> {
        let (tx, rx) = oneshot::channel();
        self.0
            .send(MonitorManagerRequest::MonitorsInfo(tx))
            .await
            .expect("actor should still be active");

        rx.await.expect("actor should respond")
    }

    async fn monitor_config(&self, monitor_id: MonitorId) -> Option<MonitorConfig> {
        let (tx, rx) = oneshot::channel();
        self.0
            .send(MonitorManagerRequest::MonitorConfig((tx, monitor_id)))
            .await
            .expect("actor should still be active");

        rx.await.expect("actor should respond")
    }

    async fn monitor_configs(&self) -> MonitorConfigs {
        let (tx, rx) = oneshot::channel();
        self.0
            .send(MonitorManagerRequest::MonitorConfigs(tx))
            .await
            .expect("actor should still be active");

        rx.await.expect("actor should respond")
    }

    async fn stop(&self) {
        let (tx, rx) = oneshot::channel();
        self.0
            .send(MonitorManagerRequest::Stop(tx))
            .await
            .expect("actor should still be active");

        rx.await.expect("actor should respond");
    }

    async fn monitor_is_running(&self, monitor_id: MonitorId) -> bool {
        let (tx, rx) = oneshot::channel();
        self.0
            .send(MonitorManagerRequest::MonitorIsRunning((tx, monitor_id)))
            .await
            .expect("actor should still be active");

        rx.await.expect("actor should respond")
    }
}

struct MonitorManagerState {
    token: CancellationToken,

    configs: MonitorConfigs,
    started_monitors: Monitors,

    rec_db: Arc<RecDb>,
    logger: ArcLogger,
    streamer: Streamer,
    path: PathBuf,

    hooks: Option<ArcMonitorHooks>,
}

impl MonitorManagerState {
    async fn run(mut self, mut rx: mpsc::Receiver<MonitorManagerRequest>) {
        loop {
            let Some(request) = rx.recv().await else {
                // Manager was dropped.
                return;
            };
            match request {
                MonitorManagerRequest::StartMonitors((res, hooks)) => {
                    self.start_monitors(hooks).await;
                    drop(res);
                }
                MonitorManagerRequest::MonitorRestart((res, monitor_id)) => {
                    res.send(self.monitor_restart(&monitor_id).await)
                        .expect("caller should receive response");
                }
                MonitorManagerRequest::MonitorSet((res, config)) => {
                    res.send(self.monitor_set(config).await)
                        .expect("caller should receive response");
                }

                MonitorManagerRequest::MonitorSetAndRestart((res, config)) => {
                    res.send(self.monitor_set_and_restart(config).await)
                        .expect("caller should receive response");
                }
                MonitorManagerRequest::MonitorDelete((res, monitor_id)) => {
                    res.send(self.monitor_delete(&monitor_id).await)
                        .expect("caller should receive response");
                }
                MonitorManagerRequest::MonitorsInfo(res) => {
                    res.send(self.monitors_info())
                        .expect("caller should receive response");
                }
                MonitorManagerRequest::MonitorConfig((res, monitor_id)) => {
                    res.send(self.configs.get(&monitor_id).cloned())
                        .expect("caller should receive response");
                }
                MonitorManagerRequest::MonitorConfigs(res) => {
                    res.send(self.configs.clone())
                        .expect("caller should receive response");
                }
                MonitorManagerRequest::Stop(res) => {
                    self.stop().await;
                    res.send(()).expect("caller should receive response");
                }
                MonitorManagerRequest::MonitorIsRunning((res, monitor_id)) => {
                    res.send(self.started_monitors.get(&monitor_id).is_some())
                        .expect("caller should receive response");
                }
            }
        }
    }

    pub async fn start_monitors(&mut self, hooks: ArcMonitorHooks) {
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
    // Returns `true` if monitor was created.
    pub async fn monitor_set(&mut self, config: MonitorConfig) -> Result<bool, MonitorSetError> {
        use MonitorSetError::*;

        let id = config.id();

        // Write config to file.
        let path = self.config_path(id);

        let mut temp_path = path.clone();
        temp_path.set_file_name(id.to_string() + ".json.tmp");

        let json = serde_json::to_vec_pretty(&config)?;

        let mut file = tokio::fs::OpenOptions::new()
            .create(true)
            .write(true)
            .truncate(true)
            .open(&temp_path)
            .await
            .map_err(OpenFile)?;

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

    pub async fn monitor_set_and_restart(
        &mut self,
        config: MonitorConfig,
    ) -> Result<bool, MonitorSetAndRestartError> {
        use MonitorSetAndRestartError::*;
        let id = config.id().clone();
        let created = self.monitor_set(config).await.map_err(Set)?;
        self.monitor_restart(&id).await.map_err(Restart)?;
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
        log_monitor(&self.logger, LogLevel::Info, id, "deleted");
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
                MonitorInfo::new(
                    c.id().to_owned(),
                    c.name().to_owned(),
                    c.enabled(),
                    c.has_sub_stream(),
                ),
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

    async fn start_monitor(&self, config: MonitorConfig) -> Option<Arc<Monitor>> {
        let hooks = self.hooks.clone().expect("hooks to be set");

        if !config.enabled() {
            log_monitor(&self.logger, LogLevel::Info, config.id(), "disabled");
            return None;
        }
        log_monitor(&self.logger, LogLevel::Info, config.id(), "starting");

        let monitor_token = self.token.child_token();
        let (shutdown_complete_tx, shutdown_complete_rx) = mpsc::channel(1);

        let (source_main, source_sub): (ArcSource, Option<ArcSource>) = match config.source() {
            SourceConfig::Rtsp(conf) => {
                let source_main = SourceRtsp::new(
                    monitor_token.child_token(),
                    shutdown_complete_tx.clone(),
                    self.logger.clone(),
                    self.streamer.clone(),
                    config.id().to_owned(),
                    conf.to_owned(),
                    StreamType::Main,
                )
                .expect("source main should never be None");

                let source_sub = SourceRtsp::new(
                    monitor_token.child_token(),
                    shutdown_complete_tx.clone(),
                    self.logger.clone(),
                    self.streamer.clone(),
                    config.id().to_owned(),
                    conf.to_owned(),
                    StreamType::Sub,
                );

                (
                    Arc::new(source_main),
                    source_sub.map(|v| {
                        let v: ArcSource = Arc::new(v);
                        v
                    }),
                )
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
}

#[allow(clippy::needless_pass_by_value, clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use bytesize::ByteSize;
    use common::{
        monitor::{Config, Protocol, SelectedSource, SourceConfig, SourceRtspConfig},
        DummyLogger, MonitorName, ParseMonitorIdError,
    };
    use hls::HlsServer;
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
            Streamer::Hls(HlsServer::new(token, DummyLogger::new())),
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
    fn name(s: &str) -> MonitorName {
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
            Streamer::Hls(HlsServer::new(token, DummyLogger::new())),
        )
        .unwrap();

        let want = manager.monitor_configs().await[&m_id("1")].clone();
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
                Streamer::Hls(HlsServer::new(token, DummyLogger::new())),
            ),
            Err(NewMonitorManagerError::Deserialize(..))
        ));
    }

    #[tokio::test]
    async fn test_monitor_set_create_new() {
        let (_temp_dir, config_dir, manager) = new_test_manager();

        let config = MonitorConfig::new(
            Config {
                id: m_id("new"),
                name: name("new"),
                enable: false,
                source: SelectedSource::Rtsp,
                always_record: false,
                video_length: 0.0,
            },
            SourceConfig::Rtsp(SourceRtspConfig {
                protocol: Protocol::Tcp,
                main_stream: "rtsp://x1".parse().unwrap(),
                sub_stream: None,
            }),
            json!({
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
        );

        let created = manager.monitor_set(config).await.unwrap();
        assert!(created);

        let new = &m_id("new");
        let config = manager.monitor_config(new.clone()).await.unwrap();
        let new_name = config.name();
        assert_eq!(&name("new"), new_name);

        // Check if changes were saved to file.
        let saved_config = read_config(config_dir.join("new.json"));
        assert_eq!(manager.monitor_configs().await[new], saved_config);
    }

    #[tokio::test]
    async fn test_monitor_set_update() {
        let (_temp_dir, config_dir, manager) = new_test_manager();

        let one = m_id("1");
        let old_monitor = &manager.monitor_configs().await[&one];

        let old_name = old_monitor.name();
        assert_eq!(&name("one"), old_name);

        let config = MonitorConfig::new(
            Config {
                id: m_id("1"),
                name: name("two"),
                enable: false,
                source: SelectedSource::Rtsp,
                always_record: false,
                video_length: 0.0,
            },
            SourceConfig::Rtsp(SourceRtspConfig {
                protocol: Protocol::Tcp,
                main_stream: "rtsp://x1".parse().unwrap(),
                sub_stream: None,
            }),
            json!({
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
        );

        let created = manager.monitor_set(config).await.unwrap();
        assert!(!created);

        let config = manager.monitor_config(one.clone()).await.unwrap();
        let new_name = config.name();
        assert_eq!(&name("two"), new_name);

        // Check if changes were saved to file.
        let saved_config = read_config(config_dir.join("1.json"));
        assert_eq!(manager.monitor_configs().await[&one], saved_config);
    }

    #[tokio::test]
    async fn test_monitor_delete_exist_error() {
        let (_, _, manager) = new_test_manager();
        assert!(matches!(
            manager.monitor_delete(m_id("nil")).await,
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

        let got = manager.monitor_configs().await;
        let want: HashMap<MonitorId, MonitorConfig> = HashMap::from([
            (
                m_id("1"),
                MonitorConfig::new(
                    Config {
                        id: m_id("1"),
                        name: name("one"),
                        enable: false,
                        source: SelectedSource::Rtsp,
                        always_record: false,
                        video_length: 0.0,
                    },
                    SourceConfig::Rtsp(SourceRtspConfig {
                        protocol: Protocol::Tcp,
                        main_stream: "rtsp://x1".parse().unwrap(),
                        sub_stream: None,
                    }),
                    json!({
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
                ),
            ),
            (
                m_id("2"),
                MonitorConfig::new(
                    Config {
                        id: m_id("2"),
                        name: name("two"),
                        enable: false,
                        source: SelectedSource::Rtsp,
                        always_record: false,
                        video_length: 0.0,
                    },
                    SourceConfig::Rtsp(SourceRtspConfig {
                        protocol: Protocol::Udp,
                        main_stream: "rtsp://x1".parse().unwrap(),
                        sub_stream: Some("rtsp://x2".parse().unwrap()),
                    }),
                    json!({
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
                ),
            ),
        ]);

        assert_eq!(want, got);
    }

    #[tokio::test]
    async fn test_restart_monitor_not_exist_error() {
        let (_, _, manager) = new_test_manager();
        assert!(matches!(
            manager.monitor_restart(m_id("x")).await,
            Err(MonitorRestartError::NotExist(_))
        ));
    }
}
