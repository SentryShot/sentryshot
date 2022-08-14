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
	"nvr/pkg/ffmpeg"
	"nvr/pkg/log"
	"nvr/pkg/storage"
	"nvr/pkg/video"
	"nvr/pkg/video/hls"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// StartHook is called when monitor start.
type StartHook func(context.Context, *Monitor)

// StartInputHook is called when input process start.
type StartInputHook func(context.Context, *InputProcess, *[]string)

// EventHook is called on every event.
type EventHook func(*Recorder, *storage.Event)

// RecSaveHook is called when recording is saved.
type RecSaveHook func(*Recorder, *string)

// RecSavedHook is called after recording have been saved successfully.
type RecSavedHook func(*Recorder, string, storage.RecordingData)

// MigationHook is called when each monitor config is loaded.
type MigationHook func(Config) error

// Hooks monitor hooks.
type Hooks struct {
	Start      StartHook
	StartInput StartInputHook
	Event      EventHook
	RecSave    RecSaveHook
	RecSaved   RecSavedHook
	Migrate    MigationHook
}

// Manager for the monitors.
type Manager struct {
	Monitors    monitors
	env         storage.ConfigEnv
	log         *log.Logger
	videoServer *video.Server
	path        string
	hooks       Hooks
	mu          sync.Mutex
}

// NewManager return new monitor manager.
func NewManager(
	configPath string,
	env storage.ConfigEnv,
	log *log.Logger,
	videoServer *video.Server,
	hooks *Hooks,
) (*Manager, error) {
	if err := os.MkdirAll(configPath, 0o700); err != nil {
		return nil, fmt.Errorf("create monitors directory: %w", err)
	}

	configFiles, err := readConfigs(configPath)
	if err != nil {
		return nil, fmt.Errorf("read configuration files: %w", err)
	}

	manager := &Manager{
		env:         env,
		log:         log,
		videoServer: videoServer,
		path:        configPath,
		hooks:       *hooks,
	}

	monitors := make(monitors)
	for _, file := range configFiles {
		var config Config
		if err := json.Unmarshal(file, &config); err != nil {
			return nil, fmt.Errorf("unmarshal config: %w: %v", err, string(file))
		}
		if err := hooks.Migrate(config); err != nil {
			return nil, fmt.Errorf("migration failed: %w", err)
		}

		configPath := manager.configPath(config.ID())
		migratedConf, _ := json.MarshalIndent(config, "", "    ")
		err := os.WriteFile(configPath, migratedConf, 0o600)
		if err != nil {
			return nil, fmt.Errorf("write migrated config: %w", err)
		}
		monitors[config.ID()] = manager.newMonitor(config)
	}
	manager.Monitors = monitors

	return manager, nil
}

func readConfigs(path string) ([][]byte, error) {
	var files [][]byte
	fileSystem := os.DirFS(path)
	err := fs.WalkDir(fileSystem, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.Contains(path, ".json") {
			return nil
		}
		file, err := fs.ReadFile(fileSystem, path)
		if err != nil {
			return fmt.Errorf("read file: %v %w", path, err)
		}
		files = append(files, file)
		return nil
	})
	return files, err
}

// MonitorSet sets config for specified monitor.
func (m *Manager) MonitorSet(id string, c Config) error {
	defer m.mu.Unlock()
	m.mu.Lock()

	monitor, exist := m.Monitors[id]
	if exist {
		monitor.Lock.Lock()
		monitor.Config = c
		monitor.Lock.Unlock()
	} else {
		monitor = m.newMonitor(c)
		m.Monitors[id] = monitor
	}

	// Update file.
	monitor.Lock.Lock()
	config, _ := json.MarshalIndent(monitor.Config, "", "    ")

	if err := os.WriteFile(m.configPath(id), config, 0o600); err != nil {
		return err
	}
	monitor.Lock.Unlock()

	return nil
}

// ErrNotExist monitor does not exist.
var ErrNotExist = errors.New("monitor does not exist")

// MonitorDelete deletes monitor by id.
func (m *Manager) MonitorDelete(id string) error {
	defer m.mu.Unlock()
	m.mu.Lock()
	monitors := m.Monitors

	monitor, exists := monitors[id]
	if !exists {
		return ErrNotExist
	}
	monitor.Stop()

	delete(m.Monitors, id)

	if err := os.Remove(m.configPath(id)); err != nil {
		return err
	}

	return nil
}

// MonitorsInfo returns common information about the monitors.
// This will be accessesable by normal users.
func (m *Manager) MonitorsInfo() Configs {
	configs := make(map[string]Config)
	m.mu.Lock()
	for _, monitor := range m.Monitors {
		monitor.Lock.Lock()
		c := monitor.Config
		monitor.Lock.Unlock()

		enable := "false"
		if c.enabled() {
			enable = "true"
		}

		audioEnabled := "false"
		if c.audioEnabled() {
			audioEnabled = "true"
		}

		subInputEnabled := "false"
		if c.SubInputEnabled() {
			subInputEnabled = "true"
		}

		configs[c.ID()] = Config{
			"id":              c.ID(),
			"name":            c.Name(),
			"enable":          enable,
			"audioEnabled":    audioEnabled,
			"subInputEnabled": subInputEnabled,
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
		monitor.Lock.Lock()
		configs[monitor.Config.ID()] = monitor.Config
		monitor.Lock.Unlock()
	}
	m.mu.Unlock()

	return configs
}

// monitors map.
type monitors map[string]*Monitor

// Monitor service.
type Monitor struct {
	running bool

	Config Config
	Lock   sync.Mutex

	Env         storage.ConfigEnv
	Log         *log.Logger
	videoServer *video.Server

	mainInput *InputProcess
	subInput  *InputProcess
	recorder  *Recorder
	Recorder
	hooks      Hooks
	NewProcess ffmpeg.NewProcessFunc
	logf       logFunc

	WG     *sync.WaitGroup
	cancel func()
}

type (
	logFunc func(log.Level, string, ...interface{})
)

func (m *Manager) newMonitor(config Config) *Monitor {
	logf := func(level log.Level, format string, a ...interface{}) {
		m.log.Level(level).Src("monitor").Monitor(config.ID()).Msgf(format, a...)
	}

	monitor := &Monitor{
		Env:         m.env,
		Log:         m.log,
		videoServer: m.videoServer,
		Config:      config,

		hooks:      m.hooks,
		NewProcess: ffmpeg.NewProcess,
		logf:       logf,

		WG: &sync.WaitGroup{},
	}
	monitor.mainInput = newInputProcess(monitor, false)
	monitor.subInput = newInputProcess(monitor, true)
	monitor.recorder = newRecorder(monitor)

	return monitor
}

// ErrRunning monitor is already running.
var ErrRunning = errors.New("monitor is aleady running")

// Start monitor.
func (m *Monitor) Start() error {
	defer m.Lock.Unlock()
	m.Lock.Lock()
	if m.running {
		return ErrRunning
	}
	m.running = true

	if !m.Config.enabled() {
		m.logf(log.LevelInfo, "disabled")
		return nil
	}

	m.logf(log.LevelInfo, "starting")

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	if m.Config.alwaysRecord() {
		infinte := time.Duration(1<<63 - 62135596801)
		go func() {
			select {
			case <-ctx.Done():
			case <-time.After(15 * time.Second):
				err := m.SendEvent(storage.Event{
					Time:        time.Now(),
					RecDuration: infinte,
				})
				if err != nil {
					m.logf(log.LevelError, "could not start continuous recording: %v", err)
				}
			}
		}()
	}

	m.hooks.Start(ctx, m)

	m.WG.Add(1)
	go m.mainInput.start(ctx)

	if m.Config.SubInputEnabled() {
		m.WG.Add(1)
		go m.subInput.start(ctx)
	}

	m.WG.Add(1)
	go m.recorder.start(ctx)

	return nil
}

// SendEventFunc send event signature.
type SendEventFunc func(storage.Event) error

// SendEvent sends event to recorder.
func (m *Monitor) SendEvent(event storage.Event) error {
	m.Lock.Lock()
	if !m.running {
		m.Lock.Unlock()
		return context.Canceled
	}
	m.Lock.Unlock()
	return m.recorder.sendEvent(event)
}

// Stop monitor.
func (m *Monitor) Stop() {
	m.Lock.Lock()
	m.running = false
	m.Lock.Unlock()

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

// InputProcess monitor input process.
type InputProcess struct {
	Config      Config
	MonitorLock *sync.Mutex

	serverPath video.ServerPath

	isSubInput bool

	cancel func()

	hooks     Hooks
	Env       storage.ConfigEnv
	Log       *log.Logger
	WG        *sync.WaitGroup
	SendEvent SendEventFunc

	logf               logFunc
	newVideoServerPath newVideoServerPathFunc
	runInputProcess    runInputProcessFunc
	newProcess         ffmpeg.NewProcessFunc
}

type newVideoServerPathFunc func(string, video.PathConf) (*video.ServerPath, video.CancelFunc, error)

type runInputProcessFunc func(context.Context, *InputProcess) error

func newInputProcess(m *Monitor, isSubInput bool) *InputProcess {
	i := &InputProcess{
		Config:      m.Config,
		MonitorLock: &m.Lock,

		isSubInput: isSubInput,

		hooks:     m.hooks,
		Env:       m.Env,
		Log:       m.Log,
		WG:        m.WG,
		SendEvent: m.SendEvent,

		logf:               m.logf,
		newVideoServerPath: m.videoServer.NewPath,
		runInputProcess:    runInputProcess,
		newProcess:         ffmpeg.NewProcess,
	}

	return i
}

// IsSubInput if the input is the sub stream.
func (i *InputProcess) IsSubInput() bool {
	return i.isSubInput
}

// HLSaddress internal HLS address.
func (i *InputProcess) HLSaddress() string {
	return i.serverPath.HlsAddress
}

// RTSPaddress internal RTSP address.
func (i *InputProcess) RTSPaddress() string {
	return i.serverPath.RtspAddress
}

// RTSPprotocol protocol used by RTSP address.
func (i *InputProcess) RTSPprotocol() string {
	return i.serverPath.RtspProtocol
}

// StreamInfo returns the stream information of the input.
func (i *InputProcess) StreamInfo(ctx context.Context) (*hls.StreamInfo, error) {
	// It may take a few seconds for the stream to
	// become available after the monitor started.
	for {
		if ctx.Err() != nil {
			return nil, context.Canceled
		}
		// Returns nil if the stream isn't available yet.
		info, err := i.serverPath.StreamInfo()
		if err != nil {
			return nil, err
		}
		if info == nil {
			i.logf(log.LevelDebug, "could not get stream info")
			select {
			case <-time.After(3 * time.Second):
				continue
			case <-ctx.Done():
				return nil, context.Canceled
			}
		}
		return info, nil
	}
}

// ProcessName name of process "main" or "sub".
func (i *InputProcess) ProcessName() string {
	if i.isSubInput {
		return "sub"
	}
	return "main"
}

func (i *InputProcess) input() string {
	if i.IsSubInput() {
		return i.Config.SubInput()
	}
	return i.Config.MainInput()
}

func (i *InputProcess) rtspPathName() string {
	if i.isSubInput {
		return i.Config.ID() + "_sub"
	}
	return i.Config.ID()
}

// WaitForNewHLSsegment waits for a new HLS segment and
// returns the combined duration of the last nSegments.
// Used to calculate start time of the recordings.
func (i *InputProcess) WaitForNewHLSsegment(
	ctx context.Context, nSegments int,
) (time.Duration, error) {
	return i.serverPath.WaitForNewHLSsegment(ctx, nSegments)
}

// Cancel process context.
func (i *InputProcess) Cancel() {
	i.cancel()
}

func (i *InputProcess) start(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			i.logf(log.LevelInfo, "%v process: stopped", i.ProcessName())
			i.WG.Done()
			return
		}

		if err := i.runInputProcess(ctx, i); err != nil {
			i.logf(log.LevelError, "%v process: crashed: %v", i.ProcessName(), err)
			select {
			case <-ctx.Done():
			case <-time.After(1 * time.Second):
			}
			continue
		}
	}
}

func runInputProcess(ctx context.Context, i *InputProcess) error {
	i.MonitorLock.Lock()
	pathConf := video.PathConf{MonitorID: i.Config.ID(), IsSub: i.IsSubInput()}
	i.MonitorLock.Unlock()

	serverPath, cancel, err := i.newVideoServerPath(i.rtspPathName(), pathConf)
	if err != nil {
		return fmt.Errorf("add path to RTSP server: %w", err)
	}
	defer cancel()
	i.serverPath = *serverPath

	i.MonitorLock.Lock()
	logLevel := log.FFmpegLevel(i.Config.LogLevel())
	args := ffmpeg.ParseArgs(i.generateArgs())
	i.MonitorLock.Unlock()

	processCTX, cancel2 := context.WithCancel(ctx)
	i.cancel = cancel2
	defer cancel2()

	i.hooks.StartInput(processCTX, i, &args)

	cmd := exec.Command(i.Env.FFmpegBin, args...)

	logFunc := func(msg string) {
		i.logf(logLevel, "%v process: %v", i.ProcessName(), msg)
	}

	process := i.newProcess(cmd).
		Timeout(10 * time.Second).
		StdoutLogger(logFunc).
		StderrLogger(logFunc)

	i.logf(log.LevelInfo, "starting %v process: %v", i.ProcessName(), cmd)

	err = process.Start(processCTX) // Blocks until process exits.
	if err != nil {
		return fmt.Errorf("crashed: %w", err)
	}

	return nil
}

func (i *InputProcess) generateArgs() string {
	// OUTPUT
	// -threads 1 -loglevel error -hwaccel x -i rtsp://x -c:a aac -c:v libx264
	// -f rtsp -rtsp_transport tcp rtsp://127.0.0.1:2021/test

	c := i.Config
	var args string

	args += "-threads 1 -loglevel " + c.LogLevel()
	if c.Hwaccel() != "" {
		args += " -hwaccel " + c.Hwaccel()
	}

	if c.InputOpts() != "" {
		args += " " + c.InputOpts()
	}
	args += " -i " + i.input()

	if c.audioEnabled() {
		args += " -c:a " + c.AudioEncoder()
	} else {
		args += " -an" // Skip audio.
	}

	args += " -c:v " + c.VideoEncoder()
	args += " -f rtsp -rtsp_transport " + i.RTSPprotocol() + " " + i.RTSPaddress()

	return args
}
