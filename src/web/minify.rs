use std::{borrow::Cow, collections::HashMap};

#[allow(clippy::implicit_hasher)]
pub fn minify(files: &mut HashMap<String, Cow<'static, [u8]>>) {
    for (name, file) in files.iter_mut() {
        if std::path::Path::new(name)
            .extension()
            .map_or(false, |ext| ext.eq_ignore_ascii_case("js"))
        {
            *file = minify_javascript(file).into();
        }
    }
}

// Removes comments while keeping newlines to avoid
// effecting stack traces and needing source maps.
fn minify_javascript(file: &[u8]) -> Vec<u8> {
    let mut s = State::OutsideEverything;
    WindowIter::new(file)
        .filter_map(|(b, next_b)| {
            let c = char::from(b);
            match s {
                State::OutsideEverything => {
                    if let Some(next_b) = next_b {
                        if c == '/' && char::from(next_b) == '/' {
                            s = State::InsideLineComment;
                            return None;
                        }
                        if c == '/' && char::from(next_b) == '*' {
                            s = State::InsideMultilineComment;
                            return None;
                        }
                    }
                    match c {
                        '\\' => s = State::OutsideEscape,
                        '"' => s = State::InsideDoubleQoutes,
                        '\'' => s = State::InsideSingleQoutes,
                        '`' => s = State::InsideBacktickQoutes,
                        _ => {}
                    }
                    Some(b)
                }
                State::OutsideEscape => {
                    if c != '\\' {
                        s = State::OutsideEverything;
                    }
                    Some(b)
                }
                State::InsideLineComment => {
                    //
                    (c == '\n').then(|| {
                        s = State::OutsideEverything;
                        b
                    })
                }
                State::InsideMultilineComment => {
                    if c == '\n' {
                        return Some(b);
                    } else if let Some(next_b) = next_b {
                        if c == '*' && char::from(next_b) == '/' {
                            s = State::MultilineCommentEnd;
                        }
                    }
                    None
                }
                State::MultilineCommentEnd => {
                    s = State::OutsideEverything;
                    None
                }
                State::InsideDoubleQoutes => {
                    if c == '"' {
                        s = State::OutsideEverything;
                    } else if c == '\\' {
                        s = State::InsideDoubleQoutesEscape;
                    }
                    Some(b)
                }
                State::InsideDoubleQoutesEscape => {
                    if c != '\\' {
                        s = State::InsideDoubleQoutes;
                    }
                    Some(b)
                }
                State::InsideSingleQoutes => {
                    if c == '\'' {
                        s = State::OutsideEverything;
                    } else if c == '\\' {
                        s = State::InsideSingleQoutesEscape;
                    }
                    Some(b)
                }
                State::InsideSingleQoutesEscape => {
                    if c != '\\' {
                        s = State::InsideSingleQoutes;
                    }
                    Some(b)
                }
                State::InsideBacktickQoutes => {
                    if c == '`' {
                        s = State::OutsideEverything;
                    } else if c == '\\' {
                        s = State::InsideBacktickQoutesEscape;
                    }
                    Some(b)
                }
                State::InsideBacktickQoutesEscape => {
                    if c != '\\' {
                        s = State::InsideBacktickQoutes;
                    }
                    Some(b)
                }
            }
        })
        .collect()
}

enum State {
    OutsideEverything,
    OutsideEscape,
    InsideLineComment,
    InsideMultilineComment,
    MultilineCommentEnd,
    InsideDoubleQoutes,
    InsideDoubleQoutesEscape,
    InsideSingleQoutes,
    InsideSingleQoutesEscape,
    InsideBacktickQoutes,
    InsideBacktickQoutesEscape,
}

struct WindowIter<'a, T: Copy> {
    index: usize,
    slice: &'a [T],
}

impl<'a, T: Copy> WindowIter<'a, T> {
    fn new(slice: &'a [T]) -> Self {
        Self { index: 0, slice }
    }
}

impl<'a, T: Copy> Iterator for WindowIter<'a, T> {
    type Item = (T, Option<T>);

    fn next(&mut self) -> Option<Self::Item> {
        self.index += 1;
        let i = self.index;
        match i.cmp(&self.slice.len()) {
            std::cmp::Ordering::Less => Some((self.slice[i - 1], Some(self.slice[i]))),
            std::cmp::Ordering::Equal => Some((self.slice[i - 1], None)),
            std::cmp::Ordering::Greater => None,
        }
    }
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use pretty_assertions::assert_eq;

    #[test]
    fn test_minify_javascript() {
        let file = "
let x = 0;

// comment

let y = 1;

/*
 * comment
 */

let z /** comment */ = 3;

console.log(\"/* str */ // str\")
console.log(\'/* str */ // str\')
console.log(`/* str */ // str`)
let x = /\\//;
let y = \"\\\\\"//\";
let z = \'\\\\\'//\';
let b = `\\\\`//`;

    */"
        .as_bytes();

        let want = "
let x = 0;



let y = 1;





let z  = 3;

console.log(\"/* str */ // str\")
console.log(\'/* str */ // str\')
console.log(`/* str */ // str`)
let x = /\\//;
let y = \"\\\\\"//\";
let z = \'\\\\\'//\';
let b = `\\\\`//`;

    */"
        .as_bytes();

        let got = minify_javascript(file);
        assert_eq!(
            String::from_utf8(want.to_owned()).unwrap(),
            String::from_utf8(got).unwrap()
        );
    }
}
