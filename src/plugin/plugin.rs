// SPDX-License-Identifier: GPL-2.0-or-later

pub mod object_detection;
pub mod types;

use async_trait::async_trait;
use common::{
    ArcAuth, ArcDisk, ArcLogger, DynEnvConfig, DynError, EnvPlugin, Event, LogEntry, LogLevel,
    LogSource,
    monitor::{ArcMonitor, ArcMonitorManager, MonitorConfig, MonitorHooks},
};
use libloading::{Library, Symbol};
use sentryshot_util::Frame;
use serde_json::Value;
use std::{
    ffi::{CStr, c_char},
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

    fn migrate_monitor(&self, _config: &mut Value) -> Result<(), DynError> {
        Ok(())
    }
}

pub trait Application {
    fn rt_handle(&self) -> Handle;
    fn token(&self) -> CancellationToken;
    fn auth(&self) -> ArcAuth;
    fn monitor_manager(&self) -> ArcMonitorManager;
    fn disk(&self) -> ArcDisk;
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
    #[error("pre load {0}: {1}")]
    PreLoadPlugin(String, PreLoadPluginError),
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

    let core_version = get_version2();

    for plugin in plugins {
        if !plugin.enable() {
            continue;
        }
        pre_load_plugin(plugin_dir, &mut pre_loaded_plugins, plugin, &core_version)
            .map_err(|e| PreLoadPlugin(plugin.name().to_owned(), e))?;
    }

    Ok(pre_loaded_plugins)
}

#[derive(Debug, Error)]
pub enum PreLoadPluginError {
    #[error(transparent)]
    FindPluginPath(#[from] FindPluginPathError),

    #[error("load library: {0}")]
    LoadLibrary(libloading::Error),

    #[error("check plugin version: {0}")]
    CheckPluginVersion(#[from] CheckPluginVersionError),

    #[error("load symbol: {0}")]
    LoadSymbol(libloading::Error),
}

pub fn pre_load_plugin(
    plugin_dir: &Path,
    pre_loaded_plugins: &mut PreLoadedPlugins,
    plugin: &EnvPlugin,
    core_version: &str,
) -> Result<(), PreLoadPluginError> {
    use PreLoadPluginError::*;
    let plugin_path = find_plugin_path(plugin_dir, plugin.name())?;
    let dylib = unsafe { Library::new(plugin_path).map_err(LoadLibrary)? };
    check_plugin_version(&dylib, core_version).map_err(CheckPluginVersion)?;

    // If pre_load is defined.
    let pre_load = unsafe { dylib.get::<Symbol<fn() -> Box<dyn PreLoadPlugin>>>(b"pre_load") };
    if let Ok(pre_load) = pre_load {
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

    let load_fn: Symbol<LoadFn> = unsafe { dylib.get(b"load").map_err(LoadSymbol)? };
    pre_loaded_plugins
        .load_fns
        .push((plugin.name().to_owned(), *load_fn));

    // Keep the shared library loaded until the program exits.
    Box::leak(Box::new(dylib));
    Ok(())
}

#[derive(Debug, Error)]
pub enum FindPluginPathError {
    #[error("plugin not found: {0}")]
    NotFound(PathBuf),
}

pub fn find_plugin_path(
    plugin_dir: &Path,
    plugin_name: &str,
) -> Result<PathBuf, FindPluginPathError> {
    // The short name is used during development.
    let long = plugin_dir.join(format!("libsentryshot_{plugin_name}.so"));
    let short = plugin_dir.join(format!("lib{plugin_name}.so"));
    if long.exists() {
        Ok(long)
    } else if short.exists() {
        Ok(short)
    } else {
        Err(FindPluginPathError::NotFound(long))
    }
}

#[derive(Debug, Error)]
pub enum CheckPluginVersionError {
    #[error("load 'version' symbol: {0}")]
    LoadVersionSymbol(libloading::Error),

    #[error("version missmatch expected='{0}' got='{1}'")]
    VersionMismatch(String, String),
}

pub fn check_plugin_version(
    dylib: &Library,
    expected_version: &str,
) -> Result<(), CheckPluginVersionError> {
    use CheckPluginVersionError::*;
    unsafe {
        let version_fn: Symbol<fn() -> *const c_char> =
            dylib.get(b"version").map_err(LoadVersionSymbol)?;
        let version = CStr::from_ptr(version_fn()).to_string_lossy().to_string();
        if version != expected_version {
            return Err(VersionMismatch(expected_version.to_owned(), version));
        }
    }
    Ok(())
}

impl PreLoadedPlugins {
    #[must_use]
    pub fn log_sources(&self) -> &[LogSource] {
        &self.log_sources
    }

    #[must_use]
    pub fn new_auth_fn(&self) -> NewAuthFn {
        let Some(new_auth_fn) = self.new_auth_fn else {
            eprint!(
                "\n\nPlease enable one of the authentication plugins in the generated sentryshot.toml file. See docs for more info\n\n"
            );
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
        let log = |msg: &str| {
            logger.log(LogEntry::new2(LogLevel::Info, "app", msg));
        };
        let mut plugins = Vec::new();
        for (name, load_fn) in prepared_plugins.load_fns {
            log(&format!("loading plugin {name}"));
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
    fn migrate_monitor(&self, config: &mut Value) -> Result<(), DynError> {
        for plugin in &self.plugins {
            plugin.migrate_monitor(config)?;
        }
        Ok(())
    }
}

#[inline]
#[must_use]
pub fn get_version() -> *const c_char {
    #[cfg(not(debug_assertions))]
    static VERSION: &str = concat![
        "v",
        env!("CARGO_PKG_VERSION"),
        "_r",
        env!("CARGO_PKG_RUST_VERSION"),
        "\0"
    ];
    #[cfg(debug_assertions)]
    static VERSION: &str = concat![
        "v",
        env!("CARGO_PKG_VERSION"),
        "_r",
        env!("CARGO_PKG_RUST_VERSION"),
        "_debug\0"
    ];

    VERSION.as_ptr().cast::<c_char>()
}

#[must_use]
pub fn get_version2() -> String {
    unsafe { CStr::from_ptr(get_version()) }
        .to_string_lossy()
        .to_string()
}
