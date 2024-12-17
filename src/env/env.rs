// SPDX-License-Identifier: GPL-2.0-or-later

use bytesize::ByteSize;
use common::{EnvConfig, EnvPlugin, NonZeroGb};
use serde::Deserialize;
use std::{
    collections::HashMap,
    fs::{self, File},
    io::Write,
    path::{Path, PathBuf},
};
use thiserror::Error;

/// Main config. Should not be editable from the Web UI.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct EnvConf {
    port: u16,
    storage_dir: PathBuf,
    recordings_dir: PathBuf,
    config_dir: PathBuf,
    plugin_dir: PathBuf,
    max_disk_usage: NonZeroGb,
    plugin: Option<Vec<EnvPlugin>>,
    raw: String,
}

#[derive(Debug, Deserialize)]
pub struct RawEnvConf {
    port: u16,
    storage_dir: PathBuf,
    config_dir: PathBuf,
    plugin_dir: PathBuf,
    max_disk_usage: NonZeroGb,
    plugin: Option<Vec<EnvPlugin>>,
}

impl EnvConf {
    pub fn new(config_path: &PathBuf) -> Result<EnvConf, EnvConfigNewError> {
        use EnvConfigNewError::*;
        let file_exist = config_path.exists();
        if !file_exist {
            print!(
                "\n\nGenerating '{}' and exiting..\n\n\n",
                config_path.to_string_lossy()
            );

            let cwd = std::env::current_dir().map_err(GetCwd)?;
            generate_config(config_path, &cwd)?;
            std::process::exit(0);
        }

        let env_toml = fs::read_to_string(config_path).map_err(ReadFile)?;
        let env = parse_config(env_toml)?;

        Ok(env)
    }
}

impl EnvConfig for EnvConf {
    fn port(&self) -> u16 {
        self.port
    }
    fn storage_dir(&self) -> &Path {
        &self.storage_dir
    }
    fn recordings_dir(&self) -> &Path {
        &self.recordings_dir
    }
    fn config_dir(&self) -> &Path {
        &self.config_dir
    }
    fn plugin_dir(&self) -> &Path {
        &self.plugin_dir
    }
    fn max_disk_usage(&self) -> ByteSize {
        *self.max_disk_usage
    }
    fn plugins(&self) -> &Option<Vec<EnvPlugin>> {
        &self.plugin
    }
    fn raw(&self) -> &str {
        &self.raw
    }
}

#[derive(Debug, Error)]
pub enum EnvConfigNewError {
    #[error("read env config file: {0}")]
    ReadFile(std::io::Error),

    #[error("generate env config: {0}")]
    Generate(#[from] GenerateEnvConfigError),

    #[error("parse env config: {0}")]
    Parse(#[from] ParseEnvConfigError),

    #[error("get current working directory: {0}")]
    GetCwd(std::io::Error),
}

#[derive(Debug, Error)]
pub enum GenerateEnvConfigError {
    #[error("create file: {0}")]
    CreateFile(std::io::Error),

    #[error("templater error: {0}")]
    AddTemplate(upon::Error),

    #[error("render template: {0}")]
    RenderTemplate(upon::Error),

    #[error("get parent directory")]
    GetParentDir(),

    #[error("create directory: {0}")]
    CreateDir(std::io::Error),

    #[error("write file: {0}")]
    WriteFile(std::io::Error),
}

fn generate_config(path: &Path, cwd: &Path) -> Result<(), GenerateEnvConfigError> {
    use GenerateEnvConfigError::*;

    let data = HashMap::from([("cwd", cwd)]);

    let mut engine = upon::Engine::new();
    engine
        .add_template("config", CONFIG_TEMPLATE)
        .map_err(AddTemplate)?;

    let config = engine
        .get_template("config")
        .expect("template should just have been added")
        .render(data)
        .to_string()
        .map_err(RenderTemplate)?;

    let config_dir = path.parent().ok_or(GetParentDir())?;
    fs::create_dir_all(config_dir).map_err(CreateDir)?;

    let mut file = File::create(path).map_err(CreateFile)?;
    write!(file, "{config}").map_err(WriteFile)?;

    Ok(())
}

const CONFIG_TEMPLATE: &str = include_str!("./default_config.tpl");

#[derive(Debug, Error)]
pub enum ParseEnvConfigError {
    #[error("{0}")]
    DeserializeToml(#[from] toml::de::Error),

    #[error("{0} path is not absolute '{1}'")]
    PathNotAbsolute(String, PathBuf),

    #[error("create storage dir: {0} {1}")]
    CreateStorageDir(PathBuf, std::io::Error),

    #[error("create recordings dir: {0} {1}")]
    CreateRecDir(PathBuf, std::io::Error),

    #[error("canonicalize path: {0:?} {1}")]
    Canonicalize(PathBuf, std::io::Error),
}

fn parse_config(env_toml: String) -> Result<EnvConf, ParseEnvConfigError> {
    use ParseEnvConfigError::*;
    let raw: RawEnvConf = toml::from_str(&env_toml)?;

    if !raw.storage_dir.is_absolute() {
        return Err(PathNotAbsolute("storage_dir".to_owned(), raw.storage_dir));
    }
    if !raw.config_dir.is_absolute() {
        return Err(PathNotAbsolute("config_dir".to_owned(), raw.config_dir));
    }
    if !raw.plugin_dir.is_absolute() {
        return Err(PathNotAbsolute("plugin_dir".to_owned(), raw.plugin_dir));
    }

    std::fs::create_dir_all(&raw.storage_dir)
        .map_err(|e| CreateStorageDir(raw.storage_dir.clone(), e))?;
    let storage_dir = raw
        .storage_dir
        .canonicalize()
        .map_err(|e| Canonicalize(raw.storage_dir, e))?;

    let recordings_dir = storage_dir.join("recordings");
    std::fs::create_dir_all(&recordings_dir)
        .map_err(|e| CreateRecDir(recordings_dir.clone(), e))?;
    let recordings_dir = recordings_dir
        .canonicalize()
        .map_err(|e| Canonicalize(recordings_dir, e))?;

    let config_dir = raw
        .config_dir
        .canonicalize()
        .map_err(|e| Canonicalize(raw.config_dir, e))?;

    let plugin_dir = raw
        .plugin_dir
        .canonicalize()
        .map_err(|e| Canonicalize(raw.plugin_dir, e))?;

    Ok(EnvConf {
        port: raw.port,
        storage_dir,
        recordings_dir,
        config_dir,
        plugin_dir,
        max_disk_usage: raw.max_disk_usage,
        plugin: raw.plugin,
        raw: env_toml,
    })
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use bytesize::GB;
    use pretty_assertions::assert_eq;
    use tempfile::TempDir;

    #[test]
    fn test_config() {
        let temp_dir = TempDir::new().unwrap();
        std::env::set_current_dir(temp_dir.path()).unwrap();
        std::fs::create_dir(temp_dir.path().join("plugins")).unwrap();
        let config_file = temp_dir.path().join("configs").join("env.toml");

        generate_config(&config_file, temp_dir.path()).unwrap();
        EnvConf::new(&config_file).unwrap();
    }
    #[test]
    fn test_parse_config_ok() {
        let temp_dir = TempDir::new().unwrap();
        std::fs::create_dir(temp_dir.path().join("plugins")).unwrap();
        let storage_dir = temp_dir.path().join("storage");
        let config_dir = temp_dir.path().join("config");
        let plugin_dir = temp_dir.path().join("plugin");
        let storage_dir = storage_dir.to_str().unwrap();
        let config_dir = config_dir.to_str().unwrap();
        let plugin_dir = plugin_dir.to_str().unwrap();
        std::fs::create_dir(config_dir).unwrap();
        std::fs::create_dir(plugin_dir).unwrap();

        let config = format!(
            "
            port = 2020
            storage_dir = \"{storage_dir}\"
            config_dir = \"{config_dir}\"
            plugin_dir = \"/{plugin_dir}\"
            max_disk_usage = 1
        ",
        );

        let storage_dir: PathBuf = storage_dir.parse().unwrap();
        let want = EnvConf {
            port: 2020,
            storage_dir: storage_dir.clone(),
            recordings_dir: storage_dir.join("recordings"),
            config_dir: config_dir.parse().unwrap(),
            plugin_dir: plugin_dir.parse().unwrap(),
            max_disk_usage: NonZeroGb::new(ByteSize(GB)).unwrap(),
            plugin: None,
            raw: config.clone(),
        };
        let got = parse_config(config).unwrap();
        assert_eq!(want, got);
    }
    #[test]
    fn test_parse_config_deserialize_error() {
        assert!(matches!(
            parse_config("&".to_owned()),
            Err(ParseEnvConfigError::DeserializeToml(_)),
        ));
    }
    #[test]
    fn test_parse_config_storage_dir_abs_error() {
        let config = "
            port = 2020
            storage_dir = \".\"
            config_dir = \"/ok\"
            plugin_dir = \"/ok\"
            max_disk_usage = 1
        ";

        assert!(matches!(
            parse_config(config.to_owned()),
            Err(ParseEnvConfigError::PathNotAbsolute(..))
        ));
    }
    #[test]
    fn test_parse_config_config_dir_abs_error() {
        let config = "
            port = 2020
            storage_dir = \"/ok\"
            config_dir = \".\"
            plugin_dir = \"/ok\"
            max_disk_usage = 1
        ";

        assert!(matches!(
            parse_config(config.to_owned()),
            Err(ParseEnvConfigError::PathNotAbsolute(..))
        ));
    }
    #[test]
    fn test_parse_config_plugin_dir_abs_error() {
        let config = "
            port = 2020
            storage_dir = \"/ok\"
            config_dir = \"/ok\"
            plugin_dir = \".\"
            max_disk_usage = 1
        ";

        assert!(matches!(
            parse_config(config.to_owned()),
            Err(ParseEnvConfigError::PathNotAbsolute(..))
        ));
    }
}
