// SPDX-License-Identifier: GPL-2.0-or-later

use chrono::NaiveDateTime;
use serde::{Deserialize, Serialize};
use std::{
    ops::Deref,
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
#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct UnixNano(i64);

impl UnixNano {
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

    pub fn add_duration(&self, duration: Duration) -> Option<Self> {
        Some(Self(self.0.checked_add(duration.0)?))
    }

    pub fn sub_duration(&self, duration: Duration) -> Option<Self> {
        Some(Self(self.0.checked_sub(duration.0)?))
    }

    // Reports whether the time intant `self` is after `other`.
    pub fn after(&self, other: Self) -> bool {
        self.0 > other.0
    }

    // Reports whether the time intant `self` is before `other`.
    pub fn before(&self, other: Self) -> bool {
        self.0 < other.0
    }

    // Returns the duration self - u.
    pub fn sub(&self, u: Self) -> Option<Duration> {
        self.0.checked_sub(u.0).map(Duration)
    }

    pub fn as_chrono(&self) -> Option<NaiveDateTime> {
        let sec = self.0 / SECOND;
        let nsec = self.0 % SECOND;
        NaiveDateTime::from_timestamp_opt(sec, nsec as u32)
    }

    pub const MAX: UnixNano = UnixNano(i64::MAX);
}

impl From<i64> for UnixNano {
    fn from(v: i64) -> Self {
        Self(v)
    }
}

impl Deref for UnixNano {
    type Target = i64;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

// `std::time::Duration` but without the u128 to u64 conversions.
#[repr(transparent)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct Duration(i64);

impl Duration {
    pub fn from_nanos(nanos: i64) -> Self {
        Self(nanos)
    }

    pub fn from_millis(millis: u32) -> Self {
        Self(millis as i64 * MILLISECOND)
    }

    pub fn from_secs(secs: u32) -> Self {
        Self(secs as i64 * SECOND)
    }

    pub fn from_minutes(minutes: u32) -> Self {
        Self(minutes as i64 * MINUTE)
    }

    pub fn from_hours(hours: u32) -> Self {
        Self(hours as i64 * HOUR)
    }

    pub fn as_std(&self) -> Option<std::time::Duration> {
        Some(std::time::Duration::from_nanos(u64::try_from(self.0).ok()?))
    }

    pub fn as_h264(&self) -> DurationH264 {
        DurationH264::from(nano_to_timescale(self.0, H264_TIMESCALE.into()))
    }

    pub fn until(time: UnixNano) -> Option<Self> {
        Some(Self(time.checked_sub(*UnixNano::now())?))
    }
}

impl From<i64> for Duration {
    fn from(v: i64) -> Self {
        Self(v)
    }
}

impl From<f64> for Duration {
    fn from(v: f64) -> Self {
        Self(v as i64)
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

pub const H264_SECOND: i64 = H264_TIMESCALE as i64;
pub const H264_MILLISECOND: i64 = H264_SECOND / 1000;

// 90khz time since the Unix epoch.
#[repr(transparent)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct UnixH264(i64);

impl UnixH264 {
    pub fn now() -> Self {
        let nanos = i64::try_from(
            SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .expect("time went backwards")
                .as_nanos(),
        )
        .expect("timestamp to fit u64");

        Self(nano_to_timescale(nanos, H264_TIMESCALE.into()))
    }

    pub fn checked_add_duration(&self, duration: DurationH264) -> Option<Self> {
        Some(Self(self.0.checked_add(duration.0)?))
    }

    pub fn checked_sub_duration(&self, duration: DurationH264) -> Option<Self> {
        Some(Self(self.0.checked_sub(duration.0)?))
    }

    pub fn checked_sub(&self, other: Self) -> Option<Self> {
        Some(Self(self.0.checked_sub(other.0)?))
    }

    pub fn as_nanos(&self) -> UnixNano {
        let clock_rate = i64::from(H264_TIMESCALE);
        let secs = self.0 / clock_rate;
        let dec = self.0 % clock_rate;
        UnixNano((secs * SECOND) + ((dec * SECOND) / clock_rate))
    }

    // Reports whether the time intant `self` is after `other`.
    pub fn after(&self, other: Self) -> bool {
        self.0 > other.0
    }

    pub fn as_chrono(&self) -> Option<NaiveDateTime> {
        let nanos = *self.as_nanos();
        let sec = nanos / SECOND;
        let nsec = nanos % SECOND;
        NaiveDateTime::from_timestamp_opt(sec, nsec as u32)
    }
}

impl From<i64> for UnixH264 {
    fn from(v: i64) -> Self {
        Self(v)
    }
}

impl Deref for UnixH264 {
    type Target = i64;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

// H264 duration with 90khz timescale.
#[repr(transparent)]
#[derive(Clone, Copy, Debug, Default, Hash, PartialEq, Eq, PartialOrd, Ord)]
pub struct DurationH264(i64);

impl DurationH264 {
    pub const fn new(v: i64) -> Self {
        Self(v)
    }

    pub fn is_zero(&self) -> bool {
        self.0 == 0
    }

    pub fn checked_add(&self, rhs: Self) -> Option<Self> {
        Some(Self(self.0.checked_add(rhs.0)?))
    }

    pub fn checked_sub(&self, rhs: Self) -> Option<Self> {
        Some(Self(self.0.checked_sub(rhs.0)?))
    }

    pub fn checked_mul(&self, rhs: Self) -> Option<Self> {
        Some(Self(self.0.checked_mul(rhs.0)?))
    }

    pub fn checked_div(&self, rhs: Self) -> Option<Self> {
        Some(Self(self.0.checked_div(rhs.0)?))
    }

    pub fn checked_rem(&self, rhs: Self) -> Option<Self> {
        Some(Self(self.0.checked_rem(rhs.0)?))
    }

    pub fn as_secs_f64(&self) -> f64 {
        let ts = self.as_nanos();
        let sec = ts / SECOND;
        let nsec = ts % SECOND;
        (sec as f64) + (nsec as f64) / (SECOND as f64)
    }

    pub fn as_i32(&self) -> Result<i32, std::num::TryFromIntError> {
        i32::try_from(self.0)
    }

    pub fn as_u32(&self) -> Result<u32, std::num::TryFromIntError> {
        u32::try_from(self.0)
    }

    pub fn as_millis(&self) -> i64 {
        self.as_nanos() / MILLISECOND
    }

    pub fn as_nanos(&self) -> i64 {
        let clock_rate = i64::from(H264_TIMESCALE);
        let secs = self.0 / clock_rate;
        let dec = self.0 % clock_rate;
        (secs * SECOND) + ((dec * SECOND) / clock_rate)
    }
}

impl From<i32> for DurationH264 {
    fn from(v: i32) -> Self {
        Self(i64::from(v))
    }
}

impl From<u32> for DurationH264 {
    fn from(v: u32) -> Self {
        Self(i64::from(v))
    }
}

impl From<i64> for DurationH264 {
    fn from(v: i64) -> Self {
        Self(v)
    }
}

impl From<UnixH264> for DurationH264 {
    fn from(time: UnixH264) -> Self {
        Self(time.0)
    }
}

impl Deref for DurationH264 {
    type Target = i64;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

// Converts value in nanoseconds into a different timescale.
pub fn nano_to_timescale(value: i64, timescale: i64) -> i64 {
    let secs = value / SECOND;
    let dec = value % SECOND;
    (secs * timescale) + (dec * timescale / SECOND)
}

#[cfg(test)]
mod tests {
    use super::*;
    use test_case::test_case;

    #[test_case(100000, H264_TIMESCALE.into(), 9; "9")]
    #[test_case(100000000, H264_TIMESCALE.into(), 9000; "9k")]
    #[test_case(100000000000, H264_TIMESCALE.into(), 9000000; "9m")]
    #[test_case(100000000000000, H264_TIMESCALE.into(), 9000000000; "3days")]
    #[test_case(1000000000000000, H264_TIMESCALE.into(), 90000000000; "30days")]
    #[test_case(10000000000000000, H264_TIMESCALE.into(), 900000000000; "300days")]
    #[test_case(100000000000000000, H264_TIMESCALE.into(), 9000000000000; "3000days")]
    fn test_nano_to_timescale(input: i64, scale: i64, want: i64) {
        assert_eq!(want, nano_to_timescale(input, scale))
    }
}
