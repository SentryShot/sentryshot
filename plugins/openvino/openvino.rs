// SPDX-License-Identifier: GPL-2.0-or-later

use async_trait::async_trait;
use common::LogSource;
use plugin::{Application, Plugin, PreLoadPlugin};
use std::{ffi::c_char, sync::Arc};

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
pub extern "Rust" fn load(_app: &dyn Application) -> Arc<dyn Plugin> {
    Arc::new(OpenvinoPlugin)
}

struct OpenvinoPlugin;

#[async_trait]
impl Plugin for OpenvinoPlugin {}
