// SPDX-License-Identifier: GPL-2.0-or-later

mod app;
mod rec2mp4;

use app::run;
pub use rec2mp4::rec_to_mp4;

use std::{path::PathBuf, process::ExitCode};

#[tokio::main]
async fn main() -> ExitCode {
    #[cfg(tokio_unstable)]
    {
        println!("tokio tracing enabled");
        console_subscriber::init();
    }

    let rt_handle = tokio::runtime::Handle::current();

    let mut pargs = pico_args::Arguments::from_env();

    if pargs.contains(["-V", "--version"]) {
        print!("{}", env!("CARGO_PKG_VERSION").to_owned());
        return ExitCode::SUCCESS;
    }

    let Ok(subcommand) = pargs.subcommand() else {
        println!("invalid args");
        return ExitCode::FAILURE;
    };
    let Some(subcommand) = subcommand else {
        print!("{HELP}");
        return ExitCode::FAILURE;
    };
    match subcommand.as_str() {
        "run" => {
            if pargs.contains(["-h", "--help"]) {
                print!("{HELP_RUN}");
                return ExitCode::SUCCESS;
            }
            let config = pargs
                .value_from_str("--config")
                .unwrap_or_else(|_| PathBuf::from(DEFAULT_CONFIG_PATH));
            if let Err(e) = run(rt_handle, &config).await {
                eprintln!("failed to run app: {e}");
                return ExitCode::FAILURE;
            };
        }
        "rec2mp4" => {
            if pargs.contains(["-h", "--help"]) {
                print!("{HELP_REC2MP4}");
                return ExitCode::SUCCESS;
            }
            let Ok(path) = pargs.free_from_str() else {
                println!("missing path");
                return ExitCode::FAILURE;
            };
            if let Err(e) = rec_to_mp4(path).await {
                eprintln!("error: {e}");
                return ExitCode::FAILURE;
            }
        }
        v => {
            println!("invalid subcommand '{v}'");
            return ExitCode::FAILURE;
        }
    }
    ExitCode::SUCCESS
}

const DEFAULT_CONFIG_PATH: &str = "./configs/sentryshot.toml";

const HELP: &str = "\
Usage: sentryshot [OPTIONS] <COMMAND>

Commands:
  run      Run the program
  rec2mp4  Convert recordings into mp4 videos
  help     Print this message or the help of the given subcommand(s)

Options:
      --config <CONFIG>  [default: ./configs/sentryshot.toml]
  -h, --help             Print help
  -V, --version          Print version
";

const HELP_RUN: &str = "\
Run the program

Usage: sentryshot run [OPTIONS]

Options:
      --config <CONFIG>  [default: ./configs/sentryshot.toml]
  -h, --help             Print help
";

const HELP_REC2MP4: &str = "\
Convert recordings into mp4 videos

Usage: sentryshot rec2mp4 <PATH>

Arguments:
  <PATH>

Options:
  -h, --help  Print help
";
