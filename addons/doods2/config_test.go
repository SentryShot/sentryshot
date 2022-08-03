package doods

import (
	"encoding/json"
	"testing"
	"time"

	"nvr/pkg/ffmpeg"
	"nvr/pkg/monitor"

	"github.com/stretchr/testify/require"
)

func TestParseConfig(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		c := monitor.Config{
			"id":                "1",
			"subInput":          "x",
			"timestampOffset":   "6",
			"doodsEnable":       "true",
			"doodsUseSubStream": "true",
			"doodsThresholds":   `{"4":5}`,
			"doodsDuration":     "0.000000003",
			"doodsFrameScale":   "half",
			"doodsFeedRate":     "2",
			"doodsDetectorName": "7",
			"doodsCrop":         "[8,9,10]",
			"logLevel":          "11",
			"doodsMask":         `{"enable":true,"area":[[1,2],[3,4]]}`,
			"hwaccel":           "12",
		}
		actual, enable, err := parseConfig(c)
		require.NoError(t, err)
		require.True(t, enable)

		expected := config{
			useSubStream:    true,
			monitorID:       "1",
			feedRate:        2,
			recDuration:     3,
			thresholds:      thresholds{"4": 5},
			timestampOffset: 6000000,
			detectorName:    "7",
			cropX:           8,
			cropY:           9,
			cropSize:        10,
			ffmpegLogLevel:  "11",
			mask: mask{
				Enable: true,
				Area:   ffmpeg.Polygon{{1, 2}, {3, 4}},
			},
			hwaccel: "12",
		}
		require.Equal(t, expected, *actual)
	})
	t.Run("gray", func(t *testing.T) {
		c := monitor.Config{
			"timestampOffset":   "0",
			"doodsEnable":       "true",
			"doodsThresholds":   `{}`,
			"doodsDuration":     "0.000000003",
			"doodsFeedRate":     "2",
			"doodsDetectorName": "gray_x",
			"doodsCrop":         "[]",
		}
		actual, enable, err := parseConfig(c)
		require.NoError(t, err)
		require.True(t, enable)

		expected := config{
			recDuration:  3,
			feedRate:     2,
			thresholds:   thresholds{},
			detectorName: "gray_x",
			grayMode:     true,
		}
		require.Equal(t, expected, *actual)
	})
	t.Run("disabled", func(t *testing.T) {
		c := monitor.Config{
			"timestampOffset": "6",
			"doodsEnable":     "false",
			"doodsThresholds": `{"4":5}`,
			"doodsDuration":   "0.000000003",
			"doodsFrameScale": "half",
			"doodsFeedRate":   "500000000",
		}
		config, enable, err := parseConfig(c)
		require.NoError(t, err)
		require.False(t, enable)
		require.Nil(t, config)
	})
	t.Run("threshErr", func(t *testing.T) {
		c := monitor.Config{
			"doodsEnable":     "true",
			"doodsThresholds": "nil",
		}
		_, enable, err := parseConfig(c)
		var e *json.SyntaxError
		require.ErrorAs(t, err, &e)
		require.False(t, enable)
	})
	t.Run("cleanThresh", func(t *testing.T) {
		c := monitor.Config{
			"timestampOffset": "0",
			"doodsEnable":     "true",
			"doodsDuration":   "1",
			"doodsThresholds": `{"a":1,"b":2,"c":-1}`,
			"doodsFeedRate":   "1",
			"doodsCrop":       "[8,9,10]",
		}
		config, enable, err := parseConfig(c)
		require.NoError(t, err)
		require.True(t, enable)

		actual := config.thresholds
		expected := thresholds{"a": 1, "b": 2}
		require.Equal(t, expected, actual)
	})
	t.Run("empty", func(t *testing.T) {
		c := monitor.Config{
			"doodsEnable": "true",
		}
		actual, enable, err := parseConfig(c)
		require.NoError(t, err)
		require.True(t, enable)
		require.Equal(t, config{}, *actual)
	})

	// Errors.
	cases := map[string]monitor.Config{
		"durationErr": {
			"doodsEnable":   "true",
			"doodsFeedRate": "nil",
		},
		"recDurationErr": {
			"doodsEnable":   "true",
			"doodsDuration": "nil",
		},
		"timestampOffsetErr": {
			"timestampOffset": "nil",
			"doodsEnable":     "true",
		},
		"cropErr": {
			"doodsEnable": "true",
			"doodsCrop":   `[1,2,"x"]`,
		},
		"maskErr": {
			"doodsEnable": "true",
			"doodsMask":   `{"enable":true,"area":[[1,x]]}`,
		},
	}
	for name, conf := range cases {
		t.Run(name, func(t *testing.T) {
			_, enable, err := parseConfig(conf)
			require.Error(t, err)
			require.False(t, enable)
		})
	}
}

func TestFillMissing(t *testing.T) {
	actual := config{}
	actual.fillMissing()
	expected := config{
		feedRate:    defaultFeedRate,
		recDuration: defaultRecDuration,
		cropSize:    defaultCropSize,
		thresholds:  thresholds{},
	}
	require.Equal(t, expected, actual)
}

func TestValidate(t *testing.T) {
	cases := map[string]struct {
		input config
		err   error
	}{
		"ok": {
			config{
				detectorName: "1",
				feedRate:     2,
				monitorID:    "3",
				recDuration:  4 * time.Second,
			},
			nil,
		},
		"cropSizeLow": {
			config{
				cropSize:     -1,
				detectorName: "1",
				feedRate:     2,
				monitorID:    "3",
				recDuration:  4 * time.Second,
			},
			ErrInvalidCropSize,
		},
		"cropSizeHigh": {
			config{
				cropSize:     101,
				detectorName: "1",
				feedRate:     2,
				monitorID:    "3",
				recDuration:  4 * time.Second,
			},
			ErrInvalidCropSize,
		},
		"cropXLow": {
			config{
				cropX:        -1,
				detectorName: "1",
				feedRate:     2,
				monitorID:    "3",
				recDuration:  4 * time.Second,
			},
			ErrInvalidCropX,
		},
		"cropXHigh": {
			config{
				cropX:        101,
				detectorName: "1",
				monitorID:    "3",
				recDuration:  4 * time.Second,
			},
			ErrInvalidCropX,
		},
		"cropYLow": {
			config{
				cropY:        -1,
				detectorName: "1",
				feedRate:     2,
				monitorID:    "3",
				recDuration:  4 * time.Second,
			},
			ErrInvalidCropY,
		},
		"cropYHigh": {
			config{
				cropY:        101,
				detectorName: "1",
				feedRate:     2,
				monitorID:    "3",
				recDuration:  4 * time.Second,
			},
			ErrInvalidCropY,
		},
		"feedRateErr": {
			config{
				detectorName: "1",
				feedRate:     0,
				monitorID:    "3",
				recDuration:  4 * time.Second,
			},
			ErrInvalidFeedRate,
		},
		"durationErr": {
			config{
				detectorName: "1",
				feedRate:     2,
				monitorID:    "3",
				recDuration:  -1,
			},
			ErrInvalidDuration,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := tc.input.validate()
			require.ErrorIs(t, err, tc.err)
		})
	}
}
