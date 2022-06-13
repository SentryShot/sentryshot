package monitor

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"nvr/pkg/ffmpeg"
	"nvr/pkg/ffmpeg/ffmock"
	"nvr/pkg/storage"

	"github.com/stretchr/testify/require"
)

func TestStartRecorder(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		onRunProcess := make(chan struct{})
		onCanceled := make(chan struct{})
		mockRunRecordingProcess := func(ctx context.Context, _ *Monitor) error {
			close(onRunProcess)
			<-ctx.Done()
			close(onCanceled)
			return nil
		}

		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.WG.Add(1)
		m.runRecordingProcess = mockRunRecordingProcess
		go m.startRecorder(ctx)

		err := m.SendEvent(storage.Event{
			Time:        time.Now().Add(time.Duration(-1) * time.Hour),
			RecDuration: 1,
		})
		require.NoError(t, err)

		<-onRunProcess
		<-onCanceled
	})
	t.Run("timeoutUpdate", func(t *testing.T) {
		onRunProcess := make(chan struct{})
		mockRunRecordingProcess := func(ctx context.Context, _ *Monitor) error {
			close(onRunProcess)
			<-ctx.Done()
			return nil
		}

		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.WG.Add(1)
		m.runRecordingProcess = mockRunRecordingProcess

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		go m.startRecorder(ctx)

		now := time.Now()
		m.eventChan <- storage.Event{Time: now, RecDuration: 20 * time.Millisecond}
		m.eventChan <- storage.Event{Time: now, RecDuration: 60 * time.Millisecond}

		<-onRunProcess
	})
	t.Run("recordingCheck", func(t *testing.T) {
		onRunProcess := make(chan struct{})
		mockRunRecordingProcess := func(ctx context.Context, _ *Monitor) error {
			close(onRunProcess)
			<-ctx.Done()
			return nil
		}

		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		m.WG.Add(1)
		m.runRecordingProcess = mockRunRecordingProcess
		go m.startRecorder(ctx)

		now := time.Now()
		m.eventChan <- storage.Event{Time: now, RecDuration: 10 * time.Millisecond}
		m.eventChan <- storage.Event{Time: now, RecDuration: 11 * time.Millisecond}
		m.eventChan <- storage.Event{Time: now, RecDuration: 0 * time.Millisecond}

		<-onRunProcess
	})
	// Only update timeout if new time is after current time.
	t.Run("updateTimeout", func(t *testing.T) {
		onCancel := make(chan struct{})
		mockRunRecordingProcess := func(ctx context.Context, _ *Monitor) error {
			<-ctx.Done()
			close(onCancel)
			return nil
		}

		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.WG.Add(1)
		m.runRecordingProcess = mockRunRecordingProcess
		go m.startRecorder(ctx)

		now := time.Now()
		m.eventChan <- storage.Event{Time: now, RecDuration: 30 * time.Millisecond}
		m.eventChan <- storage.Event{Time: now, RecDuration: 1 * time.Millisecond}

		select {
		case <-time.After(15 * time.Millisecond):
		case <-onCancel:
			t.Fatal("the second trigger reset the timeout")
		}
	})
	t.Run("normalExit", func(t *testing.T) {
		onRunProcess := make(chan struct{})
		exitProcess := make(chan error)
		mockRunRecordingProcess := func(ctx context.Context, _ *Monitor) error {
			onRunProcess <- struct{}{}
			return <-exitProcess
		}

		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.WG.Add(1)
		m.runRecordingProcess = mockRunRecordingProcess
		go m.startRecorder(ctx)

		now := time.Now()
		m.eventChan <- storage.Event{Time: now, RecDuration: 1 * time.Hour}

		<-onRunProcess
		exitProcess <- nil
		<-onRunProcess
		exitProcess <- nil
		<-onRunProcess
		close(onRunProcess)
		exitProcess <- ffmock.ErrMock
	})
	t.Run("canceled", func(t *testing.T) {
		m, _, cancel := newTestMonitor(t)
		defer cancel()

		ctx2, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		mockRunRecordingProcess := func(context.Context, *Monitor) error {
			cancel2()
			return nil
		}

		m.WG.Add(1)
		m.runRecordingProcess = mockRunRecordingProcess
		go m.startRecorder(ctx2)

		now := time.Now()
		m.eventChan <- storage.Event{Time: now, RecDuration: 1 * time.Hour}
	})
	t.Run("canceledRecording", func(t *testing.T) {
		onCancel := make(chan struct{})
		mockRunRecordingProcess := func(ctx context.Context, _ *Monitor) error {
			<-ctx.Done()
			close(onCancel)
			return nil
		}

		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.WG.Add(1)
		m.runRecordingProcess = mockRunRecordingProcess
		go m.startRecorder(ctx)

		now := time.Now()
		m.eventChan <- storage.Event{Time: now, RecDuration: 0}
		<-onCancel
	})
	t.Run("crashed", func(t *testing.T) {
		onRunProcess := make(chan struct{})
		mockRunRecordingProcess := func(ctx context.Context, _ *Monitor) error {
			close(onRunProcess)
			return ffmock.ErrMock
		}

		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.WG.Add(1)
		m.runRecordingProcess = mockRunRecordingProcess
		go m.startRecorder(ctx)

		now := time.Now()
		m.eventChan <- storage.Event{Time: now, RecDuration: 1 * time.Hour}
		<-onRunProcess
	})
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
			<-ctx.Done()
		}
		err := runRecordingProcess(ctx, m)
		require.NoError(t, err)
	})
	t.Run("waitForKeyframeErr", func(t *testing.T) {
		mockWaitForNewHLSsegmentErr := func(context.Context, int) (time.Duration, error) {
			return 0, ffmock.ErrMock
		}

		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.NewProcess = ffmock.NewProcess
		m.mainInput.waitForNewHLSsegment = mockWaitForNewHLSsegmentErr

		err := runRecordingProcess(ctx, m)
		require.ErrorIs(t, err, ffmock.ErrMock)
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
		m := &Monitor{
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
