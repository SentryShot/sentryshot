// SPDX-License-Identifier: GPL-2.0-or-later

use axum::{
    response::Html,
    routing::{any, delete, get, post, put},
};
use bytesize::ByteSize;
use common::{
    ArcAuth, ArcLogger, ArcStorage, DynEnvConfig, EnvConfig, ILogger, LogEntry, LogLevel,
    monitor::{ArcMonitorManager, ArcStreamer},
    time::Duration,
};
use env::{EnvConf, EnvConfigNewError};
use eventdb::EventDb;
use hls::HlsServer;

use jiff::tz::TimeZone;
use log::{
    Logger,
    log_db::{CreateLogDBError, LogDb},
    slow_poller::SlowPoller,
};
use monitor::{InitializeMonitorManagerError, MonitorManager};
use monitor_groups::{ArcMonitorGroups, CreateMonitorGroupsError, MonitorGroups};
use plugin::{
    Application, PluginManager, PreLoadPluginsError, PreLoadedPlugins, pre_load_plugins,
    types::{NewAuthError, Router},
};
use rand::{Rng, distr::Alphanumeric};
use recdb::{RecDb, StorageImpl};
use recording::VideoCache;
use rust_embed::RustEmbed;
use std::{
    collections::HashMap,
    net::{IpAddr, Ipv4Addr, SocketAddr},
    path::PathBuf,
    sync::Arc,
};
use thiserror::Error;
use tokio::{
    net::TcpListener,
    runtime::Handle,
    signal,
    sync::{Mutex, oneshot},
};
use tokio_util::{
    sync::CancellationToken,
    task::{TaskTracker, task_tracker::TaskTrackerToken},
};
use vod::VodCache;
use web::{Templater, minify};

#[allow(clippy::wildcard_imports)]
use handler::*;

#[derive(RustEmbed)]
#[folder = "../../web/assets"]
struct Asset;

#[derive(RustEmbed)]
#[folder = "../../web/templates"]
struct Tpls;

#[derive(Debug, Error)]
pub enum RunError {
    #[error("create env config: {0}")]
    NewEnvConfig(#[from] EnvConfigNewError),

    #[error("prepare plugins: {0}")]
    PreparePlugins(#[from] PreLoadPluginsError),

    #[error("create log db: {0}")]
    NewLogDb(#[from] CreateLogDBError),

    #[error("create authenticator: {0}")]
    NewAuth(#[from] NewAuthError),

    #[error("create monitor manager: {0}")]
    NewMonitorManager(#[from] InitializeMonitorManagerError),

    #[error("create monitor groups: {0}")]
    CreateMonitorGroups(#[from] CreateMonitorGroupsError),

    #[error("determine time zone")]
    TimeZone,

    #[error("listen on sigterm: {0}")]
    SigTermListener(std::io::Error),
}

pub async fn run(rt_handle: Handle, config_path: &PathBuf) -> Result<(), RunError> {
    // Initialize app.
    let (mut app, pre_loaded_plugins) = App::new(rt_handle.clone(), config_path).await?;
    let mut plugin_manager = PluginManager::new(pre_loaded_plugins, &app);
    app.setup_routes(&mut plugin_manager)?;

    // Run app.
    let tracker = app.run(plugin_manager).await?;
    // Block until app stops.
    tracker.close();
    tracker.wait().await;

    Ok(())
}

pub struct App {
    rt_handle: Handle,
    token: CancellationToken,
    env: EnvConf,
    logger: Arc<Logger>,
    tracker: TaskTracker,
    log_db: LogDb,
    auth: ArcAuth,
    streamer: Streamer,
    monitor_manager: Arc<MonitorManager>,
    monitor_groups: ArcMonitorGroups,
    storage: ArcStorage,
    recdb: Arc<RecDb>,
    eventdb: EventDb,
    router: Router,
}

#[derive(Clone)]
pub enum Streamer {
    Hls(HlsServer),
    Sp(streamer::Streamer),
}

impl From<Streamer> for ArcStreamer {
    fn from(val: Streamer) -> Self {
        match val {
            Streamer::Hls(v) => Arc::new(v),
            Streamer::Sp(v) => Arc::new(v),
        }
    }
}

impl App {
    pub async fn new(
        rt_handle: Handle,
        config_path: &PathBuf,
    ) -> Result<(App, PreLoadedPlugins), RunError> {
        let token = CancellationToken::new();
        let env = EnvConf::new(config_path)?;

        let pre_loaded_plugins = pre_load_plugins(env.plugin_dir(), env.plugins())?;
        let tracker = TaskTracker::new();

        let logger = Arc::new(Logger::new(
            pre_loaded_plugins.log_sources().to_owned(),
            env.debug_log_stdout(),
        ));

        let log_dir = env.storage_dir().join("logs");
        let log_db = LogDb::new(
            token.child_token(),
            tracker.token(),
            log_dir,
            env.max_disk_usage(),
            ByteSize::mb(100),
            10000,
            1024,
        )?;

        {
            let log_db2 = log_db.clone();
            let token2 = token.clone();
            let feed = logger.subscribe();
            tokio::spawn(async move {
                log_db2.save_logs(token2, feed).await;
            });

            let log_db2 = log_db.clone();
            let token2 = token.clone();
            let logger2 = logger.clone();
            tokio::spawn(async move {
                log_db2.prune_loop(token2, logger2).await;
            });
        }

        let new_auth = pre_loaded_plugins.new_auth_fn();
        let auth = new_auth(rt_handle.clone(), env.config_dir(), logger.clone())?;

        let storage = StorageImpl::new(env.storage_dir().to_path_buf(), env.max_disk_usage());
        let recdb = Arc::new(RecDb::new(
            logger.clone(),
            env.recordings_dir().to_path_buf(),
            storage.clone(),
        ));
        let eventdb = EventDb::new(
            token.clone(),
            tracker.token(),
            logger.clone(),
            env.storage_dir().join("eventdb"),
        );

        let streamer = match env.flags().streamer {
            common::Streamer::Hls => Streamer::Hls(HlsServer::new(token.clone(), logger.clone())),
            common::Streamer::Sp => {
                Streamer::Sp(streamer::Streamer::new(token.clone(), logger.clone()))
            }
        };

        let monitor_manager = Arc::new(MonitorManager::new());

        let monitor_groups =
            Arc::new(MonitorGroups::new(env.storage_dir(), env.config_dir()).await?);

        let router = Router::new(axum::Router::new(), auth.clone());

        Ok((
            App {
                rt_handle,
                token,
                env,
                logger,
                tracker,
                log_db,
                auth,
                streamer,
                monitor_manager,
                monitor_groups,
                storage,
                recdb,
                eventdb,
                router,
            },
            pre_loaded_plugins,
        ))
    }

    #[allow(clippy::similar_names, clippy::too_many_lines)]
    pub fn setup_routes(&mut self, plugin_manager: &mut PluginManager) -> Result<(), RunError> {
        // https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/ETag
        let assets_etag: String = rand::rng()
            .sample_iter(&Alphanumeric)
            .take(8)
            .map(char::from)
            .collect();

        let mut assets = Asset::load();
        plugin_manager.edit_assets_hooks(&mut assets);
        minify(&mut assets);

        let tpls = Tpls::load();

        let mut templates = {
            fn to_string(input: &[u8]) -> String {
                String::from_utf8(input.to_vec()).expect("file to only contain valid characters")
            }
            HashMap::from([
                ("html", to_string(&tpls["include/html.tpl"])),
                ("html2", to_string(&tpls["include/html2.tpl"])),
                ("meta", to_string(&tpls["include/meta.tpl"])),
                ("sidebar", to_string(&tpls["include/sidebar.tpl"])),
                ("live", to_string(&tpls["live.tpl"])),
                ("recordings", to_string(&tpls["recordings.tpl"])),
                ("settings", to_string(&tpls["settings.tpl"])),
                ("logs", to_string(&tpls["logs.tpl"])),
            ])
        };
        plugin_manager.edit_templates_hooks(&mut templates);

        let time_zone = TimeZone::system()
            .iana_name()
            .ok_or(RunError::TimeZone)?
            .to_owned();
        self.log(LogLevel::Debug, &format!("TZ={time_zone}"));

        let templater = Arc::new(Templater::new(
            self.logger.clone(),
            self.monitor_manager.clone(),
            self.monitor_groups.clone(),
            templates,
            time_zone,
            self.env.flags(),
        ));

        let mut router = self
            .router
            .clone()
            // Root.
            .route_user_no_csrf("/", get(|| async { Html("<a href='./live'>/live</a>") }))
            // Live page.
            .route_user_no_csrf("/live", get(template_handler).with_state(templater.clone()))
            // Recordings page.
            .route_user_no_csrf(
                "/recordings",
                get(template_handler).with_state(templater.clone()),
            )
            // Settings page.
            .route_admin_no_csrf(
                "/settings",
                get(template_handler).with_state(templater.clone()),
            )
            // Logs page.
            .route_admin_no_csrf("/logs", get(template_handler).with_state(templater.clone()))
            // Assets.
            .route_user_no_csrf(
                "/assets/{*file}",
                get(asset_handler).with_state((assets, assets_etag)),
            )
            // Video on demand.
            .route_user_no_csrf(
                "/vod/vod.mp4",
                get(vod_handler).with_state(VodHandlerState {
                    logger: self.logger.clone(),
                    recdb: self.recdb.clone(),
                    cache: VodCache::new(),
                }),
            )
            // API page.
            .route_admin_no_csrf("/api", get(api_page_handler))
            // Account.
            .route_admin(
                "/api/account",
                delete(account_delete_handler)
                    .put(account_put_handler)
                    .with_state(self.auth.clone()),
            )
            // Account token.
            .route_user_no_csrf("/api/account/my-token", get(account_my_token_handler))
            // Accounts list.
            .route_admin_no_csrf(
                "/api/accounts",
                get(accounts_handler).with_state(self.auth.clone()),
            )
            // Recording query.
            .route_user_no_csrf(
                "/api/recording/query",
                get(recording_query_handler).with_state(RecordingQueryHandlerState {
                    logger: self.logger.clone(),
                    recdb: self.recdb.clone(),
                    eventdb: self.eventdb.clone(),
                }),
            )
            // Log slow poll.
            .route_admin_no_csrf(
                "/api/log/slow-poll",
                get(log_slow_poll_handler).with_state(SlowPoller::new(
                    self.token.child_token(),
                    self.logger.subscribe(),
                )),
            )
            // Log query.
            .route_admin_no_csrf(
                "/api/log/query",
                get(log_query_handler).with_state(self.log_db.clone()),
            )
            // Monitor.
            .route_admin(
                "/api/monitor",
                delete(monitor_delete_handler).with_state(self.monitor_manager.clone()),
            )
            .route_admin(
                "/api/monitor",
                put(monitor_put_handler).with_state(self.monitor_manager.clone()),
            )
            // Monitor restart.
            .route_admin(
                "/api/monitor/restart",
                post(monitor_restart_handler).with_state(self.monitor_manager.clone()),
            )
            // Monitors.
            .route_admin_no_csrf(
                "/api/monitors",
                get(monitors_handler).with_state(self.monitor_manager.clone()),
            )
            // Monitor groups.
            .route_admin_no_csrf(
                "/api/monitor-groups",
                get(monitor_groups_get_handler).with_state(self.monitor_groups.clone()),
            )
            .route_admin(
                "/api/monitor-groups",
                put(monitor_groups_put_handler).with_state(self.monitor_groups.clone()),
            )
            // Recording delete.
            .route_user_no_csrf(
                "/api/recording/delete/{*id}",
                delete(recording_delete_handler).with_state(self.recdb.clone()),
            )
            // Recording thumbnail.
            .route_user_no_csrf(
                "/api/recording/thumbnail/{*id}",
                get(recording_thumbnail_handler).with_state(self.recdb.clone()),
            )
            // Recording video.
            .route_user_no_csrf(
                "/api/recording/video/{*id}",
                get(recording_video_handler).with_state(RecordingVideoState {
                    rec_db: self.recdb.clone(),
                    video_cache: Arc::new(Mutex::new(VideoCache::new())),
                    logger: self.logger.clone(),
                }),
            );

        match &self.streamer {
            Streamer::Hls(hls_server) => {
                router = router
                    // Hls server.
                    .route_user_no_csrf(
                        "/hls/{*path}",
                        any(hls_handler).with_state(hls_server.clone()),
                    );
            }
            Streamer::Sp(mp4_streamer) => {
                router = router
                    // Streamer start session.
                    .route_user_no_csrf(
                        "/api/streamer/start-session",
                        post(streamer_start_session_handler).with_state(mp4_streamer.clone()),
                    )
                    // Streamer play.
                    .route_user_no_csrf(
                        "/api/streamer/play",
                        get(streamer_play_handler).with_state(mp4_streamer.clone()),
                    );
            }
        }

        self.router = plugin_manager.router_hooks(router);

        Ok(())
    }

    // `App` must be dropped when this returns.
    pub async fn run(self, plugin_manager: PluginManager) -> Result<TaskTracker, RunError> {
        let token2 = self.token.clone();
        let task_token = self.tracker.token();
        let logger = self.logger.clone();
        let recdb = self.recdb.clone();
        let eventdb = self.eventdb.clone();
        tokio::spawn(async move {
            prune_loop(
                token2,
                task_token,
                logger,
                recdb,
                eventdb,
                Duration::from_minutes(10).as_std().expect(""),
            )
            .await;
        });

        let port = self.env.port();
        self.log(LogLevel::Info, &format!("Serving app on port {port}"));

        let monitors_dir = self.env.config_dir().join("monitors");
        self.monitor_manager
            .initialize(
                monitors_dir,
                self.recdb.clone(),
                self.eventdb.clone(),
                self.logger.clone(),
                self.streamer.clone().into(),
                Arc::new(plugin_manager),
            )
            .await?;

        let token = self.token.clone();
        let task_token = self.tracker.token();
        tokio::spawn(async move {
            token.cancelled().await;
            self.monitor_manager.cancel().await;
            drop(task_token);
        });

        let (server_exited_tx, server_exited_rx) = oneshot::channel();
        let addr = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(0, 0, 0, 0)), self.env.port());

        tokio::spawn(start_server(
            self.token.child_token(),
            self.tracker.token(),
            server_exited_tx,
            addr,
            self.router,
        ));

        // Shutdown conditions.
        let mut sigterm = signal::unix::signal(signal::unix::SignalKind::terminate())
            .map_err(RunError::SigTermListener)?;
        tokio::spawn(async move {
            tokio::select! {
                result = signal::ctrl_c() => {
                    match result {
                        Ok(()) => eprintln!("\nreceived interrupt, stopping..\n"),
                        Err(e) => eprintln!("\ninterrupt error: {e}"),
                    }
                }
                _ = sigterm.recv() => eprintln!("\nreceived terminate, stopping..\n"),
                res = server_exited_rx => {
                    if let Err(e) = res {
                        eprintln!("server error: {e}");
                    }
                },
            }
            self.token.cancel();
        });

        Ok(self.tracker)
    }

    fn log(&self, level: LogLevel, msg: &str) {
        self.logger.log(LogEntry::new2(level, "app", msg));
    }
}

impl Application for App {
    fn rt_handle(&self) -> Handle {
        self.rt_handle.clone()
    }
    fn token(&self) -> CancellationToken {
        self.token.clone()
    }
    fn auth(&self) -> ArcAuth {
        self.auth.clone()
    }
    fn monitor_manager(&self) -> ArcMonitorManager {
        self.monitor_manager.clone()
    }
    fn storage(&self) -> ArcStorage {
        self.storage.clone()
    }
    fn task_token(&self) -> TaskTrackerToken {
        self.tracker.token()
    }
    fn logger(&self) -> common::ArcLogger {
        self.logger.clone()
    }
    fn env(&self) -> DynEnvConfig {
        Box::new(self.env.clone())
    }
}

#[derive(Debug, Error)]
enum ServerError {
    #[error("bind: {0}")]
    Bind(std::io::Error),

    #[error("{0}")]
    Server(std::io::Error),
}

async fn start_server(
    token: CancellationToken,
    _task_token: TaskTrackerToken,
    on_exit: oneshot::Sender<Result<(), ServerError>>,
    addr: SocketAddr,
    router: Router,
) {
    let listener = match TcpListener::bind(addr).await {
        Ok(v) => v,
        Err(e) => {
            let _ = on_exit.send(Err(ServerError::Bind(e)));
            return;
        }
    };
    let graceful = axum::serve(listener, router.inner())
        .with_graceful_shutdown(async move { token.cancelled().await });
    let _ = on_exit.send(graceful.await.map_err(ServerError::Server));
}

pub async fn prune_loop(
    token: CancellationToken,
    _task_token: TaskTrackerToken,
    logger: ArcLogger,
    recdb: Arc<RecDb>,
    eventdb: EventDb,
    interval: std::time::Duration,
) {
    tokio::select! {
        () = token.cancelled() => return,
        () = tokio::time::sleep(std::time::Duration::from_secs(30)) => {}
    }
    loop {
        let (oldest_recording, err) = recdb.prune().await;
        if let Some(e) = err {
            logger.log(LogEntry::new2(
                LogLevel::Error,
                "recdb",
                &format!("prune recordings: {e}"),
            ));
        }
        if let Some(oldest_recording) = oldest_recording {
            if let Err(e) = eventdb.prune(oldest_recording).await {
                logger.log(LogEntry::new2(
                    LogLevel::Error,
                    "eventdb",
                    &format!("prune events: {e}"),
                ));
            }
        }
        tokio::select! {
            () = token.cancelled() => return,
            () = tokio::time::sleep(interval) => {}
        }
    }
}
