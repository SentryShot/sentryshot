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

package doods

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"nvr/addons/doods/odrpc"
	"nvr/pkg/ffmpeg/ffmock"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestGenArgs(t *testing.T) {
	args := []string{"x"}
	m := &monitor.Monitor{
		Env: &storage.ConfigEnv{
			SHMDir: "a",
		},
		Config: monitor.Config{
			"id": "b",
		},
	}

	modifyArgs(&args, m)

	expected := []string{
		"x", "-c:v", "copy", "-map", "0:v", "-f", "fifo", "-fifo_format", "mpegts",
		"-drop_pkts_on_overflow", "1", "-attempt_recovery", "1",
		"-restart_with_keyframe", "1", "-recovery_wait_time", "1", "a/doods/b/main.fifo",
	}

	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("\nexpected:\n%v.\ngot:\n%v.", expected, args)
	}
}

func TestParseConfig(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		m := &monitor.Monitor{
			Config: monitor.Config{
				"timestampOffset": "6",
				"doodsThresholds": `{"4":5}`,
				"doodsDuration":   "0.000000003",
				"doodsFrameScale": "half",
				"doodsFeedRate":   "500000000",
				"doodsDelay":      "7",
			},
		}
		config, err := parseConfig(m, "1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		actual := fmt.Sprintf("%v", config)
		expected := "&{1 2 3 map[4:5] 6000000 7000000 }"

		if actual != expected {
			t.Fatalf("\nexpected:\n%v.\ngot:\n%v.\n", expected, actual)
		}
	})
	t.Run("threshErr", func(t *testing.T) {
		m := &monitor.Monitor{
			Config: monitor.Config{
				"doodsThresholds": "nil",
			},
		}
		if _, err := parseConfig(m, ""); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("cleanThresh", func(t *testing.T) {
		m := &monitor.Monitor{
			Config: monitor.Config{
				"timestampOffset": "0",
				"doodsDuration":   "1",
				"doodsThresholds": `{"a":1,"b":2,"c":-1}`,
				"doodsFeedRate":   "1",
				"doodsDelay":      "1",
			},
		}
		config, err := parseConfig(m, "1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		actual := fmt.Sprintf("%v", config.thresholds)
		expected := "map[a:1 b:2]"

		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
		}
	})
	cases := []struct {
		name        string
		input       monitor.Config
		expectedErr error
	}{
		{
			"durationErr",
			monitor.Config{
				"doodsThresholds": "{}",
				"doodsFeedRate":   "nil",
			},
			strconv.ErrSyntax,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &monitor.Monitor{Config: tc.input}
			_, err := parseConfig(m, "")
			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("expected: %v, got: %v", tc.expectedErr, err)
			}
		})
	}
	t.Run("durationErr", func(t *testing.T) {
		m := &monitor.Monitor{
			Config: monitor.Config{
				"doodsThresholds": "{}",
				"doodsFeedRate":   "nil",
			},
		}
		_, err := parseConfig(m, "")
		if !errors.Is(err, strconv.ErrSyntax) {
			t.Fatalf("expected: %v, got: %v", strconv.ErrSyntax, err)
		}
	})
	t.Run("recDurationErr", func(t *testing.T) {
		m := &monitor.Monitor{
			Config: monitor.Config{
				"doodsThresholds": "{}",
				"doodsDuration":   "nil",
				"doodsFeedRate":   "1",
			},
		}
		_, err := parseConfig(m, "")
		if !errors.Is(err, strconv.ErrSyntax) {
			t.Fatalf("expected: %v, got: %v", strconv.ErrSyntax, err)
		}
	})
	t.Run("timestampOffsetErr", func(t *testing.T) {
		m := &monitor.Monitor{
			Config: monitor.Config{
				"size":            "1x1",
				"doodsThresholds": "{}",
				"doodsDuration":   "1",
				"doodsFeedRate":   "1",
			},
		}
		_, err := parseConfig(m, "")
		if !errors.Is(err, strconv.ErrSyntax) {
			t.Fatalf("expected: %v, got: %v", strconv.ErrSyntax, err)
		}
	})
}

func TestPrepareEnv(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "")
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		a := addon{
			id: "id",
			env: &storage.ConfigEnv{
				SHMDir: tempDir + "/shm",
			},
		}
		if err := a.prepareEnvironment(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !dirExist(a.fifoDir()) {
			t.Fatal("fifoDir wasn't created")
		}
		if !dirExist(a.mainPipe()) {
			t.Fatal("main pipe wasn't created")
		}
	})
	t.Run("mkdirErr", func(t *testing.T) {
		a := addon{
			env: &storage.ConfigEnv{
				SHMDir: "/dev/null",
			},
		}
		if err := a.prepareEnvironment(); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestParseInputs(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		inputs, err := parseInputs("1x2", "[3,4,5]", 6, 7, "gray_x")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		actual := fmt.Sprintf("%v", inputs)
		expected := "&{1 2 3 4 5 6 7 true}"
		if actual != expected {
			t.Fatalf("expected:\n%v.\ngot:\n%v.", expected, actual)
		}
	})
	t.Run("gray", func(t *testing.T) {
		inputs, err := parseInputs("1x2", "[3,4,5]", 6, 7, "gray-x")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		actual := fmt.Sprintf("%v", inputs)
		expected := "&{1 2 3 4 5 6 7 false}"
		if actual != expected {
			t.Fatalf("expected:\n%v.\ngot:\n%v.", expected, actual)
		}
	})
	t.Run("widthErr", func(t *testing.T) {
		if _, err := parseInputs("nilx1", "", 0, 0, ""); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("heightErr", func(t *testing.T) {
		if _, err := parseInputs("1xnil", "", 0, 0, ""); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("cropErr", func(t *testing.T) {
		if _, err := parseInputs("1x1", `[1,2,"x"]`, 0, 0, ""); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestCaclulateOutputs(t *testing.T) {
	t.Run("frameSizes", func(t *testing.T) {
		cases := []struct {
			inputWidth   float64
			inputHeight  float64
			cropX        float64
			cropY        float64
			cropSize     float64
			outputWidth  float64
			outputHeight float64
			expected     string
		}{
			{600, 400, 0, 0, 100, 300, 300, "300x200 300x300 0:0 1:1.5 0.5:0.5"},
			{400, 600, 0, 0, 100, 300, 300, "200x300 300x300 0:0 1.5:1 0.5:0.5"},
			{640, 480, 0, 0, 100, 420, 280, "373x280 420x280 0:0 1.125:1 0.5:0.5"},
			{480, 640, 0, 0, 100, 280, 420, "280x373 280x420 0:0 1:1.125 0.5:0.5"},
			{100, 100, 5, 5, 90, 90, 90, "100x100 100x100 5:5 1:1 0.5:0.5"},
			{100, 200, 5, 5, 90, 90, 90, "50x100 100x100 5:5 2:1 0.5:0.5"},
			{200, 100, 5, 5, 90, 90, 90, "100x50 100x100 5:5 1:2 0.5:0.5"},
			{200, 100, 0, 0, 90, 90, 90, "100x50 100x100 0:0 1:2 0.45:0.45"},
			{200, 100, 0, 20, 80, 80, 80, "100x50 100x100 0:20 1:2 0.4:0.6"},
			{854, 480, 20, 10, 60, 300, 300, "500x281 500x500 100:50 1:1.7791667 0.5:0.4"},
		}

		for i, tc := range cases {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				outputs, err := calculateOutputs(&inputs{
					inputWidth:   tc.inputWidth,
					inputHeight:  tc.inputHeight,
					cropX:        tc.cropX,
					cropY:        tc.cropY,
					cropSize:     tc.cropSize,
					outputWidth:  tc.outputWidth,
					outputHeight: tc.outputHeight,
				})
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				actual := fmt.Sprintf("%vx%v %vx%v %v:%v %v:%v %v:%v",
					outputs.scaledWidth, outputs.scaledHeight,
					outputs.paddedWidth, outputs.paddedHeight,
					outputs.cropX, outputs.cropY,
					outputs.paddingXmultiplier, outputs.paddingYmultiplier,
					outputs.uncropXfunc(0.5), outputs.uncropYfunc(0.5),
				)

				if actual != tc.expected {
					t.Fatalf("expected:\n%v.\ngot:\n%v.", tc.expected, actual)
				}
			})
		}
	})
	t.Run("widthErr", func(t *testing.T) {
		if _, err := calculateOutputs(&inputs{
			inputWidth:   1,
			inputHeight:  2,
			outputWidth:  2,
			outputHeight: 1,
		}); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("heightErr", func(t *testing.T) {
		if _, err := calculateOutputs(&inputs{
			inputWidth:   2,
			inputHeight:  1,
			outputWidth:  1,
			outputHeight: 2,
		}); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("scaledWidthErr", func(t *testing.T) {
		if _, err := calculateOutputs(&inputs{
			inputWidth:   100,
			inputHeight:  100,
			outputWidth:  80,
			outputHeight: 80,
			cropSize:     70,
		}); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestGenerateMask(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "")
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		if err := os.Mkdir(filepath.Join(tempDir, "doods"), 0o700); err != nil {
			t.Fatalf("could not create doods directory %v", err)
		}

		a := &addon{
			env: &storage.ConfigEnv{
				SHMDir: tempDir,
			},
			outputs: &outputs{
				scaledWidth:  1,
				scaledHeight: 1,
			},
		}
		_, err = a.generateMask(`{"enable":true}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		a := &addon{}
		if _, err := a.generateMask(""); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("disabled", func(t *testing.T) {
		a := &addon{}
		path, err := a.generateMask(`{"enable":false}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "" {
			t.Fatalf("expected '': got: %v", path)
		}
	})
	t.Run("saveErr", func(t *testing.T) {
		a := &addon{
			env: &storage.ConfigEnv{
				SHMDir: "/dev/null",
			},
			outputs: &outputs{},
		}
		_, err := a.generateMask(`{"enable":true}`)
		if err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestGenerateArgs(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		a := addon{
			id: "3",
			c:  &doodsConfig{},
			env: &storage.ConfigEnv{
				SHMDir: "2",
			},
			outputs: &outputs{
				scaledWidth:  5,
				scaledHeight: 6,
				paddedWidth:  7,
				paddedHeight: 8,
				outputWidth:  11,
				outputHeight: 12,
				cropX:        "13",
				cropY:        "14",
			},
		}
		config := monitor.Config{
			"logLevel":      "1",
			"doodsFeedRate": "4",
		}
		args := a.generateFFmpegArgs(config, "", false)

		actual := fmt.Sprintf("%v", args)
		expected := "[-y -loglevel 1 -i 2/doods/3/main.fifo -filter fps=fps=4," +
			"scale=5:6,pad=8:9:0:0,crop=11:12:13:14 -f rawvideo -pix_fmt rgb24 -]"

		if actual != expected {
			t.Fatalf("\nexpected:\n%v.\ngot:\n%v.", expected, actual)
		}
	})
	t.Run("maximal", func(t *testing.T) {
		a := addon{
			id: "4",
			c:  &doodsConfig{},
			env: &storage.ConfigEnv{
				SHMDir: "3",
			},
			outputs: &outputs{
				scaledWidth:  7,
				scaledHeight: 8,
				paddedWidth:  9,
				paddedHeight: 10,
				outputWidth:  13,
				outputHeight: 14,
				cropX:        "15",
				cropY:        "16",
			},
		}
		config := monitor.Config{
			"logLevel":      "1",
			"doodsFeedRate": "6",
			"hwaccel":       "2",
		}
		args := a.generateFFmpegArgs(config, "5", true)

		actual := fmt.Sprintf("%v", args)
		expected := "[-y -loglevel 1 -hwaccel 2 -i 3/doods/4/main.fifo -i 5" +
			" -filter_complex [0:v]fps=fps=6,scale=7:8[bg];" +
			"[bg][1:v]overlay,pad=10:11:0:0,crop=13:14:15:16,hue=s=0" +
			" -f rawvideo -pix_fmt rgb24 -]"

		if actual != expected {
			t.Fatalf("\nexpected:\n%v.\ngot:\n%v.", expected, actual)
		}
	})
}

func newTestFFmpeg() (*ffmpegConfig, log.Feed, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	logger := log.NewMockLogger()
	go logger.Start(ctx)
	feed, cancel2 := logger.Subscribe()

	cancelFunc := func() {
		cancel2()
		cancel()
	}

	f := &ffmpegConfig{
		a: &addon{
			log: logger,
			wg:  &sync.WaitGroup{},
			env: &storage.ConfigEnv{},
		},
		d:           &doodsClient{},
		runFFmpeg:   mockRunFFmpeg,
		newProcess:  ffmock.NewProcess,
		startClient: mockStartClient,
	}

	return f, feed, cancelFunc
}

func mockRunFFmpeg(context.Context, *ffmpegConfig) error    { return nil }
func mockRunFFmpegErr(context.Context, *ffmpegConfig) error { return errors.New("mock") }
func mockStartClient(context.Context, *doodsClient)         {}

func TestStartFFmpeg(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		f, feed, cancel := newTestFFmpeg()
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		cancel2()

		f.a.wg.Add(1)
		go f.start(ctx)

		actual := <-feed
		expected := "process stopped"

		if actual.Msg != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual.Msg)
		}
	})
	t.Run("crashed", func(t *testing.T) {
		f, feed, cancel := newTestFFmpeg()
		defer cancel()

		f.runFFmpeg = mockRunFFmpegErr

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		f.a.wg.Add(1)
		go f.start(ctx)

		actual := <-feed
		expected := "process crashed: mock"

		if actual.Msg != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual.Msg)
		}
	})
}

func TestRunFFmpeg(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		f, _, cancel := newTestFFmpeg()
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		if err := runFFmpeg(ctx, f); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("crashed", func(t *testing.T) {
		f, _, cancel := newTestFFmpeg()
		defer cancel()

		f.newProcess = ffmock.NewProcessErr

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		if err := runFFmpeg(ctx, f); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("startClientCalled", func(t *testing.T) {
		mu := sync.Mutex{}
		called := false
		mockStartClient := func(context.Context, *doodsClient) {
			mu.Lock()
			called = true
			mu.Unlock()
		}

		f, _, cancel := newTestFFmpeg()
		defer cancel()

		f.startClient = mockStartClient

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		if err := runFFmpeg(ctx, f); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		time.Sleep(10 * time.Millisecond)

		defer mu.Unlock()
		mu.Lock()
		if !called {
			t.Fatal("startClient wasn't called")
		}
	})
}

var frames = []byte{
	255, 0, 0, 0, 255, 0, 0, 0, 255, 128, 128, 128,
	255, 0, 0, 0, 255, 0, 0, 0, 255, 128, 128, 128,
}

var imgFeed = func() *bytes.Reader {
	return bytes.NewReader(frames)
}

var framePNG = "[137 80 78 71 13 10 26 10 0 0 0 13 73 72 68 82 0 0 0 2 0 0 0 2 16 2 0 0 0 173 68 70 48 0 0 0 42 73 68 65 84 120 1 0 26 0 229 255 0 255 255 0 0 0 0 0 0 255 255 0 0 0 0 0 0 0 255 255 128 128 128 128 128 128 1 0 0 255 255 107 57 8 251 44 117 64 132 0 0 0 0 73 69 78 68 174 66 96 130]"

func newTestClient() (*doodsClient, log.Feed, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	logger := log.NewMockLogger()
	go logger.Start(ctx)
	feed, cancel2 := logger.Subscribe()

	cancelFunc := func() {
		cancel2()
		cancel()
	}

	d := &doodsClient{
		a: &addon{
			outputs: &outputs{
				outputWidth:        2,
				outputHeight:       2,
				paddingXmultiplier: 1.1,
				paddingYmultiplier: 0.9,
				uncropXfunc:        func(i float32) float32 { return i },
				uncropYfunc:        func(i float32) float32 { return i },
			},

			log:     logger,
			wg:      &sync.WaitGroup{},
			trigger: make(monitor.Trigger),
		},
		c:         &doodsConfig{},
		stdout:    imgFeed(),
		runClient: mockRunClient,
		encoder: png.Encoder{
			CompressionLevel: png.NoCompression,
		},
	}

	return d, feed, cancelFunc
}

func mockRunClient(context.Context, *doodsClient) error    { return nil }
func mockRunClientErr(context.Context, *doodsClient) error { return errors.New("mock") }

func TestStartClient(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		d, feed, cancel := newTestClient()
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		cancel2()

		d.a.wg.Add(1)
		go startClient(ctx, d)

		actual := <-feed
		expected := "client stopped"

		if actual.Msg != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual.Msg)
		}
	})
	t.Run("crashed", func(t *testing.T) {
		d, feed, cancel := newTestClient()
		defer cancel()

		d.runClient = mockRunClientErr

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		go startClient(ctx, d)

		actual := <-feed
		expected := "client crashed: mock"

		if actual.Msg != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual.Msg)
		}
	})
}

func TestReadFrames(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		mu := sync.Mutex{}

		var firstFrame string
		var secondFrame string
		mockSendFrame := func(d *doodsClient, t time.Time, b *bytes.Buffer) error {
			if firstFrame == "" {
				firstFrame = fmt.Sprintf("%v", b.Bytes())
			} else {
				secondFrame = fmt.Sprintf("%v", b.Bytes())
				mu.Unlock()
			}
			return nil
		}

		d, _, cancel := newTestClient()
		defer cancel()

		d.sendFrame = mockSendFrame

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		go d.readFrames(ctx)
		mu.Lock()

		defer mu.Unlock()
		mu.Lock()

		if firstFrame != secondFrame {
			t.Fatalf("frames dosen't match:\nfirst:\n%v.\nsecond:\n%v", firstFrame, secondFrame)
		}

		if firstFrame != framePNG {
			t.Fatalf("expected: %v, got: %v", framePNG, firstFrame)
		}
	})
	t.Run("canceled", func(t *testing.T) {
		d, _, cancel := newTestClient()
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		cancel2()

		if err := d.readFrames(ctx); err != nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("sendFrameErr", func(t *testing.T) {
		mockSendFrameErr := func(*doodsClient, time.Time, *bytes.Buffer) error {
			return errors.New("mock")
		}

		d, _, cancel := newTestClient()
		defer cancel()

		d.sendFrame = mockSendFrameErr

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		if err := d.readFrames(ctx); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestParseDetections(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		d, _, cancel := newTestClient()
		defer cancel()

		d.a.c = d.c
		d.a.c.thresholds = thresholds{
			"b": 1,
		}

		detections := []*odrpc.Detection{
			{
				Label: "a",
			},
			{
				Top:        0.1,
				Left:       0.1,
				Bottom:     1,
				Right:      1,
				Label:      "b",
				Confidence: 5,
			},
		}

		go d.a.parseDetections(time.Time{}, detections)
		output := <-d.a.trigger

		actual := fmt.Sprintf("%v", output)

		expected := `
 Time: 0001-01-01 00:00:00 +0000 UTC
 Detections: [{b 5 &[9 11 90 110], <nil>}]
 Duration: 0s
 RecDuration: 0s`

		if actual != expected {
			t.Fatalf("\nexpected:\n%v.\ngot:\n%v.", expected, actual)
		}
	})
	t.Run("noDetections", func(t *testing.T) {
		d, _, cancel := newTestClient()
		defer cancel()

		d.a.parseDetections(time.Time{}, []*odrpc.Detection{})
	})
}
