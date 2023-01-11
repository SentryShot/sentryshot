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
	"nvr/pkg/video/hls"

	"github.com/stretchr/testify/require"
)

func prepareDir(t *testing.T) string {
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

	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	return testConfigDir
}

func newTestManager(t *testing.T) (string, *Manager) {
	t.Helper()
	configDir := prepareDir(t)

	manager, err := NewManager(
		configDir,
		storage.ConfigEnv{},
		log.NewDummyLogger(),
		nil,
		&Hooks{Migrate: func(RawConfig) error { return nil }},
	)
	require.NoError(t, err)

	return configDir, manager
}

func readConfig(t *testing.T, path string) RawConfig {
	t.Helper()
	file, err := os.ReadFile(path)
	require.NoError(t, err)

	var c RawConfig
	err = json.Unmarshal(file, &c)
	require.NoError(t, err)

	return c
}

func TestNewManager(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		configDir, manager := newTestManager(t)

		config := readConfig(t, filepath.Join(configDir, "1.json"))
		require.Equal(t, config, manager.rawConfigs["1"])
	})
	t.Run("migration", func(t *testing.T) {
		configDir := prepareDir(t)

		data := []byte(`{"id":"x", "test": "a"}`)
		configPath := configDir + "/x.json"
		err := os.WriteFile(configPath, data, 0o600)
		require.NoError(t, err)

		migrate := func(c RawConfig) error {
			delete(c, "test")
			c["test2"] = "b"
			return nil
		}

		manager, err := NewManager(
			configDir,
			storage.ConfigEnv{},
			&log.Logger{},
			&video.Server{},
			&Hooks{Migrate: migrate},
		)
		require.NoError(t, err)

		actual := manager.rawConfigs["x"]
		expected := RawConfig{"id": "x", "test2": "b"}
		require.Equal(t, expected, actual)

		actual2, err := os.ReadFile(configPath)
		require.NoError(t, err)
		expected2 := `{
    "id": "x",
    "test2": "b"
}`
		require.Equal(t, expected2, string(actual2))
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
			&Hooks{Migrate: func(RawConfig) error { return nil }},
		)
		require.Error(t, err)
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		configDir := prepareDir(t)

		data := []byte("{")
		err := os.WriteFile(configDir+"/1.json", data, 0o600)
		require.NoError(t, err)

		_, err = NewManager(
			configDir,
			storage.ConfigEnv{},
			&log.Logger{},
			&video.Server{},
			&Hooks{Migrate: func(RawConfig) error { return nil }},
		)
		var e *json.SyntaxError
		require.ErrorAs(t, err, &e)
	})
	t.Run("migrationErr", func(t *testing.T) {
		configDir := prepareDir(t)

		data := []byte("{}")
		err := os.WriteFile(configDir+"/1.json", data, 0o600)
		require.NoError(t, err)

		stubErr := errors.New("stub")

		_, err = NewManager(
			configDir,
			storage.ConfigEnv{},
			&log.Logger{},
			&video.Server{},
			&Hooks{Migrate: func(RawConfig) error { return stubErr }},
		)
		require.ErrorIs(t, err, stubErr)
	})
}

func TestMonitorSet(t *testing.T) {
	t.Run("createNew", func(t *testing.T) {
		configDir, manager := newTestManager(t)

		config := manager.rawConfigs["1"]
		config["name"] = "new"

		err := manager.MonitorSet("new", config)
		require.NoError(t, err)

		newName := manager.rawConfigs["new"]["name"]
		require.Equal(t, newName, "new")

		// Check if changes were saved to file.
		savedConfig := readConfig(t, configDir+"/new.json")
		require.Equal(t, manager.rawConfigs["new"], savedConfig)
	})
	t.Run("setOld", func(t *testing.T) {
		configDir, manager := newTestManager(t)

		oldMonitor := manager.rawConfigs["1"]

		oldname := oldMonitor["name"]
		require.Equal(t, oldname, "one")

		config := RawConfig{"name": "two"}

		err := manager.MonitorSet("1", config)
		require.NoError(t, err)

		newName := manager.rawConfigs["1"]["name"]
		require.Equal(t, newName, "two")

		// Check if changes were saved to file.
		savedConfig := readConfig(t, configDir+"/1.json")
		require.Equal(t, manager.rawConfigs["1"], savedConfig)
	})
	t.Run("writeFileErr", func(t *testing.T) {
		_, manager := newTestManager(t)
		manager.path = "/dev/null"

		err := manager.MonitorSet("1", RawConfig{})
		require.Error(t, err)
	})
}

func TestMonitorDelete(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		configDir, manager := newTestManager(t)
		manager.runningMonitors["1"] = &Monitor{}

		err := manager.MonitorDelete("1")
		require.NoError(t, err)

		require.Nil(t, manager.runningMonitors["1"])
		require.NoFileExists(t, filepath.Join(configDir, "1.json"))
	})
	t.Run("existErr", func(t *testing.T) {
		_, manager := newTestManager(t)
		err := manager.MonitorDelete("nil")
		require.ErrorIs(t, err, ErrNotExist)
	})
	t.Run("removeErr", func(t *testing.T) {
		_, manager := newTestManager(t)
		manager.path = "/dev/null"

		err := manager.MonitorDelete("1")
		require.Error(t, err)
	})
}

func TestMonitorList(t *testing.T) {
	manager := Manager{
		rawConfigs: RawConfigs{
			"1": RawConfig{
				"id":   "1",
				"name": "2",
			},
			"2": RawConfig{
				"id":           "3",
				"name":         "4",
				"enable":       "true",
				"audioEncoder": "x",
				"subInput":     "x",
				"secret":       "x",
			},
		},
	}

	actual := manager.MonitorsInfo()
	expected := RawConfigs{
		"1": {
			"audioEnabled":    "false",
			"enable":          "false",
			"id":              "1",
			"name":            "2",
			"subInputEnabled": "false",
		},
		"3": {
			"audioEnabled":    "true",
			"enable":          "true",
			"id":              "3",
			"name":            "4",
			"subInputEnabled": "true",
		},
	}
	require.Equal(t, expected, actual)
}

func TestMonitorConfigs(t *testing.T) {
	_, manager := newTestManager(t)

	actual := manager.MonitorConfigs()
	expected := RawConfigs{
		"1": {
			"audioEncoder": "copy",
			"enable":       "false",
			"id":           "1",
			"mainInput":    "x1",
			"name":         "one",
		},
		"2": {
			"enable":   "false",
			"id":       "2",
			"name":     "two",
			"subInput": "x2",
		},
	}

	require.Equal(t, actual, expected)
}

func TestStartAllMonitors(t *testing.T) {
	_, manager := newTestManager(t)
	manager.StartMonitors()
	manager.StopMonitors()
}

func TestStopAllMonitors(t *testing.T) {
	m := Manager{runningMonitors: map[string]*Monitor{
		"1": {}, "2": {},
	}}
	m.StopMonitors()
	require.Zero(t, len(m.runningMonitors))
}

func TestRestartMonitor(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		_, manager := newTestManager(t)
		err := manager.RestartMonitor("1")
		require.NoError(t, err)

		require.NotNil(t, manager.runningMonitors["1"])
		manager.StopMonitors()
	})
	t.Run("notExistErr", func(t *testing.T) {
		err := new(Manager).RestartMonitor("x")
		require.ErrorIs(t, err, ErrMonitorNotExist)
	})
}

func stubNewVideoServerPath(
	_ context.Context,
	name string,
	_ video.PathConf,
) (*video.ServerPath, error) {
	if name == "" {
		return nil, video.ErrEmptyPathName
	}
	return &video.ServerPath{}, nil
}

func newTestInputProcess() *InputProcess {
	return &InputProcess{
		Config: NewConfig(RawConfig{
			"id": "test",
		}),
		isSubInput: false,

		serverPath: video.ServerPath{
			HlsAddress: "hls.m3u8",
		},

		logf:  func(level log.Level, format string, a ...interface{}) {},
		hooks: stubHooks(),
		WG:    &sync.WaitGroup{},

		newVideoServerPath: stubNewVideoServerPath,
		runInputProcess:    stubRunInputProcess,
		newProcess:         ffmock.NewProcess,
	}
}

func stubHooks() Hooks {
	return Hooks{
		Start:      func(context.Context, *Monitor) {},
		StartInput: func(context.Context, *InputProcess, *[]string) {},
		Event:      func(*Recorder, *storage.Event) {},
		RecSave:    func(*Recorder, *string) {},
		RecSaved:   func(*Recorder, string, storage.RecordingData) {},
	}
}

func stubRunInputProcess(context.Context, *InputProcess) error {
	return nil
}

func newTestMonitor(t *testing.T) *Monitor {
	logf := func(level log.Level, format string, a ...interface{}) {}
	return &Monitor{
		ctx:       context.Background(),
		logf:      logf,
		mainInput: &InputProcess{},
		subInput:  &InputProcess{},
	}
}

func TestStartMonitor(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		logs := make(chan string)
		defer close(logs)

		m := newTestMonitor(t)
		m.Config = NewConfig(RawConfig{"name": "test"})
		m.logf = func(level log.Level, format string, a ...interface{}) {
			logs <- fmt.Sprintf(format, a...)
		}
		go m.start()

		require.Equal(t, "disabled", <-logs)
	})
}

func TestStartInputProcess(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		logs := make(chan string)
		defer close(logs)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		input := newTestInputProcess()
		input.logf = func(level log.Level, format string, a ...interface{}) {
			logs <- fmt.Sprintf(format, a...)
		}
		input.WG.Add(1)
		go input.start(ctx)

		require.Equal(t, "main process: stopped", <-logs)
	})
	t.Run("crashed", func(t *testing.T) {
		logs := make(chan string)
		defer close(logs)

		stubRunInputProcess := func(context.Context, *InputProcess) error {
			return errors.New("stub")
		}
		ctx, cancel := context.WithCancel(context.Background())

		input := newTestInputProcess()
		input.runInputProcess = stubRunInputProcess
		input.logf = func(level log.Level, format string, a ...interface{}) {
			logs <- fmt.Sprintf(format, a...)
		}
		input.WG.Add(1)
		go input.start(ctx)

		require.Equal(t, "main process: crashed: stub", <-logs)
		cancel()
		<-logs
	})
}

func TestRunInputProcess(t *testing.T) {
	t.Run("crashed", func(t *testing.T) {
		i := newTestInputProcess()
		i.newProcess = ffmock.NewProcessErr
		err := runInputProcess(context.Background(), i)
		require.Error(t, err)
	})
	t.Run("rtspPathErr", func(t *testing.T) {
		i := newTestInputProcess()
		i.Config.v["id"] = ""
		err := runInputProcess(context.Background(), i)
		require.ErrorIs(t, err, video.ErrEmptyPathName)
	})
}

func TestGenInputArgs(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		i := &InputProcess{
			Config: NewConfig(RawConfig{
				"logLevel":     "1",
				"mainInput":    "2",
				"audioEncoder": "none",
				"videoEncoder": "3",
			}),
			serverPath: video.ServerPath{
				RtspProtocol: "4",
				RtspAddress:  "5",
			},
		}
		actual := i.generateArgs()
		expected := "-threads 1 -loglevel 1 -i 2 -an -c:v 3 -f rtsp -rtsp_transport 4 5"
		require.Equal(t, expected, actual)
	})
	t.Run("maximal", func(t *testing.T) {
		i := &InputProcess{
			Config: NewConfig(RawConfig{
				"logLevel":     "1",
				"hwaccel":      "2",
				"inputOptions": "3",
				"subInput":     "4",
				"audioEncoder": "5",
				"videoEncoder": "6",
			}),
			isSubInput: true,
			serverPath: video.ServerPath{
				HlsAddress:   "7",
				RtspProtocol: "8",
				RtspAddress:  "9",
			},
		}
		actual := i.generateArgs()
		expected := "-threads 1 -loglevel 1 -hwaccel 2 3 -i 4 -c:a 5 -c:v 6 -f rtsp -rtsp_transport 8 9"
		require.Equal(t, expected, actual)
	})
}

func TestInputStreamInfo(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		mockStreamInfo := &hls.StreamInfo{}
		muxer := newMockMuxerFunc(&mockMuxer{
			streamInfo: mockStreamInfo,
		})
		i := &InputProcess{
			serverPath: video.ServerPath{
				HLSMuxer: muxer,
			},
		}
		actual, err := i.StreamInfo(context.Background())
		require.NoError(t, err)
		require.Equal(t, mockStreamInfo, actual)
	})
	t.Run("getMuxerErr", func(t *testing.T) {
		mockError := errors.New("mock")
		muxer := newMockMuxerFunc(&mockMuxer{
			getMuxerErr: mockError,
		})
		i := &InputProcess{
			serverPath: video.ServerPath{
				HLSMuxer: muxer,
			},
		}
		actual, err := i.StreamInfo(context.Background())
		require.ErrorIs(t, err, mockError)
		require.Nil(t, actual)
	})
}

func TestSendEvent(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		m := &Monitor{ctx: ctx, recorder: &Recorder{}}

		err := m.SendEvent(storage.Event{
			Time:        time.Unix(1, 0),
			RecDuration: 1,
		})
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("missingTimeErr", func(t *testing.T) {
		m := newTestMonitor(t)

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
		m := newTestMonitor(t)

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
