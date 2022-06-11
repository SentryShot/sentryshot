package monitor

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"nvr/pkg/ffmpeg"
	"nvr/pkg/ffmpeg/ffmock"
	"nvr/pkg/storage"

	"github.com/stretchr/testify/require"
)

func TestStartRecorder(t *testing.T) {
	t.Run("missingTime", func(t *testing.T) {
		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.startRecording = mockStartRecording
		m.WG.Add(1)

		go m.startRecorder(ctx)

		actual := m.SendEvent(storage.Event{RecDuration: 1}).Error()
		expected := `invalid event: {
 Time: 0001-01-01 00:00:00 +0000 UTC
 Detections: []
 Duration: 0s
 RecDuration: 1ns
}
'Time': ` + storage.ErrValueMissing.Error()

		require.Equal(t, actual, expected)
	})
	t.Run("missingRecDuration", func(t *testing.T) {
		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.startRecording = mockStartRecording
		m.WG.Add(1)

		go m.startRecorder(ctx)
		err := m.SendEvent(storage.Event{Time: (time.Unix(1, 0).UTC())})

		actual := err.Error()
		expected := `invalid event: {
 Time: 1970-01-01 00:00:01 +0000 UTC
 Detections: []
 Duration: 0s
 RecDuration: 0s
}
'RecDuration': ` + storage.ErrValueMissing.Error()

		require.Equal(t, actual, expected)
	})

	t.Run("timeout", func(t *testing.T) {
		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.startRecording = mockStartRecording

		feed, cancel2 := m.Log.Subscribe()
		defer cancel2()

		go m.startRecorder(ctx)
		err := m.SendEvent(storage.Event{
			Time:        time.Now().Add(time.Duration(-1) * time.Hour),
			RecDuration: 1,
		})
		require.NoError(t, err)

		actual := <-feed
		expected := "trigger reached end, stopping recording"
		require.Equal(t, actual.Msg, expected)
	})
	t.Run("timeoutUpdate", func(t *testing.T) {
		mu := sync.Mutex{}
		mockStartRecording := func(context.Context, *Monitor) {
			mu.Unlock()
		}

		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.WG.Add(1)
		m.startRecording = mockStartRecording

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		mu.Lock()
		go m.startRecorder(ctx)

		now := time.Now()
		m.eventChan <- storage.Event{Time: now, RecDuration: 10 * time.Millisecond}
		m.eventChan <- storage.Event{Time: now, RecDuration: 50 * time.Millisecond}

		mu.Lock()
		mu.Unlock()
	})
	t.Run("recordingCheck", func(t *testing.T) {
		mu := sync.Mutex{}
		mockStartRecording := func(context.Context, *Monitor) {
			mu.Unlock()
		}

		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.startRecording = mockStartRecording

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		mu.Lock()
		go m.startRecorder(ctx)

		now := time.Now()
		m.eventChan <- storage.Event{Time: now, RecDuration: 10 * time.Millisecond}
		m.eventChan <- storage.Event{Time: now, RecDuration: 11 * time.Millisecond}
		m.eventChan <- storage.Event{Time: now, RecDuration: 0 * time.Millisecond}

		mu.Lock()
		mu.Unlock()
	})
	// Only update timeout if new time is after current time.
	t.Run("updateTimeout", func(t *testing.T) {
		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.startRecording = mockStartRecording

		feed, cancel2 := m.Log.Subscribe()
		defer cancel2()

		go m.startRecorder(ctx)

		now := time.Now()
		m.eventChan <- storage.Event{Time: now, RecDuration: 40 * time.Millisecond}
		m.eventChan <- storage.Event{Time: now, RecDuration: 1 * time.Millisecond}

		select {
		case <-time.After(30 * time.Millisecond):
		case <-feed:
			t.Fatal("the second trigger reset the timeout")
		}
	})
}

func mockStartRecording(context.Context, *Monitor) {}

func TestStartRecording(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		m, _, cancel := newTestMonitor(t)
		defer cancel()

		feed, cancel2 := m.Log.Subscribe()
		defer cancel2()

		// Cancel the recording not the monitor.
		ctx2, cancel3 := context.WithCancel(context.Background())
		cancel3()

		m.WG.Add(1)
		go startRecording(ctx2, m)

		actual := <-feed
		require.Equal(t, actual.Msg, "recording stopped")
	})
	t.Run("crashed", func(t *testing.T) {
		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.runRecordingProcess = mockRunRecordingProcessErr

		feed, cancel2 := m.Log.Subscribe()
		defer cancel2()

		m.WG.Add(1)
		go startRecording(ctx, m)

		actual := <-feed
		require.Equal(t, actual.Msg, "recording process: mock")
	})
}

func mockRunRecordingProcess(context.Context, *Monitor) error {
	return nil
}

func mockRunRecordingProcessErr(context.Context, *Monitor) error {
	return errors.New("mock")
}

func TestRunRecordingProcess(t *testing.T) {
	t.Run("finished", func(t *testing.T) {
		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.NewProcess = ffmock.NewProcessNil

		feed, cancel2 := m.Log.Subscribe()
		defer cancel2()

		go func() {
			runRecordingProcess(ctx, m)
		}()

		<-feed
		<-feed
		actual := <-feed
		require.Equal(t, actual.Msg, "recording finished")
	})
	t.Run("saveRecordingAsync", func(t *testing.T) {
		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.NewProcess = ffmock.NewProcessNil
		m.hooks.RecSave = func(*Monitor, *string) {
			time.Sleep(1 * time.Hour)
		}

		runRecordingProcess(ctx, m)
	})
	t.Run("waitForKeyframeErr", func(t *testing.T) {
		mockWaitForNewHLSsegmentErr := func(context.Context, int) (time.Duration, error) {
			return 0, errors.New("mock")
		}

		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.NewProcess = ffmock.NewProcess
		m.mainInput.waitForNewHLSsegment = mockWaitForNewHLSsegmentErr

		m.WG.Add(1)
		err := runRecordingProcess(ctx, m)
		require.Error(t, err)
	})
	t.Run("mkdirErr", func(t *testing.T) {
		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.Env = storage.ConfigEnv{
			StorageDir: "/dev/null",
		}

		err := runRecordingProcess(ctx, m)
		require.Error(t, err)
	})
	t.Run("genArgsErr", func(t *testing.T) {
		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.Config["videoLength"] = ""

		err := runRecordingProcess(ctx, m)
		require.ErrorIs(t, err, strconv.ErrSyntax)
	})
	t.Run("parseOffsetErr", func(t *testing.T) {
		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.Config["timestampOffset"] = ""

		err := runRecordingProcess(ctx, m)
		require.ErrorIs(t, err, strconv.ErrSyntax)
	})
	t.Run("crashed", func(t *testing.T) {
		m, ctx, cancel := newTestMonitor(t)
		defer cancel()
		m.NewProcess = ffmock.NewProcessErr

		err := runRecordingProcess(ctx, m)
		require.Error(t, err)
	})
}

func TestGenRecorderArgs(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		m := &Monitor{
			Env: storage.ConfigEnv{},
			Config: map[string]string{
				"logLevel":    "1",
				"videoLength": "3",
				"id":          "id",
			},
		}
		m.mainInput = newMockInputProcess(m, false)

		args, err := m.generateRecorderArgs("path")
		require.NoError(t, err)

		expected := "-y -threads 1 -loglevel 1 -live_start_index -2 -i hls.m3u8 -t 180 -c:v copy path.mp4"
		require.Equal(t, args, expected)
	})
	t.Run("videoLengthErr", func(t *testing.T) {
		m := Monitor{
			Env: storage.ConfigEnv{},
		}
		_, err := m.generateRecorderArgs("path")
		require.ErrorIs(t, err, strconv.ErrSyntax)
	})
}

func mockVideoDuration(string) (time.Duration, error) {
	return 10 * time.Minute, nil
}

func mockVideoDurationErr(string) (time.Duration, error) {
	return 0, errors.New("mock")
}

func TestSaveRecording(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		m, _, cancel := newTestMonitor(t)
		defer cancel()

		m.events = storage.Events{
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
		tempdir := m.Env.SHMDir
		filePath := tempdir + "file"

		err := m.saveRec(filePath, start)
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
		m, _, cancel := newTestMonitor(t)
		defer cancel()

		m.NewProcess = ffmock.NewProcessErr

		err := m.saveRec("", time.Time{})
		require.Error(t, err)
	})
	t.Run("durationErr", func(t *testing.T) {
		m, _, cancel := newTestMonitor(t)
		defer cancel()

		m.videoDuration = mockVideoDurationErr

		err := m.saveRec("", time.Time{})
		require.Error(t, err)
	})
	t.Run("writeFileErr", func(t *testing.T) {
		m, _, cancel := newTestMonitor(t)
		defer cancel()

		err := m.saveRec("/dev/null/", time.Time{})
		require.Error(t, err)
	})
}
