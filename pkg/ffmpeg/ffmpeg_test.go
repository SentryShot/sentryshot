// Copyright 2020-2021 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; version 2.
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
	"io/ioutil"
	"nvr/pkg/log"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("startWithLogger", func(t *testing.T) {
		t.Run("working", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			logger := log.NewLogger(ctx)
			feed, cancel2 := logger.Subscribe()

			p := NewProcess(fakeExecCommand())
			p.SetTimeout(0)
			p.SetPrefix("test ")
			p.SetStdoutLogger(logger)
			p.SetStderrLogger(logger)

			if err := p.Start(ctx); err != nil {
				t.Fatalf("failed to start %v", err)
			}

			compareOutput := func(input string) {
				output1 := "test stdout: out\n"
				output2 := "test stderr: err\n"
				switch {
				case input == output1:
				case input == output2:
				default:
					t.Fatal("output does not match")
				}
			}

			compareOutput(<-feed)
			compareOutput(<-feed)
			cancel2()

			cancel()
		})
	})
	_, pw, err := os.Pipe()
	if err != nil {
		t.Fatal("could not create pipe")
	}

	t.Run("stdoutErr", func(t *testing.T) {
		p := process{cmd: fakeExecCommand()}
		p.cmd.Stdout = pw
		p.SetStdoutLogger(&log.Logger{})

		if err := p.Start(context.Background()); err == nil {
			t.Fatalf("nil")
		}
	})
	t.Run("stderrErr", func(t *testing.T) {
		p := process{cmd: fakeExecCommand()}
		p.cmd.Stderr = pw
		p.SetStderrLogger(&log.Logger{})

		if err := p.Start(context.Background()); err == nil {
			t.Fatalf("nil")
		}
	})

}
func TestMakePipe(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		pipePath := tempDir + "/pipe.fifo"
		if err := MakePipe(pipePath); err != nil {
			t.Fatalf("could not create pipe: %v", err)
		}

		if _, err := os.Stat(pipePath); os.IsNotExist(err) {
			t.Fatal("pipe were not created")
		}
	})
	t.Run("MkfifoErr", func(t *testing.T) {
		if err := MakePipe(""); err == nil {
			t.Fatal("nil")
		}
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
	t.Run("working", func(t *testing.T) {
		f := New("")
		f.command = fakeExecCommandSize

		actual, err := f.SizeFromStream("")
		if err != nil {
			t.Fatalf("could not get stream size %v", err)
		}

		expected := "720x1280"
		if expected != actual {
			t.Fatalf("expected: %v, got: %v", expected, actual)
		}
	})
	t.Run("runErr", func(t *testing.T) {
		f := New("")
		if _, err := f.SizeFromStream(""); err == nil {
			t.Fatal("nil")
		}
	})
	t.Run("regexErr", func(t *testing.T) {
		f := New("")
		f.command = fakeExecCommandNoOutput

		if _, err := f.SizeFromStream(""); err == nil {
			t.Fatal("nil")
		}
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

func TestCreateMask(t *testing.T) {
	cases := []struct {
		name     string
		input    [][2]int
		expected string
	}{
		{
			"triangle",
			[][2]int{
				{3, 1},
				{6, 6},
				{0, 6},
			}, `
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
			[][2]int{
				{2, 0},
				{5, 0},
				{7, 3},
				{7, 4},
				{4, 7},
				{0, 4},
				{0, 2},
			}, `
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
			[][2]int{
				{7, 0},
				{7, 7},
				{1, 5},
				{6, 5},
				{0, 7},
				{0, 0},
			}, `
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
			mask := CreateMask(7, 7, tc.input)
			actual := imageToText(mask)
			expected := strings.ReplaceAll(tc.expected, "\t", "")

			if expected != actual {
				t.Fatalf("\nexpected:\n%v\ngot:\n%v", expected, actual)
			}
		})

	}
}

func TestSaveImage(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		imgPath := tempDir + "/img.png"
		img := image.NewAlpha(image.Rect(0, 0, 1, 1))

		if err := SaveImage(imgPath, img); err != nil {
			t.Fatalf("could not save image: %v", err)
		}

		if _, err := os.Stat(imgPath); os.IsNotExist(err) {
			t.Fatal("image were not created")
		}
	})
	t.Run("createErr", func(t *testing.T) {
		img := image.NewAlpha(image.Rect(0, 0, 1, 1))
		if err := SaveImage("", img); err == nil {
			t.Fatal("nil")
		}
	})
	t.Run("encodeErr", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}

		imgPath := tempDir + "/img.png"
		file, err := os.Create(imgPath)
		if err != nil {
			t.Fatalf("could not create image: %v", err)
		}
		defer file.Close()

		img := image.NewAlpha(image.Rect(0, 0, 1, 1))
		img.Rect = image.Rectangle{}
		if err := SaveImage(imgPath, img); err == nil {
			t.Fatal("nil")
		}
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

			if !reflect.DeepEqual(actual, tc.expected) {
				t.Fatalf("expected: %v, got: %v", tc.expected, actual)
			}
		})
	}

}

func TestWaitForKeyframe(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		expected  time.Duration
		expectErr bool
	}{
		{"empty", "", 0, true},
		{
			"working",
			`
INPUT
#EXTM3U
#EXT-X-VERSION:3
#EXT-X-ALLOW-CACHE:NO
#EXT-X-TARGETDURATION:2
#EXT-X-MEDIA-SEQUENCE:251
#EXTINF:4.250000,
10.ts
#EXTINF:3.500000,
11.ts
`,
			3500 * time.Millisecond,
			false,
		},
		{
			"invalidDuration",
			`
#EXTINF:3.5#0000,
11.ts
`,
			0,
			true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatalf("could not create tempoary directory: %v", err)
			}
			defer os.RemoveAll(tempDir)

			path := tempDir + "/test.m3u8"
			if err := ioutil.WriteFile(path, []byte(tc.input), 0600); err != nil {
				t.Fatalf("could not write file: %v", err)
			}

			go func() {
				time.Sleep(10 * time.Millisecond)
				// Simulate new keyframe
				os.Chmod(path, 0601)
			}()

			actual, err := WaitForKeyframe(context.Background(), path)
			gotError := err != nil
			if tc.expectErr != gotError {
				t.Fatalf("\nexpected error %v\n     got %v", tc.expectErr, err)
			}

			if actual != tc.expected {
				t.Fatalf("expected: %v, got: %v", tc.expected, actual)
			}
		})

	}
	t.Run("addErr", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		_, err = WaitForKeyframe(context.Background(), tempDir+"/nil")
		if err == nil {
			t.Fatal("expected error got: nil")
		}
	})
	t.Run("canceled", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		path := tempDir + "/test.m3u8"
		if err := ioutil.WriteFile(path, []byte{}, 0600); err != nil {
			t.Fatalf("could not write file: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = WaitForKeyframe(ctx, path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
