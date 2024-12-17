// SPDX-License-Identifier: GPL-2.0-or-later

use serde::Deserialize;
use std::str::FromStr;
use thiserror::Error;
use toml::Value;

#[derive(Debug, PartialEq, Eq, Deserialize)]
pub(crate) struct Config {
    pub(crate) host: String,
    pub(crate) port: u16,
}

#[derive(Debug, Error)]
pub(crate) enum ParseConfigError {
    #[error("no table")]
    NoTable,

    #[error("no plugins")]
    NoPlugins,

    #[error("empty plugins")]
    EmptyPlugins,

    #[error("deserialize: {0}")]
    Deserialize(#[from] toml::de::Error),

    #[error("no config found")]
    NoMqttConfig,
}

impl FromStr for Config {
    type Err = ParseConfigError;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        use ParseConfigError::*;
        let value: Value = toml::from_str(s)?;
        let Value::Table(table) = value else {
            return Err(NoTable);
        };
        let Value::Array(plugins) = table.get("plugin").ok_or(NoPlugins)? else {
            return Err(EmptyPlugins);
        };
        for plugin in plugins {
            let Value::Table(plugin) = plugin else {
                continue;
            };
            let Some(Value::String(name)) = plugin.get("name") else {
                continue;
            };
            if name != "mqtt" {
                continue;
            }
            return Ok(plugin.to_owned().try_into::<Config>()?);
        }
        Err(NoMqttConfig)
    }
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_config() {
        let raw = "
# Port app will be served on.
port = 2020

# Thumbnail downscaling.
# Downscale video thumbnails to improve page load times and data usage.
[[plugin]]
name = \"thumb_scale\"
enable = true

# MQTT API.
# Documentation: ./docs/4_API.md
[[plugin]]
name = \"mqtt\"
enable = true
host = \"127.0.0.1\"
port = 1883";
        assert_eq!(
            Config {
                host: "127.0.0.1".to_owned(),
                port: 1883,
            },
            Config::from_str(raw).unwrap()
        );
    }
}
