// Copyright 2020-2022 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; either version 2 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package motion

import (
	"testing"
	"time"

	"nvr/pkg/ffmpeg"
	"nvr/pkg/monitor"

	"github.com/stretchr/testify/require"
)

func TestParseConfig(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		motion := `
		{
			"enable":     "true",
			"feedRate":   "5",
			"frameScale": "full",
			"duration":   "6",
			"zones":[
				{
					"enable": true,
					"sensitivity": 7,
					"thresholdMin": 8,
					"thresholdMax": 9,
					"area":[[10,11],[12,13],[14,15]]
				}
			]
		}`
		c := monitor.NewConfig(monitor.RawConfig{
			"id":              "1",
			"logLevel":        "2",
			"hwaccel":         "3",
			"timestampOffset": "4",
			"subInput":        "x",
			"motion":          motion,
		})
		actual, enable, err := parseConfig(c)
		require.NoError(t, err)
		require.True(t, enable)

		expected := config{
			monitorID:       "1",
			logLevel:        "2",
			hwaccel:         "3",
			timestampOffset: 4000000,
			feedRate:        "5",
			duration:        200 * time.Millisecond,
			recDuration:     6 * time.Second,
			scale:           1,
			zones: []zoneConfig{{
				Enable:       true,
				Sensitivity:  7,
				ThresholdMin: 8,
				ThresholdMax: 9,
				Area:         []ffmpeg.Point{{10, 11}, {12, 13}, {14, 15}},
			}},
		}
		require.Equal(t, expected, *actual)
	})
	t.Run("empty", func(t *testing.T) {
		c := monitor.Config{}
		actual, enable, err := parseConfig(c)
		require.NoError(t, err)
		require.Nil(t, actual)
		require.False(t, enable)
	})
	t.Run("disabled", func(t *testing.T) {
		motion := `
		{
			"enable":       "false",
			"detectorName": "x"
		}`
		c := monitor.NewConfig(monitor.RawConfig{
			"motion": motion,
		})
		actual, enable, err := parseConfig(c)
		require.NoError(t, err)
		require.Nil(t, actual)
		require.False(t, enable)
	})
	// Errors.
	cases := map[string]monitor.RawConfig{
		"motionErr": {
			"motion": `{"enable": "true",}`,
		},
		"timestampOffsetErr": {
			"timestampOffset": "nil",
			"motion":          `{"enable": "true"}`,
		},
		"feedRateErr": {
			"motion": `{"enable": "true", "feedRate":"nil"}`,
		},
		"durationErr": {
			"motion": `{"enable": "true", "feedRate":"0", "duration":"nil"}`,
		},
	}
	for name, conf := range cases {
		t.Run(name, func(t *testing.T) {
			_, enable, err := parseConfig(monitor.NewConfig(conf))
			require.Error(t, err)
			require.False(t, enable)
		})
	}
}
