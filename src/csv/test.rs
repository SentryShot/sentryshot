// SPDX-License-Identifier: GPL-2.0-or-later

use crate::deserialize_csv_option;
use pretty_assertions::assert_eq;
use serde::de::value::{Error as ValueError, StringDeserializer};
use test_case::test_case;

#[test_case("a,b,c", Some(vec!["a","b","c"]); "ok")]
#[test_case(",",     None;                    "comma")]
#[test_case("",      None;                    "empty")]
fn test_deserialize_csv_option(input: &str, want: Option<Vec<&str>>) {
    let want: Option<Vec<String>> =
        want.and_then(|v| Some(v.into_iter().map(ToOwned::to_owned).collect()));

    let deserializer = StringDeserializer::<ValueError>::new(input.to_owned());
    let got: Option<Vec<String>> = deserialize_csv_option(deserializer).unwrap();
    assert_eq!(want, got);
}
