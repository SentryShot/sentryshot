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

package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"nvr/pkg/ffmpeg/ffmock"
	"nvr/pkg/log"
	"nvr/pkg/storage"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type cancelFunc func()

func prepareDir(t *testing.T) (string, cancelFunc) {
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	configDir := tempDir + "/monitors"

	if err := os.Mkdir(configDir, 0700); err != nil {
		t.Fatal(err)
	}

	err = filepath.Walk("./testdata/monitors/", func(path string, info os.FileInfo, _ error) error {
		if !info.IsDir() {
			file, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}
			if err := ioutil.WriteFile(configDir+"/"+info.Name(), file, 0600); err != nil {
				return err
			}

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
	return configDir, cancel
}

func newTestManager(t *testing.T) (string, *Manager, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	logger := log.NewLogger(ctx)

	configDir, cancel2 := prepareDir(t)

	cancelFunc := func() {
		cancel()
		cancel2()
	}

	manager, err := NewMonitorManager(
		configDir,
		&storage.ConfigEnv{},
		logger,
		Hooks{},
	)
	if err != nil {
		t.Fatal(err)
	}

	return configDir, manager, cancelFunc
}

func readConfig(path string) (Config, error) {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if json.Unmarshal(file, &config); err != nil {
		return nil, err
	}
	return config, nil
}

func TestNewManager(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		configDir, manager, cancel := newTestManager(t)
		defer cancel()

		config, err := readConfig(configDir + "/1.json")
		if err != nil {
			t.Fatalf("%v", err)
		}

		expected := fmt.Sprintf("%v", config)
		actual := fmt.Sprintf("%v", manager.Monitors["1"].Config)

		if expected != actual {
			t.Fatalf("expected: %v, got %v", expected, actual)
		}
	})
	t.Run("readFileErr", func(t *testing.T) {
		_, err := NewMonitorManager(
			"/dev/null/nil.json",
			&storage.ConfigEnv{},
			&log.Logger{},
			Hooks{},
		)

		if err == nil {
			t.Fatal("nil")
		}
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		configDir, cancel := prepareDir(t)
		defer cancel()

		data := []byte("{")
		if err := ioutil.WriteFile(configDir+"/1.json", data, 0600); err != nil {
			t.Fatalf("%v", err)
		}

		_, err := NewMonitorManager(
			configDir,
			&storage.ConfigEnv{},
			&log.Logger{},
			Hooks{},
		)

		if err == nil {
			t.Fatal("nil")
		}
	})

}

func TestMonitorSet(t *testing.T) {
	t.Run("createNew", func(t *testing.T) {
		configDir, manager, cancel := newTestManager(t)
		defer cancel()

		config := manager.Monitors["1"].Config
		config["name"] = "new"
		err := manager.MonitorSet("new", config)
		if err != nil {
			t.Fatalf("%v", err)
		}

		newName := manager.Monitors["new"].Config["name"]
		if newName != "new" {
			t.Fatalf("expected: new, got: %v", newName)
		}

		// Check if changes were saved to file.
		config, err = readConfig(configDir + "/new.json")
		if err != nil {
			t.Fatalf("%v", err)
		}

		expected := fmt.Sprintf("%v", manager.Monitors["new"].Config)
		actual := fmt.Sprintf("%v", config)

		if expected != actual {
			t.Fatalf("expected: %v, got %v", expected, actual)
		}
	})
	t.Run("setOld", func(t *testing.T) {
		configDir, manager, cancel := newTestManager(t)
		defer cancel()

		oldMonitor := manager.Monitors["1"]
		oldMonitor.running = true

		oldname := oldMonitor.Config["name"]
		if oldname != "one" {
			t.Fatalf("expected: one, got: %v", oldname)
		}

		config := oldMonitor.Config
		config["name"] = "two"
		err := manager.MonitorSet("1", config)
		if err != nil {
			t.Fatalf("%v", err)
		}

		if !manager.Monitors["1"].running {
			t.Fatal("old monitor was reset")
		}

		newName := manager.Monitors["1"].Config["name"]
		if newName != "two" {
			t.Fatalf("expected: two, got: %v", newName)
		}

		// Check if changes were saved to file.
		config, err = readConfig(configDir + "/1.json")
		if err != nil {
			t.Fatalf("%v", err)
		}

		expected := fmt.Sprintf("%v", manager.Monitors["1"].Config)
		actual := fmt.Sprintf("%v", config)

		if expected != actual {
			t.Fatalf("expected: %v, got %v", expected, actual)
		}
	})
	t.Run("writeFileErr", func(t *testing.T) {
		_, manager, cancel := newTestManager(t)
		defer cancel()

		manager.path = "/dev/null"
		if err := manager.MonitorSet("1", Config{}); err == nil {
			t.Fatal("nil")
		}
	})
}

func TestMonitorDelete(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		_, manager, cancel := newTestManager(t)
		defer cancel()

		if _, exists := manager.Monitors["1"]; !exists {
			t.Fatal("test monitor does not exist")
		}

		if err := manager.MonitorDelete("1"); err != nil {
			t.Fatalf("%v", err)
		}

		if _, exists := manager.Monitors["1"]; exists {
			t.Fatal("monitor was not deleted")
		}
	})
	t.Run("existErr", func(t *testing.T) {
		_, manager, cancel := newTestManager(t)
		defer cancel()

		if err := manager.MonitorDelete("nil"); err == nil {
			t.Fatal("nil")
		}
	})
	t.Run("removeErr", func(t *testing.T) {
		_, manager, cancel := newTestManager(t)
		defer cancel()

		manager.path = "/dev/null"

		if err := manager.MonitorDelete("1"); err == nil {
			t.Fatal("nil")
		}
	})
}
func TestMonitorList(t *testing.T) {
	_, manager, cancel := newTestManager(t)
	defer cancel()

	expected := "map[1:map[audioEnabled:true enable:false id:1 name:one] 2:map[audioEnabled:false enable:false id:2 name:two]]"

	actual := fmt.Sprintf("%v", manager.MonitorList())
	if actual != expected {
		t.Fatalf("\nexpected:\n%v.\ngot:\n%v.", expected, actual)
	}
}

func TestMonitorConfigs(t *testing.T) {
	_, manager, cancel := newTestManager(t)
	defer cancel()

	expected := "map[1:map[audioEncoder:copy enable:false id:1 name:one url:x1] 2:map[enable:false id:2 name:two url:x2]]"

	actual := fmt.Sprintf("%v", manager.MonitorConfigs())
	if actual != expected {
		t.Fatalf("\nexpected:\n%v.\ngot:\n%v", expected, actual)
	}
}

func TestStopAllMonitors(t *testing.T) {
	runningMonitor := func() *Monitor {
		return &Monitor{
			running: true,
			WG:      &sync.WaitGroup{},
			cancel:  func() {},
		}
	}
	m := Manager{
		Monitors: map[string]*Monitor{
			"1": runningMonitor(),
			"2": runningMonitor(),
		},
	}
	if !m.Monitors["1"].running || !m.Monitors["2"].running {
		t.Fatal("monitors are not running")
	}
	m.StopAll()
	if m.Monitors["1"].running || m.Monitors["2"].running {
		t.Fatal("monitors did not stop")
	}
}

func mockWaitForKeyframe(_ context.Context, _ string) (time.Duration, error) {
	return 0, nil
}

func newTestMonitor() (*Monitor, func()) {
	tempDir, _ := ioutil.TempDir("", "")
	ctx, cancel := context.WithCancel(context.Background())

	ctx, cancel2 := context.WithCancel(context.Background())
	logger := log.NewLogger(ctx)

	cancelFunc := func() {
		cancel()
		cancel2()
		os.RemoveAll(tempDir)
	}

	m := &Monitor{
		Env: &storage.ConfigEnv{
			SHMDir:     tempDir,
			StorageDir: tempDir + "/storage",
		},
		Config: map[string]string{
			"enable":          "true",
			"videoLength":     "0.0003", // 18ms
			"timestampOffset": "0",
		},
		Trigger:         make(chan Event),
		running:         true,
		hooks:           mockHooks,
		newProcess:      ffmock.NewProcess,
		sizeFromStream:  mockSizeFromStream,
		waitForKeyframe: mockWaitForKeyframe,
		WG:              &sync.WaitGroup{},
		Log:             logger,
		Ctx:             ctx,
	}
	return m, cancelFunc
}

func TestStartMonitor(t *testing.T) {
	t.Run("runningErr", func(t *testing.T) {
		m := Monitor{running: true}
		if err := m.Start(); err == nil {
			t.Fatal("nil")
		}
	})
	t.Run("disabled", func(t *testing.T) {
		m, cancel := newTestMonitor()
		defer cancel()

		m.running = false
		m.Config = map[string]string{"name": "test"}

		feed, cancel2 := m.Log.Subscribe()
		defer cancel2()

		go func() {
			if err := m.Start(); err != nil {
				t.Fatalf("%v", err)
			}
		}()

		expected := "test: disabled\n"
		actual := <-feed
		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
		}
	})
	t.Run("resetCtx", func(t *testing.T) {
		m, cancel := newTestMonitor()
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		cancel2()

		m.Ctx = ctx
		m.running = false

		if err := m.Start(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if m.Ctx.Err() != nil {
			t.Fatal("context did not reset")
		}
	})
	t.Run("mainProcessCrashed", func(t *testing.T) {
		m, cancel := newTestMonitor()
		defer cancel()

		m.running = false
		m.newProcess = ffmock.NewProcessErr

		feed, cancel2 := m.Log.Subscribe()
		defer cancel2()

		if err := m.Start(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		<-feed
		<-feed

		actual := <-feed
		expected := ": main process: crashed: mock\n"

		if actual != expected {
			t.Fatalf("\nexpected: \n%v \ngot: \n%v", expected, actual)
		}

	})
	t.Run("tmpDirErr", func(t *testing.T) {
		m, cancel := newTestMonitor()
		defer cancel()

		m.running = false
		m.Env.SHMDir = "/dev/null"

		if err := m.Start(); err == nil {
			t.Fatal("nil")
		}
	})
}

func TestStartMainProcess(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		m, cancel := newTestMonitor()
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		cancel2()
		m.Ctx = ctx

		feed, cancel3 := m.Log.Subscribe()
		defer cancel3()

		m.running = false
		m.newProcess = ffmock.NewProcessErr

		m.startMainProcess()

		actual := <-feed
		expected := ": main process: stopped\n"

		if actual != expected {
			t.Fatalf("\nexpected: \n%v \ngot: \n%v", expected, actual)
		}
	})
}

func TestMainProcess(t *testing.T) {
	t.Run("sizeFromStream", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("%v", err)
		}
		defer os.RemoveAll(tempDir)

		m, cancel := newTestMonitor()
		m.Env = &storage.ConfigEnv{SHMDir: tempDir}
		defer cancel()

		go func() {
			if err := m.mainProcess(m.Ctx); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}()

		time.Sleep(10 * time.Millisecond)
		m.mu.Lock()
		actual := m.Size()
		m.mu.Unlock()

		expected := "123x456"
		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
		}
	})
	t.Run("sizeFromStreamErr", func(t *testing.T) {
		m, cancel := newTestMonitor()
		m.sizeFromStream = mockSizeFromStreamErr
		defer cancel()

		if err := m.mainProcess(m.Ctx); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("stopped", func(t *testing.T) {
		m, cancel := newTestMonitor()
		m.newProcess = ffmock.NewProcessNil
		m.cancel = cancel
		defer cancel()

		go func() {
			if err := m.mainProcess(m.Ctx); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}()

		time.Sleep(10 * time.Millisecond)
		m.Stop()

		if m.running {
			t.Fatal("monitor did not stop")
		}
	})
	t.Run("noError", func(t *testing.T) {
		m, cancel := newTestMonitor()
		defer cancel()
		m.cancel = cancel

		if err := m.mainProcess(m.Ctx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestStartRecorder(t *testing.T) {
	const msgFinished = ": recording finished\n"

	t.Run("finished", func(t *testing.T) {
		m, cancel := newTestMonitor()
		defer cancel()

		m.newProcess = ffmock.NewProcessNil

		feed, cancel2 := m.Log.Subscribe()
		defer cancel2()

		go m.startRecorder()
		m.Trigger <- Event{End: time.Now().Add(1 * time.Hour)}

		<-feed
		<-feed
		actual := <-feed
		if actual != msgFinished {
			t.Fatalf("\nexpected: \n%v \ngot: \n%v", msgFinished, actual)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		m, cancel := newTestMonitor()
		defer cancel()

		m.newProcess = ffmock.NewProcess

		feed, cancel2 := m.Log.Subscribe()
		defer cancel2()

		go m.startRecorder()
		m.Trigger <- Event{End: time.Now().Add(time.Duration(-1) * time.Hour)}

		msg := <-feed
		actual1 := msg[:42]
		var actual2 string

		select {
		case msg = <-feed:
			actual2 = msg
		case <-time.After(1 * time.Millisecond):
		}

		expected := ": trigger reached end, stopping recording\n"

		switch {
		case actual1 == expected:
		case actual2 == expected:
		default:
			t.Fatalf("neither first or second output matches expected:\n%v\n%v", actual1, actual2)
		}
	})
	t.Run("timeoutUpdate", func(t *testing.T) {
		m, cancel := newTestMonitor()
		defer cancel()

		feed, cancel2 := m.Log.Subscribe()
		defer cancel2()

		go m.startRecorder()
		m.Trigger <- Event{End: time.Now().Add(10 * time.Millisecond)}
		m.Trigger <- Event{End: time.Now().Add(50 * time.Millisecond)}

		<-feed
		<-feed
		actual := <-feed
		if actual != msgFinished {
			t.Fatalf("\nexpected: \n%v \ngot: \n%v", msgFinished, actual)
		}
	})
	t.Run("recordingCheck", func(t *testing.T) {
		m, cancel := newTestMonitor()
		defer cancel()

		feed, cancel2 := m.Log.Subscribe()
		defer cancel2()

		go m.startRecorder()
		m.Trigger <- Event{End: time.Now().Add(10 * time.Millisecond)}
		m.Trigger <- Event{End: time.Now().Add(11 * time.Millisecond)}
		m.Trigger <- Event{End: time.Now().Add(0 * time.Millisecond)}

		<-feed
		<-feed

		expected := ": saving recording: /tmp/"

		msg := <-feed
		actual := msg[:25]
		if actual != expected {
			t.Fatalf("\nexpected: \n%v \ngot: \n%v", expected, actual)
		}
	})
}

func TestStartRecording(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		m, cancel := newTestMonitor()
		defer cancel()

		feed, cancel2 := m.Log.Subscribe()
		defer cancel2()

		// Cancel the recording and not the monitor.
		ctx2, cancel3 := context.WithCancel(context.Background())
		cancel3()

		m.WG.Add(1)
		go m.startRecording(ctx2)

		expected := ": recording stopped\n"

		actual := <-feed
		if actual != expected {
			t.Fatalf("\nexpected: \n%v \ngot: \n%v", expected, actual)
		}
	})
	t.Run("waitForKeyframeErr", func(t *testing.T) {
		mockWaitForKeyframeErr := func(_ context.Context, _ string) (time.Duration, error) {
			return 0, errors.New("e")
		}

		m, cancel := newTestMonitor()
		defer cancel()

		m.newProcess = ffmock.NewProcess
		m.waitForKeyframe = mockWaitForKeyframeErr

		feed, cancel := m.Log.Subscribe()
		defer cancel()

		m.WG.Add(1)
		go m.startRecording(m.Ctx)

		expected := ": recording process: could not get keyframe duration: e\n"

		actual := <-feed
		if actual != expected {
			t.Fatalf("\nexpected: \n%v \ngot: \n%v", expected, actual)
		}
	})
}

func TestRecordingProcess(t *testing.T) {
	t.Run("mkdirErr", func(t *testing.T) {
		m, cancel := newTestMonitor()
		defer cancel()

		m.Env = &storage.ConfigEnv{
			StorageDir: "/dev/null",
		}

		if err := m.recordingProcess(m.Ctx); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("genArgsErr", func(t *testing.T) {
		m, cancel := newTestMonitor()
		defer cancel()

		m.Config["videoLength"] = ""

		if err := m.recordingProcess(m.Ctx); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("parseOffsetErr", func(t *testing.T) {
		m, cancel := newTestMonitor()
		defer cancel()

		m.Config["timestampOffset"] = ""

		if err := m.recordingProcess(m.Ctx); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("crashed", func(t *testing.T) {
		m, cancel := newTestMonitor()
		defer cancel()
		m.newProcess = ffmock.NewProcessErr

		if err := m.recordingProcess(m.Ctx); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func mockSizeFromStream(string) (string, error) {
	return "123x456", nil
}
func mockSizeFromStreamErr(string) (string, error) {
	return "", errors.New("mock")
}

var mockHooks = Hooks{
	Start:     func(_ *Monitor) {},
	StartMain: func(_ *Monitor, _ *string) {},
}

func TestGenMainArgs(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		m := Monitor{
			Env: &storage.ConfigEnv{},
			Config: map[string]string{
				"logLevel":     "1",
				"url":          "2",
				"audioEncoder": "3",
				"videoEncoder": "4",
				"id":           "id",
			},
		}
		actual := m.generateMainArgs()
		expected := "-loglevel 1 -i 2 -c:a 3 -c:v 4 -preset veryfast -f hls -hls_flags" +
			" delete_segments -hls_list_size 2 -hls_allow_cache 0 /hls/id/id.m3u8"
		if actual != expected {
			t.Fatalf("\nexpected: \n%v \ngot \n%v", expected, actual)
		}
	})
	t.Run("hwaccel", func(t *testing.T) {
		m := Monitor{
			Env: &storage.ConfigEnv{},
			Config: map[string]string{
				"logLevel":     "1",
				"hwaccel":      "2",
				"url":          "3",
				"audioEncoder": "4",
				"videoEncoder": "5",
				"id":           "id",
			},
		}
		actual := m.generateMainArgs()
		expected := "-loglevel 1 -hwaccel 2 -i 3 -c:a 4 -c:v 5 -preset veryfast -f hls -hls_flags" +
			" delete_segments -hls_list_size 2 -hls_allow_cache 0 /hls/id/id.m3u8"
		if actual != expected {
			t.Fatalf("\nexpected: \n%v \ngot \n%v", expected, actual)
		}
	})
}

func TestGenRecorderArgs(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		m := Monitor{
			Env: &storage.ConfigEnv{},
			Config: map[string]string{
				"logLevel":    "1",
				"videoLength": "2",
				"id":          "id",
			},
		}
		actual, err := m.generateRecorderArgs("path", "/hls")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "-y -loglevel 1 -live_start_index -1 -i /hls -t 120 -c:v copy path.mp4"
		if actual != expected {
			t.Fatalf("\nexpected: \n%v \ngot \n%v", expected, actual)
		}
	})
	t.Run("videoLengthErr", func(t *testing.T) {
		m := Monitor{
			Env: &storage.ConfigEnv{},
		}
		_, err := m.generateRecorderArgs("path", "")
		if err == nil {
			t.Fatal("nil")
		}
	})
}
