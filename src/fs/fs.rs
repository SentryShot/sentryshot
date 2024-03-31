// SPDX-License-Identifier: GPL-2.0-or-later

mod dir_fs;
mod error;
mod map_fs;
mod sub_fs;

#[cfg(test)]
mod test;

#[cfg(test)]
mod test_fs;

pub use dir_fs::dir_fs;
pub use error::FsError;
pub use map_fs::{MapEntry, MapFs};

use crate::sub_fs::SubFs;
use std::{
    path::{Path, PathBuf},
    sync::Arc,
};

pub type DynFs = Box<dyn Fs + Send + Sync>;
pub type ArcFs = Arc<dyn Fs + Send + Sync>;

pub trait Fs {
    // Opens the named file.
    //
    // open should reject attempts to open names that do not satisfy valid_path(name).
    fn open(&self, path: &Path) -> Result<Open, FsError>;

    // Returns an Fs corresponding to the subtree rooted at fsys's dir.
    //fn sub(&self, path: &Path) -> Result<Box<dyn Fs>, FsError>;

    // ReadDir reads the named directory
    // and returns a list of directory entries sorted by filename.
    //
    // Calls open and uses read_dir_file on the returned file.
    fn read_dir(&self) -> Result<Vec<Entry>, FsError> {
        let Open::Dir(mut dir) = self.open(Path::new("."))? else {
            return Err(FsError::NotADirectory);
        };
        let mut list = dir.read_dir_file()?;
        list.sort_by_key(|v| v.name().to_owned());
        Ok(list)
    }

    // sub returns an FS corresponding to the subtree rooted at self's dir.
    //
    // If dir is ".", sub returns a cloned self unchanged.
    // Otherwise, sub returns a new FS implementation sub that,
    // in effect, implements sub.Open(name) as fsys.Open(dir.join(name)).
    // The implementation also translates calls to read_dir appropriately.
    //
    // Note that dir_fs("/").sub("prefix") is equivalent to dir_fs("/prefix")
    // and that neither of them guarantees to avoid operating system
    // accesses outside "/prefix", because the implementation of DirFs
    // does not check for symbolic links inside "/prefix" that point to
    // other directories. That is, DirFS is not a general substitute for a
    // chroot-style security mechanism, and sub does not change that fact.
    fn sub(&self, path: &Path) -> Result<DynFs, FsError> {
        if !valid_path(path) {
            return Err(FsError::InvalidPath(path.to_path_buf()));
        }

        if path == PathBuf::from(".") {
            return Ok(self.clone());
        }

        Ok(Box::new(SubFs {
            fsys: self.clone(),
            dir: path.to_path_buf(),
        }))
    }

    fn clone(&self) -> DynFs;
}

pub enum Entry {
    Dir(Box<dyn Dir>),
    File(Box<dyn File>),
    Symlink(Box<dyn Symlink>),
}

#[allow(unused)]
impl Entry {
    #[must_use]
    pub fn name(&self) -> &Path {
        match self {
            Entry::Dir(v) => v.name(),
            Entry::File(v) => v.name(),
            Entry::Symlink(v) => v.name(),
        }
    }

    fn is_dir(&self) -> bool {
        matches!(self, Entry::Dir(_))
    }

    fn is_symlink(&self) -> bool {
        matches!(self, Entry::Symlink(_))
    }
}

pub enum Open {
    File(Box<dyn File>),
    Dir(Box<dyn Dir>),
    Symlink(Box<dyn Symlink>),
}

#[allow(unused)]
impl Open {
    fn name(&self) -> &Path {
        match self {
            Open::File(v) => v.name(),
            Open::Dir(v) => v.name(),
            Open::Symlink(v) => v.name(),
        }
    }

    fn is_dir(&self) -> bool {
        matches!(self, Open::Dir(_))
    }

    fn is_symlink(&self) -> bool {
        matches!(self, Open::Symlink(_))
    }
}

pub trait File {
    fn name(&self) -> &Path;
    fn read(&mut self) -> Result<Vec<u8>, FsError>;
}

pub trait Dir {
    fn name(&self) -> &Path;
    fn read_dir_file(&mut self) -> Result<Vec<Entry>, FsError>;
}

pub trait Symlink {
    fn name(&self) -> &Path;
}

#[allow(clippy::unwrap_used)]
fn valid_path(name: &Path) -> bool {
    let mut name = name.to_str().unwrap();
    if name == "." {
        // Special case.
        return true;
    }

    // Iterate over elements in name, checking each.
    loop {
        let mut i = 0;
        while i < name.len() && name.as_bytes()[i] != b'/' {
            i += 1;
        }
        #[allow(clippy::unwrap_used)]
        let elem = String::from_utf8(name.as_bytes()[..i].to_owned()).unwrap();
        if elem.is_empty() || elem == "." || elem == ".." {
            return false;
        }
        if i == name.len() {
            return true; // Reached clean ending.
        }
        name = &name[i + 1..];
    }
}
