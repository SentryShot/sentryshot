// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{error::FsError, valid_path, Dir, DynFs, Entry, File, Fs, Open};
use std::{
    collections::HashMap,
    path::{Path, PathBuf},
};

#[derive(Clone)]
pub struct MapFs(pub HashMap<PathBuf, MapEntry>);

#[derive(Clone, Default)]
pub struct MapEntry {
    pub data: Vec<u8>,
    pub is_file: bool,
    pub is_symlink: bool,
}

impl Fs for MapFs {
    fn open(&self, name: &Path) -> Result<Open, FsError> {
        if !valid_path(name) {
            return Err(FsError::InvalidPath(name.to_owned()));
        }

        // Ordinary file.
        if let Some(file) = self.0.get(name) {
            let name = PathBuf::from(name.file_name().unwrap().to_owned());
            return Ok(Open::File(Box::new(OpenMapFile {
                name: PathBuf::from(name.file_name().unwrap()),
                map_file_info: MapEntryInfo {
                    name: PathBuf::from(name.file_name().unwrap()),
                    file: file.to_owned(),
                },
            })));
        }

        // Directory, possibly synthesized.
        // Note that file can be nil here: the map need not contain explicit parent directories for all its files.
        // But file can also be non-nil, in case the user wants to set metadata for the directory explicitly.
        // Either way, we need to construct the list of children of this directory.
        let mut list = Vec::new();
        let elem: &str;
        let mut need = HashMap::new();
        if name == PathBuf::from(".") {
            elem = ".";
            for (fname, f) in &self.0 {
                if let Some(i) = index(fname.to_str().unwrap(), '/') {
                    need.insert(fname.to_str().unwrap()[..i].to_owned(), true);
                } else if fname.to_str().unwrap() != "." {
                    list.push(MapEntryInfo {
                        name: fname.to_owned(),
                        file: f.to_owned(),
                    });
                }
            }
        } else {
            let last_index = match last_index(name.to_str().unwrap(), '/') {
                Some(v) => v + 1,
                None => 0,
            };
            elem = &name.to_str().unwrap()[last_index..];
            let prefix = name.to_str().unwrap().to_owned() + "/";
            for (fname, f) in &self.0 {
                if fname.to_str().unwrap().starts_with(&prefix) {
                    let felem = &fname.to_str().unwrap()[prefix.len()..];
                    if let Some(i) = index(felem, '/') {
                        need.insert(
                            fname.to_str().unwrap()[prefix.len()..prefix.len() + i].to_owned(),
                            true,
                        );
                    } else {
                        list.push(MapEntryInfo {
                            name: PathBuf::from(felem),
                            file: f.to_owned(),
                        });
                    }
                }
            }
            // If the directory name is not in the map,
            // and there are no children of the name in the map,
            // then the directory is treated as not existing.
            if list.is_empty() && need.is_empty() {
                return Err(FsError::OpenNotExist);
            }
        }
        for fi in &list {
            need.remove(fi.name.to_str().unwrap());
        }
        for name in need.keys() {
            list.push(MapEntryInfo {
                name: PathBuf::from(name),
                file: MapEntry {
                    ..Default::default()
                },
            })
        }
        list.sort_by_key(|v| v.name.to_owned());

        Ok(Open::Dir(Box::new(MapDir {
            name: PathBuf::from(elem),
            entries: list,
        })))
    }

    fn clone(&self) -> DynFs {
        Box::new(Clone::clone(self))
    }
}

pub fn index(s: &str, c: char) -> Option<usize> {
    s.chars().position(|v| v == c)
}

pub fn last_index(s: &str, c: char) -> Option<usize> {
    Some(s.chars().count() - s.chars().rev().position(|v| v == c)? - 1)
}

#[derive(Clone, Default)]
struct MapEntryInfo {
    name: PathBuf,
    file: MapEntry,
}

impl MapEntryInfo {
    fn is_dir(&self) -> bool {
        !self.file.is_file
    }
}

impl File for MapEntryInfo {
    fn name(&self) -> &Path {
        &self.name
    }

    fn read(&mut self) -> Result<Vec<u8>, FsError> {
        Ok(self.file.data.to_owned())
    }
}

impl Dir for MapEntryInfo {
    fn name(&self) -> &Path {
        &self.name
    }

    fn read_dir_file(&mut self) -> Result<Vec<Entry>, FsError> {
        todo!()
    }
}

struct OpenMapFile {
    name: PathBuf,
    map_file_info: MapEntryInfo,
}

impl File for OpenMapFile {
    fn name(&self) -> &Path {
        &self.name
    }

    fn read(&mut self) -> Result<Vec<u8>, FsError> {
        Ok(self.map_file_info.file.data.to_owned())
    }
}

struct MapDir {
    name: PathBuf,
    entries: Vec<MapEntryInfo>,
}

impl File for MapDir {
    fn name(&self) -> &Path {
        &self.name
    }

    fn read(&mut self) -> Result<Vec<u8>, FsError> {
        todo!()
    }
}

impl Dir for MapDir {
    fn name(&self) -> &Path {
        &self.name
    }

    fn read_dir_file(&mut self) -> Result<Vec<Entry>, FsError> {
        Ok(self
            .entries
            .iter()
            .map(|entry| {
                if entry.is_dir() {
                    Entry::Dir(Box::new(entry.to_owned()))
                } else {
                    Entry::File(Box::new(entry.to_owned()))
                }
            })
            .collect())
    }
}
