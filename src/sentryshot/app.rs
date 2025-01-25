// SPDX-License-Identifier: GPL-2.0-or-later

use axum::{
    middleware,
    response::Html,
    routing::{any, delete, get, post},
    Router,
};
use bytesize::ByteSize;
use common::{
    monitor::ArcMonitorManager, time::Duration, ArcAuth, DynEnvConfig, EnvConfig, ILogger,
    LogEntry, LogLevel,
};
use env::{EnvConf, EnvConfigNewError};
use hls::HlsServer;
use log::{
    log_db::{LogDb, LogDbHandle, NewLogDbError},
    Logger,
};
use monitor::{MonitorManager, NewMonitorManagerError};
use monitor_groups::{ArcMonitorGroups, CreateMonitorGroupsError, MonitorGroups};
use plugin::{
    pre_load_plugins,
    types::{admin, csrf, user, NewAuthError},
    Application, PluginManager, PreLoadPluginsError, PreLoadedPlugins,
};
use rand::{distributions::Alphanumeric, Rng};
use recdb::{Disk, RecDb};
use recording::VideoCache;
use rust_embed::RustEmbed;
use std::{
    collections::HashMap,
    ffi::OsStr,
    net::{IpAddr, Ipv4Addr, SocketAddr},
    path::{Path, PathBuf},
    sync::Arc,
};
use thiserror::Error;
use tokio::{
    net::TcpListener,
    runtime::Handle,
    signal,
    sync::{mpsc, oneshot, Mutex},
};
use tokio_util::sync::CancellationToken;
use tower::ServiceBuilder;
use vod::VodCache;
use web::Templater;

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
    NewLogDb(#[from] NewLogDbError),

    #[error("create authenticator: {0}")]
    NewAuth(#[from] NewAuthError),

    #[error("create monitor manager: {0}")]
    NewMonitorManager(#[from] NewMonitorManagerError),

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
    let mut shutdown_complete_rx = app.run(plugin_manager).await?;
    // Block until app stops.
    shutdown_complete_rx.recv().await;

    Ok(())
}

pub struct App {
    rt_handle: Handle,
    token: CancellationToken,
    env: EnvConf,
    logger: Arc<Logger>,
    shutdown_complete_tx: mpsc::Sender<()>,
    shutdown_complete_rx: mpsc::Receiver<()>,
    log_db: Arc<LogDbHandle>,
    auth: ArcAuth,
    hls_server: Arc<HlsServer>,
    monitor_manager: ArcMonitorManager,
    monitor_groups: ArcMonitorGroups,
    recdb: Arc<RecDb>,
    router: Router,
}

impl App {
    pub async fn new(
        rt_handle: Handle,
        config_path: &PathBuf,
    ) -> Result<(App, PreLoadedPlugins), RunError> {
        let token = CancellationToken::new();
        let env = EnvConf::new(config_path)?;

        let pre_loaded_plugins = pre_load_plugins(env.plugin_dir(), env.plugins())?;
        let (shutdown_complete_tx, shutdown_complete_rx) = mpsc::channel::<()>(1);

        let logger = Arc::new(Logger::new(pre_loaded_plugins.log_sources().to_owned()));

        let log_dir = env.storage_dir().join("logs");
        let log_db = Arc::new(LogDb::new(
            shutdown_complete_tx.clone(),
            log_dir,
            env.max_disk_usage(),
            ByteSize::mb(100),
        )?);

        {
            let log_db2 = log_db.clone();
            let token2 = token.clone();
            let logger2 = logger.clone();
            tokio::spawn(async move {
                log_db2.save_logs(token2, logger2).await;
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

        let rec_db = Arc::new(RecDb::new(
            logger.clone(),
            env.recordings_dir().to_path_buf(),
            Disk::new(env.storage_dir().to_path_buf(), env.max_disk_usage()),
        ));

        let hls_server = Arc::new(HlsServer::new(token.clone(), logger.clone()));

        let monitors_dir = env.config_dir().join("monitors");
        let monitor_manager = Arc::new(MonitorManager::new(
            monitors_dir,
            rec_db.clone(),
            logger.clone(),
            hls_server.clone(),
        )?);

        let monitor_groups = Arc::new(MonitorGroups::new(env.storage_dir()).await?);

        let router = Router::new();

        Ok((
            App {
                rt_handle,
                token,
                env,
                logger,
                shutdown_complete_tx,
                shutdown_complete_rx,
                log_db,
                auth,
                hls_server,
                monitor_manager,
                monitor_groups,
                recdb: rec_db,
                router,
            },
            pre_loaded_plugins,
        ))
    }

    #[allow(clippy::similar_names)]
    pub fn setup_routes(&mut self, plugin_manager: &mut PluginManager) -> Result<(), RunError> {
        // https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/ETag
        let assets_etag: String = rand::thread_rng()
            .sample_iter(&Alphanumeric)
            .take(8)
            .map(char::from)
            .collect();

        let mut assets = Asset::load();
        plugin_manager.edit_assets_hooks(&mut assets);

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

        let templater = Arc::new(Templater::new(
            self.logger.clone(),
            self.monitor_manager.clone(),
            self.monitor_groups.clone(),
            templates,
            time_zone().ok_or(RunError::TimeZone)?,
        ));
        let template_handler_state = TemplateHandlerState {
            templater: templater.clone(),
            auth: self.auth.clone(),
        };

        let router = self
            .router
            .clone()
            // Root.
            .route(
                "/",
                get(|| async { Html("<a href='./live'>/live</a>") })
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), user))
                    .with_state(self.auth.clone()),
            )
            // Live page.
            .route(
                "/live",
                get(template_handler)
                    .with_state(template_handler_state.clone())
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), user))
                    .with_state(self.auth.clone()),
            )
            // Recordings page.
            .route(
                "/recordings",
                get(template_handler)
                    .with_state(template_handler_state.clone())
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), user))
                    .with_state(self.auth.clone()),
            )
            // Settings page.
            .route(
                "/settings",
                get(template_handler)
                    .with_state(template_handler_state.clone())
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), admin))
                    .with_state(self.auth.clone()),
            )
            // Logs page.
            .route(
                "/logs",
                get(template_handler)
                    .with_state(template_handler_state.clone())
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), admin))
                    .with_state(self.auth.clone()),
            )
            // Assets.
            .route(
                "/assets/*file",
                get(asset_handler)
                    .with_state((assets, assets_etag))
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), user))
                    .with_state(self.auth.clone()),
            )
            // Hls server.
            .route(
                "/hls/*path",
                any(hls_handler)
                    .with_state(self.hls_server.clone())
                    .layer(middleware::from_fn_with_state(self.auth.clone(), user))
                    .with_state(self.auth.clone()),
            )
            // Video on demand.
            .route(
                "/vod/vod.mp4",
                get(vod_handler)
                    .with_state(VodHandlerState {
                        logger: self.logger.clone(),
                        recdb: self.recdb.clone(),
                        cache: VodCache::new(),
                    })
                    .layer(middleware::from_fn_with_state(self.auth.clone(), user))
                    .with_state(self.auth.clone()),
            )
            // API page.
            .route(
                "/api",
                get(api_page_handler)
                    .layer(middleware::from_fn_with_state(self.auth.clone(), admin))
                    .with_state(self.auth.clone()),
            )
            // Account.
            .route(
                "/api/account",
                delete(account_delete_handler)
                    .put(account_put_handler)
                    .route_layer(
                        ServiceBuilder::new()
                            .layer(middleware::from_fn_with_state(self.auth.clone(), admin))
                            .layer(middleware::from_fn_with_state(self.auth.clone(), csrf)),
                    )
                    .with_state(self.auth.clone()),
            )
            // Account token.
            .route(
                "/api/account/my-token",
                get(account_my_token_handler)
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), user))
                    .with_state(self.auth.clone()),
            )
            // Accounts list.
            .route(
                "/api/accounts",
                get(accounts_handler)
                    .with_state(self.auth.clone())
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), admin))
                    .with_state(self.auth.clone()),
            )
            // Recording query.
            .route(
                "/api/recording/query",
                get(recording_query_handler)
                    .with_state(RecordingQueryHandlerState {
                        logger: self.logger.clone(),
                        rec_db: self.recdb.clone(),
                    })
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), user))
                    .with_state(self.auth.clone()),
            )
            // Log WebSocket feed.
            .route(
                "/api/log/feed",
                get(log_feed_handler)
                    .with_state(LogFeedHandlerState {
                        logger: self.logger.clone(),
                        auth: self.auth.clone(),
                    })
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), admin))
                    .with_state(self.auth.clone()),
            )
            // Log query.
            .route(
                "/api/log/query",
                get(log_query_handler)
                    .with_state(self.log_db.clone())
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), admin))
                    .with_state(self.auth.clone()),
            )
            // Monitor.
            .route(
                "/api/monitor",
                delete(monitor_delete_handler)
                    .put(monitor_put_handler)
                    .with_state(self.monitor_manager.clone())
                    .route_layer(
                        ServiceBuilder::new()
                            .layer(middleware::from_fn_with_state(self.auth.clone(), admin))
                            .layer(middleware::from_fn_with_state(self.auth.clone(), csrf)),
                    )
                    .with_state(self.auth.clone()),
            )
            // Monitor restart.
            .route(
                "/api/monitor/restart",
                post(monitor_restart_handler)
                    .with_state(self.monitor_manager.clone())
                    .route_layer(
                        ServiceBuilder::new()
                            .layer(middleware::from_fn_with_state(self.auth.clone(), admin))
                            .layer(middleware::from_fn_with_state(self.auth.clone(), csrf)),
                    )
                    .with_state(self.auth.clone()),
            )
            // Monitors.
            .route(
                "/api/monitors",
                get(monitors_handler)
                    .with_state(self.monitor_manager.clone())
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), admin))
                    .with_state(self.auth.clone()),
            )
            // Monitor groups.
            .route(
                "/api/monitor-groups",
                get(monitor_groups_get_handler)
                    .put(monitor_groups_put_handler)
                    .with_state(self.monitor_groups.clone())
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), admin))
                    .with_state(self.auth.clone()),
            )
            // Recording delete.
            .route(
                "/api/recording/delete/*id",
                delete(recording_delete_handler)
                    .with_state(self.recdb.clone())
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), user))
                    .with_state(self.auth.clone()),
            )
            // Recording thumbnail.
            .route(
                "/api/recording/thumbnail/*id",
                get(recording_thumbnail_handler)
                    .with_state(self.recdb.clone())
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), user))
                    .with_state(self.auth.clone()),
            )
            // Recording video.
            .route(
                "/api/recording/video/*id",
                get(recording_video_handler)
                    .with_state(RecordingVideoState {
                        rec_db: self.recdb.clone(),
                        video_cache: Arc::new(Mutex::new(VideoCache::new())),
                        logger: self.logger.clone(),
                    })
                    .route_layer(middleware::from_fn_with_state(self.auth.clone(), user))
                    .with_state(self.auth.clone()),
            );

        self.router = plugin_manager.router_hooks(router);

        Ok(())
    }

    // `App` must be dropped when this returns.
    pub async fn run(self, plugin_manager: PluginManager) -> Result<mpsc::Receiver<()>, RunError> {
        let rec_db = self.recdb.clone();
        let token2 = self.token.clone();
        tokio::spawn(async move {
            rec_db
                .prune_loop(token2, Duration::from_minutes(10).as_std().expect(""))
                .await;
        });

        self.logger.log(LogEntry {
            level: LogLevel::Info,
            source: "app".try_into().expect("valid"),
            monitor_id: None,
            message: format!("Serving app on port {}", self.env.port())
                .try_into()
                .expect("not empty"),
        });

        self.monitor_manager
            .start_monitors(Arc::new(plugin_manager))
            .await;

        let token = self.token.clone();
        let shutdown_complete_tx = self.shutdown_complete_tx.clone();
        tokio::spawn(async move {
            token.cancelled().await;
            self.monitor_manager.stop().await;
            drop(shutdown_complete_tx);
        });

        let (server_exited_tx, server_exited_rx) = oneshot::channel();
        let addr = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(0, 0, 0, 0)), self.env.port());

        tokio::spawn(start_server(
            self.token.child_token(),
            self.shutdown_complete_tx.clone(),
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

        Ok(self.shutdown_complete_rx)
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
    fn shutdown_complete_tx(&self) -> mpsc::Sender<()> {
        self.shutdown_complete_tx.clone()
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
    _shutdown_complete: mpsc::Sender<()>,
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
    let graceful = axum::serve(listener, router)
        .with_graceful_shutdown(async move { token.cancelled().await });
    let _ = on_exit.send(graceful.await.map_err(ServerError::Server));
}

// TimeZone returns system time zone location.
fn time_zone() -> Option<String> {
    // Try 'TZ'.
    if let Ok(tz) = std::env::var("TZ") {
        if tz.is_empty() {
            return Some("UTC".to_owned());
        }
        if !tz.starts_with(':') && !tz.starts_with('/') {
            return Some(tz);
        }
    }

    // Try reading '/etc/timezone'.
    if let Ok(zone) = std::fs::read_to_string("/etc/timezone") {
        if !zone.is_empty() {
            return Some(zone.trim().to_owned());
        }
    }

    // Try matching '/etc/localtime' to a location.
    let localtime = std::fs::read("/etc/localtime").unwrap_or_default();
    let mut zone = None;
    let mut dirs = vec![PathBuf::from("/usr/share/zoneinfo")];
    while let Some(dir) = dirs.pop() {
        let Ok(entries) = std::fs::read_dir(dir) else {
            continue;
        };
        for entry in entries {
            let Ok(entry) = entry else {
                continue;
            };
            let Ok(metadata) = entry.metadata() else {
                continue;
            };
            if metadata.is_dir() {
                dirs.push(entry.path());
                continue;
            }

            let file_path = entry.path();
            let Ok(file) = std::fs::read(&file_path) else {
                continue;
            };
            if file == localtime {
                let dir = file_path.parent().unwrap_or_else(|| Path::new(""));
                let city = file_path.file_name().unwrap_or_else(|| OsStr::new(""));
                let region = dir.file_name().unwrap_or_else(|| OsStr::new(""));
                zone = Some(PathBuf::from(city));

                if let Some("zoneinfo" | "posix") = region.to_str() {
                } else {
                    let mut region = PathBuf::from(region);
                    region.push(city);
                    zone = Some(region);
                }
            }
        }
    }
    zone.map(|v| v.to_string_lossy().to_string().trim().to_owned())
}
