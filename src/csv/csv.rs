// SPDX-License-Identifier: GPL-2.0-or-later

#[cfg(test)]
mod test;

use serde::{Deserialize, Deserializer};
use std::str::FromStr;

pub fn deserialize_csv_option<'de, D, T>(deserializer: D) -> Result<Option<Vec<T>>, D::Error>
where
    D: Deserializer<'de>,
    T: FromStr,
    <T as FromStr>::Err: std::fmt::Display,
{
    use serde::de::Error;
    let input = String::deserialize(deserializer)?;
    let mut out = Vec::new();
    for s in input.split(',') {
        if s.is_empty() {
            continue;
        }
        out.push(T::from_str(s).map_err(Error::custom)?);
    }
    Ok((!out.is_empty()).then_some(out))
}
