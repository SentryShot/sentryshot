// SPDX-License-Identifier: GPL-2.0-or-later

mod recorder;
mod source;

pub use source::MonitorSource;

use crate::{recorder::new_recorder, source::SourceRtsp};
use async_trait::async_trait;
use common::{
    ArcLogger, DynError, Event, LogEntry, LogLevel, MonitorId, StreamType,
    monitor::{
        ArcMonitorHooks, ArcSource, ArcStreamer, CreateEventDbError, IMonitorManager,
        MonitorConfig, MonitorConfigs, MonitorDeleteError, MonitorImpl, MonitorInfo,
        MonitorRestartError, MonitorSetAndRestartError, MonitorSetError, SourceConfig,
    },
    time::Duration,
    write_file_atomic2,
};
use eventdb::EventDb;
use recdb::RecDb;
use std::{
    collections::HashMap,
    path::{Path, PathBuf},
    sync::Arc,
};
use thiserror::Error;
use tokio::{
    self,
    runtime::Handle,
    sync::{Mutex, mpsc, oneshot},
};
use tokio_util::sync::CancellationToken;

type Monitors = HashMap<MonitorId, Arc<Monitor>>;
pub struct Monitor {
    token: CancellationToken,
    id: MonitorId,
    eventdb: EventDb,
    hooks: ArcMonitorHooks,
    config: MonitorConfig,
    shutdown_complete: Mutex<mpsc::Receiver<()>>,
    source_main_tx: mpsc::Sender<oneshot::Sender<ArcSource>>,
    source_sub_tx: mpsc::Sender<oneshot::Sender<Option<ArcSource>>>,
    send_event_tx: mpsc::Sender<(Duration, Event)>,
}

#[async_trait]
impl MonitorImpl for Monitor {
    fn config(&self) -> &MonitorConfig {
        &self.config
    }

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

    async fn trigger(
        &self,
        trigger_duration: Duration,
        event: Event,
    ) -> Result<(), CreateEventDbError> {
        self.eventdb
            .write_event_deduplicate_time(&self.id, event.clone())
            .await?;

        self.hooks
            .on_event(event.clone(), self.config.clone())
            .await;

        tokio::select! {
            () = self.token.cancelled() => {},
            _ = self.send_event_tx.send((trigger_duration, event)) => {},
        }
        Ok(())
    }
}

pub(crate) fn log_monitor(logger: &ArcLogger, level: LogLevel, id: &MonitorId, msg: &str) {
    logger.log(LogEntry::new(level, "monitor", id, msg));
}

#[derive(Debug, Error)]
pub enum InitializeMonitorManagerError {
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

    #[error("migrate config for monitor {0}: {1}")]
    Migrate(MonitorId, DynError),
}

#[rustfmt::skip]
enum MonitorManagerRequest {
    Initialize((oneshot::Sender<Result<(), InitializeMonitorManagerError>>, InitializeRequest)),
    MonitorRestart((oneshot::Sender<Result<(), MonitorRestartError>>, MonitorId)),
    MonitorSet((oneshot::Sender<Result<bool, MonitorSetError>>, MonitorConfig)),
    MonitorSetAndRestart((oneshot::Sender<Result<bool, MonitorSetAndRestartError>>, MonitorConfig)),
    MonitorDelete((oneshot::Sender<Result<(), MonitorDeleteError>>, MonitorId)),
    MonitorsInfo(oneshot::Sender<HashMap<MonitorId, MonitorInfo>>),
    MonitorConfig((oneshot::Sender<Option<MonitorConfig>>, MonitorId)),
    MonitorConfigs(oneshot::Sender<MonitorConfigs>),
    Cancel(oneshot::Sender<()>),
}

struct InitializeRequest {
    config_path: PathBuf,
    recdb: Arc<RecDb>,
    eventdb: EventDb,
    logger: ArcLogger,
    streamer: ArcStreamer,
    hooks: ArcMonitorHooks,
}

#[derive(Clone)]
pub struct MonitorManager(mpsc::Sender<MonitorManagerRequest>);

impl Default for MonitorManager {
    fn default() -> Self {
        Self::new()
    }
}

impl MonitorManager {
    #[must_use]
    pub fn new() -> Self {
        let (tx, rx) = mpsc::channel(1);
        tokio::spawn(async move {
            run_monitor_manager(rx).await;
        });
        Self(tx)
    }

    // Monitor manager can't be initialized during creation because
    // there's a circular dependency between it and the plugin manager.
    pub async fn initialize(
        &self,
        config_path: PathBuf,
        rec_db: Arc<RecDb>,
        eventdb: EventDb,
        logger: ArcLogger,
        streamer: ArcStreamer,
        hooks: ArcMonitorHooks,
    ) -> Result<(), InitializeMonitorManagerError> {
        let (tx, rx) = oneshot::channel();
        let req = InitializeRequest {
            config_path,
            recdb: rec_db,
            eventdb,
            logger,
            streamer,
            hooks,
        };
        self.0
            .send(MonitorManagerRequest::Initialize((tx, req)))
            .await
            .expect("actor should still be active");
        rx.await.expect("actor should respond")
    }

    pub async fn cancel(&self) {
        let (tx, rx) = oneshot::channel();
        self.0
            .send(MonitorManagerRequest::Cancel(tx))
            .await
            .expect("actor should still be active");

        rx.await.expect("actor should respond");
    }
}

#[async_trait]
impl IMonitorManager for MonitorManager {
    async fn monitor_restart(
        &self,
        monitor_id: MonitorId,
    ) -> Option<Result<(), MonitorRestartError>> {
        let (tx, rx) = oneshot::channel();
        let req = MonitorManagerRequest::MonitorRestart((tx, monitor_id));
        if self.0.send(req).await.is_err() {
            // Cancelled.
            return None;
        }
        Some(rx.await.expect("actor should respond"))
    }

    async fn monitor_set(&self, config: MonitorConfig) -> Option<Result<bool, MonitorSetError>> {
        let (tx, rx) = oneshot::channel();
        let req = MonitorManagerRequest::MonitorSet((tx, config));
        if self.0.send(req).await.is_err() {
            // Cancelled.
            return None;
        }
        Some(rx.await.expect("actor should respond"))
    }

    async fn monitor_set_and_restart(
        &self,
        config: MonitorConfig,
    ) -> Option<Result<bool, MonitorSetAndRestartError>> {
        let (tx, rx) = oneshot::channel();
        let req = MonitorManagerRequest::MonitorSetAndRestart((tx, config));
        if self.0.send(req).await.is_err() {
            // Cancelled.
            return None;
        }
        Some(rx.await.expect("actor should respond"))
    }

    async fn monitor_delete(&self, id: MonitorId) -> Option<Result<(), MonitorDeleteError>> {
        let (tx, rx) = oneshot::channel();
        let req = MonitorManagerRequest::MonitorDelete((tx, id));
        if self.0.send(req).await.is_err() {
            // Cancelled.
            return None;
        }
        Some(rx.await.expect("actor should respond"))
    }

    async fn monitors_info(&self) -> Option<HashMap<MonitorId, MonitorInfo>> {
        let (tx, rx) = oneshot::channel();
        let req = MonitorManagerRequest::MonitorsInfo(tx);
        if self.0.send(req).await.is_err() {
            // Cancelled.
            return None;
        }
        Some(rx.await.expect("actor should respond"))
    }

    async fn monitor_config(&self, monitor_id: MonitorId) -> Option<Option<MonitorConfig>> {
        let (tx, rx) = oneshot::channel();
        let req = MonitorManagerRequest::MonitorConfig((tx, monitor_id));
        if self.0.send(req).await.is_err() {
            // Cancelled.
            return None;
        }
        Some(rx.await.expect("actor should respond"))
    }

    async fn monitor_configs(&self) -> Option<MonitorConfigs> {
        let (tx, rx) = oneshot::channel();
        let req = MonitorManagerRequest::MonitorConfigs(tx);
        if self.0.send(req).await.is_err() {
            // Cancelled.
            return None;
        }
        Some(rx.await.expect("actor should respond"))
    }
}

struct MonitorManagerState {
    token: CancellationToken,

    configs: MonitorConfigs,
    started_monitors: Monitors,

    recdb: Arc<RecDb>,
    eventdb: EventDb,
    logger: ArcLogger,
    streamer: ArcStreamer,
    path: PathBuf,

    hooks: ArcMonitorHooks,
}

async fn run_monitor_manager(mut rx: mpsc::Receiver<MonitorManagerRequest>) {
    struct StateOption(Option<MonitorManagerState>);
    impl StateOption {
        fn get(&mut self) -> &mut MonitorManagerState {
            self.0.as_mut().expect("initialized")
        }
    }

    let mut state = StateOption(None);
    loop {
        let request = rx
            .recv()
            .await
            .expect("stop should be called before dropping manager");
        match request {
            MonitorManagerRequest::Initialize((res, req)) => {
                assert!(state.0.is_none(), "already initialized");
                let response = match MonitorManagerState::new(req).await {
                    Ok(v) => {
                        state.0 = Some(v);
                        Ok(())
                    }
                    Err(e) => Err(e),
                };
                res.send(response).expect("caller should receive response");
            }
            MonitorManagerRequest::MonitorRestart((res, monitor_id)) => {
                _ = res.send(state.get().monitor_restart(&monitor_id).await);
            }
            MonitorManagerRequest::MonitorSet((res, config)) => {
                _ = res.send(state.get().monitor_set(config).await);
            }
            MonitorManagerRequest::MonitorSetAndRestart((res, config)) => {
                _ = res.send(state.get().monitor_set_and_restart(config).await);
            }
            MonitorManagerRequest::MonitorDelete((res, monitor_id)) => {
                _ = res.send(state.get().monitor_delete(&monitor_id).await);
            }
            MonitorManagerRequest::MonitorsInfo(res) => _ = res.send(state.get().monitors_info()),
            MonitorManagerRequest::MonitorConfig((res, monitor_id)) => {
                _ = res.send(state.get().configs.get(&monitor_id).cloned());
            }
            MonitorManagerRequest::MonitorConfigs(res) => _ = res.send(state.get().configs.clone()),
            MonitorManagerRequest::Cancel(res) => {
                state.0.expect("initialized").cancel().await;
                res.send(()).expect("caller should receive response");
                return;
            }
        }
    }
}

impl MonitorManagerState {
    async fn new(req: InitializeRequest) -> Result<Self, InitializeMonitorManagerError> {
        use InitializeMonitorManagerError::*;
        common::create_dir_all(&req.config_path).map_err(CreateDir)?;

        let mut configs = HashMap::new();
        for entry in std::fs::read_dir(&req.config_path).map_err(ReadDir)? {
            let entry = entry.map_err(StatFile)?;

            if entry.metadata().map_err(GetFileMetadata)?.is_dir() {
                continue;
            }

            let name = entry.file_name().to_string_lossy().to_string();
            let is_json_file = Path::new(&name)
                .extension()
                .is_some_and(|ext| ext.eq_ignore_ascii_case("json"));
            if !is_json_file {
                continue;
            }

            let json = std::fs::read(entry.path()).map_err(ReadFile)?;
            let mut config: MonitorConfig =
                serde_json::from_slice(&json).map_err(|e| Deserialize(name, e))?;

            req.hooks
                .migrate_monitor(config.raw_mut())
                .map_err(|e| Migrate(config.id().to_owned(), e))?;

            configs.insert(config.id().to_owned(), config);
        }

        let mut state = Self {
            token: CancellationToken::new(),
            configs,
            started_monitors: HashMap::new(),
            recdb: req.recdb,
            eventdb: req.eventdb,
            logger: req.logger,
            streamer: req.streamer,
            path: req.config_path,
            hooks: req.hooks,
        };
        state.start_monitors().await;
        Ok(state)
    }

    pub async fn start_monitors(&mut self) {
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
            return Err(NotExist(id.clone()));
        };

        // Stop monitor if running.
        if let Some(monitor) = self.started_monitors.remove(id) {
            self.log(LogLevel::Info, id, "stopping");
            monitor.stop().await;
            self.log(LogLevel::Debug, id, "stopped");
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
        let id = config.id();

        // Write config to file.
        let path = self.config_path(id);
        let mut temp_path = path.clone();
        temp_path.set_file_name(id.to_string() + ".json.tmp");
        let json = serde_json::to_vec_pretty(&config)?;
        write_file_atomic2(Handle::current(), path, temp_path, json)
            .await
            .map_err(MonitorSetError::WriteFile)?;

        let created = !self.configs.contains_key(id);
        if created {
            self.log(LogLevel::Info, id, "created");
        } else {
            self.log(LogLevel::Info, id, "saved");
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
            self.log(LogLevel::Info, id, "stopping");
            monitor.stop().await;
            self.log(LogLevel::Debug, id, "stopped");
            self.started_monitors.remove(id);
        };

        if self.configs.remove(id).is_none() {
            return Err(NotExist(id.to_string()));
        }

        tokio::fs::remove_file(self.config_path(id)).await?;
        self.log(LogLevel::Info, id, "deleted");
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
        let id = config.id().to_owned();
        if !config.enabled() {
            self.log(LogLevel::Info, &id, "disabled");
            return None;
        }
        self.log(LogLevel::Info, &id, "starting");

        let monitor_token = self.token.child_token();
        let (shutdown_complete_tx, shutdown_complete_rx) = mpsc::channel(1);

        let (source_main, source_sub): (ArcSource, Option<ArcSource>) = match config.source() {
            SourceConfig::Rtsp(conf) => {
                let source_main = SourceRtsp::new(
                    monitor_token.child_token(),
                    shutdown_complete_tx.clone(),
                    self.logger.clone(),
                    self.streamer.clone(),
                    id.clone(),
                    conf.to_owned(),
                    StreamType::Main,
                )
                .expect("source main should never be None");

                let source_sub = SourceRtsp::new(
                    monitor_token.child_token(),
                    shutdown_complete_tx.clone(),
                    self.logger.clone(),
                    self.streamer.clone(),
                    id.clone(),
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
            self.hooks.clone(),
            self.logger.clone(),
            id.clone(),
            source_main.clone(),
            config.clone(),
            self.recdb.clone(),
        );

        let (source_main_tx, mut source_main_rx) = mpsc::channel(1);
        let (source_sub_tx, mut source_sub_rx) = mpsc::channel(1);

        let monitor = Arc::new(Monitor {
            token: monitor_token.clone(),
            id: id.clone(),
            eventdb: self.eventdb.clone(),
            hooks: self.hooks.clone(),
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

        self.hooks
            .on_monitor_start(monitor_token, monitor.clone())
            .await;

        Some(monitor)
    }

    pub async fn cancel(mut self) {
        // Cancel token.
        self.token.cancel();

        // Stop monitors.
        for (_, monitor) in self.started_monitors.drain() {
            monitor.stop().await;
        }

        // Break circular reference.
        drop(self.hooks);
    }

    fn log(&self, level: LogLevel, id: &MonitorId, msg: &str) {
        log_monitor(&self.logger, level, id, msg);
    }
}

#[allow(clippy::needless_pass_by_value, clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use bytesize::ByteSize;
    use common::{
        ArcStreamerMuxer, DummyLogger, DynError, H264Data, MonitorName, ParseMonitorIdError,
        TrackParameters,
        monitor::{
            Config, DummyMonitorHooks, DynH264Writer, Protocol, SelectedSource, SourceConfig,
            SourceRtspConfig, StreamerImpl,
        },
        time::UnixH264,
    };
    use pretty_assertions::assert_eq;
    use recdb::DiskImpl;
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
        let disk = DiskImpl::new(recordings_dir.to_path_buf(), ByteSize(0));
        RecDb::new(DummyLogger::new(), recordings_dir.to_path_buf(), disk)
    }

    fn new_test_eventdb(events_dir: &Path) -> EventDb {
        let (shutdown_complete_tx, _) = mpsc::channel(1);
        EventDb::new(
            CancellationToken::new(),
            shutdown_complete_tx,
            DummyLogger::new(),
            events_dir.to_path_buf(),
        )
    }

    async fn new_test_manager() -> (TempDir, PathBuf, MonitorManager) {
        let (temp_dir, config_dir) = prepare_dir();

        let manager = MonitorManager::new();
        manager
            .initialize(
                config_dir.clone(),
                Arc::new(new_test_recdb(temp_dir.path())),
                new_test_eventdb(temp_dir.path()),
                DummyLogger::new(),
                DummyStreamer::new(),
                DummyMonitorHooks::new(),
            )
            .await
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
        let (_temp_dir, config_dir, manager) = new_test_manager().await;

        let want = manager.monitor_configs().await.unwrap()[&m_id("1")].clone();
        let got = read_config(config_dir.join("1.json"));
        assert_eq!(want, got);
        manager.cancel().await;
    }

    #[tokio::test]
    async fn test_new_manager_unmarshal_error() {
        let (temp_dir, config_dir) = prepare_dir();

        std::fs::write(config_dir.join("1.json"), "{").unwrap();

        assert!(matches!(
            MonitorManager::new()
                .initialize(
                    config_dir,
                    Arc::new(new_test_recdb(temp_dir.path())),
                    new_test_eventdb(temp_dir.path()),
                    DummyLogger::new(),
                    DummyStreamer::new(),
                    DummyMonitorHooks::new(),
                )
                .await,
            Err(InitializeMonitorManagerError::Deserialize(..))
        ));
    }

    #[tokio::test]
    async fn test_monitor_set_create_new() {
        let (_temp_dir, config_dir, manager) = new_test_manager().await;

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

        let created = manager.monitor_set(config).await.unwrap().unwrap();
        assert!(created);

        let new = &m_id("new");
        let config = manager.monitor_config(new.clone()).await.unwrap().unwrap();
        let new_name = config.name();
        assert_eq!(&name("new"), new_name);

        // Check if changes were saved to file.
        let saved_config = read_config(config_dir.join("new.json"));
        assert_eq!(manager.monitor_configs().await.unwrap()[new], saved_config);
        manager.cancel().await;
    }

    #[tokio::test]
    async fn test_monitor_set_update() {
        let (_temp_dir, config_dir, manager) = new_test_manager().await;

        let one = m_id("1");
        let old_monitor = &manager.monitor_configs().await.unwrap()[&one];

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

        let created = manager.monitor_set(config).await.unwrap().unwrap();
        assert!(!created);

        let config = manager.monitor_config(one.clone()).await.unwrap().unwrap();
        let new_name = config.name();
        assert_eq!(&name("two"), new_name);

        // Check if changes were saved to file.
        let saved_config = read_config(config_dir.join("1.json"));
        assert_eq!(manager.monitor_configs().await.unwrap()[&one], saved_config);
        manager.cancel().await;
    }

    #[tokio::test]
    async fn test_monitor_delete_exist_error() {
        let (_temp_dir, _, manager) = new_test_manager().await;
        assert!(matches!(
            manager.monitor_delete(m_id("nil")).await.unwrap(),
            Err(MonitorDeleteError::NotExist(_))
        ));
    }

    #[tokio::test]
    async fn test_monitors_info() {
        let (_temp_dir, _, manager) = new_test_manager().await;
        let want: HashMap<MonitorId, MonitorInfo> = HashMap::from([
            (
                m_id("1"),
                MonitorInfo {
                    id: m_id("1"),
                    name: name("one"),
                    enable: false,
                    has_sub_stream: false,
                },
            ),
            (
                m_id("2"),
                MonitorInfo {
                    id: m_id("2"),
                    name: name("two"),
                    enable: false,
                    has_sub_stream: true,
                },
            ),
        ]);
        let got = manager.monitors_info().await.unwrap();
        assert_eq!(want, got);
        manager.cancel().await;
    }

    #[tokio::test]
    async fn test_monitor_configs() {
        let (_temp_dir, _, manager) = new_test_manager().await;

        let got = manager.monitor_configs().await.unwrap();
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
        manager.cancel().await;
    }

    #[tokio::test]
    async fn test_restart_monitor_not_exist_error() {
        let (_temp_dir, _, manager) = new_test_manager().await;
        assert!(matches!(
            manager.monitor_restart(m_id("x")).await.unwrap(),
            Err(MonitorRestartError::NotExist(_))
        ));
    }

    struct DummyStreamer;

    impl DummyStreamer {
        #[allow(clippy::new_ret_no_self)]
        fn new() -> ArcStreamer {
            Arc::new(Self {})
        }
    }

    #[async_trait]
    impl StreamerImpl for DummyStreamer {
        async fn new_muxer(
            &self,
            _: CancellationToken,
            _: MonitorId,
            _: bool,
            _: TrackParameters,
            _: UnixH264,
            _: H264Data,
        ) -> Result<Option<(ArcStreamerMuxer, DynH264Writer)>, DynError> {
            Ok(None)
        }
    }
}
