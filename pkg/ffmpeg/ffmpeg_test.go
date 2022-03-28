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

package ffmpeg

import (
	"context"
	"fmt"
	"image"
	"nvr/pkg/log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFakeProcess(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	if os.Getenv("SLEEP") == "1" {
		time.Sleep(1 * time.Hour)
	}

	fmt.Fprintf(os.Stdout, "%v", "out")
	fmt.Fprintf(os.Stderr, "%v", "err")

	os.Exit(0)
}

func fakeExecCommand(env ...string) *exec.Cmd {
	cs := []string{"-test.run=TestFakeProcess"}
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_TEST_PROCESS=1"}
	cmd.Env = append(cmd.Env, env...)
	return cmd
}

func TestProcess(t *testing.T) {
	t.Run("running", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		p := NewProcess(fakeExecCommand())
		err := p.Start(ctx)
		require.NoError(t, err)
	})
	t.Run("startWithLogger", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			logger := log.NewMockLogger()
			logger.Start(ctx)
			feed, cancel2 := logger.Subscribe()

			logFunc := func(msg string) {
				logger.FFmpegLevel("info").
					Msgf("test %v", msg)
			}

			p := NewProcess(fakeExecCommand()).
				Timeout(0).
				StdoutLogger(logFunc).
				StderrLogger(logFunc)

			err := p.Start(ctx)
			require.NoError(t, err)

			compareOutput := func(input log.Log) {
				output1 := "test stdout: out"
				output2 := "test stderr: err"
				switch {
				case input.Msg == output1:
				case input.Msg == output2:
				default:
					t.Fatalf("outputs doesn't match: '%v'", input.Msg)
				}
			}

			compareOutput(<-feed)
			compareOutput(<-feed)
			cancel2()

			cancel()
		})
	})
	_, pw, err := os.Pipe()
	require.NoError(t, err)

	t.Run("stdoutErr", func(t *testing.T) {
		cmd := fakeExecCommand()
		cmd.Stdout = pw

		p := process{cmd: cmd}.
			StdoutLogger(func(string) {})

		err := p.Start(context.Background())
		require.Error(t, err)
	})
	t.Run("stderrErr", func(t *testing.T) {
		cmd := fakeExecCommand()
		cmd.Stderr = pw

		p := process{cmd: cmd}.
			StderrLogger(func(string) {})

		err := p.Start(context.Background())
		require.Error(t, err)
	})
}

func TestMakePipe(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		pipePath := tempDir + "/pipe.fifo"
		err = MakePipe(pipePath)
		require.NoError(t, err)

		_, err = os.Stat(pipePath)
		require.NoError(t, err)
	})
	t.Run("MkfifoErr", func(t *testing.T) {
		err := MakePipe("")
		require.Error(t, err)
	})
}

func TestShellProcessSize(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	fmt.Fprint(os.Stderr, `
		Stream #0:0: Video: h264 (Main), yuv420p(progressive), 720x1280 fps, 30.00
	`)
}

func fakeExecCommandSize(...string) *exec.Cmd {
	cs := []string{"-test.run=TestShellProcessSize"}
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_TEST_PROCESS=1"}
	return cmd
}

func TestShellProcessNoOutput(t *testing.T) {}

func fakeExecCommandNoOutput(...string) *exec.Cmd {
	cs := []string{"-test.run=TestShellProcessNoOutput"}
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_TEST_PROCESS=1"}
	return cmd
}

func TestSizeFromStream(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		f := New("")
		f.command = fakeExecCommandSize

		actual, err := f.SizeFromStream(context.Background(), "")
		require.NoError(t, err)
		require.Equal(t, actual, "720x1280")
	})
	t.Run("runErr", func(t *testing.T) {
		f := New("")
		_, err := f.SizeFromStream(context.Background(), "")
		require.Error(t, err)
	})
	t.Run("regexErr", func(t *testing.T) {
		f := New("")
		f.command = fakeExecCommandNoOutput

		_, err := f.SizeFromStream(context.Background(), "")
		require.ErrorIs(t, err, strconv.ErrSyntax)
	})
}

func TestShellProcessDuration(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	fmt.Fprint(os.Stderr, `
		Duration: 01:02:59.99, start: 0.000000, bitrate: 614 kb/s
	`)
}

func fakeExecCommandDuration(...string) *exec.Cmd {
	cs := []string{"-test.run=TestShellProcessDuration"}
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_TEST_PROCESS=1"}
	return cmd
}

func TestVideoDuration(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		f := New("")
		f.command = fakeExecCommandDuration

		output, err := f.VideoDuration("")
		require.NoError(t, err)

		actual := fmt.Sprintf("%v", output)
		require.Equal(t, actual, "1h2m59.99s")
	})
	t.Run("runErr", func(t *testing.T) {
		f := New("")
		_, err := f.VideoDuration("")
		require.Error(t, err)
	})
	t.Run("regexErr", func(t *testing.T) {
		f := New("")
		f.command = fakeExecCommandNoOutput

		_, err := f.VideoDuration("")
		require.ErrorIs(t, err, strconv.ErrSyntax)
	})
}

func imageToText(img image.Image) string {
	var text string
	max := img.Bounds().Max
	for y := 0; y < max.Y; y++ {
		text += "\n"
		for x := 0; x < max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a == 0 {
				text += "_"
			} else {
				text += "X"
			}
		}
	}
	return text
}

func TestPolygonToAbs(t *testing.T) {
	polygon := Polygon{
		Point{5, 10},
		Point{15, 20},
		Point{25, 30},
	}
	actual := fmt.Sprintf("%v", polygon.ToAbs(400, 200))
	require.Equal(t, actual, "[[20 20] [60 40] [100 60]]")
}

func TestCreateMask(t *testing.T) {
	cases := []struct {
		name     string
		input    Polygon
		expected string
	}{
		{
			"triangle",
			Polygon{
				{3, 1},
				{6, 6},
				{0, 6},
			},
			`
			_______
			_______
			___X___
			__XXX__
			__XXX__
			_XXXXX_
			_______`,
		},
		{
			"octagon",
			Polygon{
				{2, 0},
				{5, 0},
				{7, 3},
				{7, 4},
				{4, 7},
				{0, 4},
				{0, 2},
			},
			`
			__XXX__
			_XXXXX_
			XXXXXXX
			XXXXXXX
			XXXXXXX
			_XXXXX_
			__XXX__`,
		},
		{
			"inverted", // Lines cross over themselves at the bottom.
			Polygon{
				{7, 0},
				{7, 7},
				{1, 5},
				{6, 5},
				{0, 7},
				{0, 0},
			},
			`
			XXXXXXX
			XXXXXXX
			XXXXXXX
			XXXXXXX
			XXXXXXX
			X_____X
			XXX_XXX`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mask := CreateMask(7, 7, tc.input)

			actual := imageToText(mask)
			expected := strings.ReplaceAll(tc.expected, "\t", "")
			require.Equal(t, actual, expected)
		})
	}
}

func TestCreateInvertedMask(t *testing.T) {
	cases := []struct {
		name     string
		input    Polygon
		expected string
	}{
		{
			"triangle",
			Polygon{
				{3, 1},
				{6, 6},
				{0, 6},
			},
			`
			XXXXXXX
			XXXXXXX
			XXX_XXX
			XX___XX
			XX___XX
			X_____X
			XXXXXXX`,
		},
		{
			"octagon",
			Polygon{
				{2, 0},
				{5, 0},
				{7, 3},
				{7, 4},
				{4, 7},
				{0, 4},
				{0, 2},
			},
			`
			XX___XX
			X_____X
			_______
			_______
			_______
			X_____X
			XX___XX`,
		},
		{
			"inverted", // Lines cross over themselves at the bottom.
			Polygon{
				{7, 0},
				{7, 7},
				{1, 5},
				{6, 5},
				{0, 7},
				{0, 0},
			},
			`
			_______
			_______
			_______
			_______
			_______
			_XXXXX_
			___X___`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mask := CreateInvertedMask(7, 7, tc.input)

			actual := imageToText(mask)
			expected := strings.ReplaceAll(tc.expected, "\t", "")
			require.Equal(t, actual, expected)
		})
	}
}

func TestSaveImage(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		imgPath := tempDir + "/img.png"
		img := image.NewAlpha(image.Rect(0, 0, 1, 1))

		err = SaveImage(imgPath, img)
		require.NoError(t, err)

		_, err = os.Stat(imgPath)
		require.NoError(t, err)
	})
	t.Run("createErr", func(t *testing.T) {
		img := image.NewAlpha(image.Rect(0, 0, 1, 1))
		err := SaveImage("", img)
		require.Error(t, err)
	})
	t.Run("encodeErr", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		imgPath := tempDir + "/img.png"

		file, err := os.Create(imgPath)
		require.NoError(t, err)
		defer file.Close()

		img := image.NewAlpha(image.Rect(0, 0, 1, 1))
		img.Rect = image.Rectangle{}

		err = SaveImage(imgPath, img)
		require.Error(t, err)
	})
}

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected []string
	}{
		{"1", "1 2 3 4", []string{"1", "2", "3", "4"}},
		//{"2", "1 '2 3' 4", []string{"1", "2 3", "4"}}, Not implemented.
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := ParseArgs(tc.input)
			require.Equal(t, actual, tc.expected)
		})
	}
}

func TestParseScaleString(t *testing.T) {
	cases := []struct{ input, expected string }{
		{"", "1"},
		{"full", "1"},
		{"half", "2"},
		{"third", "3"},
		{"quarter", "4"},
		{"sixth", "6"},
		{"eighth", "8"},
	}
	for _, tc := range cases {
		actual := ParseScaleString(tc.input)
		require.Equal(t, actual, tc.expected)
	}
}

func TestFeedRateToDuration(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{"one", "1", "1s"},
		{"two", "2", "500ms"},
		{"half", "0.5", "2s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := FeedRateToDuration(tc.input)
			require.NoError(t, err)

			actual := fmt.Sprintf("%v", output)
			require.Equal(t, actual, tc.expected)
		})
	}
	t.Run("parseFloatErr", func(t *testing.T) {
		_, err := FeedRateToDuration("nil")
		require.Error(t, err)
	})
}
