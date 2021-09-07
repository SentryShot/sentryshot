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

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"nvr/pkg/log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
)

// Manager handles storage interactions.
type Manager struct {
	path    string
	general *ConfigGeneral

	usage     func(string) int64
	removeAll func(string) error

	log *log.Logger
}

// NewManager returns new manager.
func NewManager(path string, general *ConfigGeneral, log *log.Logger) *Manager {
	return &Manager{
		path:    path,
		general: general,

		usage:     diskUsage,
		removeAll: os.RemoveAll,

		log: log,
	}
}

// DiskUsage in Bytes
type DiskUsage struct {
	Used      int
	Percent   int
	Max       int
	Formatted string
}

const kilobyte float64 = 1000
const megabyte = kilobyte * 1000
const gigabyte = megabyte * 1000
const terabyte = gigabyte * 1000

func formatDiskUsage(used float64) string {
	switch {
	case used < 1000*megabyte:
		return fmt.Sprintf("%.0fMB", used/megabyte)
	case used < 10*gigabyte:
		return fmt.Sprintf("%.2fGB", used/gigabyte)
	case used < 100*gigabyte:
		return fmt.Sprintf("%.1fGB", used/gigabyte)
	case used < 1000*gigabyte:
		return fmt.Sprintf("%.0fGB", used/gigabyte)
	case used < 10*terabyte:
		return fmt.Sprintf("%.2fTB", used/terabyte)
	case used < 100*terabyte:
		return fmt.Sprintf("%.1fTB", used/terabyte)
	default:
		return fmt.Sprintf("%.0fTB", used/terabyte)
	}
}

func diskUsage(path string) int64 {
	var used int64
	filepath.Walk(path+"/", func(_ string, info os.FileInfo, err error) error { //nolint:errcheck
		if info != nil && !info.IsDir() {
			used += info.Size()
		}
		return nil
	})
	return used
}

// Usage return DiskUsage.
func (s *Manager) Usage() (DiskUsage, error) {
	used := s.usage(s.path)

	diskSpace := s.general.Get().DiskSpace
	if diskSpace == "0" || diskSpace == "" {
		return DiskUsage{
			Used:      int(used),
			Formatted: formatDiskUsage(float64(used)),
		}, nil
	}

	diskSpaceGB, err := strconv.ParseFloat(diskSpace, 64)
	if err != nil {
		return DiskUsage{}, err
	}
	diskSpaceByte := diskSpaceGB * gigabyte

	var usedPercent int64
	if used != 0 {
		usedPercent = (used * 100) / int64(diskSpaceByte)
	}

	return DiskUsage{
		Used:      int(used),
		Percent:   int(usedPercent),
		Max:       int(diskSpaceGB),
		Formatted: formatDiskUsage(float64(used)),
	}, nil
}

// purge checks if disk usage is above 99%,
// if true deletes all files from the oldest day.
func (s *Manager) purge() error {
	usage, err := s.Usage()
	if err != nil {
		return err
	}
	if usage.Percent < 99 {
		return nil
	}

	const dayDepth = 3

	// Find the oldest day.
	path := s.RecordingsDir()
	for depth := 1; depth <= dayDepth; depth++ {
		list, err := ioutil.ReadDir(path)
		if err != nil {
			return fmt.Errorf("could not read directory %v: %v", path, err)
		}

		isDirEmpty := len(list) == 0
		if isDirEmpty {
			if depth <= 2 {
				return nil
			}

			if err := s.removeAll(path); err != nil {
				return fmt.Errorf("could not remove directory: %v", err)
			}

			path = s.RecordingsDir()
			depth = 0
			continue
		}

		firstFile := list[0].Name()
		path = filepath.Join(path, firstFile)
	}

	// Delete all files from that day
	if err := s.removeAll(path); err != nil {
		return fmt.Errorf("could not remove directory: %v", err)
	}
	return nil
}

// RecordingsDir Returns path to recordings diectory.
func (s *Manager) RecordingsDir() string {
	return filepath.Join(s.path, "recordings")
}

// PurgeLoop runs Purge on an interval until context is canceled.
func (s *Manager) PurgeLoop(ctx context.Context, duration time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(duration):
			if err := s.purge(); err != nil {
				s.log.Error().Msgf("failed to purge storage: %v", err)
			}
		}
	}
}

// ConfigEnv stores system configuration.
type ConfigEnv struct {
	Port      string `yaml:"port"`
	GoBin     string `yaml:"goBin"`
	FFmpegBin string `yaml:"ffmpegBin"`

	StorageDir string `yaml:"storageDir"`
	SHMDir     string `yaml:"shmDir"`

	HomeDir   string `yaml:"homeDir"`
	WebDir    string `yaml:"webDir"`
	ConfigDir string
}

// NewConfigEnv return new environment configuration.
func NewConfigEnv(envPath string, envYAML []byte) (*ConfigEnv, error) {
	var env ConfigEnv

	if err := yaml.Unmarshal(envYAML, &env); err != nil {
		return &ConfigEnv{}, fmt.Errorf("could not unmarshal env.yaml: %v", err)
	}

	env.ConfigDir = filepath.Dir(envPath)

	if env.Port == "" {
		env.Port = "2020"
	}
	if env.GoBin == "" {
		env.GoBin = "/usr/bin/go"
	}
	if env.FFmpegBin == "" {
		env.FFmpegBin = "/usr/bin/ffmpeg"
	}
	if env.HomeDir == "" {
		env.HomeDir = filepath.Dir(env.ConfigDir)
	}
	if env.StorageDir == "" {
		env.StorageDir = env.HomeDir + "/storage"
	}
	if env.WebDir == "" {
		env.WebDir = env.HomeDir + "/web"
	}
	if env.SHMDir == "" {
		env.SHMDir = "/dev/shm/nvr"
	}

	if !dirExist(env.GoBin) {
		return nil, fmt.Errorf("goBin '%v' does not exist", env.GoBin)
	}
	if !dirExist(env.FFmpegBin) {
		return nil, fmt.Errorf("ffmpegBin '%v' does not exist", env.FFmpegBin)
	}

	if !filepath.IsAbs(env.GoBin) {
		return nil, fmt.Errorf("goBin '%v' is not a absolute path", env.GoBin)
	}
	if !filepath.IsAbs(env.FFmpegBin) {
		return nil, fmt.Errorf("ffmpegBin '%v' is not a absolute path", env.FFmpegBin)
	}
	if !filepath.IsAbs(env.HomeDir) {
		return nil, fmt.Errorf("homeDir '%v' is not a absolute path", env.HomeDir)
	}
	if !filepath.IsAbs(env.StorageDir) {
		return nil, fmt.Errorf("StorageDir '%v' is not a absolute path", env.StorageDir)
	}
	if !filepath.IsAbs(env.SHMDir) {
		return nil, fmt.Errorf("shmDir '%v' is not a absolute path", env.SHMDir)
	}
	if !filepath.IsAbs(env.WebDir) {
		return nil, fmt.Errorf("webDir '%v' is not a absolute path", env.WebDir)
	}

	return &env, nil
}

// SHMhls returns path of temporary hls files.
func (env *ConfigEnv) SHMhls() string {
	return env.SHMDir + "/hls"
}

// PrepareEnvironment prepares directories.
func (env *ConfigEnv) PrepareEnvironment() error {
	monitorsDir := env.ConfigDir + "/monitors"
	if err := os.MkdirAll(monitorsDir, 0700); err != nil && err != os.ErrExist {
		return fmt.Errorf("could not create monitor directory: %v: %v", monitorsDir, err)
	}

	// Reset tempoary directories.
	os.RemoveAll(env.SHMhls())
	if err := os.MkdirAll(env.SHMhls(), 0700); err != nil && err != os.ErrExist {
		return fmt.Errorf("could not create hls directory: %v: %v", env.SHMhls(), err)
	}
	return nil
}

// GeneralConfig stores general config values.
type GeneralConfig struct {
	DiskSpace string `json:"diskSpace"`
	Theme     string `json:"theme"`
}

// ConfigGeneral stores config and path.
type ConfigGeneral struct {
	Config GeneralConfig

	path string
	mu   sync.Mutex
}

// NewConfigGeneral return new environment configuration.
func NewConfigGeneral(path string) (*ConfigGeneral, error) {
	var general ConfigGeneral
	general.Config.Theme = "default"

	configPath := path + "/general.json"

	if !dirExist(configPath) {
		if err := generateGeneralConfig(configPath); err != nil {
			return &ConfigGeneral{}, fmt.Errorf("could not generate environment config: %v", err)
		}
	}

	file, err := ioutil.ReadFile(configPath)
	if err != nil {
		return &ConfigGeneral{}, err
	}

	err = json.Unmarshal(file, &general.Config)
	if err != nil {
		return &ConfigGeneral{}, err
	}

	general.path = configPath
	return &general, nil
}

func generateGeneralConfig(path string) error {
	config := GeneralConfig{
		DiskSpace: "10000",
		Theme:     "default",
	}
	c, _ := json.MarshalIndent(config, "", "    ")

	return ioutil.WriteFile(path, c, 0600)
}

// Get returns general config.
func (general *ConfigGeneral) Get() GeneralConfig {
	defer general.mu.Unlock()
	general.mu.Lock()
	return general.Config
}

// Set sets config value and saves file.
func (general *ConfigGeneral) Set(newConfig GeneralConfig) error {
	general.mu.Lock()

	config, _ := json.MarshalIndent(newConfig, "", "    ")

	if err := ioutil.WriteFile(general.path, config, 0600); err != nil {
		return err
	}

	general.Config = newConfig

	general.mu.Unlock()
	return nil
}

func dirExist(path string) bool {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false
		}
		return false
	}
	return true
}
