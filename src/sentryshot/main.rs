// SPDX-License-Identifier: GPL-2.0-or-later

mod app;
mod rec2mp4;

use app::run;
pub use rec2mp4::rec_to_mp4;

use clap::Parser;
use std::path::PathBuf;

#[tokio::main]
async fn main() {
    #[cfg(tokio_unstable)]
    {
        println!("tokio tracing enabled");
        console_subscriber::init();
    }

    let rt_handle = tokio::runtime::Handle::current();
    let args = parse_args();

    match args.action {
        Action::Run(args) => {
            if let Err(e) = run(rt_handle, &args.config).await {
                eprintln!("failed to run app: {}", e);
            };
        }
        Action::Rec2Mp4(args) => {
            println!("{args:?}");
            if let Err(e) = rec_to_mp4(args.path).await {
                eprintln!("error: {}", e);
            }
        }
    }
}

pub fn parse_args() -> Args {
    Args::parse()
}

#[derive(Debug, Parser)]
#[command(version, about, long_about = None)]
pub struct Args {
    #[command(subcommand)]
    pub action: Action,

    // This is just for the help page.
    #[arg(long, default_value_t = DEFAULT_CONFIG_PATH.to_string())]
    config: String,
}

#[derive(Debug, clap::Subcommand)]
pub enum Action {
    #[command(about = "Run the program")]
    Run(RunArgs),

    #[command(name = "rec2mp4", about = "Convert recordings into mp4 videos")]
    Rec2Mp4(RecToMp4Args),
}

const DEFAULT_CONFIG_PATH: &str = "./configs/sentryshot.toml";

// Run the program.
#[derive(Debug, Parser)]
pub struct RunArgs {
    #[arg(long, default_value = DEFAULT_CONFIG_PATH)]
    pub config: PathBuf,
}

#[derive(Debug, Parser)]
pub struct RecToMp4Args {
    pub path: PathBuf,
}
