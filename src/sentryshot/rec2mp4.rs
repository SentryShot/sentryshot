// SPDX-License-Identifier: GPL-2.0-or-later

use common::FILE_MODE;
use recording::{CreateVideoReaderError, new_video_reader};
use std::{
    collections::VecDeque,
    path::{Path, PathBuf},
};
use thiserror::Error;
use tokio::sync::mpsc;

#[derive(Debug, Error)]
pub enum RecToMp4Error {
    #[error("read dir: {0}")]
    ReadDir(std::io::Error),

    #[error("entry: {0}")]
    Entry(std::io::Error),

    #[error("metadata: {0}")]
    Metadata(std::io::Error),
}

#[allow(clippy::unwrap_used)]
pub async fn rec_to_mp4(path: PathBuf) -> Result<(), RecToMp4Error> {
    use RecToMp4Error::*;

    let mut recording_paths = Vec::new();
    let mut dirs_to_visit = VecDeque::new();
    dirs_to_visit.push_back(path);

    while let Some(dir) = dirs_to_visit.pop_front() {
        let entries = std::fs::read_dir(dir).map_err(ReadDir)?;
        for entry in entries {
            let entry = entry.map_err(Entry)?;

            let metadata = entry.metadata().map_err(Metadata)?;

            if metadata.is_dir() {
                dirs_to_visit.push_back(entry.path());
                continue;
            }

            let meta_path = entry.path();
            let is_meta_file = meta_path
                .extension()
                .is_some_and(|ext| ext.eq_ignore_ascii_case("meta"));
            if !is_meta_file {
                continue;
            }

            let mut mdat_path = meta_path.clone();
            mdat_path.set_extension("mdat");
            if !mdat_path.exists() {
                continue;
            }

            recording_paths.push((meta_path, mdat_path));
        }
    }

    let n_recordings = recording_paths.len();
    println!("Found {n_recordings} recordings");

    if n_recordings == 0 {
        return Ok(());
    }

    let (results_tx, mut results_rx) = mpsc::channel(n_recordings);

    for (meta_path, mdat_path) in recording_paths {
        let results_tx = results_tx.clone();
        tokio::spawn(async move {
            let mut recording_path = meta_path.clone();
            recording_path.set_extension("");
            results_tx
                .send(ConvertResult {
                    recording_path,
                    res: convert(meta_path, &mdat_path).await,
                })
                .await
        });
    }

    for i in 1..=n_recordings {
        println!("[{i}/{n_recordings}]");
        let result = results_rx.recv().await.unwrap();
        let path = result.recording_path.to_string_lossy();
        if let Err(e) = result.res {
            println!("[ERR] {} {}", path, &e);
            continue;
        }
        println!("[OK] {path}.mp4");
    }

    Ok(())
}

struct ConvertResult {
    recording_path: PathBuf,
    res: Result<(), ConvertError>,
}

#[derive(Debug, Error)]
enum ConvertError {
    #[error("create video reader: {0}")]
    NewVideoReader(#[from] CreateVideoReaderError),

    #[error("open file: {0}")]
    OpenFile(std::io::Error),

    #[error("copy: {0}")]
    Copy(std::io::Error),
}

async fn convert(meta_path: PathBuf, mdat_path: &Path) -> Result<(), ConvertError> {
    use ConvertError::*;
    let mut video_reader = new_video_reader(&meta_path, mdat_path, 0, &None).await?;

    let mut mp4_path = meta_path.clone();
    mp4_path.set_extension("mp4");

    let mut file = tokio::fs::OpenOptions::new()
        .create(true)
        .mode(FILE_MODE)
        .truncate(true)
        .write(true)
        .open(mp4_path)
        .await
        .map_err(OpenFile)?;

    tokio::io::copy(&mut video_reader, &mut file)
        .await
        .map_err(Copy)?;

    Ok(())
}
