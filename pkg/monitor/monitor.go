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
	"nvr/pkg/ffmpeg"
	"nvr/pkg/log"
	"nvr/pkg/storage"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Config Monitor configuration.
type Config map[string]string

type StartHook func(context.Context, *Monitor)
type StartMainHook func(context.Context, *Monitor, *string)

// Hook monitor start addon hook.
type Hooks struct {
	Start     StartHook
	StartMain StartMainHook
}

// Event is a recording trigger event.
type Event struct {
	End time.Time // End time of recording
}

// Manager for the monitors.
type Manager struct {
	Monitors map[string]*Monitor
	env      *storage.ConfigEnv
	log      *log.Logger
	path     string
	hooks    Hooks
	mu       sync.Mutex
}

// NewMonitorManager return new monitors configuration.
func NewMonitorManager(configPath string, env *storage.ConfigEnv, log *log.Logger, hooks Hooks) (*Manager, error) {
	configFiles, err := readConfigs(configPath)
	if err != nil {
		return nil, fmt.Errorf("could not read configuration files: %s", err)
	}

	manager := &Manager{
		env:   env,
		log:   log,
		path:  configPath,
		hooks: hooks,
	}

	monitors := make(map[string]*Monitor)
	for _, file := range configFiles {
		var config Config
		if err := json.Unmarshal(file, &config); err != nil {
			return nil, fmt.Errorf("could not unmarshal config: %v: %s", err, file)
		}
		monitors[config["id"]] = manager.newMonitor(config)
	}
	manager.Monitors = monitors

	return manager, nil
}

func readConfigs(path string) ([][]byte, error) {
	var files [][]byte
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if strings.Contains(path, ".json") {
			file, err := ioutil.ReadFile(path)
			if err != nil {
				return fmt.Errorf("could not read file: %s %v", path, err)
			}
			files = append(files, file)
		}
		return nil
	})
	return files, err
}

func (m *Manager) newMonitor(config Config) *Monitor {
	return &Monitor{
		Env:     m.env,
		Config:  config,
		Trigger: make(Trigger),

		hooks:               m.hooks,
		runMainProcess:      runMainProcess,
		startRecording:      startRecording,
		runRecordingProcess: runRecordingProcess,
		newProcess:          ffmpeg.NewProcess,
		sizeFromStream:      ffmpeg.New(m.env.FFmpegBin).SizeFromStream,
		waitForKeyframe:     ffmpeg.WaitForKeyframe,
		watchdogInterval:    10 * time.Second,

		WG:  &sync.WaitGroup{},
		Log: m.log,
	}
}

// Trigger recording using event.
type Trigger chan Event

// Monitor service.
type Monitor struct {
	Env    *storage.ConfigEnv
	Config Config

	Trigger Trigger

	running   bool
	recording bool

	hooks               Hooks
	runMainProcess      runMainProcessFunc
	startRecording      startRecordingFunc
	runRecordingProcess runRecordingProcessFunc
	newProcess          ffmpeg.NewProcessFunc
	sizeFromStream      ffmpeg.SizeFromStreamFunc
	waitForKeyframe     ffmpeg.WaitForKeyframeFunc
	watchdogInterval    time.Duration

	mu     sync.Mutex
	WG     *sync.WaitGroup
	Log    *log.Logger
	cancel func()
}

// MonitorSet sets config for specified monitor.
func (m *Manager) MonitorSet(id string, c Config) error {
	defer m.mu.Unlock()
	m.mu.Lock()

	monitor, exist := m.Monitors[id]
	if exist {
		monitor.mu.Lock()
		monitor.Config = c
		monitor.mu.Unlock()
	} else {
		monitor = m.newMonitor(c)
		m.Monitors[id] = monitor
	}

	// Update file.
	monitor.mu.Lock()
	config, _ := json.MarshalIndent(monitor.Config, "", "    ")

	if err := ioutil.WriteFile(m.configPath(id), config, 0600); err != nil {
		return err
	}
	monitor.mu.Unlock()

	return nil
}

// MonitorDelete deletes monitor by id.
func (m *Manager) MonitorDelete(id string) error {
	defer m.mu.Unlock()
	m.mu.Lock()
	monitors := m.Monitors

	monitor, exists := monitors[id]
	if !exists {
		return errors.New("monitor does not exist")
	}
	monitor.Stop()

	delete(m.Monitors, id)

	if err := os.Remove(m.configPath(id)); err != nil {
		return err
	}

	return nil
}

// MonitorList returns values needed for live page.
func (m *Manager) MonitorList() map[string]Config {
	configs := make(map[string]Config)
	m.mu.Lock()
	for _, monitor := range m.Monitors {
		monitor.mu.Lock()
		c := monitor.Config
		monitor.mu.Unlock()

		audioEnabled := "false"
		if monitor.audioEnabled() {
			audioEnabled = "true"
		}

		configs[c["id"]] = Config{
			"id":           c["id"],
			"name":         c["name"],
			"enable":       c["enable"],
			"audioEnabled": audioEnabled,
		}
	}
	m.mu.Unlock()
	return configs
}

func (m *Manager) configPath(id string) string {
	return m.path + "/" + id + ".json"
}

// MonitorConfigs returns configurations for all monitors.
func (m *Manager) MonitorConfigs() map[string]Config {
	configs := make(map[string]Config)

	m.mu.Lock()
	for _, monitor := range m.Monitors {
		monitor.mu.Lock()
		configs[monitor.Config["id"]] = monitor.Config
		monitor.mu.Unlock()
	}
	m.mu.Unlock()

	return configs
}

// Start monitor.
func (m *Monitor) Start() error {
	defer m.mu.Unlock()
	m.mu.Lock()
	if m.running {
		return fmt.Errorf("monitor already running")
	}
	m.running = true

	if !m.isEnabled() {
		m.Log.Printf("%v: disabled\n", m.Name())
		return nil
	}

	m.Log.Printf("%v: starting\n", m.Name())

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	tmpDir := m.Env.SHMhls() + "/" + m.ID()

	os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return fmt.Errorf("could not create temporary directory for HLS files: %v: %v", tmpDir, err)
	}

	if m.alwaysRecord() {
		never := time.Unix(1<<63-62135596801, 999999999)
		go func() {
			select {
			case <-ctx.Done():
			case <-time.After(15 * time.Second):
				m.Trigger <- Event{End: never}
			}
		}()
	}

	m.hooks.Start(ctx, m)

	m.WG.Add(2)
	go m.startMainProcess(ctx)
	go m.startRecorder(ctx)

	return nil
}

func (m *Monitor) startMainProcess(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			m.mu.Lock()

			m.running = false
			m.Log.Printf("%v: main process: stopped\n", m.Name())
			m.WG.Done()

			m.mu.Unlock()
			return
		}
		if err := m.runMainProcess(ctx, m); err != nil {
			m.Log.Printf("%v: main process: crashed: %v\n", m.Name(), err)
			time.Sleep(1 * time.Second)
			continue
		}
	}
}

type runMainProcessFunc func(context.Context, *Monitor) error

func runMainProcess(ctx context.Context, m *Monitor) error {
	var process ffmpeg.Process

	size, err := m.sizeFromStream(m.URL())
	if err != nil {
		return fmt.Errorf("could not get size of stream: %v", err)
	}
	m.mu.Lock()
	m.Config["size"] = size
	m.mu.Unlock()

	args := m.generateMainArgs()

	m.hooks.StartMain(ctx, m, &args)

	cmd := exec.Command(m.Env.FFmpegBin, ffmpeg.ParseArgs(args)...)

	m.Log.Printf("%v: starting main process: %v\n", m.Name(), cmd)

	process = m.newProcess(cmd)
	process.SetTimeout(10 * time.Second)
	process.SetPrefix(m.Name() + ": main process: ")
	process.SetStdoutLogger(m.Log)
	process.SetStderrLogger(m.Log)

	go m.startWatchdog(ctx, process)

	err = process.Start(ctx)
	if err != nil {
		return fmt.Errorf("crashed: %v", err)
	}

	return nil
}

func (m *Monitor) generateMainArgs() string {
	hwaccel := ""
	if m.Config["hwaccel"] != "" {
		hwaccel = " -hwaccel " + m.Config["hwaccel"]
	}

	audioEncoder := " -an" // Skip audio.
	if m.audioEnabled() {
		audioEncoder = " -c:a " + m.Config["audioEncoder"]
	}

	return "-loglevel " + m.Config["logLevel"] + hwaccel +
		" -i " + m.URL() + // Input.
		audioEncoder +
		" -c:v " + m.Config["videoEncoder"] + " -preset veryfast" + // Video encoder.
		" -f hls -hls_flags delete_segments -hls_list_size 2 -hls_allow_cache 0 " + // HLS settings.
		m.Env.SHMDir + "/hls/" + m.ID() + "/" + m.ID() + ".m3u8" // HLS output.
}

// startWatchdog starts a watchdog that detects if the mainProcess freezes.
// Freeze is detected by polling the output HLS manifest for file updates.
func (m *Monitor) startWatchdog(ctx context.Context, process ffmpeg.Process) {
	watchFile := func() error {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return err
		}
		defer watcher.Close()

		err = watcher.Add(m.hlsPath())
		if err != nil {
			return err
		}
		for {
			select {
			case <-watcher.Events: // file updated, process not frozen.
				return nil
			case <-time.After(m.watchdogInterval):
				return errors.New("possible freeze detected, restarting")
			case err := <-watcher.Errors:
				return err
			case <-ctx.Done():
				return nil
			}
		}
	}
	for {
		select {
		case <-time.After(m.watchdogInterval):
		case <-ctx.Done():
			return
		}
		go func() {
			if err := watchFile(); err != nil {
				m.Log.Printf("%v: main process: watchdog: %v\n", m.Name(), err)
				process.Stop()
			}
		}()
	}
}

func (m *Monitor) startRecorder(ctx context.Context) {
	var triggerTimeout *time.Timer
	var timeout time.Time

	for {
		select {
		case <-ctx.Done():
			if triggerTimeout != nil {
				triggerTimeout.Stop()
			}
			m.WG.Done()
			return
		case event := <-m.Trigger: // Wait for trigger.
			m.mu.Lock()
			if m.recording {
				if event.End.After(timeout) {
					triggerTimeout.Reset(time.Until(event.End))
					timeout = event.End
				}
				m.mu.Unlock()
				continue
			}

			ctx2, cancel := context.WithCancel(ctx)

			// Stops recording when timeout is reached.
			triggerTimeout = time.AfterFunc(time.Until(event.End), func() {
				m.Log.Printf("%v: trigger reached end, stopping recording\n", m.Name())
				cancel()
			})
			m.WG.Add(1)

			m.recording = true
			m.mu.Unlock()

			go m.startRecording(ctx2, m)
		}
	}
}

type startRecordingFunc func(context.Context, *Monitor)

func startRecording(ctx context.Context, m *Monitor) {
	for {
		if ctx.Err() != nil {
			m.mu.Lock()

			m.recording = false
			m.Log.Printf("%v: recording stopped\n", m.Name())
			m.WG.Done()

			m.mu.Unlock()
			return
		}
		if err := m.runRecordingProcess(ctx, m); err != nil {
			m.Log.Printf("%v: recording process: %v\n", m.Name(), err)
			time.Sleep(1 * time.Second)
			continue
		}
	}
}

type runRecordingProcessFunc func(context.Context, *Monitor) error

func runRecordingProcess(ctx context.Context, m *Monitor) error {
	keyFrameDuration, err := m.waitForKeyframe(ctx, m.hlsPath())
	if err != nil {
		return fmt.Errorf("could not get keyframe duration: %v", err)
	}

	timestampOffset, err := strconv.Atoi(m.Config["timestampOffset"])
	if err != nil {
		return fmt.Errorf("could not parse timestamp offset %v", err)
	}

	offset := keyFrameDuration + time.Duration(timestampOffset)*time.Millisecond
	startTime := time.Now().UTC().Add(-offset)

	fileDir := m.Env.StorageDir + "/recordings/" + startTime.Format("2006/01/02/") + m.ID() + "/"
	filePath := fileDir + startTime.Format("2006-01-02_15-04-05_") + m.ID()

	if err := os.MkdirAll(fileDir, 0755); err != nil && err != os.ErrExist {
		return fmt.Errorf("could not make directory for video: %v", err)
	}

	args, err := m.generateRecorderArgs(filePath)
	if err != nil {
		return err
	}
	cmd := exec.Command(m.Env.FFmpegBin, ffmpeg.ParseArgs(args)...)

	process := m.newProcess(cmd)
	process.SetTimeout(10 * time.Second)
	process.SetStdoutLogger(m.Log)
	process.SetStderrLogger(m.Log)

	m.mu.Lock()
	process.SetPrefix(m.Name() + ": recording process: ")
	m.Log.Printf("%v: starting recording: %v\n", m.Name(), cmd)
	m.mu.Unlock()

	err = process.Start(ctx)

	if err := m.saveRecording(filePath, startTime); err != nil {
		m.Log.Printf("%v: could not save recording: %v\n", m.Name(), err)
	}

	if err != nil {
		return fmt.Errorf("crashed: %v", err)
	}

	m.Log.Printf("%v: recording finished\n", m.Name())
	return nil
}

func (m *Monitor) generateRecorderArgs(filePath string) (string, error) {
	videoLength, err := strconv.ParseFloat(m.Config["videoLength"], 64)
	if err != nil {
		return "", fmt.Errorf("%v: could not parse video length: %v", m.Name(), err)
	}
	videoLengthSec := strconv.Itoa((int(videoLength * 60)))

	args := "-y -loglevel " + m.Config["logLevel"] +
		" -live_start_index -1" + // HLS segment to start from.
		" -i " + m.hlsPath() + // Input.
		" -t " + videoLengthSec + // Max video length.
		" -c:v copy " + filePath + ".mp4" // Output.

	return args, nil
}

// RecData recording data marshaled to json and saved next to video and thumbnail.
type RecData struct {
	Start time.Time `json:"start"`
}

func (m *Monitor) saveRecording(filePath string, startTime time.Time) error {
	videoPath := filePath + ".mp4"
	thumbPath := filePath + ".jpeg"
	dataPath := filePath + ".json"

	m.Log.Printf("%v: saving recording: %v\n", m.Name(), videoPath)
	args := "-n -loglevel " + m.Config["logLevel"] + // LogLevel.
		" -i " + videoPath + // Input.
		" -frames:v 1 " + thumbPath // Output.

	cmd := exec.Command(m.Env.FFmpegBin, ffmpeg.ParseArgs(args)...)

	process := m.newProcess(cmd)
	process.SetPrefix(m.Name() + ": thumbnail process: ")
	process.SetStdoutLogger(m.Log)
	process.SetStderrLogger(m.Log)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := process.Start(ctx); err != nil {
		os.Remove(videoPath)
		os.Remove(thumbPath)
		return fmt.Errorf("could not generate thumbnail for %v: %v", videoPath, err)
	}

	data := RecData{Start: startTime}
	json, _ := json.MarshalIndent(data, "", "    ")

	ioutil.WriteFile(dataPath, json, 0600) //nolint:errcheck
	return nil
}

// Stop monitor.
func (m *Monitor) Stop() {
	m.mu.Lock()
	m.running = false
	m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
	}
	m.WG.Wait()
}

// StopAll monitors.
func (m *Manager) StopAll() {
	m.mu.Lock()
	for _, monitor := range m.Monitors {
		monitor.Stop()
	}
	m.mu.Unlock()
}

func (m *Monitor) isEnabled() bool {
	return m.Config["enable"] == "true"
}
func (m *Monitor) alwaysRecord() bool {
	return m.Config["alwaysRecord"] == "true"
}

// ID returns id of monitor.
func (m *Monitor) ID() string {
	return m.Config["id"]
}

// Name returns name of monitor.
func (m *Monitor) Name() string {
	return m.Config["name"]
}

// URL returns input url of monitor.
func (m *Monitor) URL() string {
	return m.Config["url"]
}

// Size returns input stream size of monitor.
func (m *Monitor) Size() string {
	return m.Config["size"]
}

func (m *Monitor) audioEnabled() bool {
	switch m.Config["audioEncoder"] {
	case "":
		return false
	case "none":
		return false
	}
	return true
}

func (m *Monitor) hlsPath() string {
	return m.Env.SHMhls() + "/" + m.ID() + "/" + m.ID() + ".m3u8"
}
