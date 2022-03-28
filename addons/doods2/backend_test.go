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

package doods

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"nvr/pkg/ffmpeg"
	"nvr/pkg/ffmpeg/ffmock"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseConfig(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		c := monitor.Config{
			"timestampOffset": "6",
			"doodsThresholds": `{"4":5}`,
			"doodsDuration":   "0.000000003",
			"doodsFrameScale": "half",
			"doodsFeedRate":   "500000000",
		}
		config, err := parseConfig(c)
		require.NoError(t, err)

		actual := fmt.Sprintf("%v", config)
		require.Equal(t, actual, "&{2 3 map[4:5] 6000000 }")
	})
	t.Run("threshErr", func(t *testing.T) {
		c := monitor.Config{"doodsThresholds": "nil"}
		_, err := parseConfig(c)
		var e *json.SyntaxError
		require.ErrorAs(t, err, &e)
	})
	t.Run("cleanThresh", func(t *testing.T) {
		c := monitor.Config{
			"timestampOffset": "0",
			"doodsDuration":   "1",
			"doodsThresholds": `{"a":1,"b":2,"c":-1}`,
			"doodsFeedRate":   "1",
		}
		config, err := parseConfig(c)
		require.NoError(t, err)

		actual := fmt.Sprintf("%v", config.thresholds)
		require.Equal(t, actual, "map[a:1 b:2]")
	})

	cases := []struct {
		name   string
		config monitor.Config
	}{
		{
			"durationErr",
			monitor.Config{
				"doodsThresholds": "{}",
				"doodsFeedRate":   "nil",
			},
		},
		{
			"recDurationErr",
			monitor.Config{
				"doodsThresholds": "{}",
				"doodsDuration":   "nil",
				"doodsFeedRate":   "1",
			},
		},
		{
			"timestampOffsetErr",
			monitor.Config{
				"size":            "1x1",
				"doodsThresholds": "{}",
				"doodsDuration":   "1",
				"doodsFeedRate":   "1",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseConfig(tc.config)
			require.ErrorIs(t, err, strconv.ErrSyntax)
		})
	}
}

func TestParseInputs(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		inputs, err := parseInputs("1x2", "[3,4,5]", 6, 7, "gray_x")
		require.NoError(t, err)

		actual := fmt.Sprintf("%v", inputs)
		require.Equal(t, actual, "&{1 2 3 4 5 6 7 true}")
	})
	t.Run("gray", func(t *testing.T) {
		inputs, err := parseInputs("1x2", "[3,4,5]", 6, 7, "gray-x")
		require.NoError(t, err)

		actual := fmt.Sprintf("%v", inputs)
		require.Equal(t, actual, "&{1 2 3 4 5 6 7 false}")
	})
	t.Run("widthErr", func(t *testing.T) {
		_, err := parseInputs("nilx1", "", 0, 0, "")
		require.ErrorIs(t, err, strconv.ErrSyntax)
	})
	t.Run("heightErr", func(t *testing.T) {
		_, err := parseInputs("1xnil", "", 0, 0, "")
		require.ErrorIs(t, err, strconv.ErrSyntax)
	})
	t.Run("cropErr", func(t *testing.T) {
		_, err := parseInputs("1x1", `[1,2,"x"]`, 0, 0, "")
		var e *json.UnmarshalTypeError
		require.ErrorAs(t, err, &e)
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
				outputs, reverse, err := calculateOutputs(&inputs{
					inputWidth:   tc.inputWidth,
					inputHeight:  tc.inputHeight,
					cropX:        tc.cropX,
					cropY:        tc.cropY,
					cropSize:     tc.cropSize,
					outputWidth:  tc.outputWidth,
					outputHeight: tc.outputHeight,
				})
				require.NoError(t, err)

				actual := fmt.Sprintf("%vx%v %vx%v %v:%v %v:%v %v:%v",
					outputs.scaledWidth, outputs.scaledHeight,
					outputs.paddedWidth, outputs.paddedHeight,
					outputs.cropX, outputs.cropY,
					reverse.paddingXmultiplier, reverse.paddingYmultiplier,
					reverse.uncropXfunc(0.5), reverse.uncropYfunc(0.5),
				)
				require.Equal(t, actual, tc.expected)
			})
		}
	})
	t.Run("widthErr", func(t *testing.T) {
		_, _, err := calculateOutputs(&inputs{
			inputWidth:   1,
			inputHeight:  2,
			outputWidth:  2,
			outputHeight: 1,
		})
		require.ErrorIs(t, err, ErrInvalidConfig)
	})
	t.Run("heightErr", func(t *testing.T) {
		_, _, err := calculateOutputs(&inputs{
			inputWidth:   2,
			inputHeight:  1,
			outputWidth:  1,
			outputHeight: 2,
		})
		require.ErrorIs(t, err, ErrInvalidConfig)
	})
	t.Run("scaledWidthErr", func(t *testing.T) {
		_, _, err := calculateOutputs(&inputs{
			inputWidth:   100,
			inputHeight:  100,
			outputWidth:  80,
			outputHeight: 80,
			cropSize:     70,
		})
		require.ErrorIs(t, err, ErrInvalidConfig)
	})
}

func TestGenerateMask(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		err = os.Mkdir(filepath.Join(tempDir, "doods"), 0o700)
		require.NoError(t, err, "could not create doods directory")

		a := &instance{
			env: storage.ConfigEnv{
				SHMDir: tempDir,
			},
			outputs: outputs{
				scaledWidth:  1,
				scaledHeight: 1,
			},
		}
		_, err = a.generateMask(`{"enable":true}`)
		require.NoError(t, err)
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		a := &instance{}
		_, err := a.generateMask("")
		var e *json.SyntaxError
		require.ErrorAs(t, err, &e)
	})
	t.Run("disabled", func(t *testing.T) {
		a := &instance{}
		path, err := a.generateMask(`{"enable":false}`)
		require.NoError(t, err)
		require.Empty(t, path)
	})
	t.Run("saveErr", func(t *testing.T) {
		a := &instance{
			env: storage.ConfigEnv{
				SHMDir: "/dev/null",
			},
			outputs: outputs{},
		}
		_, err := a.generateMask(`{"enable":true}`)
		var e *os.PathError
		require.ErrorAs(t, err, &e)
	})
}

func TestGenerateArgs(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		a := instance{
			rtspProtocol: "2",
			rtspAddress:  "3",
			outputs: outputs{
				scaledWidth:  5,
				scaledHeight: 6,
				paddedWidth:  6,
				paddedHeight: 7,
				width:        9,
				height:       10,
				cropX:        "11",
				cropY:        "12",
			},
		}
		config := monitor.Config{
			"logLevel":      "1",
			"doodsFeedRate": "4",
		}
		args := a.generateFFmpegArgs(config, "", false)

		actual := fmt.Sprintf("%v", args)
		expected := "[-y -threads 1 -loglevel 1 -rtsp_transport 2 -i 3" +
			" -filter fps=fps=4,scale=5:6,pad=7:8:0:0,crop=9:10:11:12" +
			" -f rawvideo -pix_fmt rgb24 -]"
		require.Equal(t, actual, expected)
	})
	t.Run("maximal", func(t *testing.T) {
		a := instance{
			rtspProtocol: "3",
			rtspAddress:  "4",
			outputs: outputs{
				scaledWidth:  7,
				scaledHeight: 8,
				paddedWidth:  8,
				paddedHeight: 9,
				width:        11,
				height:       12,
				cropX:        "13",
				cropY:        "14",
			},
		}
		config := monitor.Config{
			"logLevel":      "1",
			"doodsFeedRate": "6",
			"hwaccel":       "2",
		}
		args := a.generateFFmpegArgs(config, "5", true)

		actual := fmt.Sprintf("%v", args)
		expected := "[-y -threads 1 -loglevel 1 -hwaccel 2 -rtsp_transport 3" +
			" -i 4 -i 5 -filter_complex [0:v]fps=fps=6,scale=7:8[bg];" +
			"[bg][1:v]overlay,pad=9:10:0:0,crop=11:12:13:14,hue=s=0" +
			" -f rawvideo -pix_fmt rgb24 -]"
		require.Equal(t, actual, expected)
	})
}

func mockRunFFmpeg(context.Context, instance) error    { return nil }
func mockRunFFmpegErr(context.Context, instance) error { return errors.New("mock") }

func TestStartFFmpeg(t *testing.T) {
	t.Run("crashed", func(t *testing.T) {
		i, feed, cancel := newTestInstance()
		defer cancel()

		i.runFFmpeg = mockRunFFmpegErr

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		i.wg.Add(1)
		go i.startFFmpeg(ctx)

		actual := <-feed
		require.Equal(t, actual.Msg, "process crashed: mock")
	})
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		i := instance{wg: &sync.WaitGroup{}}
		i.wg.Add(1)
		i.startFFmpeg(ctx)
		i.wg.Wait()
	})
}

func TestRunFFmpeg(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		i, _, cancel := newTestInstance()
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		err := runFFmpeg(ctx, i)
		require.NoError(t, err)
	})
	t.Run("crashed", func(t *testing.T) {
		i, _, cancel := newTestInstance()
		defer cancel()

		i.newProcess = ffmock.NewProcessErr

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		err := runFFmpeg(ctx, i)
		require.ErrorIs(t, err, ffmock.ErrMock)
	})
	t.Run("startClientCalled", func(t *testing.T) {
		startReaderCalled := make(chan struct{})
		mockStartReader := func(context.Context, instance, io.Reader) {
			go close(startReaderCalled)
		}

		i, _, cancel := newTestInstance()
		defer cancel()

		i.startInstance = mockStartReader

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		err := runFFmpeg(ctx, i)
		require.NoError(t, err)

		<-startReaderCalled
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

func mockStartInstance(context.Context, instance, io.Reader) {}

func newTestInstance() (instance, log.Feed, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	logger := log.NewMockLogger()
	logger.Start(ctx)
	feed, cancel2 := logger.Subscribe()

	cancelFunc := func() {
		cancel2()
		cancel()
	}

	i := instance{
		env: storage.ConfigEnv{},
		c: config{
			eventDuration: 2,
			recDuration:   3,
		},
		outputs: outputs{
			width:     2,
			height:    2,
			frameSize: 2 * 2 * 3,
		},
		reverseValues: reverseValues{
			paddingXmultiplier: 1.1,
			paddingYmultiplier: 0.9,
			uncropXfunc:        func(i float32) float32 { return i },
			uncropYfunc:        func(i float32) float32 { return i },
		},

		log: logger,
		wg:  &sync.WaitGroup{},
		encoder: png.Encoder{
			CompressionLevel: png.NoCompression,
		},
		newProcess:    ffmock.NewProcess,
		runFFmpeg:     mockRunFFmpeg,
		startInstance: mockStartInstance,
		runInstance:   mockRunInstance,
		watchdogTimer: time.NewTimer(0),
	}

	return i, feed, cancelFunc
}

func mockRunInstance(context.Context, instance, io.Reader) error {
	return nil
}

func mockRunInstanceErr(context.Context, instance, io.Reader) error {
	return errors.New("mock")
}

func TestStartInstance(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		i, feed, cancel := newTestInstance()
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		cancel2()

		i.wg.Add(1)
		go startInstance(ctx, i, imgFeed())

		actual := <-feed
		require.Equal(t, actual.Msg, "instance stopped")
	})
	t.Run("crashed", func(t *testing.T) {
		i, feed, cancel := newTestInstance()
		defer cancel()

		i.runInstance = mockRunInstanceErr

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		go startInstance(ctx, i, imgFeed())

		actual := <-feed
		require.Equal(t, actual.Msg, "instance crashed: mock")
	})
}

func TestRunInstance(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		mu := sync.Mutex{}

		var firstRequest string
		var secondRequest string
		mockSendRequest := func(_ context.Context, request detectRequest) (*detections, error) {
			if firstRequest == "" {
				firstRequest = fmt.Sprint(*request.Data)
			} else {
				secondRequest = fmt.Sprint(*request.Data)
				mu.Unlock()
			}
			return &detections{Detection{Label: "1"}}, nil
		}

		var event storage.Event
		mockSendEvent := func(e storage.Event) error {
			event = e
			return nil
		}

		i, _, cancel := newTestInstance()
		defer cancel()

		i.sendRequest = mockSendRequest
		i.sendEvent = mockSendEvent

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		go runInstance(ctx, i, imgFeed())
		mu.Lock()

		defer mu.Unlock()
		mu.Lock()

		require.Equal(t, firstRequest, secondRequest)
		require.Equal(t, firstRequest, framePNG)

		event.Time = time.Time{}
		actual := event

		expected := storage.Event{
			Detections: []storage.Detection{{
				Label:  "1",
				Region: &storage.Region{Rect: &ffmpeg.Rect{}},
			}},
			Duration:    2,
			RecDuration: 3,
		}
		require.Equal(t, actual, expected)
	})
	t.Run("canceled", func(t *testing.T) {
		i, _, cancel := newTestInstance()
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		cancel2()

		err := runInstance(ctx, i, imgFeed())
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("sendFrameErr", func(t *testing.T) {
		mockSendRequestErr := func(context.Context, detectRequest) (*detections, error) {
			return nil, errors.New("mock")
		}

		i, _, cancel := newTestInstance()
		defer cancel()

		i.sendRequest = mockSendRequestErr

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		err := runInstance(ctx, i, imgFeed())
		require.Error(t, err)
	})
}

func TestParseDetections(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		reverse := reverseValues{
			paddingXmultiplier: 1.1,
			paddingYmultiplier: 0.9,
			uncropXfunc:        func(i float32) float32 { return i },
			uncropYfunc:        func(i float32) float32 { return i },
		}
		detections := detections{
			{
				Top:        0.1,
				Left:       0.1,
				Bottom:     1,
				Right:      1,
				Label:      "b",
				Confidence: 5,
			},
		}

		actual := parseDetections(reverse, detections)
		expected := []storage.Detection{
			{
				Label: "b",
				Score: 5,
				Region: &storage.Region{
					Rect: &ffmpeg.Rect{9, 11, 90, 110},
				},
			},
		}
		require.Equal(t, actual, expected)
	})
	t.Run("noDetections", func(t *testing.T) {
		parseDetections(reverseValues{}, detections{})
	})
}
