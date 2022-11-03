package motion

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompareFrames(t *testing.T) {
	cases := map[string]struct {
		config   zoneConfig
		frame1   []uint8
		frame2   []uint8
		expected float64
		isActive bool
	}{
		"100%": {
			config: zoneConfig{
				Sensitivity:  8,
				ThresholdMax: 90,
				Area:         area{{0, 0}, {100, 0}, {100, 100}, {0, 100}},
			},
			frame1: []uint8{
				0, 0,
				0, 0,
			},
			frame2: []uint8{
				255, 255,
				255, 255,
			},
			expected: 100,
		},
		"50%": {
			config: zoneConfig{
				Sensitivity:  8,
				ThresholdMin: 49.9,
				ThresholdMax: 50.1,
				Area:         area{{0, 0}, {100, 0}, {100, 100}, {0, 100}},
			},
			frame1: []uint8{
				0, 0,
				0, 0,
			},
			frame2: []uint8{
				255, 255,
				0, 0,
			},
			expected: 50,
			isActive: true,
		},
		"0%": {
			config: zoneConfig{
				Sensitivity:  8,
				ThresholdMin: 10,
				Area:         area{{0, 0}, {100, 0}, {100, 100}, {0, 100}},
			},
			frame1: []uint8{
				0, 0,
				0, 0,
			},
			frame2: []uint8{
				0, 0,
				0, 0,
			},
			expected: 0,
		},
		"sensitivity": {
			config: zoneConfig{
				Sensitivity:  50,
				ThresholdMin: 100,
				Area:         area{{0, 0}, {100, 0}, {100, 100}, {0, 100}},
			},
			frame1: []uint8{
				0, 0,
				0, 0,
			},
			frame2: []uint8{
				127, 127,
				127, 128,
			},
			expected: 25,
		},
		"area50%": {
			config: zoneConfig{
				Sensitivity:  8,
				ThresholdMin: 100,
				Area:         area{{0, 0}, {50, 0}, {50, 100}, {0, 100}},
			},
			frame1: []uint8{
				0, 0,
				0, 0,
			},
			frame2: []uint8{
				255, 0,
				255, 0,
			},
			expected: 100,
		},
		"area25%": {
			config: zoneConfig{
				Sensitivity:  8,
				ThresholdMin: 100,
				Area:         area{{50, 0}, {100, 0}, {100, 50}, {50, 50}},
			},
			frame1: []uint8{
				0, 0,
				0, 0,
			},
			frame2: []uint8{
				0, 255,
				0, 0,
			},
			expected: 100,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			zone := newZone(2, 2, tc.config)
			diff := make([]byte, 4)
			diffFrames(tc.frame1, tc.frame2, diff)

			actual, isActive := zone.checkDiff(diff)
			require.Equal(t, tc.expected, actual)
			require.Equal(t, tc.isActive, isActive)
		})
	}
}

func BenchmarkDetector(b *testing.B) {
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
}
