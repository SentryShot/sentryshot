// SPDX-License-Identifier: GPL-2.0-or-later

use std::str::FromStr;

use serde::{Deserialize, Deserializer};

pub fn deserialize_csv_option<'de, D, T>(deserializer: D) -> Result<Vec<T>, D::Error>
where
    D: Deserializer<'de>,
    T: TryFrom<String>,
    <T as TryFrom<String>>::Error: std::fmt::Display,
{
    use serde::de::Error;
    let input = String::deserialize(deserializer)?;
    let mut out = Vec::new();
    for s in input.split(',') {
        if s.is_empty() {
            continue;
        }
        out.push(T::try_from(s.to_owned()).map_err(Error::custom)?);
    }
    Ok(out)
}

pub fn deserialize_csv_option2<'de, D, T>(deserializer: D) -> Result<Vec<T>, D::Error>
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
    Ok(out)
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use pretty_assertions::assert_eq;
    use serde::de::value::{Error as ValueError, StringDeserializer};
    use test_case::test_case;

    #[test_case("a,b,c", vec!["a","b","c"]; "ok")]
    #[test_case(",",     vec![];            "comma")]
    #[test_case("",      vec![];            "empty")]
    fn test_deserialize_csv_option(input: &str, want: Vec<&str>) {
        let want: Vec<String> = want.into_iter().map(ToOwned::to_owned).collect();

        let deserializer = StringDeserializer::<ValueError>::new(input.to_owned());
        let got: Vec<String> = deserialize_csv_option(deserializer).unwrap();
        assert_eq!(want, got);
    }
}
