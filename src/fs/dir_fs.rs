// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{Dir, DynFs, Entry, File, Fs, Open, error::FsError, valid_path};
use std::{
    io::Read,
    path::{Path, PathBuf},
};

#[must_use]
pub fn dir_fs(dir: PathBuf) -> DynFs {
    Box::new(DirFs(dir))
}

#[derive(Clone)]
struct DirFs(PathBuf);

impl Fs for DirFs {
    fn open(&self, name: &Path) -> Result<Open, FsError> {
        if !valid_path(name) {
            return Err(FsError::OpenNotExist);
        }
        let fullname = self.0.join(name);
        Ok(open_file(&fullname)?)
    }

    fn clone(&self) -> DynFs {
        Box::new(Clone::clone(self))
    }
}

#[allow(clippy::unwrap_used)]
fn open_file(path: &Path) -> Result<Open, std::io::Error> {
    let metadata = std::fs::metadata(path)?;
    if metadata.is_dir() {
        Ok(Open::Dir(Box::new(DirFsDir {
            name: PathBuf::from(path.file_name().unwrap()),
            path: path.to_path_buf(),
        })))
    } else {
        let file = std::fs::OpenOptions::new().read(true).open(path)?;
        Ok(Open::File(Box::new(DirFsFile {
            name: PathBuf::from(path.file_name().unwrap()),
            file,
        })))
    }
}

struct DirFsFile {
    name: PathBuf,
    file: std::fs::File,
}

impl File for DirFsFile {
    fn name(&self) -> &Path {
        &self.name
    }

    fn read(&mut self) -> Result<Vec<u8>, FsError> {
        let mut buf = Vec::new();
        self.file.read_to_end(&mut buf)?;
        Ok(buf)
    }
}

struct DirFsDir {
    name: PathBuf,
    path: PathBuf,
}

impl File for DirFsDir {
    fn name(&self) -> &Path {
        &self.name
    }

    #[track_caller]
    fn read(&mut self) -> Result<Vec<u8>, FsError> {
        todo!()
    }
}

impl Dir for DirFsDir {
    fn name(&self) -> &Path {
        &self.name
    }

    #[allow(clippy::unwrap_used)]
    fn read_dir_file(&mut self) -> Result<Vec<Entry>, FsError> {
        let mut files: Vec<Entry> = Vec::new();
        for file in std::fs::read_dir(&self.path)? {
            let file = file?;
            let name = PathBuf::from(file.path().file_name().unwrap());
            let file_type = file.file_type().unwrap();
            if file_type.is_file() {
                files.push(Entry::File(Box::new(DirFsEntryFile { name, file })));
            } else if file_type.is_dir() {
                files.push(Entry::Dir(Box::new(DirFsDir {
                    name,
                    path: file.path(),
                })));
            }
        }
        files.sort_by_key(|v| v.name().to_owned());
        Ok(files)
    }
}

#[allow(unused)]
struct DirFsEntryFile {
    name: PathBuf,
    file: std::fs::DirEntry,
}

impl File for DirFsEntryFile {
    fn name(&self) -> &Path {
        &self.name
    }

    #[track_caller]
    fn read(&mut self) -> Result<Vec<u8>, FsError> {
        todo!()
    }
}
