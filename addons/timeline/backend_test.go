// SPDX-License-Identifier: GPL-2.0-or-later

package timeline

import (
	"strings"
	"testing"

	"nvr/pkg/monitor"

	"github.com/stretchr/testify/require"
)

func TestGenArgs(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		actual := genArgs(
			"2",
			"4",
			config{
				scale:     "full",
				quality:   "1",
				frameRate: "1",
			},
		)
		expected := []string{
			"-n", "-loglevel", "2",
			"-threads", "1", "-discard", "nokey",
			"-i", "-", "-an",
			"-c:v", "libx264", "-x264-params", "keyint=4",
			"-preset", "veryfast", "-tune", "fastdecode", "-crf", "18",
			"-vsync", "vfr", "-vf", "mpdecimate,fps=0.0167,mpdecimate",
			"-movflags", "empty_moov+default_base_moof+frag_keyframe",
			"-f", "mp4", "4",
		}
		require.Equal(t, actual, expected)
	})
	t.Run("maximal", func(t *testing.T) {
		actual := genArgs(
			"2",
			"4",
			config{
				scale:     "half",
				quality:   "12",
				frameRate: "60",
			},
		)
		expected := []string{
			"-n", "-loglevel", "2",
			"-threads", "1", "-discard", "nokey",
			"-i", "-", "-an",
			"-c:v", "libx264", "-x264-params", "keyint=4",
			"-preset", "veryfast", "-tune", "fastdecode", "-crf", "51",
			"-vsync", "vfr", "-vf", "mpdecimate,fps=1.0000,mpdecimate,scale='iw/2:ih/2'",
			"-movflags", "empty_moov+default_base_moof+frag_keyframe",
			"-f", "mp4", "4",
		}
		require.Equal(t, actual, expected)
	})
	t.Run("defaults", func(t *testing.T) {
		actual := genArgs("2", "4", config{})
		expected := []string{
			"-n", "-loglevel", "2",
			"-threads", "1", "-discard", "nokey",
			"-i", "-", "-an",
			"-c:v", "libx264", "-x264-params", "keyint=4",
			"-preset", "veryfast", "-tune", "fastdecode", "-crf", "27",
			"-vsync", "vfr", "-vf", "mpdecimate,fps=6,mpdecimate,scale='iw/8:ih/8'",
			"-movflags", "empty_moov+default_base_moof+frag_keyframe",
			"-f", "mp4", "4",
		}
		require.Equal(t, actual, expected)
	})
}

func TestParseConfig(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		timeline := `{
			"scale":     "1",
			"quality":   "2",
			"frameRate": "3"
		}`
		c := monitor.NewConfig(monitor.RawConfig{
			"timelineConfigVersion": "1",
			"timeline":              timeline,
		})
		actual, err := parseConfig(c)
		require.NoError(t, err)
		expected := config{
			scale:     "1",
			quality:   "2",
			frameRate: "3",
		}
		require.Equal(t, expected, *actual)
	})
	t.Run("timelineErr", func(t *testing.T) {
		c := monitor.NewConfig(monitor.RawConfig{
			"timeline": "{",
		})
		_, err := parseConfig(c)
		require.Error(t, err)
	})
}

func TestMigrate(t *testing.T) {
	c := map[string]string{
		"timelineScale":     "1",
		"timelineQuality":   "2",
		"timelineFrameRate": "3",
	}
	err := migrate(c)
	require.NoError(t, err)
	actual := c

	timeline := strings.Join(strings.Fields(`{
		"scale":     "1",
		"quality":   "2",
		"frameRate": "3"
	}`), "")
	expected := map[string]string{
		"timelineConfigVersion": "1",
		"timeline":              timeline,
	}
	require.Equal(t, expected, actual)
}

func TestMigrateV0ToV1(t *testing.T) {
	c := map[string]string{
		"timelineScale":     "1",
		"timelineQuality":   "2",
		"timelineFrameRate": "3",
	}
	err := migrate(c)
	require.NoError(t, err)
	actual := c

	timeline := strings.Join(strings.Fields(`{
		"scale":     "1",
		"quality":   "2",
		"frameRate": "3"
	}`), "")
	expected := map[string]string{
		"timelineConfigVersion": "1",
		"timeline":              timeline,
	}
	require.Equal(t, expected, actual)
}
