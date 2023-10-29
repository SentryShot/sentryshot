// SPDX-License-Identifier: GPL-2.0-or-later

use recording::{new_video_reader, NewVideoReaderError};
use std::{collections::VecDeque, path::PathBuf};
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

            let file_name = entry.file_name().to_string_lossy().to_string();
            if !file_name.ends_with(".meta") {
                continue;
            }

            let mut recording_path = entry.path();
            recording_path.set_extension("");

            let mut mdat_path = recording_path.to_owned();
            mdat_path.set_extension("mdat");

            if !mdat_path.exists() {
                continue;
            }

            recording_paths.push(recording_path);
        }
    }

    let n_recordings = recording_paths.len();
    println!("Found {} new recordings", n_recordings);

    if n_recordings == 0 {
        return Ok(());
    }

    let (results_tx, mut results_rx) = mpsc::channel(n_recordings);

    for recording_path in recording_paths {
        let results_tx = results_tx.clone();
        tokio::spawn(async move {
            results_tx
                .send(ConvertResult {
                    recording_path: recording_path.to_owned(),
                    res: convert(recording_path).await,
                })
                .await
        });
    }

    for i in 1..=n_recordings {
        println!("[{}/{}]", i, n_recordings);
        let result = results_rx.recv().await.unwrap();
        let path = result.recording_path.to_string_lossy();
        if let Err(e) = result.res {
            println!("[ERR] {} {}", path, &e);
            continue;
        }
        println!("[OK] {}.mp4", path);
    }

    Ok(())
}

struct ConvertResult {
    recording_path: PathBuf,
    res: Result<(), ConvertError>,
}

#[derive(Debug, Error)]
enum ConvertError {
    #[error("new video reader: {0}")]
    NewVideoReader(#[from] NewVideoReaderError),

    #[error("open file: {0}")]
    OpenFile(std::io::Error),

    #[error("copy: {0}")]
    Copy(std::io::Error),
}

async fn convert(recording_path: PathBuf) -> Result<(), ConvertError> {
    use ConvertError::*;
    let mut video_reader = new_video_reader(recording_path.to_owned(), &None).await?;

    let mut mp4_path = recording_path.to_owned();
    mp4_path.set_extension("mp4");

    let mut file = tokio::fs::OpenOptions::new()
        .create(true)
        .write(true)
        .open(mp4_path)
        .await
        .map_err(OpenFile)?;

    tokio::io::copy(&mut video_reader, &mut file)
        .await
        .map_err(Copy)?;

    Ok(())
}
