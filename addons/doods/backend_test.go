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
	"io/ioutil"
	"nvr/addons/doods/odrpc"
	"nvr/pkg/ffmpeg/ffmock"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"os"
	"sync"
	"testing"
	"time"
)

func TestGenArgs(t *testing.T) {
	m := &monitor.Monitor{
		Env: &storage.ConfigEnv{
			SHMDir: "a",
		},
		Config: monitor.Config{
			"id":          "b",
			"doodsEnable": "true",
		},
	}
	args := genArgs(m)

	expected := " -c:v copy -map 0:v -f fifo -fifo_format mpegts" +
		" -drop_pkts_on_overflow 1 -attempt_recovery 1" +
		" -restart_with_keyframe 1 -recovery_wait_time 1 a/doods/b/main.fifo"

	if args != expected {
		t.Fatalf("\nexpected:\n%v.\ngot:\n%v.", expected, args)
	}
}

func TestParseConfig(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		m := &monitor.Monitor{
			Config: monitor.Config{
				"sizeMain":        "4x6",
				"timestampOffset": "6",
				"doodsThresholds": `{"4":5}`,
				"doodsDuration":   "0.000000003",
				"doodsFrameScale": "half",
				"doodsFeedRate":   "500000000",
			},
		}
		config, err := parseConfig(m, "1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		actual := fmt.Sprintf("%v", config)
		expected := "&{1 2 3 map[4:5] 6000000 }"

		if actual != expected {
			t.Fatalf("\nexpected:\n%v.\ngot:\n%v.\n", expected, actual)
		}
	})
	t.Run("threshErr", func(t *testing.T) {
		m := &monitor.Monitor{
			Config: monitor.Config{
				"sizeMain":        "1x1",
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
				"sizeMain":        "1x1",
				"timestampOffset": "0",
				"doodsDuration":   "1",
				"doodsThresholds": `{"a":1,"b":2,"c":-1}`,
				"doodsFeedRate":   "1",
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
	t.Run("durationErr", func(t *testing.T) {
		m := &monitor.Monitor{
			Config: monitor.Config{
				"sizeMain":        "1x1",
				"doodsThresholds": "{}",
				"doodsFeedRate":   "nil",
			},
		}
		if _, err := parseConfig(m, ""); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("recDurationErr", func(t *testing.T) {
		m := &monitor.Monitor{
			Config: monitor.Config{
				"sizeMain":        "1x1",
				"doodsThresholds": "{}",
				"doodsDuration":   "nil",
				"doodsFeedRate":   "1",
			},
		}
		if _, err := parseConfig(m, ""); err == nil {
			t.Fatal("expected: error, got: nil")
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
		if _, err := parseConfig(m, ""); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestPrepareEnv(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
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

func TestGenerateArgs(t *testing.T) {
	t.Run("working1", func(t *testing.T) {
		a := addon{
			id: "3",
			c:  &doodsConfig{},
			env: &storage.ConfigEnv{
				SHMDir: "2",
			},
			outputWidth:  300,
			outputHeight: 300,
		}
		config := monitor.Config{
			"logLevel":      "1",
			"doodsFeedRate": "4",
		}
		args, xMultiplier, yMultiplier, err := a.generateFFmpegArgs(config, "600x400")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := fmt.Sprintf("%v %v %v", args, xMultiplier, yMultiplier)
		expected := "[-y -loglevel 1 -i 2/doods/3/main.fifo -filter fps=fps=4," +
			"scale=300:200,pad=300:300:0:0 -f rawvideo -pix_fmt rgb24 -] 1 1.5"

		if actual != expected {
			t.Fatalf("\nexpected:\n%v.\ngot:\n%v.", expected, actual)
		}
	})
	t.Run("working2", func(t *testing.T) {
		a := addon{
			id: "4",
			c:  &doodsConfig{},
			env: &storage.ConfigEnv{
				SHMDir: "3",
			},
			outputWidth:  300,
			outputHeight: 300,
		}
		config := monitor.Config{
			"logLevel":      "1",
			"hwaccel":       "2",
			"doodsFeedRate": "5",
		}
		args, xMultiplier, yMultiplier, err := a.generateFFmpegArgs(config, "400x600")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := fmt.Sprintf("%v %v %v", args, xMultiplier, yMultiplier)
		expected := "[-y -loglevel 1 -hwaccel 2 -i 3/doods/4/main.fifo -filter fps=fps=5," +
			"scale=200:300,pad=300:300:0:0 -f rawvideo -pix_fmt rgb24 -] 1.5 1"

		if actual != expected {
			t.Fatalf("\nexpected:\n%v.\ngot:\n%v.", expected, actual)
		}
	})
	t.Run("widthErr", func(t *testing.T) {
		a := addon{}
		if _, _, _, err := a.generateFFmpegArgs(monitor.Config{}, "nilx1"); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("heightErr", func(t *testing.T) {
		a := addon{}
		if _, _, _, err := a.generateFFmpegArgs(monitor.Config{}, "1xnil"); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("widthErr2", func(t *testing.T) {
		a := addon{
			outputWidth: 2,
		}
		if _, _, _, err := a.generateFFmpegArgs(monitor.Config{}, "1x1"); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("heightErr2", func(t *testing.T) {
		a := addon{
			outputWidth:  2,
			outputHeight: 2,
		}
		if _, _, _, err := a.generateFFmpegArgs(monitor.Config{}, "2x1"); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func newTestFFmpeg() (*ffmpegConfig, log.Feed, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	logger := log.NewLogger()
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
		expected := ": doods: process stopped\n"

		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
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
		expected := ": doods: process crashed: mock\n"

		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
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

	logger := log.NewLogger()
	go logger.Start(ctx)
	feed, cancel2 := logger.Subscribe()

	cancelFunc := func() {
		cancel2()
		cancel()
	}

	d := &doodsClient{
		a: &addon{
			outputWidth:  2,
			outputHeight: 2,
			xMultiplier:  1.1,
			yMultiplier:  0.9,

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
		expected := ": doods: client stopped\n"

		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
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
		expected := ": doods: client crashed: mock\n"

		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
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

		expected := "{0001-01-01 00:00:00 +0000 UTC [{b 5 &[9 11 90 110], <nil>}] 0s 0s}"

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
