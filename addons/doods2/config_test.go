package doods

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"nvr/pkg/ffmpeg"
	"nvr/pkg/monitor"

	"github.com/stretchr/testify/require"
)

func TestParseConfig(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		doods := `
		{
			"enable":       "true",
			"thresholds":   "{\"5\":6}",
			"crop":         "[7,8,9]",
			"mask":         "{\"enable\":true,\"area\":[[10,11],[12,13]]}",
			"detectorName": "14",
			"feedRate":     "15",
			"duration":     "0.000000016",
			"useSubStream": "true"
		}`
		c := monitor.Config{
			"id":              "1",
			"hwaccel":         "2",
			"logLevel":        "3",
			"timestampOffset": "4",
			"subInput":        "x",
			"doods":           doods,
		}
		actual, enable, err := parseConfig(c)
		require.NoError(t, err)
		require.True(t, enable)

		expected := config{
			monitorID:       "1",
			hwaccel:         "2",
			ffmpegLogLevel:  "3",
			timestampOffset: 4000000,
			thresholds:      thresholds{"5": 6},
			cropX:           7,
			cropY:           8,
			cropSize:        9,
			mask: mask{
				Enable: true,
				Area:   ffmpeg.Polygon{{10, 11}, {12, 13}},
			},
			detectorName: "14",
			feedRate:     15,
			recDuration:  16,
			useSubStream: true,
		}
		require.Equal(t, expected, *actual)
	})
	t.Run("gray", func(t *testing.T) {
		doods := `
		{
			"enable":       "true",
			"detectorName": "gray_x",
			"useSubStream": "true"
		}`
		c := monitor.Config{
			"doods": doods,
		}
		actual, enable, err := parseConfig(c)
		require.NoError(t, err)
		require.True(t, enable)

		expected := config{
			detectorName: "gray_x",
			grayMode:     true,
		}
		require.Equal(t, expected, *actual)
	})
	t.Run("disabled", func(t *testing.T) {
		doods := `
		{
			"enable":       "false",
			"detectorName": "x"
		}`
		c := monitor.Config{
			"doods": doods,
		}
		actual, enable, err := parseConfig(c)
		require.NoError(t, err)
		require.Nil(t, actual)
		require.False(t, enable)
	})
	t.Run("threshErr", func(t *testing.T) {
		doods := `
		{
			"enable":       "true",
			"thresholds": "nil"
		}`
		c := monitor.Config{
			"doods": doods,
		}
		_, enable, err := parseConfig(c)
		var e *json.SyntaxError
		require.ErrorAs(t, err, &e)
		require.False(t, enable)
	})
	t.Run("cleanThresh", func(t *testing.T) {
		doods := `
		{
			"enable":     "true",
			"thresholds": "{\"a\":1,\"b\":2,\"c\":-1}"
		}`
		c := monitor.Config{
			"doods": doods,
		}
		config, enable, err := parseConfig(c)
		require.NoError(t, err)
		require.True(t, enable)

		actual := config.thresholds
		expected := thresholds{"a": 1, "b": 2}
		require.Equal(t, expected, actual)
	})
	t.Run("empty", func(t *testing.T) {
		doods := `
		{
			"enable": "true"
		}`
		c := monitor.Config{
			"doods": doods,
		}
		actual, enable, err := parseConfig(c)
		require.NoError(t, err)
		require.True(t, enable)
		require.Equal(t, config{}, *actual)
	})

	// Errors.
	cases := map[string]monitor.Config{
		"doodsErr": {
			"doods": `{"enable": "true",}`,
		},
		"timestampOffsetErr": {
			"timestampOffset": "nil",
			"doods":           `{"enable": "true"}`,
		},
		"cropErr": {
			"doods": `{"enable": "true", "crop":"[1,2,x]"}`,
		},
		"maskErr": {
			"doods": `{"enable": "true", "mask":"{\"enable\":true, \"area\":[[1,x]]}"}`,
		},
		"feedRateErr": {
			"doods": `{"enable": "true", "feedRate":"nil"}`,
		},
		"recDurationErr": {
			"doods": `{"enable": "true", "duration":"nil"}`,
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
		thresholds:  thresholds{},
		cropSize:    defaultCropSize,
		feedRate:    defaultFeedRate,
		recDuration: defaultRecDuration,
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
				monitorID:    "1",
				detectorName: "2",
				feedRate:     3,
				recDuration:  4 * time.Second,
			},
			nil,
		},
		"cropSizeLow": {
			config{
				monitorID:    "1",
				cropSize:     -1,
				detectorName: "2",
				feedRate:     3,
				recDuration:  4 * time.Second,
			},
			ErrInvalidCropSize,
		},
		"cropSizeHigh": {
			config{
				monitorID:    "1",
				cropSize:     101,
				detectorName: "2",
				feedRate:     3,
				recDuration:  4 * time.Second,
			},
			ErrInvalidCropSize,
		},
		"cropXLow": {
			config{
				monitorID:    "1",
				cropX:        -1,
				detectorName: "2",
				feedRate:     3,
				recDuration:  4 * time.Second,
			},
			ErrInvalidCropX,
		},
		"cropXHigh": {
			config{
				monitorID:    "1",
				cropX:        101,
				detectorName: "2",
				feedRate:     3,
				recDuration:  4 * time.Second,
			},
			ErrInvalidCropX,
		},
		"cropYLow": {
			config{
				monitorID:    "1",
				cropY:        -1,
				detectorName: "2",
				feedRate:     3,
				recDuration:  4 * time.Second,
			},
			ErrInvalidCropY,
		},
		"cropYHigh": {
			config{
				monitorID:    "1",
				cropY:        101,
				detectorName: "2",
				feedRate:     3,
				recDuration:  4 * time.Second,
			},
			ErrInvalidCropY,
		},
		"feedRateErr": {
			config{
				monitorID:    "1",
				detectorName: "2",
				feedRate:     0,
				recDuration:  4 * time.Second,
			},
			ErrInvalidFeedRate,
		},
		"durationErr": {
			config{
				monitorID:    "1",
				detectorName: "2",
				feedRate:     3,
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

func TestMigrate(t *testing.T) {
	c := monitor.Config{
		"doodsEnable":       "true",
		"doodsThresholds":   `{"1":2}`,
		"doodsCrop":         "[3,4,5]",
		"doodsMask":         `{"enable":true,"area":[[6,7],[8,9]]}`,
		"doodsDetectorName": "10",
		"doodsFeedRate":     "11",
		"doodsDuration":     "0.000000012",
		"doodsUseSubStream": "true",
	}
	err := migrate(c)
	require.NoError(t, err)
	actual := c

	doods := strings.Join(strings.Fields(`{
		"enable":       "true",
		"thresholds":   "{\"1\":2}",
		"crop":         "[3,4,5]",
		"mask":         "{\"enable\":true,\"area\":[[6,7],[8,9]]}",
		"detectorName": "10",
		"feedRate":     "11",
		"duration":     "0.000000012",
		"useSubStream": "true"
	}`), "")
	expected := monitor.Config{
		"doodsConfigVersion": "1",
		"doods":              doods,
	}
	require.Equal(t, expected, actual)
}

func TestMigrateV0ToV1(t *testing.T) {
	c := monitor.Config{
		"doodsEnable":       "true",
		"doodsThresholds":   `{"1":2}`,
		"doodsCrop":         "[3,4,5]",
		"doodsMask":         `{"enable":true,"area":[[6,7],[8,9]]}`,
		"doodsDetectorName": "10",
		"doodsFeedRate":     "11",
		"doodsDuration":     "0.000000012",
		"doodsUseSubStream": "true",
	}
	err := migrate(c)
	require.NoError(t, err)
	actual := c

	doods := strings.Join(strings.Fields(`{
		"enable":       "true",
		"thresholds":   "{\"1\":2}",
		"crop":         "[3,4,5]",
		"mask":         "{\"enable\":true,\"area\":[[6,7],[8,9]]}",
		"detectorName": "10",
		"feedRate":     "11",
		"duration":     "0.000000012",
		"useSubStream": "true"
	}`), "")
	expected := monitor.Config{
		"doodsConfigVersion": "1",
		"doods":              doods,
	}
	require.Equal(t, expected, actual)
}
