// SPDX-License-Identifier: GPL-2.0-or-later

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
	"path/filepath"
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
type MigationHook func(RawConfig) error

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
	rawConfigs      RawConfigs
	runningMonitors monitors

	env         storage.ConfigEnv
	logger      log.ILogger
	videoServer *video.Server
	path        string
	hooks       Hooks
	mu          sync.Mutex
}

// NewManager return new monitor manager.
func NewManager(
	configPath string,
	env storage.ConfigEnv,
	logger log.ILogger,
	videoServer *video.Server,
	hooks *Hooks,
) (*Manager, error) {
	if err := os.MkdirAll(configPath, 0o700); err != nil {
		return nil, fmt.Errorf("create monitors directory: %w", err)
	}

	configFS := os.DirFS(configPath)
	configFiles, err := readConfigs(configFS)
	if err != nil {
		return nil, fmt.Errorf("read config files: %w", err)
	}

	rawConfigs := make(RawConfigs)
	for _, file := range configFiles {
		var rawConf RawConfig
		if err := json.Unmarshal(file, &rawConf); err != nil {
			return nil, fmt.Errorf("unmarshal config: %w: %v", err, string(file))
		}
		if err := hooks.Migrate(rawConf); err != nil {
			return nil, fmt.Errorf("migration failed: %w", err)
		}

		id := rawConf["id"]
		configPath := monitorConfigPath(configPath, id)

		jsonConf, _ := json.MarshalIndent(rawConf, "", "    ")
		err := os.WriteFile(configPath, jsonConf, 0o600)
		if err != nil {
			return nil, fmt.Errorf("write migrated config: %w", err)
		}

		rawConfigs[id] = rawConf
	}

	return &Manager{
		rawConfigs:      rawConfigs,
		runningMonitors: make(monitors),

		env:         env,
		logger:      logger,
		videoServer: videoServer,
		path:        configPath,
		hooks:       *hooks,
	}, nil
}

func readConfigs(fileSystem fs.FS) ([][]byte, error) {
	var files [][]byte
	walkFunc := func(path string, d fs.DirEntry, err error) error {
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
	}
	err := fs.WalkDir(fileSystem, ".", walkFunc)
	return files, err
}

func (m *Manager) unsafeStartMonitor(id string) {
	rawConf := m.rawConfigs[id]
	monitor := m.newMonitor(NewConfig(rawConf))
	monitor.start()
	m.runningMonitors[id] = monitor
}

func (m *Manager) unsafeStopMonitor(id string) {
	m.runningMonitors[id].stop()
	delete(m.runningMonitors, id)
}

// StartMonitors starts all monitors.
func (m *Manager) StartMonitors() {
	m.mu.Lock()
	for id := range m.rawConfigs {
		m.unsafeStartMonitor(id)
	}
	m.mu.Unlock()
}

// StopMonitors stops all monitors.
func (m *Manager) StopMonitors() {
	m.mu.Lock()
	for id := range m.runningMonitors {
		m.unsafeStopMonitor(id)
	}
	m.mu.Unlock()
}

// ErrMonitorNotExist monitor does not exist.
var ErrMonitorNotExist = errors.New("monitor does not exist")

// RestartMonitor restarts monitor by ID.
func (m *Manager) RestartMonitor(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exist := m.rawConfigs[id]; !exist {
		return ErrMonitorNotExist
	}

	if _, exist := m.runningMonitors[id]; exist {
		m.unsafeStopMonitor(id)
	}
	m.unsafeStartMonitor(id)
	return nil
}

// MonitorSet sets config for specified monitor.
// Changes are not applied until the montior restarts.
func (m *Manager) MonitorSet(id string, rawConf RawConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Write config to file.
	configJSON, err := json.MarshalIndent(rawConf, "", "    ")
	if err != nil {
		return fmt.Errorf("marshal config file: %w", err)
	}
	err = os.WriteFile(m.configPath(id), configJSON, 0o600)
	if err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	m.rawConfigs[id] = rawConf
	return nil
}

// ErrNotExist monitor does not exist.
var ErrNotExist = errors.New("monitor does not exist")

// MonitorDelete deletes monitor by id.
func (m *Manager) MonitorDelete(id string) error {
	defer m.mu.Unlock()
	m.mu.Lock()

	monitor, exists := m.runningMonitors[id]
	if !exists {
		return ErrNotExist
	}

	monitor.stop()
	delete(m.runningMonitors, id)
	delete(m.rawConfigs, id)

	if err := os.Remove(m.configPath(id)); err != nil {
		return err
	}

	return nil
}

// MonitorsInfo returns common information about the monitors.
// This will be accessesable by normal users.
func (m *Manager) MonitorsInfo() RawConfigs {
	m.mu.Lock()
	defer m.mu.Unlock()

	configs := make(RawConfigs)
	for _, rawConf := range m.rawConfigs {
		c := NewConfig(rawConf)

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

		configs[c.ID()] = RawConfig{
			"id":              c.ID(),
			"name":            c.Name(),
			"enable":          enable,
			"audioEnabled":    audioEnabled,
			"subInputEnabled": subInputEnabled,
		}
	}
	return configs
}

func (m *Manager) configPath(id string) string {
	return monitorConfigPath(m.path, id)
}

func monitorConfigPath(path string, id string) string {
	return filepath.Join(path, id+".json")
}

// MonitorConfigs returns configurations for all monitors.
func (m *Manager) MonitorConfigs() RawConfigs {
	m.mu.Lock()
	defer m.mu.Unlock()

	configs := make(RawConfigs)
	for id, rawConf := range m.rawConfigs {
		configs[id] = rawConf
	}
	return configs
}

// monitors map.
type monitors map[string]*Monitor

// Monitor service.
type Monitor struct {
	Config Config
	ctx    context.Context

	Env         storage.ConfigEnv
	Logger      log.ILogger
	videoServer *video.Server

	mainInput *InputProcess
	subInput  *InputProcess
	recorder  *Recorder
	Recorder
	hooks      Hooks
	NewProcess ffmpeg.NewProcessFunc
	logf       logFunc

	WG     sync.WaitGroup
	cancel func()
}

type (
	logFunc func(log.Level, string, ...interface{})
)

func (m *Manager) newMonitor(config Config) *Monitor {
	monitorID := config.ID()
	logf := func(level log.Level, format string, a ...interface{}) {
		m.logger.Log(log.Entry{
			Level:     level,
			Src:       "monitor",
			MonitorID: monitorID,
			Msg:       fmt.Sprintf(format, a...),
		})
	}

	monitor := &Monitor{
		Config:      config,
		Env:         m.env,
		Logger:      m.logger,
		videoServer: m.videoServer,

		hooks:      m.hooks,
		NewProcess: ffmpeg.NewProcess,
		logf:       logf,
	}
	monitor.mainInput = newInputProcess(monitor, false)
	monitor.subInput = newInputProcess(monitor, true)
	monitor.recorder = newRecorder(monitor)

	return monitor
}

// ErrRunning monitor is already running.
var ErrRunning = errors.New("monitor is aleady running")

func (m *Monitor) start() {
	if !m.Config.enabled() {
		m.logf(log.LevelInfo, "disabled")
		return
	}

	m.logf(log.LevelInfo, "starting")

	m.ctx, m.cancel = context.WithCancel(context.Background())

	if m.Config.alwaysRecord() {
		infinte := time.Duration(1<<63 - 62135596801)
		go func() {
			select {
			case <-m.ctx.Done():
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

	m.hooks.Start(m.ctx, m)

	m.WG.Add(1)
	go m.mainInput.start(m.ctx)

	if m.Config.SubInputEnabled() {
		m.WG.Add(1)
		go m.subInput.start(m.ctx)
	}

	m.WG.Add(1)
	go m.recorder.start(m.ctx)
}

// SendEventFunc send event signature.
type SendEventFunc func(storage.Event) error

// SendEvent sends event to recorder.
func (m *Monitor) SendEvent(event storage.Event) error {
	return m.recorder.sendEvent(m.ctx, event)
}

// Stop monitor.
func (m *Monitor) stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.WG.Wait()
}

// InputProcess monitor input process.
type InputProcess struct {
	Config     Config
	serverPath video.ServerPath
	isSubInput bool

	cancel func()

	hooks     Hooks
	Env       storage.ConfigEnv
	Logger    log.ILogger
	WG        *sync.WaitGroup
	SendEvent SendEventFunc

	logf               logFunc
	newVideoServerPath newVideoServerPathFunc
	runInputProcess    runInputProcessFunc
	newProcess         ffmpeg.NewProcessFunc
}

type newVideoServerPathFunc func(context.Context, string, video.PathConf) (*video.ServerPath, error)

type runInputProcessFunc func(context.Context, *InputProcess) error

func newInputProcess(m *Monitor, isSubInput bool) *InputProcess {
	i := &InputProcess{
		Config:     m.Config,
		isSubInput: isSubInput,

		hooks:     m.hooks,
		Env:       m.Env,
		Logger:    m.Logger,
		WG:        &m.WG,
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
	muxer, err := i.serverPath.HLSMuxer(ctx)
	if err != nil {
		return nil, fmt.Errorf("get muxer: %w", err)
	}
	return muxer.StreamInfo(), nil
}

// HLSMuxer returns the HLS muxer for this input.
func (i *InputProcess) HLSMuxer(ctx context.Context) (video.IHLSMuxer, error) {
	return i.serverPath.HLSMuxer(ctx)
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
	processCTX, cancel2 := context.WithCancel(ctx)
	i.cancel = cancel2
	defer cancel2()

	pathConf := video.PathConf{MonitorID: i.Config.ID(), IsSub: i.IsSubInput()}
	serverPath, err := i.newVideoServerPath(processCTX, i.rtspPathName(), pathConf)
	if err != nil {
		return fmt.Errorf("add path to RTSP server: %w", err)
	}
	i.serverPath = *serverPath

	logLevel := log.FFmpegLevel(i.Config.LogLevel())
	args := ffmpeg.ParseArgs(i.generateArgs())

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
