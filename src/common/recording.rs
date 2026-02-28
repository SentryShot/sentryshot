// SPDX-License-Identifier: GPL-2.0-or-later

use crate::{
    MonitorId, ParseMonitorIdError, Point, PointNormalized, Polygon, PolygonNormalized,
    time::{Duration, SECOND, UnixNano},
};
use jiff::Timestamp;
use serde::{Deserialize, Serialize};
use std::{
    cmp::Ordering::{Equal, Greater, Less},
    num::ParseIntError,
    ops::Deref,
    path::{Path, PathBuf},
    str::FromStr,
};
use thiserror::Error;

#[derive(Clone, Hash, PartialEq, Eq)]
#[allow(clippy::module_name_repetitions)]
pub struct RecordingId {
    nanos: UnixNano,
    monitor_id: MonitorId,
}

impl std::fmt::Debug for RecordingId {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}_{}", self.nanos, self.monitor_id)
    }
}

impl std::fmt::Display for RecordingId {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let time = Timestamp::from(self.nanos)
            .strftime(&format!("%Y-%m-%d_%H-%M-%S_{}", self.monitor_id))
            .to_string();
        write!(f, "{time}")
    }
}

impl RecordingId {
    pub fn from_nanos(nanos: UnixNano, monitor_id: MonitorId) -> Result<Self, RecordingIdError> {
        if nanos.is_negative() {
            return Err(RecordingIdError::NegativeTime(nanos));
        }
        Ok(Self { nanos, monitor_id })
    }

    #[must_use]
    pub fn zero() -> Self {
        Self {
            nanos: UnixNano::new(0),
            monitor_id: MonitorId::try_from("x".to_owned()).expect("should be valid"),
        }
    }

    #[must_use]
    pub fn max() -> Self {
        Self {
            nanos: UnixNano::new(i64::MAX),
            monitor_id: MonitorId::try_from("x".to_owned()).expect("should be valid"),
        }
    }

    #[must_use]
    pub fn nanos(&self) -> UnixNano {
        self.nanos
    }

    #[must_use]
    pub fn monitor_id(&self) -> &MonitorId {
        &self.monitor_id
    }

    #[must_use]
    pub fn as_string(&self) -> String {
        format!("{}_{}", self.nanos, self.monitor_id)
    }

    #[must_use]
    pub fn full_path(&self) -> PathBuf {
        Path::new(&self.level1().to_string())
            .join(self.level2().to_string())
            .join(self.as_string())
    }

    // Unix timestamp with 12 day precision.
    #[must_use]
    pub fn level1(&self) -> u16 {
        const LEN: usize = 16;
        let nanos_str = self.nanos.to_string();
        let nanos_len = nanos_str.len();
        if nanos_len < LEN {
            0
        } else {
            nanos_str[..=nanos_len - LEN]
                .to_owned()
                .parse()
                .expect("valid")
        }
    }

    // Unix timestamp with 3 hour precision.
    #[must_use]
    pub fn level2(&self) -> u32 {
        const LEN: usize = 14;
        let nanos_str = self.nanos.to_string();
        let nanos_len = nanos_str.len();
        if nanos_len < LEN {
            0
        } else {
            nanos_str[..=nanos_len - LEN]
                .to_owned()
                .parse()
                .expect("valid")
        }
    }
}

#[derive(Debug, thiserror::Error)]
#[allow(clippy::module_name_repetitions)]
pub enum RecordingIdError {
    #[error("too long: {0}")]
    TooLong(usize),

    #[error("split timestamp and monitor ID: '{0}'")]
    Split(String),

    #[error("parse nanos: {0}")]
    ParseNanos(#[from] ParseIntError),

    #[error("invalid monitor id: {0}")]
    InvalidMonitorId(#[from] ParseMonitorIdError),

    #[error("time is negative: {0:?}")]
    NegativeTime(UnixNano),
}

impl FromStr for RecordingId {
    type Err = RecordingIdError;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        use RecordingIdError::*;

        if 128 < s.len() {
            // Extra resource exhaustion check.
            return Err(TooLong(s.len()));
        }
        let [nanos, m_id] = s.split('_').collect::<Vec<_>>()[..] else {
            return Err(Split(s.to_owned()));
        };

        let nanos = UnixNano::new(nanos.parse::<i64>()?);
        let m_id = MonitorId::try_from(m_id.to_owned())?;
        RecordingId::from_nanos(nanos, m_id)
    }
}

impl Serialize for RecordingId {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        self.as_string().serialize(serializer)
    }
}

impl<'de> Deserialize<'de> for RecordingId {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        String::deserialize(deserializer)?
            .parse()
            .map_err(serde::de::Error::custom)
    }
}

impl PartialOrd for RecordingId {
    fn partial_cmp(&self, other: &Self) -> Option<std::cmp::Ordering> {
        Some(self.cmp(other))
    }
}

impl Ord for RecordingId {
    fn cmp(&self, other: &Self) -> std::cmp::Ordering {
        match self.nanos.cmp(&other.nanos) {
            Less => Less,
            // Compare monitor ID if timestamp is equal.
            Equal => self.monitor_id.cmp(&other.monitor_id),
            Greater => Greater,
        }
    }
}

#[must_use]
pub fn normalize_polygon(input: &Polygon, w: u16, h: u16) -> PolygonNormalized {
    input
        .iter()
        .map(|point| PointNormalized {
            x: normalize(point.x, w),
            y: normalize(point.y, h),
        })
        .collect()
}

#[must_use]
#[allow(clippy::cast_possible_truncation, clippy::as_conversions)]
pub fn normalize(input: u16, max: u16) -> u32 {
    ((1_000_000 * u64::from(input)) / u64::from(max)) as u32
}

#[must_use]
pub fn denormalize_polygon(input: &PolygonNormalized, w: u16, h: u16) -> Polygon {
    input
        .iter()
        .map(|point| Point {
            x: denormalize(point.x, w),
            y: denormalize(point.y, h),
        })
        .collect()
}

#[must_use]
#[allow(clippy::cast_possible_truncation, clippy::as_conversions)]
pub fn denormalize(input: u32, max: u16) -> u16 {
    (div_ceil(u64::from(input) * u64::from(max), 1_000_000)) as u16
}

fn div_ceil(a: u64, b: u64) -> u64 {
    let d = a / b;
    let r = a % b;
    if r > 0 && b > 0 { d + 1 } else { d }
}

// CreateMask creates an image mask from a polygon.
// Pixels outside the polygon are masked.
#[must_use]
pub fn create_mask(poly: &Polygon, w: u16, h: u16) -> Vec<Vec<bool>> {
    let mut img = vec![vec![false; usize::from(w)]; usize::from(h)];
    for y in 0..w {
        for x in 0..h {
            img[usize::from(y)][usize::from(x)] = !vertex_inside_poly(x, y, poly);
        }
    }
    img
}

// CreateInvertedMask creates an image mask from a polygon.
// Pixels inside the polygon are masked.
#[must_use]
pub fn create_inverted_mask(poly: &Polygon, w: u16, h: u16) -> Vec<Vec<bool>> {
    let mut img = vec![vec![false; usize::from(w)]; usize::from(h)];
    for y in 0..h {
        for x in 0..w {
            img[usize::from(y)][usize::from(x)] = vertex_inside_poly(x, y, poly);
        }
    }
    img
}

// Returns true if point is inside polygon.
#[must_use]
pub fn vertex_inside_poly(x: u16, y: u16, poly: &Polygon) -> bool {
    if poly.is_empty() {
        return false;
    }
    let x = i32::from(x);
    let y = i32::from(y);
    let mut inside = false;

    let mut j = poly.len() - 1;
    for i in 0..poly.len() {
        let xi = i32::from(poly[i].x);
        let yi = i32::from(poly[i].y);
        let xj = i32::from(poly[j].x);
        let yj = i32::from(poly[j].y);
        if ((yi > y) != (yj > y)) && (x < (xj - xi) * (y - yi) / (yj - yi) + xi) {
            inside = !inside;
        }
        j = i;
    }
    inside
}

// Returns true if point is inside polygon. All parameters are normalized.
#[must_use]
pub fn vertex_inside_poly2(x: u32, y: u32, poly: &PolygonNormalized) -> bool {
    if poly.is_empty() {
        return false;
    }
    let x = i64::from(x);
    let y = i64::from(y);
    let mut inside = false;

    let mut j = poly.len() - 1;
    for i in 0..poly.len() {
        let xi = i64::from(poly[i].x);
        let yi = i64::from(poly[i].y);
        let xj = i64::from(poly[j].x);
        let yj = i64::from(poly[j].y);
        if ((yi > y) != (yj > y)) && (x < (xj - xi) * (y - yi) / (yj - yi) + xi) {
            inside = !inside;
        }
        j = i;
    }
    inside
}

#[repr(transparent)]
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct FeedRateSec(Duration);

impl FeedRateSec {
    #[must_use]
    pub fn new(v: Duration) -> Self {
        Self(v)
    }
}

impl<'de> Deserialize<'de> for FeedRateSec {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        let feed_rate = f32::deserialize(deserializer)?;
        Ok(FeedRateSec(feed_rate_to_duration(feed_rate)))
    }
}

impl Deref for FeedRateSec {
    type Target = Duration;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

#[repr(transparent)]
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct DurationSec(Duration);

impl DurationSec {
    #[must_use]
    pub fn new(v: Duration) -> Self {
        Self(v)
    }
}

impl<'de> Deserialize<'de> for DurationSec {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        let secs = u32::deserialize(deserializer)?;
        Ok(DurationSec(Duration::from_secs(secs)))
    }
}

impl Deref for DurationSec {
    type Target = Duration;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

// Calculates frame duration from feed rate (fps).
#[allow(
    clippy::cast_precision_loss,
    clippy::cast_possible_truncation,
    clippy::as_conversions
)]
fn feed_rate_to_duration(feed_rate: f32) -> Duration {
    if feed_rate == 0.0 {
        return Duration::new(0);
    }
    Duration::new(((1.0 * SECOND as f32) / feed_rate) as i64)
}

pub struct FrameRateLimiter {
    max_rate: u64,

    first_ts: Option<u64>,
    prev_ts: u64,
    count: u64,
}

#[derive(Debug, Error)]
pub enum FrameRateLimiterError {
    #[error("prev ts is greater than current")]
    PrevTsGreaterThanCurrent,

    #[error("timestamp spike: {0} vs {1}")]
    Spike(u64, u64),

    #[error("timestamp is zero")]
    Zero,
}

impl FrameRateLimiter {
    #[must_use]
    pub fn new(max_rate: u64) -> Self {
        FrameRateLimiter {
            max_rate,
            first_ts: None,
            prev_ts: 0,
            count: 0,
        }
    }

    // Returns true if the frame should be discarded.
    pub fn discard(&mut self, ts: u64) -> Result<bool, FrameRateLimiterError> {
        use FrameRateLimiterError::*;
        if ts == 0 {
            return Err(Zero);
        }

        if self.max_rate == 0 {
            return Ok(false);
        }

        let Some(first_frame) = self.first_ts else {
            self.first_ts = Some(ts);
            self.prev_ts = ts;
            self.count += 1;
            return Ok(false);
        };

        if self.prev_ts > ts {
            return Err(PrevTsGreaterThanCurrent);
        }

        if (ts - self.prev_ts) > (self.max_rate * 100) {
            return Err(Spike(self.prev_ts, ts));
        }

        if ((ts - first_frame) / self.count) >= self.max_rate {
            self.prev_ts = ts;
            self.count += 1;
            return Ok(false);
        }

        Ok(true)
    }
}

#[allow(clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use crate::time::MILLISECOND;
    use pretty_assertions::assert_eq;
    use test_case::test_case;

    #[test]
    fn test_recording_id_full_path() {
        let id: RecordingId = "123456789000000000_x".parse().unwrap();

        let want = Path::new("123/12345/123456789000000000_x");
        let got = id.full_path();
        assert_eq!(want, got);
    }

    #[test_case(543_210_987_654_321, 0)]
    #[test_case(6_543_210_987_654_321, 6)]
    #[test_case(76_543_210_987_654_321, 76)]
    fn test_recording_id_level1(nanos: i64, want: u16) {
        let id = RecordingId::from_nanos(UnixNano::new(nanos), "x".to_owned().try_into().unwrap())
            .unwrap();
        assert_eq!(want, id.level1());
    }

    #[test_case(3_210_987_654_321, 0)]
    #[test_case(43_210_987_654_321, 4)]
    #[test_case(543_210_987_654_321, 54)]
    fn test_recording_id_level2(nanos: i64, want: u32) {
        let id = RecordingId::from_nanos(UnixNano::new(nanos), "x".to_owned().try_into().unwrap())
            .unwrap();
        assert_eq!(want, id.level2());
    }

    #[test_case(640, 1, 1_562)]
    #[test_case(640, 64, 100_000)]
    #[test_case(640, 100, 156_250)]
    #[test_case(640, 640, 1_000_000)]
    #[test_case(480, 1, 2_083)]
    #[test_case(480, 64, 133_333)]
    #[test_case(480, 100, 208_333)]
    #[test_case(480, 480, 1_000_000)]
    #[test_case(100, 6553, 65_530_000)]
    #[test_case(100, 65535, 655_350_000)]
    #[test_case(655, 100, 152_671)]
    #[test_case(6553, 100, 15_260)]
    #[test_case(65535, 100, 1_525)]
    #[test_case(6553, 6553, 1_000_000)]
    fn test_normalize(max: u16, value: u16, normalized: u32) {
        let got = normalize(value, max);
        assert_eq!(normalized, got);

        let got = denormalize(normalized, max);
        assert_eq!(value, got);
    }

    #[test]
    fn test_create_mask() {
        fn p(x: u16, y: u16) -> Point {
            Point { x, y }
        }
        let cases = &[
            (
                vec![p(3, 1), p(6, 6), p(0, 6)],
                "
                _______
                _______
                ___X___
                __XXX__
                __XXX__
                _XXXXX_
                _______",
            ),
            (
                vec![
                    p(2, 0),
                    p(5, 0),
                    p(7, 3),
                    p(7, 4),
                    p(4, 7),
                    p(0, 4),
                    p(0, 2),
                ],
                "
                __XXX__
                _XXXXX_
                XXXXXXX
                XXXXXXX
                XXXXXXX
                _XXXXX_
                __XXX__",
            ),
            (
                vec![p(7, 0), p(7, 7), p(1, 5), p(6, 5), p(0, 7), p(0, 0)],
                "
                XXXXXXX
                XXXXXXX
                XXXXXXX
                XXXXXXX
                XXXXXXX
                X_____X
                XXX_XXX", // Lines cross over themselves at the bottom.
            ),
        ];

        for (input, want) in cases {
            let mask = create_mask(input, 7, 7);

            let got = image_to_text(&mask);
            let want = want.replace(' ', "");
            assert_eq!(want, got);
        }
    }

    #[test]
    fn test_create_inverted_mask() {
        fn p(x: u16, y: u16) -> Point {
            Point { x, y }
        }
        let cases = &[
            (
                vec![p(3, 1), p(6, 6), p(0, 6)],
                "
                XXXXXXX
                XXXXXXX
                XXX_XXX
                XX___XX
                XX___XX
                X_____X
                XXXXXXX",
            ),
            (
                vec![
                    p(2, 0),
                    p(5, 0),
                    p(7, 3),
                    p(7, 4),
                    p(4, 7),
                    p(0, 4),
                    p(0, 2),
                ],
                "
                XX___XX
                X_____X
                _______
                _______
                _______
                X_____X
                XX___XX",
            ),
            (
                vec![p(7, 0), p(7, 7), p(1, 5), p(6, 5), p(0, 7), p(0, 0)],
                "
                _______
                _______
                _______
                _______
                _______
                _XXXXX_
                ___X___", // Lines cross over themselves at the bottom.
            ),
        ];

        for (input, want) in cases {
            let mask = create_inverted_mask(input, 7, 7);

            let got = image_to_text(&mask);
            let want = want.replace(' ', "");
            assert_eq!(want, got);
        }
    }

    #[test]
    fn test_vertex_inside_poly_empty() {
        assert!(!vertex_inside_poly(0, 0, &Vec::new()));
    }

    #[allow(clippy::needless_range_loop)]
    fn image_to_text(img: &[Vec<bool>]) -> String {
        let mut text = String::new();
        let max_y = img.len();
        let max_x = img[0].len();
        for y in 0..max_y {
            text.push('\n');
            for x in 0..max_x {
                let pixel = img[y][x];
                if pixel {
                    text.push('_');
                } else {
                    text.push('X');
                }
            }
        }
        text
    }

    #[test_case(0.0, 0)]
    #[test_case(1.0, SECOND)]
    #[test_case(2.0, 500 * MILLISECOND)]
    #[test_case(0.5, 2 * SECOND)]
    fn test_feed_rate_to_duration(input: f32, want: i64) {
        assert_eq!(Duration::new(want), feed_rate_to_duration(input));
    }
    /*cases := []struct {
        input    float64
        expected time.Duration
    }{
        {1, 1 * time.Second},
        {2, 500 * time.Millisecond},
        {0.5, 2 * time.Second},
    }
    for _, tc := range cases {
        name := strconv.FormatFloat(tc.input, 'f', -1, 64)
        t.Run(name, func(t *testing.T) {
            actual := FeedRateToDuration(tc.input)
            require.Equal(t, tc.expected, actual)
        })
    }*/

    #[test]
    fn test_frame_rate_limiter() {
        let mut limiter = FrameRateLimiter::new(10);

        assert!(!limiter.discard(100).unwrap());
        assert!(!limiter.discard(110).unwrap()); // 10.
        assert!(!limiter.discard(120).unwrap()); // 10.
        assert!(limiter.discard(121).unwrap()); // 7.
        assert!(limiter.discard(122).unwrap()); // 7.
        assert!(!limiter.discard(135).unwrap()); // 11.
        assert!(!limiter.discard(140).unwrap()); // 10.
        assert!(!limiter.discard(160).unwrap()); // 12.
        assert!(!limiter.discard(168).unwrap()); // 11.
        assert!(!limiter.discard(176).unwrap()); // 10.
        assert!(!limiter.discard(184).unwrap()); // 10.
        assert!(!limiter.discard(193).unwrap()); // 10.
        assert!(!limiter.discard(202).unwrap()); // 10.
        assert!(!limiter.discard(211).unwrap()); // 10.
        assert!(!limiter.discard(220).unwrap()); // 10.
        assert!(limiter.discard(229).unwrap()); // 9.
        assert!(!limiter.discard(239).unwrap()); // 10.
    }

    #[test]
    fn test_frame_rate_limiter_decreasing() {
        let mut limiter = FrameRateLimiter::new(10);

        assert!(!limiter.discard(100).unwrap());
        assert!(matches!(
            limiter.discard(50),
            Err(FrameRateLimiterError::PrevTsGreaterThanCurrent)
        ));
    }

    #[test]
    fn test_frame_rate_limiter_spike() {
        let mut limiter = FrameRateLimiter::new(10);

        assert!(!limiter.discard(100).unwrap());
        assert!(!limiter.discard(110).unwrap());
        assert!(matches!(
            limiter.discard(100_500),
            Err(FrameRateLimiterError::Spike(110, 100_500))
        ));
    }

    #[test]
    fn test_frame_rate_limiter_zero() {
        let mut limiter = FrameRateLimiter::new(0);

        assert!(!limiter.discard(100).unwrap());
        assert!(!limiter.discard(100_000_000).unwrap());
    }

    #[test]
    fn test_frame_rate_limiter_zero2() {
        let mut limiter = FrameRateLimiter::new(10);

        assert!(matches!(
            limiter.discard(0),
            Err(FrameRateLimiterError::Zero)
        ));
    }
}
