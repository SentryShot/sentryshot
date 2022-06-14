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
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"nvr/pkg/ffmpeg/ffmock"
	"nvr/pkg/log"
	"nvr/pkg/storage"
	"nvr/pkg/video"

	"github.com/stretchr/testify/require"
)

type cancelFunc func()

func prepareDir(t *testing.T) (string, cancelFunc) {
	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)

	testConfigDir := tempDir + "/monitors"
	err = os.Mkdir(testConfigDir, 0o700)
	require.NoError(t, err)

	fileSystem := os.DirFS("./testdata/monitors/")
	err = fs.WalkDir(fileSystem, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		file, err := fs.ReadFile(fileSystem, path)
		if err != nil {
			return err
		}
		newFilePath := filepath.Join(testConfigDir, d.Name())
		if err := os.WriteFile(newFilePath, file, 0o600); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatal(err)
	}

	cancel := func() {
		os.RemoveAll(tempDir)
	}
	return testConfigDir, cancel
}

func newTestManager(t *testing.T) (string, *Manager, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := log.NewMockLogger()
	logger.Start(ctx)

	wg := sync.WaitGroup{}
	videoServer := video.NewServer(logger, &wg, 2021, 2022)

	err := videoServer.Start(ctx)
	require.NoError(t, err)

	configDir, cancel2 := prepareDir(t)

	cancelFunc := func() {
		cancel()
		cancel2()
		wg.Wait()
	}

	manager, err := NewManager(
		configDir,
		storage.ConfigEnv{},
		logger,
		videoServer,
		&Hooks{},
	)
	require.NoError(t, err)

	return configDir, manager, cancelFunc
}

func readConfig(t *testing.T, path string) Config {
	file, err := os.ReadFile(path)
	require.NoError(t, err)

	var config Config
	err = json.Unmarshal(file, &config)
	require.NoError(t, err)

	return config
}

func TestNewManager(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		configDir, manager, cancel := newTestManager(t)
		defer cancel()

		config := readConfig(t, filepath.Join(configDir, "1.json"))
		require.Equal(t, config, manager.Monitors["1"].Config)
	})
	t.Run("mkDirErr", func(t *testing.T) {
		_, err := NewManager("/dev/null/nil", storage.ConfigEnv{}, nil, nil, nil)
		require.Error(t, err)
	})
	t.Run("readFileErr", func(t *testing.T) {
		_, err := NewManager(
			"/dev/null/nil.json",
			storage.ConfigEnv{},
			&log.Logger{},
			&video.Server{},
			&Hooks{},
		)
		require.Error(t, err)
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		configDir, cancel := prepareDir(t)
		defer cancel()

		data := []byte("{")
		err := os.WriteFile(configDir+"/1.json", data, 0o600)
		require.NoError(t, err)

		_, err = NewManager(
			configDir,
			storage.ConfigEnv{},
			&log.Logger{},
			&video.Server{},
			&Hooks{},
		)
		var e *json.SyntaxError
		require.ErrorAs(t, err, &e)
	})
}

func TestMonitorSet(t *testing.T) {
	t.Run("createNew", func(t *testing.T) {
		configDir, manager, cancel := newTestManager(t)
		defer cancel()

		config := manager.Monitors["1"].Config
		config["name"] = "new"

		err := manager.MonitorSet("new", config)
		require.NoError(t, err)

		newName := manager.Monitors["new"].Config["name"]
		require.Equal(t, newName, "new")

		// Check if changes were saved to file.
		config = readConfig(t, configDir+"/new.json")
		require.Equal(t, config, manager.Monitors["new"].Config)
	})
	t.Run("setOld", func(t *testing.T) {
		configDir, manager, cancel := newTestManager(t)
		defer cancel()

		oldMonitor := manager.Monitors["1"]
		oldMonitor.running = true

		oldname := oldMonitor.Config["name"]
		require.Equal(t, oldname, "one")

		config := oldMonitor.Config
		config["name"] = "two"

		err := manager.MonitorSet("1", config)
		require.NoError(t, err)

		running := manager.Monitors["1"].running
		require.True(t, running, "old monitor was reset")

		newName := manager.Monitors["1"].Config["name"]
		require.Equal(t, newName, "two")

		// Check if changes were saved to file.
		config = readConfig(t, configDir+"/1.json")
		require.Equal(t, config, manager.Monitors["1"].Config)
	})
	t.Run("writeFileErr", func(t *testing.T) {
		_, manager, cancel := newTestManager(t)
		defer cancel()

		manager.path = "/dev/null"

		err := manager.MonitorSet("1", Config{})
		require.Error(t, err)
	})
}

func TestMonitorDelete(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		_, manager, cancel := newTestManager(t)
		defer cancel()

		require.NotNil(t, manager.Monitors["1"])

		err := manager.MonitorDelete("1")
		require.NoError(t, err)

		require.Nil(t, manager.Monitors["1"])
	})
	t.Run("existErr", func(t *testing.T) {
		_, manager, cancel := newTestManager(t)
		defer cancel()

		err := manager.MonitorDelete("nil")
		require.ErrorIs(t, err, ErrNotExist)
	})
	t.Run("removeErr", func(t *testing.T) {
		_, manager, cancel := newTestManager(t)
		defer cancel()

		manager.path = "/dev/null"

		err := manager.MonitorDelete("1")
		require.Error(t, err)
	})
}

func TestMonitorList(t *testing.T) {
	manager := Manager{
		Monitors: monitors{
			"x": &Monitor{
				Config: Config{
					"id":   "1",
					"name": "2",
				},
			},
			"y": &Monitor{
				Config: Config{
					"id":           "3",
					"name":         "4",
					"enable":       "true",
					"audioEncoder": "x",
					"subInput":     "x",
					"secret":       "x",
				},
			},
		},
	}

	actual := manager.MonitorsInfo()
	expected := Configs{
		"1": Config{
			"audioEnabled":    "false",
			"enable":          "false",
			"id":              "1",
			"name":            "2",
			"subInputEnabled": "false",
		},
		"3": Config{
			"audioEnabled":    "true",
			"enable":          "true",
			"id":              "3",
			"name":            "4",
			"subInputEnabled": "true",
		},
	}

	require.Equal(t, actual, expected)
}

func TestMonitorConfigs(t *testing.T) {
	_, manager, cancel := newTestManager(t)
	defer cancel()

	actual := fmt.Sprintf("%v", manager.MonitorConfigs())
	expected := "map[1:map[audioEncoder:copy enable:false id:1 mainInput:x1 name:one]" +
		" 2:map[enable:false id:2 name:two subInput:x2]]"

	require.Equal(t, actual, expected)
}

func TestStopAllMonitors(t *testing.T) {
	runningMonitor := func() *Monitor {
		return &Monitor{
			eventChan: make(chan storage.Event),
			running:   true,
			WG:        &sync.WaitGroup{},
			cancel:    func() {},
		}
	}
	m := Manager{
		Monitors: map[string]*Monitor{
			"1": runningMonitor(),
			"2": runningMonitor(),
		},
	}

	require.True(t, m.Monitors["1"].running)
	require.True(t, m.Monitors["2"].running)

	m.StopAll()

	require.False(t, m.Monitors["1"].running)
	require.False(t, m.Monitors["2"].running)
}

func mockWaitForNewHLSsegment(context.Context, int) (time.Duration, error) {
	return 0, nil
}

func newMockInputProcess(m *Monitor, isSubInput bool) *InputProcess {
	return &InputProcess{
		isSubInput: isSubInput,
		hlsAddress: "hls.m3u8",

		waitForNewHLSsegment: mockWaitForNewHLSsegment,

		M: m,

		sizeFromStream:   mockSizeFromStream,
		runInputProcess:  mockRunInputProcess,
		newProcess:       ffmock.NewProcess,
		watchdogInterval: 10 * time.Second,
	}
}

func newTestMonitor(t *testing.T) (*Monitor, context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := log.NewMockLogger()
	logger.Start(ctx)

	wg := sync.WaitGroup{}
	videoServer := video.NewServer(logger, &wg, 2021, 2022)

	err := videoServer.Start(ctx)
	require.NoError(t, err)

	tempDir := t.TempDir()
	wg2 := sync.WaitGroup{}
	m := &Monitor{
		Env: storage.ConfigEnv{
			TempDir:    tempDir,
			StorageDir: tempDir + "/storage",
		},
		Config: map[string]string{
			"id":              "test",
			"enable":          "true",
			"videoLength":     "0.0003", // 18ms
			"timestampOffset": "0",
		},
		eventsMu: sync.Mutex{},

		eventChan: make(chan storage.Event),
		running:   true,

		hooks:         mockHooks(),
		NewProcess:    ffmock.NewProcess,
		videoDuration: mockVideoDuration,

		WG:          &wg2,
		Log:         logger,
		videoServer: videoServer,
	}

	cancelFunc := func() {
		cancel()
		wg.Wait()
		wg2.Wait()
		os.RemoveAll(tempDir)
		close(m.eventChan)
	}

	m.mainInput = newMockInputProcess(m, false)
	m.subInput = newMockInputProcess(m, true)

	return m, ctx, cancelFunc
}

func TestStartMonitor(t *testing.T) {
	t.Run("runningErr", func(t *testing.T) {
		m := Monitor{running: true}

		err := m.Start()
		require.ErrorIs(t, err, ErrRunning)
	})
	t.Run("disabled", func(t *testing.T) {
		m, _, cancel := newTestMonitor(t)
		defer cancel()

		m.running = false
		m.Config = map[string]string{"name": "test"}

		feed, cancel2 := m.Log.Subscribe()
		defer cancel2()

		go func() {
			m.Start()
		}()

		actual := <-feed
		require.Equal(t, actual.Msg, "disabled")
	})
}

func mockRunInputProcess(context.Context, *InputProcess) error {
	return nil
}

func mockRunInputProcessErr(context.Context, *InputProcess) error {
	return errors.New("mock")
}

func TestStartInputProcess(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		cancel2()

		feed, cancel3 := m.Log.Subscribe()
		defer cancel3()

		m.WG.Add(1)
		go m.newInputProcess(false).start(ctx, m)

		actual := <-feed
		require.Equal(t, actual.Msg, "main process: stopped")
	})
	t.Run("crashed", func(t *testing.T) {
		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.mainInput.runInputProcess = mockRunInputProcessErr
		m.subInput.runInputProcess = mockRunInputProcessErr

		feed, cancel2 := m.Log.Subscribe()
		defer cancel2()

		m.WG.Add(1)
		go m.mainInput.start(ctx, m)

		actual := <-feed
		require.Equal(t, actual.Msg, "main process: crashed: mock")
	})
}

func newTestInputProcess(t *testing.T) (*InputProcess, context.Context, func()) {
	m, ctx, cancel := newTestMonitor(t)

	i := newMockInputProcess(m, false)
	i.M = m

	return i, ctx, cancel
}

func TestRunInputProcess(t *testing.T) {
	t.Run("sizeFromStream", func(t *testing.T) {
		i, ctx, cancel := newTestInputProcess(t)
		defer cancel()

		err := runInputProcess(ctx, i)
		require.NoError(t, err)
		require.Equal(t, i.width, 123)
		require.Equal(t, i.height, 456)
	})
	t.Run("sizeFromStreamErr", func(t *testing.T) {
		i, ctx, cancel := newTestInputProcess(t)
		defer cancel()

		i.sizeFromStream = mockSizeFromStreamErr

		err := runInputProcess(ctx, i)
		require.Error(t, err)
	})
	t.Run("crashed", func(t *testing.T) {
		i, ctx, cancel := newTestInputProcess(t)
		defer cancel()

		i.newProcess = ffmock.NewProcessErr

		err := runInputProcess(ctx, i)
		require.Error(t, err)
	})
	t.Run("rtspPath", func(t *testing.T) {
		i, ctx, cancel := newTestInputProcess(t)
		i.isSubInput = true

		go runInputProcess(ctx, i)

		time.Sleep(10 * time.Millisecond)

		path := i.M.Config.ID() + "_sub"
		require.True(t, i.M.videoServer.PathExist(path))

		cancel()
		i.M.WG.Wait()

		require.False(t, i.M.videoServer.PathExist(path))
	})

	t.Run("rtspPathErr", func(t *testing.T) {
		i, ctx, cancel := newTestInputProcess(t)
		defer cancel()

		i.M.Config["id"] = ""

		err := runInputProcess(ctx, i)
		require.ErrorIs(t, err, video.ErrEmptyPathName)
	})
}

func mockSizeFromStream(context.Context, string, string) (int, int, error) {
	return 123, 456, nil
}

func mockSizeFromStreamErr(context.Context, string, string) (int, int, error) {
	return 0, 0, errors.New("mock")
}

func mockHooks() Hooks {
	return Hooks{
		Start:      func(context.Context, *Monitor) {},
		StartInput: func(context.Context, *InputProcess, *[]string) {},
		Event:      func(*Monitor, *storage.Event) {},
		RecSave:    func(*Monitor, *string) {},
		RecSaved:   func(*Monitor, string, storage.RecordingData) {},
	}
}

func TestGenInputArgs(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		i := &InputProcess{
			rtspProtocol: "4",
			rtspAddress:  "5",
			M: &Monitor{
				Env: storage.ConfigEnv{},
				Config: map[string]string{
					"logLevel":     "1",
					"mainInput":    "2",
					"audioEncoder": "none",
					"videoEncoder": "3",
				},
			},
		}
		actual := i.generateArgs()
		expected := "-threads 1 -loglevel 1 -i 2 -an -c:v 3 -f rtsp -rtsp_transport 4 5"

		require.Equal(t, expected, actual)
	})
	t.Run("maximal", func(t *testing.T) {
		i := &InputProcess{
			isSubInput:   true,
			hlsAddress:   "7",
			rtspProtocol: "8",
			rtspAddress:  "9",
			M: &Monitor{
				Env: storage.ConfigEnv{},
				Config: map[string]string{
					"logLevel":     "1",
					"hwaccel":      "2",
					"inputOptions": "3",
					"subInput":     "4",
					"audioEncoder": "5",
					"videoEncoder": "6",
				},
			},
		}
		args := i.generateArgs()
		expected := "-threads 1 -loglevel 1 -hwaccel 2 3 -i 4 -c:a 5 -c:v 6 -f rtsp -rtsp_transport 8 9"

		require.Equal(t, expected, args)
	})
}

func TestSendEvent(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		m := &Monitor{running: false}

		err := m.SendEvent(storage.Event{})
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("missingTimeErr", func(t *testing.T) {
		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

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
	t.Run("missingRecDurationErr", func(t *testing.T) {
		m, ctx, cancel := newTestMonitor(t)
		defer cancel()

		m.WG.Add(1)
		go m.startRecorder(ctx)

		actual := m.SendEvent(storage.Event{Time: (time.Unix(1, 0).UTC())}).Error()
		expected := `invalid event: {
 Time: 1970-01-01 00:00:01 +0000 UTC
 Detections: []
 Duration: 0s
 RecDuration: 0s
}
'RecDuration': ` + storage.ErrValueMissing.Error()

		require.Equal(t, actual, expected)
	})
}
