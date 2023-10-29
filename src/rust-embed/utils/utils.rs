#![forbid(unsafe_code)]

use std::borrow::Cow;
use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::{fs, io};

pub struct FileEntry {
    pub relative_path: String,
    pub full_path: String,
}

pub fn get_files(root: String) -> Vec<FileEntry> {
    let mut files = Vec::<FileEntry>::new();

    let mut dirs = vec![PathBuf::from(&root)];
    while let Some(dir_path) = dirs.pop() {
        fs::read_dir(dir_path).unwrap().flatten().for_each(|entry| {
            if entry.file_type().unwrap().is_dir() {
                dirs.push(entry.path());
            } else {
                files.push(FileEntry {
                    relative_path: path_to_str(entry.path().strip_prefix(&root).unwrap()),
                    full_path: path_to_str(
                        std::fs::canonicalize(entry.path()).expect("Could not get full path"),
                    ),
                });
            }
        });
    }
    files
}

fn path_to_str<P: AsRef<std::path::Path>>(p: P) -> String {
    p.as_ref()
        .to_str()
        .expect("Path does not have a string representation")
        .to_owned()
}

/// A file embedded into the binary
pub type EmbeddedFile = Cow<'static, [u8]>;

/// HashMap of filepath and `EmbeddedFile` pairs.
pub type EmbeddedFiles = HashMap<String, EmbeddedFile>;

pub fn read_file_from_fs(file_path: &Path) -> io::Result<EmbeddedFile> {
    let data = fs::read(file_path)?;
    let data = Cow::from(data);

    Ok(data)
}
