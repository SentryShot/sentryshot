// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{error::FsError, valid_path, DynFs, Fs, Open};
use std::path::{Path, PathBuf};

pub struct SubFs {
    pub fsys: DynFs,
    pub dir: PathBuf,
}

impl SubFs {
    // Maps name to the fully-qualified name dir/name.
    fn full_name(&self, name: &Path) -> Result<PathBuf, FsError> {
        if !valid_path(name) {
            return Err(FsError::InvalidPath(name.to_owned()));
        }
        Ok(clean(&self.dir.join(name)))
    }

    // Shorten maps name, which should start with self.dir, back to the suffix after self.dir.
    #[allow(clippy::unwrap_used)]
    fn shorten(&self, path: &Path) -> PathBuf {
        if path == self.dir {
            return PathBuf::from(".");
        }

        let name = path.to_str().unwrap();
        let name_b = name.as_bytes();
        let dir = self.dir.to_str().unwrap();
        let dir_b = dir.as_bytes();
        if name.len() > dir.len() && name_b[dir.len()] == b'/' && [name_b[dir.len()]] == dir_b {
            return PathBuf::from(name[dir.len() + 1..].to_owned());
        }
        /*if len(name) >= len(f.dir)+2 && name[len(f.dir)] == '/' && name[:len(f.dir)] == f.dir {
            return name[len(f.dir)+1:], true
        }*/
        PathBuf::from("")
    }

    // Shortens any reported names in PathErrors by stripping f.dir.
    fn fix_err(&self, e: FsError) -> FsError {
        match e {
            FsError::InvalidPath(path) => FsError::InvalidPath(self.shorten(&path)),
            _ => e,
        }
    }
}

impl Fs for SubFs {
    fn open(&self, name: &Path) -> Result<Open, FsError> {
        match self.fsys.open(&self.full_name(name)?) {
            Ok(v) => Ok(v),
            Err(e) => Err(self.fix_err(e)),
        }
    }

    fn clone(&self) -> DynFs {
        Box::new(Self {
            fsys: self.fsys.clone(),
            dir: self.dir.clone(),
        })
    }
}

// https://cs.opensource.google/go/go/+/refs/tags/go1.20.4:src/path/path.go;l=70
//
// Clean returns the shortest path name equivalent to path
// by purely lexical processing. It applies the following rules
// iteratively until no further processing can be done:
//
//  1. Replace multiple slashes with a single slash.
//  2. Eliminate each . path name element (the current directory).
//  3. Eliminate each inner .. path name element (the parent directory)
//     along with the non-.. element that precedes it.
//  4. Eliminate .. elements that begin a rooted path:
//     that is, replace "/.." by "/" at the beginning of a path.
//
// The returned path ends in a slash only if it is the root "/".
//
// If the result of this process is an empty string, Clean
// returns the string ".".
//
// See also Rob Pike, “Lexical File Names in Plan 9 or
// Getting Dot-Dot Right,”
// https://9p.io/sys/doc/lexnames.html
#[allow(clippy::if_same_then_else, clippy::unwrap_used)]
fn clean(path: &Path) -> PathBuf {
    if path == PathBuf::from("") {
        return PathBuf::from(".");
    }

    let rooted = path.to_str().unwrap().as_bytes()[0] == b'/';
    let n = path.to_str().unwrap().len();

    // Invariants:
    //	reading from path; r is index of next byte to process.
    //	writing to buf; w is index of next byte to write.
    //	dotdot is index in buf where .. must stop, either because
    //		it is the leading slash or it is a leading ../../.. prefix.
    let (mut out, mut r, mut dotdot) = if rooted {
        ("/".to_owned(), 1, 1)
    } else {
        (String::new(), 0, 0)
    };

    let path_s = path.to_str().unwrap();
    let path_b = path_s.as_bytes();
    while r < n {
        if path_b[r] == b'/' {
            // empty path element
            r += 1;
        } else if path_b[r] == b'.' && (r + 1 == n || path_b[r + 1] == b'/') {
            // . element
            r += 1;
        } else if path_b[r] == b'.'
            && path_b[r + 1] == b'.'
            && (r + 2 == n || path_b[r + 2] == b'/')
        {
            // .. element: remove to last /
            r += 2;
            if out.len() > dotdot {
                // can backtrack
                let mut last = out.pop().unwrap();
                while out.len() > dotdot && last != '/' {
                    last = out.pop().unwrap();
                }
            } else if !rooted {
                // cannot backtrack, but not rooted, so append .. element.
                if out.is_empty() {
                    out += "..";
                } else {
                    out += "/..";
                }
                dotdot = out.len();
            }
        } else {
            // real path element.
            // add slash if needed
            if rooted && out.len() != 1 || !rooted && !out.is_empty() {
                out.push('/');
            }

            // copy element
            while r < n && path_b[r] != b'/' {
                out.push(char::from_u32(path_b[r].into()).unwrap());
                r += 1;
            }
        }
    }

    // Turn empty string into "."
    if out.is_empty() {
        return PathBuf::from(".");
    }

    PathBuf::from(out)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_clean() {
        let cleantests = [
            // Already clean
            ("", "."),
            ("abc", "abc"),
            ("abc/def", "abc/def"),
            ("a/b/c", "a/b/c"),
            (".", "."),
            ("..", ".."),
            ("../..", "../.."),
            ("../../abc", "../../abc"),
            ("/abc", "/abc"),
            ("/", "/"),
            // Remove trailing slash
            ("abc/", "abc"),
            ("abc/def/", "abc/def"),
            ("a/b/c/", "a/b/c"),
            ("./", "."),
            ("../", ".."),
            ("../../", "../.."),
            ("/abc/", "/abc"),
            // Remove doubled slash
            ("abc//def//ghi", "abc/def/ghi"),
            ("//abc", "/abc"),
            ("///abc", "/abc"),
            ("//abc//", "/abc"),
            ("abc//", "abc"),
            // Remove . elements
            ("abc/./def", "abc/def"),
            ("/./abc/def", "/abc/def"),
            ("abc/.", "abc"),
            // Remove .. elements
            ("abc/def/ghi/../jkl", "abc/def/jkl"),
            ("abc/def/../ghi/../jkl", "abc/jkl"),
            ("abc/def/..", "abc"),
            ("abc/def/../..", "."),
            ("/abc/def/../..", "/"),
            ("abc/def/../../..", ".."),
            ("/abc/def/../../..", "/"),
            ("abc/def/../../../ghi/jkl/../../../mno", "../../mno"),
            // Combinations
            ("abc/./../def", "def"),
            ("abc//./../def", "def"),
            ("abc/../../././../def", "../../def"),
        ];

        for (path, result) in cleantests {
            let s = clean(Path::new(path));
            assert!(
                s == PathBuf::from(result),
                "Clean({path}) = {s:?}, want {result}"
            );
            let s = clean(Path::new(result));
            assert!(
                s == PathBuf::from(result),
                "Clean({result}) = {s:?}, want {result}"
            );
        }
    }
}
