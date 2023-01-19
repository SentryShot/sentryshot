// SPDX-License-Identifier: GPL-2.0-or-later

package ffmpeg

import (
	"context"
	"fmt"
	"image"
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
			defer cancel()

			logs := make(chan string)
			logFunc := func(msg string) {
				logs <- fmt.Sprintf("test %v", msg)
			}

			p := NewProcess(fakeExecCommand()).
				Timeout(0).
				StdoutLogger(logFunc).
				StderrLogger(logFunc)

			err := p.Start(ctx)
			require.NoError(t, err)

			compareOutput := func(input string) {
				output1 := "test stdout: out"
				output2 := "test stderr: err"
				switch {
				case input == output1:
				case input == output2:
				default:
					t.Fatalf("outputs doesn't match: '%v'", input)
				}
			}

			compareOutput(<-logs)
			compareOutput(<-logs)
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

func TestShellProcessNoOutput(t *testing.T) {}

func fakeExecCommandNoOutput(...string) *exec.Cmd {
	cs := []string{"-test.run=TestShellProcessNoOutput"}
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_TEST_PROCESS=1"}
	return cmd
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
	require.Equal(t, "[[20 20] [60 40] [100 60]]", actual)
}

func TestCreateMask(t *testing.T) {
	cases := map[string]struct {
		input    Polygon
		expected string
	}{
		"triangle": {
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
		"octagon": {
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
		"inverted": {
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
			XXX_XXX`, // Lines cross over themselves at the bottom.
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mask := CreateMask(7, 7, tc.input)

			actual := imageToText(mask)
			expected := strings.ReplaceAll(tc.expected, "\t", "")
			require.Equal(t, actual, expected)
		})
	}
}

func TestCreateInvertedMask(t *testing.T) {
	cases := map[string]struct {
		input    Polygon
		expected string
	}{
		"triangle": {
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
		"octagon": {
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
		"inverted": {
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
			___X___`, // Lines cross over themselves at the bottom.
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
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
	cases := map[string]struct {
		input    string
		expected []string
	}{
		"simple": {"1 2 3 4", []string{"1", "2", "3", "4"}},
		//"x":{ "1 '2 3' 4", []string{"1", "2 3", "4"}}, Not implemented.
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			actual := ParseArgs(tc.input)
			require.Equal(t, actual, tc.expected)
		})
	}
}

func TestParseScaleString(t *testing.T) {
	cases := []struct{ input, expected string }{
		{"", ""},
		{"x", ""},
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
	}
}

func TestParseTimestampOffset(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		actual, err := ParseTimestampOffset("500")
		require.NoError(t, err)
		require.Equal(t, 500*time.Millisecond, actual)
	})
	t.Run("empty", func(t *testing.T) {
		actual, err := ParseTimestampOffset("")
		require.NoError(t, err)
		require.Equal(t, time.Duration(0), actual)
	})
	t.Run("error", func(t *testing.T) {
		_, err := ParseTimestampOffset("x")
		require.Error(t, err)
	})
}
