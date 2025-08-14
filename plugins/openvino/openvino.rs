// SPDX-License-Identifier: GPL-2.0-or-later

use async_trait::async_trait;
use common::{
    monitor::ArcMonitor, ArcLogger, LogEntry, LogLevel, LogSource, MonitorId, MsgLogger,
};
use plugin::{Application, Plugin, PreLoadPlugin};
use std::{ffi::c_char, sync::Arc};
use tokio_util::sync::CancellationToken;

#[unsafe(no_mangle)]
pub extern "C" fn version() -> *const c_char {
    plugin::get_version()
}

#[unsafe(no_mangle)]
pub extern "Rust" fn pre_load() -> Box<dyn PreLoadPlugin> {
    Box::new(PreLoadOpenvino)
}

struct PreLoadOpenvino;

impl PreLoadPlugin for PreLoadOpenvino {
    fn add_log_source(&self) -> Option<LogSource> {
        #[allow(clippy::unwrap_used)]
        Some("openvino".try_into().unwrap())
    }
}

#[unsafe(no_mangle)]
pub extern "Rust" fn load(app: &dyn Application) -> Arc<dyn Plugin> {
    Arc::new(OpenvinoPlugin {
        logger: app.logger(),
    })
}

struct OpenvinoPlugin {
    logger: ArcLogger,
}

struct OpenvinoMsgLogger {
    logger: ArcLogger,
    monitor_id: MonitorId,
}

impl MsgLogger for OpenvinoMsgLogger {
    fn log(&self, level: LogLevel, msg: &str) {
        self.logger
            .log(LogEntry::new(level, "openvino", &self.monitor_id, msg));
    }
}

#[async_trait]
impl Plugin for OpenvinoPlugin {
    async fn on_monitor_start(&self, _token: CancellationToken, monitor: ArcMonitor) {
        let msg_logger = Arc::new(OpenvinoMsgLogger {
            logger: self.logger.clone(),
            monitor_id: monitor.config().id().to_owned(),
        });

        msg_logger.log(LogLevel::Info, "start successful");
    }
}