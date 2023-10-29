// SPDX-License-Identifier: GPL-2.0-or-later

use std::path::PathBuf;
use thiserror::Error;

#[derive(Debug, Error)]
pub enum FsError {
    #[error("open: not exist")]
    OpenNotExist,

    #[error("invalid path: {0:?}")]
    InvalidPath(PathBuf),

    #[error("io: {0}")]
    Io(#[from] std::io::Error),

    #[error("not a directory")]
    NotADirectory,
}

#[cfg(test)]
#[derive(Debug, Error)]
pub enum TestFileSystemError {
    #[error("fs: {0}")]
    Fs(#[from] FsError),

    #[error("testing fs.Sub(fsys, {0}): {1}")]
    TestFsSub(String, String),

    #[error("found error: {0}")]
    FoundError(String),
}
