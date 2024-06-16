// SPDX-License-Identifier: GPL-2.0-or-later

use crate::config::ZoneConfig;
use common::recording::{create_inverted_mask, denormalize_polygon};

#[derive(Debug, PartialEq)]
pub struct Zones(pub Vec<Zone>);

impl Zones {
    pub fn analyze(&self, frame1: &[u8], frame2: &[u8], diff: &mut [u8]) -> Detections {
        diff_frames(frame1, frame2, diff);

        let mut detections = Vec::new();
        for (i, zone) in self.0.iter().enumerate() {
            let (score, is_active) = zone.check_diff(diff);
            if is_active {
                detections.push((i, score));
            }
        }
        detections
    }
}

pub type Detections = Vec<(usize, f32)>;

fn diff_frames(frame1: &[u8], frame2: &[u8], diff: &mut [u8]) {
    for i in 0..frame1.len() {
        diff[i] = u8::abs_diff(frame1[i], frame2[i]);
    }
}

#[allow(clippy::struct_field_names)]
#[derive(Debug, PartialEq)]
pub struct Zone {
    mask: Vec<bool>,

    zone_size: u64,
    frame_size: i64,

    sensitivity: u8,
    threshold_min: f32,
    threshold_max: f32,
}

impl Zone {
    #[allow(
        clippy::cast_sign_loss,
        clippy::cast_possible_truncation,
        clippy::as_conversions
    )]
    pub(crate) fn new(width: u16, height: u16, config: &ZoneConfig) -> Self {
        let polygon = denormalize_polygon(&config.area, width, height);
        let mask_image = create_inverted_mask(&polygon, width, height);
        let (mask, zone_size) = parse_mask_image(&mask_image);

        Zone {
            mask,
            zone_size,
            frame_size: i64::from(width) * i64::from(height),
            sensitivity: (config.sensitivity * 2.56) as u8,
            threshold_min: config.threshold_min,
            threshold_max: config.threshold_max,
        }
    }

    #[allow(clippy::cast_precision_loss, clippy::as_conversions)]
    fn check_diff(&self, diff: &[u8]) -> (f32, bool) {
        let mut n_changed_pixels = 0;
        for (i, is_masked) in self.mask.iter().enumerate() {
            if *is_masked && diff[i] >= self.sensitivity {
                n_changed_pixels += 1;
            }
        }

        let percent_changed = (n_changed_pixels as f32 / self.zone_size as f32) * 100.0;

        let is_active =
            percent_changed > self.threshold_min && percent_changed < self.threshold_max;

        (percent_changed, is_active)
    }
}

fn parse_mask_image(img: &Vec<Vec<bool>>) -> (Vec<bool>, u64) {
    let mut mask = Vec::new();
    let mut zone_size = 0;
    for row in img {
        for pixel in row {
            mask.push(*pixel);
            if *pixel {
                zone_size += 1;
            }
        }
    }
    (mask, zone_size)
}

#[allow(clippy::float_cmp, clippy::needless_pass_by_value)]
#[cfg(test)]
mod tests {
    use super::*;
    use common::{recording::normalize, PointNormalized};
    use pretty_assertions::assert_eq;
    use test_case::test_case;

    struct Case<'a> {
        name: &'a str,
        config: ZoneConfig,
        frame1: &'a [u8],
        frame2: &'a [u8],
        want: f32,
        is_active: bool,
    }

    fn p(x: u16, y: u16) -> PointNormalized {
        PointNormalized {
            x: normalize(x, 100),
            y: normalize(y, 100),
        }
    }

    #[test_case(
        "100%",
        ZoneConfig {
            enable: false,
            sensitivity: 8.0,
            threshold_min: 0.0,
            threshold_max: 90.0,
            area: vec![p(0, 0), p(100, 0), p(100, 100), p(0, 100)],
        },
        &[0, 0, 0, 0],
        &[255, 255, 255, 255],
        100.0,
        false
    )]
    #[test_case(
        "50%",
        ZoneConfig {
            enable: false,
            sensitivity: 8.0,
            threshold_min: 49.9,
            threshold_max: 50.1,
            area: vec![p(0, 0), p(100, 0), p(100, 100), p(0, 100)],
        },
        &[0, 0, 0, 0],
        &[255, 255, 0, 0],
        50.0,
        true
    )]
    #[test_case(
        "0%",
        ZoneConfig {
            enable: false,
            sensitivity: 8.0,
            threshold_min: 10.0,
            threshold_max: 0.0,
            area: vec![p(0, 0), p(100, 0), p(100, 100), p(0, 100)],
        },
        &[0, 0, 0, 0],
        &[0, 0, 0, 0],
        0.0,
        false
    )]
    fn test_compare_frames2(
        name: &str,
        config: ZoneConfig,
        frame1: &[u8],
        frame2: &[u8],
        want: f32,
        is_active: bool,
    ) {
        let zone = Zone::new(2, 2, &config);
        let mut diff = [0; 4];
        diff_frames(frame1, frame2, &mut diff);

        let (got, is_active2) = zone.check_diff(&diff);
        assert_eq!(want, got, "{0}", name);
        assert_eq!(is_active, is_active2, "{0}", name);
    }

    #[test]
    fn test_compare_frames() {
        let cases: Vec<Case> = vec![
            Case {
                name: "sensitivity",
                config: ZoneConfig {
                    enable: false,
                    sensitivity: 50.0,
                    threshold_min: 100.0,
                    threshold_max: 0.0,
                    area: vec![p(0, 0), p(100, 0), p(100, 100), p(0, 100)],
                },
                frame1: &[0, 0, 0, 0],
                frame2: &[127, 127, 127, 128],
                want: 25.0,
                is_active: false,
            },
            Case {
                name: "area 50%",
                config: ZoneConfig {
                    enable: false,
                    sensitivity: 8.0,
                    threshold_min: 100.0,
                    threshold_max: 0.0,
                    area: vec![p(0, 0), p(50, 0), p(50, 100), p(0, 100)],
                },
                frame1: &[0, 0, 0, 0],
                frame2: &[255, 0, 255, 0],
                want: 100.0,
                is_active: false,
            },
            Case {
                name: "area 25%",
                config: ZoneConfig {
                    enable: false,
                    sensitivity: 8.0,
                    threshold_min: 100.0,
                    threshold_max: 0.0,
                    area: vec![p(50, 0), p(100, 0), p(100, 50), p(50, 50)],
                },
                frame1: &[0, 0, 0, 0],
                frame2: &[0, 255, 0, 0],
                want: 100.0,
                is_active: false,
            },
        ];

        for tc in cases {
            let zone = Zone::new(2, 2, &tc.config);
            let mut diff = [0; 4];
            diff_frames(tc.frame1, tc.frame2, &mut diff);

            let (got, is_active) = zone.check_diff(&diff);
            assert_eq!(tc.want, got, "{0}", tc.name);
            assert_eq!(tc.is_active, is_active, "{0}", tc.name);
        }
    }

    /*func BenchmarkDetector(b *testing.B) {
        width := 500
        height := 500
        frameSize := width * height
        frame1 := bytes.Repeat([]byte{0}, frameSize)
        frame2 := bytes.Repeat([]byte{255}, frameSize)
        diff := make([]byte, frameSize)

        newTestZone := func(area area) *zone {
            return newZone(
                width,
                height,
                zoneConfig{
                    Enable:       true,
                    Sensitivity:  8,
                    ThresholdMin: 10,
                    ThresholdMax: 100,
                    Area:         area,
                },
            )
        }

        zones := zones{
            // Full frame.
            newTestZone(area{{0, 0}, {100, 0}, {100, 100}, {0, 100}}),
            // Large diamond 50%.
            newTestZone(area{{50, 0}, {100, 50}, {50, 100}, {0, 50}}),
            // Medium diamond.
            newTestZone(area{{50, 25}, {75, 50}, {50, 75}, {25, 50}}),
        }

        var zone int
        var score float64
        var active bool
        b.ResetTimer()
        for i := 0; i < b.N; i++ {
            onActive := func(zone int, s float64) {
                score = s
            }
            zones.analyze(frame1, frame2, diff, onActive)
        }
        _, _, _ = zone, score, active

    }*/
}
