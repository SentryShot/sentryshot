// SPDX-License-Identifier: GPL-2.0-or-later

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use std::{
    fmt::Display,
    ops::{Add, Deref, Sub},
    time::{SystemTime, UNIX_EPOCH},
};

pub const NANOSECOND: i64 = 1;
pub const MICROSECOND: i64 = NANOSECOND * 1000;
pub const MILLISECOND: i64 = MICROSECOND * 1000;
pub const SECOND: i64 = MILLISECOND * 1000;
pub const MINUTE: i64 = SECOND * 60;
pub const HOUR: i64 = MINUTE * 60;

// Nanoseconds since the Unix epoch.
#[repr(transparent)]
#[derive(
    Clone, Copy, Debug, Default, Hash, PartialEq, Eq, PartialOrd, Ord, Serialize, Deserialize,
)]
pub struct UnixNano(i64);

impl UnixNano {
    #[must_use]
    pub fn new(v: i64) -> Self {
        Self(v)
    }

    #[must_use]
    pub fn now() -> Self {
        Self(
            i64::try_from(
                SystemTime::now()
                    .duration_since(UNIX_EPOCH)
                    .expect("time went backwards")
                    .as_nanos(),
            )
            .expect("timestamp to fit i64"),
        )
    }

    #[must_use]
    pub fn checked_add(&self, rhs: Self) -> Option<Self> {
        Some(Self(self.0.checked_add(rhs.0)?))
    }

    #[must_use]
    pub fn checked_sub(&self, rhs: Self) -> Option<Self> {
        Some(Self(self.0.checked_sub(rhs.0)?))
    }

    // Reports whether the time intant `self` is after `other`.
    #[must_use]
    pub fn after(&self, other: Self) -> bool {
        self.0 > other.0
    }

    // Reports whether the time intant `self` is before `other`.
    #[must_use]
    pub fn before(&self, other: Self) -> bool {
        self.0 < other.0
    }

    // Returns the duration self - u.
    pub fn sub(&self, u: Self) -> Option<Duration> {
        self.0.checked_sub(u.0).map(Duration)
    }

    #[must_use]
    pub fn until(time: Self) -> Option<Duration> {
        Some(time.checked_sub(UnixNano::now())?.into())
    }
}

impl From<UnixNano> for DateTime<Utc> {
    fn from(val: UnixNano) -> Self {
        DateTime::from_timestamp_nanos(val.0)
    }
}

impl From<Duration> for UnixNano {
    fn from(value: Duration) -> Self {
        Self(value.0)
    }
}

impl From<UnixH264> for UnixNano {
    fn from(value: UnixH264) -> Self {
        let clock_rate = i64::from(H264_TIMESCALE);
        let secs = value.0 / clock_rate;
        let dec = value.0 % clock_rate;
        UnixNano((secs * SECOND) + ((dec * SECOND) / clock_rate))
    }
}

impl Deref for UnixNano {
    type Target = i64;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

impl Add for UnixNano {
    type Output = Self;

    fn add(self, rhs: Self) -> Self::Output {
        Self(self.0 + rhs.0)
    }
}

impl Sub for UnixNano {
    type Output = Self;

    fn sub(self, rhs: Self) -> Self::Output {
        Self(self.0 - rhs.0)
    }
}

impl Display for UnixNano {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        self.0.fmt(f)
    }
}

// `std::time::Duration` but without the u128 to u64 conversions.
#[repr(transparent)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct Duration(i64);

impl Duration {
    #[must_use]
    pub fn new(v: i64) -> Self {
        Self(v)
    }

    #[allow(clippy::cast_possible_truncation, clippy::as_conversions)]
    #[must_use]
    pub fn from_f64(v: f64) -> Self {
        Self(v as i64)
    }

    #[must_use]
    pub fn from_nanos(nanos: i64) -> Self {
        Self(nanos)
    }

    #[must_use]
    pub fn from_millis(millis: u32) -> Self {
        Self(i64::from(millis) * MILLISECOND)
    }

    #[must_use]
    pub fn from_secs(secs: u32) -> Self {
        Self(i64::from(secs) * SECOND)
    }

    #[must_use]
    pub fn from_minutes(minutes: u32) -> Self {
        Self(i64::from(minutes) * MINUTE)
    }

    #[must_use]
    pub fn from_hours(hours: u32) -> Self {
        Self(i64::from(hours) * HOUR)
    }

    #[must_use]
    pub fn as_seconds(&self) -> i64 {
        self.0 / SECOND
    }

    #[must_use]
    pub fn as_std(&self) -> Option<std::time::Duration> {
        Some(std::time::Duration::from_nanos(u64::try_from(self.0).ok()?))
    }
}

impl From<UnixNano> for Duration {
    fn from(v: UnixNano) -> Self {
        Self(v.0)
    }
}

impl Deref for Duration {
    type Target = i64;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

// The number of time units that pass per second.
pub const H264_TIMESCALE: u32 = 90000;

#[allow(clippy::as_conversions)]
pub const H264_SECOND: i64 = H264_TIMESCALE as i64;
pub const H264_MILLISECOND: i64 = H264_SECOND / 1000;

// 90khz time since the Unix epoch.
#[repr(transparent)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, PartialOrd, Ord)]
pub struct UnixH264(i64);

impl UnixH264 {
    #[must_use]
    pub fn new(v: i64) -> Self {
        Self(v)
    }

    #[must_use]
    pub fn now() -> Self {
        UnixNano::now().into()
    }

    #[must_use]
    pub fn checked_add(&self, other: Self) -> Option<Self> {
        Some(Self(self.0.checked_add(other.0)?))
    }

    #[must_use]
    pub fn checked_sub(&self, other: Self) -> Option<Self> {
        Some(Self(self.0.checked_sub(other.0)?))
    }

    // Reports whether the time intant `self` is after `other`.
    #[must_use]
    pub fn after(&self, other: Self) -> bool {
        self.0 > other.0
    }
}

impl From<UnixH264> for DateTime<Utc> {
    fn from(val: UnixH264) -> Self {
        UnixNano::from(val).into()
    }
}

impl From<DurationH264> for UnixH264 {
    fn from(value: DurationH264) -> Self {
        Self(value.0)
    }
}

impl From<DtsOffset> for UnixH264 {
    fn from(value: DtsOffset) -> Self {
        Self(value.0.into())
    }
}

impl From<UnixNano> for UnixH264 {
    fn from(nanos: UnixNano) -> Self {
        Self(nano_to_timescale(*nanos, H264_TIMESCALE.into()))
    }
}

impl Add for UnixH264 {
    type Output = Self;

    fn add(self, rhs: Self) -> Self::Output {
        Self(self.0 + rhs.0)
    }
}

impl Sub for UnixH264 {
    type Output = Self;

    fn sub(self, rhs: Self) -> Self::Output {
        Self(self.0 - rhs.0)
    }
}

impl Deref for UnixH264 {
    type Target = i64;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

impl Display for UnixH264 {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        self.0.fmt(f)
    }
}

// H264 duration with 90khz timescale.
#[repr(transparent)]
#[derive(Clone, Copy, Debug, Default, Hash, PartialEq, Eq, PartialOrd, Ord)]
pub struct DurationH264(i64);

impl DurationH264 {
    #[must_use]
    pub const fn new(v: i64) -> Self {
        Self(v)
    }

    #[must_use]
    pub fn is_zero(&self) -> bool {
        self.0 == 0
    }

    #[must_use]
    pub fn checked_add(&self, rhs: Self) -> Option<Self> {
        Some(Self(self.0.checked_add(rhs.0)?))
    }

    #[must_use]
    pub fn checked_sub(&self, rhs: Self) -> Option<Self> {
        Some(Self(self.0.checked_sub(rhs.0)?))
    }

    #[must_use]
    pub fn checked_mul(&self, rhs: Self) -> Option<Self> {
        Some(Self(self.0.checked_mul(rhs.0)?))
    }

    #[must_use]
    pub fn checked_div(&self, rhs: Self) -> Option<Self> {
        Some(Self(self.0.checked_div(rhs.0)?))
    }

    #[must_use]
    pub fn checked_rem(&self, rhs: Self) -> Option<Self> {
        Some(Self(self.0.checked_rem(rhs.0)?))
    }

    #[must_use]
    #[allow(clippy::cast_precision_loss, clippy::as_conversions)]
    pub fn as_secs_f64(&self) -> f64 {
        let ts = self.as_nanos();
        let sec = ts / SECOND;
        let nanosec = ts % SECOND;
        (sec as f64) + (nanosec as f64) / (SECOND as f64)
    }

    pub fn as_u32(&self) -> Result<u32, std::num::TryFromIntError> {
        u32::try_from(self.0)
    }

    #[must_use]
    pub fn as_millis(&self) -> i64 {
        self.as_nanos() / MILLISECOND
    }

    #[must_use]
    #[allow(clippy::cast_precision_loss)]
    pub fn as_nanos(&self) -> i64 {
        let clock_rate = i64::from(H264_TIMESCALE);
        let secs = self.0 / clock_rate;
        let dec = self.0 % clock_rate;
        (secs * SECOND) + ((dec * SECOND) / clock_rate)
    }
}

impl From<UnixH264> for DurationH264 {
    fn from(value: UnixH264) -> Self {
        Self(value.0)
    }
}

impl From<DtsOffset> for DurationH264 {
    fn from(value: DtsOffset) -> Self {
        Self(value.0.into())
    }
}

impl Deref for DurationH264 {
    type Target = i64;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

impl From<Duration> for DurationH264 {
    fn from(nanos: Duration) -> Self {
        Self(nano_to_timescale(*nanos, H264_TIMESCALE.into()))
    }
}

impl From<UnixNano> for DurationH264 {
    fn from(nanos: UnixNano) -> Self {
        Self(nano_to_timescale(*nanos, H264_TIMESCALE.into()))
    }
}

impl Display for DurationH264 {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        self.0.fmt(f)
    }
}

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
#[repr(transparent)]
pub struct DtsOffset(i32);

impl DtsOffset {
    #[must_use]
    pub fn new(value: i32) -> Self {
        Self(value)
    }
}

impl Deref for DtsOffset {
    type Target = i32;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

// Converts value in nanoseconds into a different timescale.
#[must_use]
pub fn nano_to_timescale(value: i64, timescale: i64) -> i64 {
    let secs = value / SECOND;
    let dec = value % SECOND;
    (secs * timescale) + (dec * timescale / SECOND)
}

#[cfg(test)]
mod tests {
    use super::*;
    use test_case::test_case;

    #[test_case(100_000, H264_TIMESCALE.into(), 9; "9")]
    #[test_case(100_000_000, H264_TIMESCALE.into(), 9000; "9k")]
    #[test_case(100_000_000_000, H264_TIMESCALE.into(), 9_000_000; "9m")]
    #[test_case(100_000_000_000_000, H264_TIMESCALE.into(), 9_000_000_000; "3days")]
    #[test_case(1_000_000_000_000_000, H264_TIMESCALE.into(), 90_000_000_000; "30days")]
    #[test_case(10_000_000_000_000_000, H264_TIMESCALE.into(), 900_000_000_000; "300days")]
    #[test_case(100_000_000_000_000_000, H264_TIMESCALE.into(), 9_000_000_000_000; "3000days")]
    fn test_nano_to_timescale(input: i64, scale: i64, want: i64) {
        assert_eq!(want, nano_to_timescale(input, scale));
    }
}
