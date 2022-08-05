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
			argOpts{
				logLevel:   "2",
				inputPath:  "3",
				outputPath: "4",
			},
			config{
				scale:     "full",
				quality:   "1",
				frameRate: "1",
			},
		)
		expected := []string{
			"-n", "-loglevel", "2",
			"-threads", "1", "-discard", "nokey",
			"-i", "3", "-an",
			"-c:v", "libx264", "-x264-params", "keyint=4",
			"-preset", "veryfast", "-tune", "fastdecode", "-crf", "18",
			"-vsync", "vfr", "-vf", "mpdecimate,fps=0.0167,mpdecimate",
			"-movflags", "empty_moov+default_base_moof+frag_keyframe", "4",
		}
		require.Equal(t, actual, expected)
	})
	t.Run("maximal", func(t *testing.T) {
		actual := genArgs(
			argOpts{
				logLevel:   "2",
				inputPath:  "3",
				outputPath: "4",
			},
			config{
				scale:     "half",
				quality:   "12",
				frameRate: "60",
			},
		)
		expected := []string{
			"-n", "-loglevel", "2",
			"-threads", "1", "-discard", "nokey",
			"-i", "3", "-an",
			"-c:v", "libx264", "-x264-params", "keyint=4",
			"-preset", "veryfast", "-tune", "fastdecode", "-crf", "51",
			"-vsync", "vfr", "-vf", "mpdecimate,fps=1.0000,mpdecimate,scale='iw/2:ih/2'",
			"-movflags", "empty_moov+default_base_moof+frag_keyframe", "4",
		}
		require.Equal(t, actual, expected)
	})
	t.Run("defaults", func(t *testing.T) {
		actual := genArgs(
			argOpts{
				logLevel:   "2",
				inputPath:  "3",
				outputPath: "4",
			},
			config{},
		)
		expected := []string{
			"-n", "-loglevel", "2",
			"-threads", "1", "-discard", "nokey",
			"-i", "3", "-an",
			"-c:v", "libx264", "-x264-params", "keyint=4",
			"-preset", "veryfast", "-tune", "fastdecode", "-crf", "27",
			"-vsync", "vfr", "-vf", "mpdecimate,fps=6,mpdecimate,scale='iw/8:ih/8'",
			"-movflags", "empty_moov+default_base_moof+frag_keyframe", "4",
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
		c := monitor.Config{
			"timelineConfigVersion": "1",
			"timeline":              timeline,
		}
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
		c := monitor.Config{
			"timeline": "{",
		}
		_, err := parseConfig(c)
		require.Error(t, err)
	})
}

func TestMigrate(t *testing.T) {
	c := monitor.Config{
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
	expected := monitor.Config{
		"timelineConfigVersion": "1",
		"timeline":              timeline,
	}
	require.Equal(t, expected, actual)
}

func TestMigrateV0ToV1(t *testing.T) {
	c := monitor.Config{
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
	expected := monitor.Config{
		"timelineConfigVersion": "1",
		"timeline":              timeline,
	}
	require.Equal(t, expected, actual)
}
