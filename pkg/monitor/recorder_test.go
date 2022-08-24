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

package monitor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"nvr/pkg/ffmpeg"
	"nvr/pkg/ffmpeg/ffmock"
	"nvr/pkg/log"
	"nvr/pkg/storage"
	"nvr/pkg/video"

	"github.com/stretchr/testify/require"
)

func newTestRecorder(t *testing.T) *Recorder {
	tempDir := t.TempDir()
	t.Cleanup(func() {
		os.Remove(tempDir)
	})

	logf := func(level log.Level, format string, a ...interface{}) {}
	return &Recorder{
		Config: &Config{
			"timestampOffset": "0",
			"videoLength":     "0.0003",
		},
		MonitorLock: &sync.Mutex{},

		logf:          logf,
		runProcess:    runRecordingProcess,
		NewProcess:    ffmock.NewProcess,
		videoDuration: mockVideoDuration,

		input: &InputProcess{
			isSubInput: false,

			serverPath: video.ServerPath{
				HlsAddress:           "hls.m3u8",
				WaitForNewHLSsegment: mockWaitForNewHLSsegment,
			},

			logf: logf,

			runInputProcess: mockRunInputProcess,
			newProcess:      ffmock.NewProcess,
		},
		wg: &sync.WaitGroup{},
		Env: storage.ConfigEnv{
			TempDir:    tempDir,
			StorageDir: tempDir,
		},
		eventsLock: sync.Mutex{},
		eventChan:  make(chan storage.Event),
		hooks:      mockHooks(),
	}
}

func mockVideoDuration(string) (time.Duration, error) {
	return 10 * time.Minute, nil
}

func TestStartRecorder(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		onRunProcess := make(chan struct{})
		onCanceled := make(chan struct{})
		mockRunRecordingProcess := func(ctx context.Context, _ *Recorder) error {
			close(onRunProcess)
			<-ctx.Done()
			close(onCanceled)
			return nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		r := newTestRecorder(t)
		r.wg.Add(1)
		r.runProcess = mockRunRecordingProcess
		go r.start(ctx)

		err := r.sendEvent(storage.Event{
			Time:        time.Now().Add(time.Duration(-1) * time.Hour),
			RecDuration: 1,
		})
		require.NoError(t, err)

		<-onRunProcess
		<-onCanceled
	})
	t.Run("timeoutUpdate", func(t *testing.T) {
		onRunProcess := make(chan struct{})
		mockRunRecordingProcess := func(ctx context.Context, _ *Recorder) error {
			close(onRunProcess)
			<-ctx.Done()
			return nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		r := newTestRecorder(t)
		r.wg.Add(1)
		r.runProcess = mockRunRecordingProcess
		go r.start(ctx)

		now := time.Now()
		r.eventChan <- storage.Event{Time: now, RecDuration: 20 * time.Millisecond}
		r.eventChan <- storage.Event{Time: now, RecDuration: 60 * time.Millisecond}

		<-onRunProcess
	})
	t.Run("recordingCheck", func(t *testing.T) {
		onRunProcess := make(chan struct{})
		mockRunRecordingProcess := func(ctx context.Context, _ *Recorder) error {
			close(onRunProcess)
			<-ctx.Done()
			return nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		r := newTestRecorder(t)
		r.wg.Add(1)
		r.runProcess = mockRunRecordingProcess
		go r.start(ctx)

		now := time.Now()
		r.eventChan <- storage.Event{Time: now, RecDuration: 10 * time.Millisecond}
		r.eventChan <- storage.Event{Time: now, RecDuration: 11 * time.Millisecond}
		r.eventChan <- storage.Event{Time: now, RecDuration: 0 * time.Millisecond}

		<-onRunProcess
	})
	// Only update timeout if new time is after current time.
	t.Run("updateTimeout", func(t *testing.T) {
		onCancel := make(chan struct{})
		mockRunRecordingProcess := func(ctx context.Context, _ *Recorder) error {
			<-ctx.Done()
			close(onCancel)
			return nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		r := newTestRecorder(t)
		r.wg.Add(1)
		r.runProcess = mockRunRecordingProcess
		go r.start(ctx)

		now := time.Now()
		r.eventChan <- storage.Event{Time: now, RecDuration: 30 * time.Millisecond}
		r.eventChan <- storage.Event{Time: now, RecDuration: 1 * time.Millisecond}

		select {
		case <-time.After(15 * time.Millisecond):
		case <-onCancel:
			t.Fatal("the second trigger reset the timeout")
		}
	})
	t.Run("normalExit", func(t *testing.T) {
		onRunProcess := make(chan struct{})
		exitProcess := make(chan error)
		mockRunRecordingProcess := func(ctx context.Context, _ *Recorder) error {
			onRunProcess <- struct{}{}
			return <-exitProcess
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		r := newTestRecorder(t)
		r.wg.Add(1)
		r.runProcess = mockRunRecordingProcess
		go r.start(ctx)

		now := time.Now()
		r.eventChan <- storage.Event{Time: now, RecDuration: 1 * time.Hour}

		<-onRunProcess
		exitProcess <- nil
		<-onRunProcess
		exitProcess <- nil
		<-onRunProcess
		close(onRunProcess)
		exitProcess <- ffmock.ErrMock
	})
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		mockRunRecordingProcess := func(context.Context, *Recorder) error {
			cancel()
			return nil
		}

		r := newTestRecorder(t)
		r.wg.Add(1)
		r.runProcess = mockRunRecordingProcess
		go r.start(ctx)

		now := time.Now()
		r.eventChan <- storage.Event{Time: now, RecDuration: 1 * time.Hour}
	})
	t.Run("canceledRecording", func(t *testing.T) {
		onCancel := make(chan struct{})
		mockRunRecordingProcess := func(ctx context.Context, _ *Recorder) error {
			<-ctx.Done()
			close(onCancel)
			return nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		r := newTestRecorder(t)
		r.wg.Add(1)
		r.runProcess = mockRunRecordingProcess
		go r.start(ctx)

		now := time.Now()
		r.eventChan <- storage.Event{Time: now, RecDuration: 0}
		<-onCancel
	})
	t.Run("crashed", func(t *testing.T) {
		onRunProcess := make(chan struct{})
		mockRunRecordingProcess := func(ctx context.Context, _ *Recorder) error {
			close(onRunProcess)
			return ffmock.ErrMock
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		r := newTestRecorder(t)
		r.wg.Add(1)
		r.runProcess = mockRunRecordingProcess
		go r.start(ctx)

		now := time.Now()
		r.eventChan <- storage.Event{Time: now, RecDuration: 1 * time.Hour}
		<-onRunProcess
	})
}

func createTempDir(t *testing.T, r *Recorder) {
}

func TestRunRecordingProcess(t *testing.T) {
	t.Run("finished", func(t *testing.T) {
		logs := make(chan string)
		logf := func(level log.Level, format string, a ...interface{}) {
			logs <- fmt.Sprintf(format, a...)
		}

		r := newTestRecorder(t)
		r.logf = logf
		r.NewProcess = ffmock.NewProcessNil

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			runRecordingProcess(ctx, r)
		}()

		<-logs
		<-logs
		require.Equal(t, "recording finished", <-logs)
	})
	t.Run("saveRecordingAsync", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		r := newTestRecorder(t)
		r.NewProcess = ffmock.NewProcessNil
		r.hooks.RecSave = func(*Recorder, *string) {
			<-ctx.Done()
		}
		err := runRecordingProcess(ctx, r)
		require.NoError(t, err)
	})
	t.Run("waitForKeyframeErr", func(t *testing.T) {
		mockWaitForNewHLSsegmentErr := func(context.Context, int) (time.Duration, error) {
			return 0, ffmock.ErrMock
		}

		r := newTestRecorder(t)
		r.NewProcess = ffmock.NewProcess
		r.input.serverPath.WaitForNewHLSsegment = mockWaitForNewHLSsegmentErr

		err := runRecordingProcess(context.Background(), r)
		require.ErrorIs(t, err, ffmock.ErrMock)
	})
	t.Run("mkdirErr", func(t *testing.T) {
		r := newTestRecorder(t)
		r.Env.StorageDir = "/dev/null"

		err := runRecordingProcess(context.Background(), r)
		require.Error(t, err)
	})
	t.Run("genArgsErr", func(t *testing.T) {
		r := newTestRecorder(t)
		(*r.Config)["videoLength"] = ""

		err := runRecordingProcess(context.Background(), r)
		require.ErrorIs(t, err, strconv.ErrSyntax)
	})
	t.Run("parseOffsetErr", func(t *testing.T) {
		r := newTestRecorder(t)
		(*r.Config)["timestampOffset"] = ""

		err := runRecordingProcess(context.Background(), r)
		require.ErrorIs(t, err, strconv.ErrSyntax)
	})
	t.Run("crashed", func(t *testing.T) {
		r := newTestRecorder(t)
		r.NewProcess = ffmock.NewProcessErr

		err := runRecordingProcess(context.Background(), r)
		require.Error(t, err)
	})
}

func TestGenRecorderArgs(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		r := newTestRecorder(t)
		(*r.Config)["id"] = "id"
		(*r.Config)["logLevel"] = "1"
		(*r.Config)["videoLength"] = "3"

		args, err := r.generateRecorderArgs("path")
		require.NoError(t, err)

		expected := "-y -threads 1 -loglevel 1 -live_start_index -2 -i hls.m3u8 -t 180 -c:v copy path.mp4"
		require.Equal(t, args, expected)
	})
	t.Run("videoLengthErr", func(t *testing.T) {
		r := newTestRecorder(t)
		(*r.Config)["videoLength"] = ""

		_, err := r.generateRecorderArgs("path")
		require.ErrorIs(t, err, strconv.ErrSyntax)
	})
}

func mockVideoDurationErr(string) (time.Duration, error) {
	return 0, errors.New("mock")
}

func TestSaveRecording(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		r := newTestRecorder(t)
		r.events = storage.Events{
			storage.Event{
				Time: time.Time{},
			},
			storage.Event{
				Time: time.Time{}.Add(2 * time.Minute),
				Detections: []storage.Detection{
					{
						Label: "10",
						Score: 9,
						Region: &storage.Region{
							Rect: &ffmpeg.Rect{1, 2, 3, 4},
							Polygon: &ffmpeg.Polygon{
								ffmpeg.Point{5, 6},
								ffmpeg.Point{7, 8},
							},
						},
					},
				},
				Duration: 11,
			},
			storage.Event{
				Time: time.Time{}.Add(11 * time.Minute),
			},
		}

		start := time.Time{}.Add(1 * time.Minute)
		tempdir := r.Env.TempDir
		filePath := tempdir + "file"

		err := r.saveRec(filePath, start)
		require.NoError(t, err)

		b, err := os.ReadFile(filePath + ".json")
		require.NoError(t, err)

		actual := string(b)
		actual = strings.ReplaceAll(actual, " ", "")
		actual = strings.ReplaceAll(actual, "\n", "")

		expected := `{"start":"0001-01-01T00:01:00Z","end":"0001-01-01T00:11:00Z",` +
			`"events":[{"time":"0001-01-01T00:02:00Z","detections":` +
			`[{"label":"10","score":9,"region":{"rect":[1,2,3,4],` +
			`"polygon":[[5,6],[7,8]]}}],"duration":11}]}`

		require.Equal(t, actual, expected)
	})
	t.Run("genThumbnailErr", func(t *testing.T) {
		r := newTestRecorder(t)
		r.NewProcess = ffmock.NewProcessErr

		err := r.saveRec("", time.Time{})
		require.Error(t, err)
	})
	t.Run("durationErr", func(t *testing.T) {
		r := newTestRecorder(t)
		r.videoDuration = mockVideoDurationErr

		err := r.saveRec("", time.Time{})
		require.Error(t, err)
	})
	t.Run("writeFileErr", func(t *testing.T) {
		r := newTestRecorder(t)

		err := r.saveRec("/dev/null/", time.Time{})
		require.Error(t, err)
	})
}
