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
	"reflect"
	"testing"
)

func TestGenArgs(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		actual := genArgs(argOpts{
			logLevel:   "2",
			inputPath:  "3",
			outputPath: "4",
			scale:      "full",
			quality:    "1",
			frameRate:  "1",
		})
		expected := []string{
			"-n", "-loglevel", "2",
			"-threads", "1", "-discard", "nokey",
			"-i", "3", "-an",
			"-c:v", "libx264", "-x264-params", "keyint=4",
			"-preset", "veryfast", "-tune", "fastdecode", "-crf", "18",
			"-vsync", "vfr", "-vf", "mpdecimate,fps=0.0167,mpdecimate",
			"-movflags", "empty_moov+default_base_moof+frag_keyframe", "4",
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("\nexpected:\n%v.\ngot:\n%v.", expected, actual)
		}
	})
	t.Run("maximal", func(t *testing.T) {
		actual := genArgs(argOpts{
			logLevel:   "2",
			inputPath:  "3",
			outputPath: "4",
			scale:      "half",
			quality:    "12",
			frameRate:  "60",
		})
		expected := []string{
			"-n", "-loglevel", "2",
			"-threads", "1", "-discard", "nokey",
			"-i", "3", "-an",
			"-c:v", "libx264", "-x264-params", "keyint=4",
			"-preset", "veryfast", "-tune", "fastdecode", "-crf", "51",
			"-vsync", "vfr", "-vf", "mpdecimate,fps=1.0000,mpdecimate,scale='iw/2:ih/2'",
			"-movflags", "empty_moov+default_base_moof+frag_keyframe", "4",
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("\nexpected:\n%v.\ngot:\n%v.", expected, actual)
		}
	})
}
