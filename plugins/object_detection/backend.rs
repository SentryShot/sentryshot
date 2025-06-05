// SPDX-License-Identifier: GPL-2.0-or-later

use libloading::{Library, Symbol};
use plugin::{
    CheckPluginVersionError, FindPluginPathError, check_plugin_version, find_plugin_path,
    get_version2, object_detection::DynTfliteBackend,
};
use std::path::PathBuf;
use thiserror::Error;
use tokio::runtime::Handle;

pub(crate) struct BackendLoader {
    rt_handle: Handle,
    plugin_dir: PathBuf,
    plugin_version: String,

    tflite: Option<DynTfliteBackend>,
}

impl BackendLoader {
    pub(crate) fn new(rt_handle: Handle, plugin_dir: PathBuf) -> Self {
        Self {
            rt_handle,
            plugin_dir,
            plugin_version: get_version2(),
            tflite: None,
        }
    }

    pub(crate) fn tflite_backend(
        &mut self,
    ) -> Result<&mut DynTfliteBackend, LoadTfliteBackendError> {
        // No fallible Option::create_or_insert()
        // https://github.com/rust-lang/libs-team/issues/577
        use LoadTfliteBackendError::*;
        if self.tflite.is_none() {
            let plugin_name = "object_detection_tflite";
            let plugin_path = find_plugin_path(&self.plugin_dir, plugin_name)?;
            let dylib = unsafe { Library::new(plugin_path).map_err(LoadLibrary)? };
            check_plugin_version(&dylib, &self.plugin_version).map_err(CheckPluginVersion)?;

            let new_fn: Symbol<fn(rt_handle: Handle) -> DynTfliteBackend> =
                unsafe { dylib.get(b"new_tflite_backend").map_err(LoadSymbol)? };

            self.tflite = Some(new_fn(self.rt_handle.clone()));

            // Keep the shared library loaded until the program exits.
            Box::leak(Box::new(dylib));
        }
        Ok(self.tflite.as_mut().expect("Some"))
    }
}

#[derive(Debug, Error)]
pub(crate) enum LoadTfliteBackendError {
    #[error(transparent)]
    FindPluginPath(#[from] FindPluginPathError),

    #[error("load library: {0}")]
    LoadLibrary(libloading::Error),

    #[error("check plugin version: {0}")]
    CheckPluginVersion(#[from] CheckPluginVersionError),

    #[error("load symbol: {0}")]
    LoadSymbol(libloading::Error),
}
