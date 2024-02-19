// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{
    error::TestFileSystemError,
    map_fs::{index, last_index},
    Dir, Entry, Fs, Open,
};
use std::{
    collections::{HashMap, HashSet},
    path::{Path, PathBuf},
};

// Tests a file system implementation.
// It walks the entire tree of files in fsys,
// opening and checking that each file behaves correctly.
// It also checks that the file system contains at least the expected files.
// As a special case, if no expected files are listed, fsys must be empty.
// Otherwise, fsys must contain at least the listed files; it can also contain others.
// The contents of fsys must not change concurrently with TestFS.
//
// If test_file_system finds any misbehaviors, it returns an error reporting all of them.
// The error text spans multiple lines, one per detected misbehavior.
//
// Typical usage inside a test is:
//
//	if err := fstest.TestFS(myFS, "file/that/should/be/present"); err != nil {
//		t.Fatal(err)
//	}
#[allow(clippy::unwrap_used)]
pub fn test_file_system(fsys: &dyn Fs, expected: &[PathBuf]) -> Result<(), TestFileSystemError> {
    test_fs(fsys.clone(), expected)?;
    for name in expected {
        if let Some(i) = index(name.to_str().unwrap(), '/') {
            let name_s = name.to_str().unwrap();
            let dir = &name_s[..i];
            let dir_slash = &name_s[..=i];

            let mut sub_expected = Vec::new();
            for name in expected {
                let name = name.to_str().unwrap();
                if let Some(stripped) = name.strip_prefix(dir_slash) {
                    sub_expected.push(PathBuf::from(stripped.to_owned()));
                }
            }
            let sub = fsys
                .sub(Path::new(dir))
                .map_err(|e| TestFileSystemError::TestFsSub(dir.to_owned(), e.to_string()))?;
            test_file_system(&*sub, &sub_expected)?;
            break;
        }
    }

    Ok(())
}

#[allow(clippy::unwrap_used)]
fn test_fs(fsys: Box<dyn Fs>, expected: &[PathBuf]) -> Result<(), TestFileSystemError> {
    let mut t = FsTester {
        fsys,
        err_text: Vec::new(),
        dirs: Vec::new(),
        files: Vec::new(),
    };
    t.check_dir(Path::new("."));
    t.check_open(".");
    let mut found = HashSet::new();
    for dir in &t.dirs {
        found.insert(dir.to_owned());
    }
    for file in &t.files {
        found.insert(file.to_owned());
    }
    found.remove(Path::new("."));
    if expected.is_empty() && !found.is_empty() {
        let mut list = Vec::new();
        for k in &found {
            if k != Path::new(".") {
                list.push(k.to_owned());
            }
        }
        list.sort();
        if list.len() > 15 {
            list = list[..10].to_owned();
            list.push(PathBuf::from("..."));
        }
        t.errorf(&format!(
            "expected empty file system found files:\n{}",
            list.iter()
                .map(|v| v.to_str().unwrap())
                .collect::<Vec<_>>()
                .join("\n")
        ));
    }

    for name in expected {
        if !found.contains(name) {
            t.errorf(&format!("expected but not found: {name:?}"));
        }
    }
    if t.err_text.is_empty() {
        return Ok(());
    }

    Err(TestFileSystemError::FoundError(
        String::from_utf8(t.err_text).unwrap(),
    ))
}

// An fsTester holds state for running the test.
struct FsTester {
    fsys: Box<dyn Fs>,
    err_text: Vec<u8>,
    dirs: Vec<PathBuf>,
    files: Vec<PathBuf>,
}

impl FsTester {
    fn errorf(&mut self, msg: &str) {
        if !self.err_text.is_empty() {
            self.err_text.push(b'\n');
        }
        self.err_text.extend_from_slice(msg.as_bytes());
    }

    fn open_dir(&mut self, dir: &Path) -> Option<Box<dyn Dir>> {
        match self.fsys.open(dir) {
            Ok(Open::Dir(dir)) => Some(dir),
            Ok(_) => {
                self.errorf(&format!("{dir:?}: Open did not return a ReadDirFile"));
                None
            }
            Err(e) => {
                self.errorf(&format!("open_dir: {dir:?}: {e}"));
                None
            }
        }
    }

    #[allow(clippy::unwrap_used)]
    fn check_dir(&mut self, dir: &Path) {
        // Read entier directory.
        self.dirs.push(dir.to_path_buf());
        let Some(mut d) = self.open_dir(dir) else {
            return;
        };
        let list = match d.read_dir_file() {
            Ok(v) => v,
            Err(e) => {
                self.errorf(&format!("{dir:?}: read_dir_file: {e}"));
                return;
            }
        };

        // Check all children.
        let prefix = if dir.to_str().unwrap() == "." {
            String::new()
        } else {
            dir.to_str().unwrap().to_owned() + "/"
        };
        for info in &list {
            let name = info.name();
            let name = name.to_str().unwrap();
            if name == "." || name == ".." || name.is_empty() {
                self.errorf(&format!(
                    "{dir:?}: read_dir: child has invalid name: {name}"
                ));
                return;
            } else if name.contains('/') {
                self.errorf(&format!(
                    "{dir:?}: read_dir: child name contans slash: {name}"
                ));
                return;
            } else if name.contains('\\') {
                self.errorf(&format!(
                    "{dir:?}: read_dir: child name contans backslash: {name}"
                ));
                return;
            }

            let path_string = prefix.clone() + name;
            let path = PathBuf::from(&path_string);
            self.check_stat(&path, info);
            self.check_open(&path_string);
            if info.is_dir() {
                self.check_dir(&path);
            } else {
                self.check_file(&path);
            }
        }

        // Reopen directory, read a second time, make sure contents match.
        let Some(mut d) = self.open_dir(dir) else {
            return;
        };
        let list2 = match d.read_dir_file() {
            Ok(v) => v,
            Err(e) => {
                self.errorf(&format!("{dir:?}: second open+read_dir_file: {e}"));
                return;
            }
        };
        self.check_dir_list(
            dir,
            "first open+read_dir_file(-1) vs second open+read_dir_file",
            &list,
            &list2,
        );
    }

    // Checks that a direct stat of path matches entry,
    // which was found in the parent's directory listing.
    #[allow(clippy::similar_names)]
    fn check_stat(&mut self, path: &Path, entry: &Entry) {
        let file = match self.fsys.open(path) {
            Ok(v) => v,
            Err(e) => {
                self.errorf(&format!("{path:?}: open: {e}"));
                return;
            }
        };

        let info = file;

        let fentry = format_entry(entry);
        let fientry = format_info_entry2(&info);

        // Note: mismatch here is OK for symlink, because Open dereferences symlink.
        if fentry != fientry && !entry.is_symlink() {
            self.errorf(&format!(
                "{path:?}: mismatch:\n\tentry = {fentry}\n\tfile.stat() = {fientry}"
            ));
        }

        let finfo = format_info2(&info);
        if entry.is_symlink() {
            // For symlink, just check that entry.Info matches entry on common fields.
            // Open deferences symlink, so info itself may differ.
            let feentry = format_entry(entry);
            if fentry != feentry {
                self.errorf(&format!(
                    "{path:?}: mismatch\n\tentry = {fentry}\n\tentry.info() = {feentry}\n",
                ));
            }
        } else {
            let feinfo = format_info(entry);
            if feinfo != finfo {
                self.errorf(&format!(
                    "{path:?}: mismatch\n\tentry = {feinfo}\n\tentry.stat() = {finfo}\n",
                ));
            }
        }

        // Stat should be the same as Open+Stat, even for symlinks.
        let info2 = match self.fsys.open(path) {
            Ok(file) => file,
            Err(e) => {
                self.errorf(&format!("{path:?}: stat open: {e}"));
                return;
            }
        };

        let finfo2 = format_info2(&info2);
        if finfo2 != finfo {
            self.errorf(&format!("{path:?}: stat(...) = {finfo2}\n\t {finfo}"));
        }

        /*let info2 = match self.fsys.open(path) {
            Ok(file) => match file.stat() {
                Ok(v) => v,
                Err(e) => {
                    self.errorf(format!("{:?}: stat: {}", path, e));
                    return;
                }
            },
            Err(e) => {
                self.errorf(format!("{:?}: stat open: {}", path, e));
                return;
            }
        };*/

        /*
        if fsys, ok := t.fsys.(fs.StatFS); ok {
            info2, err := fsys.Stat(path)
            if err != nil {
                t.errorf("%s: fsys.Stat: %v", path, err)
                return
            }
            finfo2 := formatInfo(info2)
            if finfo2 != finfo {
                t.errorf("%s: fsys.Stat(...) = %s\n\twant %s", path, finfo2, finfo)
            }
        }
        */
    }

    // Checks that two directory lists contain the same files and file info.
    // The order of the lists need not match.
    fn check_dir_list(&mut self, dir: &Path, desc: &str, list1: &Vec<Entry>, list2: &Vec<Entry>) {
        let mut old = HashMap::new();
        for entry1 in list1 {
            old.insert(entry1.name(), entry1.to_owned());
        }

        let mut diffs = Vec::new();
        for entry2 in list2 {
            let Some(entry1) = old.remove(&entry2.name()) else {
                diffs.push(format!("+ {}", format_entry(entry2)));
                continue;
            };
            if format_entry(entry1) != format_entry(entry2) {
                diffs.push(format!(
                    "- {}+ {}",
                    format_entry(entry1),
                    format_entry(entry2)
                ));
            }
        }

        for entry1 in old.values() {
            diffs.push(format!("- {}", format_entry(entry1)));
        }

        if diffs.is_empty() {
            return;
        }
        diffs.sort();

        /*sort.Slice(diffs, func(i, j int) bool {
            fi := strings.Fields(diffs[i])
            fj := strings.Fields(diffs[j])
            // sort by name (i < j) and then +/- (j < i, because + < -)
            return fi[1]+" "+fj[0] < fj[1]+" "+fi[0]
        })*/

        self.errorf(&format!(
            "{:?}: diff {}:\n\t{}",
            dir,
            desc,
            diffs.join("\n\t")
        ));
    }

    // Checks that basic file reading works correctly.
    fn check_file(&mut self, file: &Path) {
        self.files.push(file.to_path_buf());

        // Read entire file.
        let f = match self.fsys.open(file) {
            Ok(f) => f,
            Err(e) => {
                self.errorf(&format!("{file:?}: open: {e}"));
                return;
            }
        };
        let Open::File(mut f) = f else {
            panic!("");
        };

        let _data = match f.read() {
            Ok(v) => v,
            Err(e) => {
                self.errorf(&format!("{file:?}: open+read: {e}"));
                return;
            }
        };
    }

    /*fn check_file_read(&mut self, file: &Path, desc: &str, data1: &[u8], data2: &[u8]) {
        if data1 != data2 {
            self.errorf(format!(
                "{:?}: {}: different data returned\n\t{:?}\n\t{:?}",
                file, desc, data1, data2,
            ));
        }
    }*/

    // Checks that various invalid forms of file's name cannot be opened using t.fsys.Open.
    #[allow(clippy::unwrap_used)]
    fn check_open(&self, file: &str) {
        let mut bad = Vec::from([("/".to_owned() + file), (file.to_owned() + "/.")]);
        if file == "." {
            bad.push("/".to_owned());
        }
        if let Some(i) = index(file, '/') {
            let a = String::from_utf8(file.as_bytes()[..i].to_vec()).unwrap();
            let b = String::from_utf8(file.as_bytes()[i + 1..].to_vec()).unwrap();
            bad.push(format!("{a}//{b}"));
            bad.push(format!("{a}/./{b}"));
            bad.push(format!("{a}\\{b}"));
            bad.push(format!("{a}/../{file}"));
        }
        if let Some(i) = last_index(file, '/') {
            let a = String::from_utf8(file.as_bytes()[..i].to_vec()).unwrap();
            let b = String::from_utf8(file.as_bytes()[i + 1..].to_vec()).unwrap();
            bad.push(format!("{a}//{b}"));
            bad.push(format!("{a}/./{b}"));
            bad.push(format!("{a}\\{b}"));
            bad.push(format!("{a}/../{file}"));
        }

        for b in &bad {
            assert!(
                self.fsys.open(Path::new(b)).is_err(),
                "{file}: Open({b}) succeeded, want error"
            );
        }
    }
}

// Formats an DirEntry into a string for error messages and comparison.
fn format_entry(entry: &Entry) -> String {
    format!("{:?} is_dir={}", entry.name(), entry.is_dir())
}

// Formats an fs.FileInfo into a string like the result of formatEntry, for error messages and comparison.
fn format_info_entry2(info: &Open) -> String {
    format!("{:?} is_dir={}", info.name(), info.is_dir())
}

// formatInfo formats an fs.FileInfo into a string for error messages and comparison.
fn format_info(info: &Entry) -> String {
    format!(
        "{:?} is_dir={} is_symlink={} Size=x ModTime=x",
        info.name(),
        info.is_dir(),
        info.is_symlink(),
        //info.Size(),
        //info.ModTime(),
    )
}

// formatInfo formats an fs.FileInfo into a string for error messages and comparison.
fn format_info2(info: &Open) -> String {
    format!(
        "{:?} is_dir={} is_symlink={} Size=x ModTime=x",
        info.name(),
        info.is_dir(),
        info.is_symlink(),
        //info.Size(),
        //info.ModTime(),
    )
}
