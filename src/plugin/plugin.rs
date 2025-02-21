// SPDX-License-Identifier: GPL-2.0-or-later

pub mod types;

use async_trait::async_trait;
use common::{
    monitor::{ArcMonitor, ArcMonitorManager, MonitorConfig, MonitorHooks},
    ArcAuth, ArcLogger, DynEnvConfig, EnvPlugin, Event, LogEntry, LogLevel, LogSource,
};
use libloading::{Library, Symbol};
use sentryshot_util::Frame;
use std::{
    path::{Path, PathBuf},
    process,
    sync::Arc,
};
use thiserror::Error;
use tokio::{self, runtime::Handle, sync::mpsc};
use tokio_util::sync::CancellationToken;
use types::{Assets, NewAuthFn, Router, Templates};

pub trait PreLoadPlugin {
    fn add_log_source(&self) -> Option<LogSource> {
        None
    }
    fn set_new_auth(&self) -> Option<NewAuthFn> {
        None
    }
}
#[async_trait]
pub trait Plugin {
    fn edit_templates(&self, _templates: &mut Templates) {}
    fn edit_assets(&self, _assets: &mut Assets) {}
    fn route(&self, router: Router) -> Router {
        router
    }

    // Non-blocking.
    /*async fn on_source_start(
        &self,
        _token: CancellationToken,
        _monitor_id: MonitorId,
        _stream_type: StreamType,
    ) {
    }*/

    async fn on_monitor_start(&self, _token: CancellationToken, _monitor: ArcMonitor) {}
    fn on_thumb_save(&self, _config: &MonitorConfig, frame: Frame) -> Frame {
        frame
    }
    async fn on_event(&self, _event: Event, _config: MonitorConfig) {}
}

pub trait Application {
    fn rt_handle(&self) -> Handle;
    fn token(&self) -> CancellationToken;
    fn auth(&self) -> ArcAuth;
    fn monitor_manager(&self) -> ArcMonitorManager;
    fn shutdown_complete_tx(&self) -> mpsc::Sender<()>;
    fn logger(&self) -> ArcLogger;
    fn env(&self) -> DynEnvConfig;
}

#[derive(Default)]
pub struct PreLoadedPlugins {
    log_sources: Vec<LogSource>,
    new_auth_fn: Option<NewAuthFn>,
    load_fns: Vec<(String, LoadFn)>,
}

type LoadFn = fn(app: &dyn Application) -> Arc<dyn Plugin + Send + Sync>;

#[derive(Debug, Error)]
pub enum PreLoadPluginsError {
    #[error("plugin not found: {0}")]
    NotFound(PathBuf),

    #[error("load library: '{0}' {1}")]
    LoadLibrary(String, libloading::Error),

    #[error("load symbol: {0}")]
    LoadSymbol(libloading::Error),

    #[error("version missmatch: {0} expected='{1}' got='{2}'")]
    VersionMismatch(String, String, String),
}

pub fn pre_load_plugins(
    plugin_dir: &Path,
    plugins: &Option<Vec<EnvPlugin>>,
) -> Result<PreLoadedPlugins, PreLoadPluginsError> {
    use PreLoadPluginsError::*;

    let mut pre_loaded_plugins = PreLoadedPlugins::default();

    let Some(plugins) = plugins else {
        return Ok(pre_loaded_plugins);
    };

    let core_version = get_version();

    for plugin in plugins {
        if !plugin.enable() {
            continue;
        }

        let plugin_name = plugin.name();
        let plugin_path = {
            // The long name is use in development builds.
            let short = plugin_dir.join(plugin_name);
            let long = plugin_dir.join(format!("lib{plugin_name}.so"));
            if short.exists() {
                short
            } else if long.exists() {
                long
            } else {
                return Err(NotFound(short));
            }
        };

        unsafe {
            let dylib =
                Library::new(plugin_path).map_err(|e| LoadLibrary(plugin_name.to_owned(), e))?;

            // Check version first.
            let version: Symbol<fn() -> String> = dylib.get(b"version").map_err(LoadSymbol)?;
            if version() != core_version {
                return Err(VersionMismatch(
                    plugin_name.to_owned(),
                    core_version,
                    version(),
                ));
            }

            // If pre_load is defined.
            if let Ok(pre_load) = dylib.get::<Symbol<fn() -> Box<dyn PreLoadPlugin>>>(b"pre_load") {
                let plugin = pre_load();

                if let Some(new_auth_fn) = plugin.set_new_auth() {
                    if pre_loaded_plugins.new_auth_fn.is_some() {
                        eprint!("\n\nERROR: Only a single autentication plugin is allowed.\n\n");
                        process::exit(1);
                    }
                    pre_loaded_plugins.new_auth_fn = Some(new_auth_fn);
                }

                if let Some(source) = plugin.add_log_source() {
                    pre_loaded_plugins.log_sources.push(source);
                }
            }

            let load_fn: Symbol<LoadFn> = dylib.get(b"load").map_err(LoadSymbol)?;

            pre_loaded_plugins
                .load_fns
                .push((plugin_name.to_owned(), *load_fn));

            // Keep the shared library loaded until the program exits.
            Box::leak(Box::new(dylib));
        };
    }

    Ok(pre_loaded_plugins)
}

impl PreLoadedPlugins {
    #[must_use]
    pub fn log_sources(&self) -> &[LogSource] {
        &self.log_sources
    }

    #[must_use]
    pub fn new_auth_fn(&self) -> NewAuthFn {
        let Some(new_auth_fn) = self.new_auth_fn else {
            eprint!("\n\nERROR: No authentication plugin enabled.\n\n");
            process::exit(1);
        };
        new_auth_fn
    }
}

#[derive(Default)]
pub struct PluginManager {
    plugins: Vec<Arc<dyn Plugin + Send + Sync>>,
}

impl PluginManager {
    pub fn new(prepared_plugins: PreLoadedPlugins, app: &dyn Application) -> Self {
        let logger = app.logger();
        let log = |msg: String| {
            logger.log(LogEntry::new(LogLevel::Info, "app", None, msg));
        };
        let mut plugins = Vec::new();
        for (name, load_fn) in prepared_plugins.load_fns {
            log(format!("loading plugin {name}"));
            let plugin = load_fn(app);
            plugins.push(plugin);
        }
        PluginManager { plugins }
    }

    pub fn edit_templates_hooks(&self, templates: &mut Templates) {
        for plugin in &self.plugins {
            plugin.edit_templates(templates);
        }
    }

    pub fn edit_assets_hooks(&self, assets: &mut Assets) {
        for plugin in &self.plugins {
            plugin.edit_assets(assets);
        }
    }

    #[must_use]
    pub fn router_hooks(&self, router: Router) -> Router {
        let mut router = router;
        for plugin in &self.plugins {
            router = plugin.route(router);
        }
        router
    }
}

#[async_trait]
impl MonitorHooks for PluginManager {
    async fn on_monitor_start(&self, token: CancellationToken, monitor: ArcMonitor) {
        let plugins = self.plugins.clone();
        for plugin in plugins {
            // Execute every call in a co-routine to avoid blocking.
            let token = token.clone();
            let monitor = monitor.clone();
            tokio::spawn(async move {
                plugin.on_monitor_start(token, monitor).await;
            });
        }
    }
    fn on_thumb_save(&self, config: &MonitorConfig, mut frame: Frame) -> Frame {
        let plugins = self.plugins.clone();
        for plugin in plugins {
            frame = plugin.on_thumb_save(config, frame);
        }
        frame
    }
    async fn on_event(&self, event: Event, config: MonitorConfig) {
        let plugins = self.plugins.clone();
        for plugin in plugins {
            // Execute every call in a co-routine to avoid blocking.
            let event = event.clone();
            let config = config.clone();
            tokio::spawn(async move {
                plugin.on_event(event, config).await;
            });
        }
    }
}

#[must_use]
pub fn get_version() -> String {
    let version = format!(
        "v{}_r{}",
        env!("CARGO_PKG_VERSION").to_owned(),
        env!("CARGO_PKG_RUST_VERSION"),
    );

    #[cfg(debug_assertions)]
    let version = version + "_debug";

    version
}
