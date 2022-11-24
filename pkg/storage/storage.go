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

package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"nvr/pkg/log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Manager storage manager.
type Manager struct {
	storageDir   string
	storageDirFS fs.FS
	disk         *disk
	removeAll    func(string) error

	logger log.ILogger
}

// NewManager returns new manager.
func NewManager(storageDir string, general *ConfigGeneral, log log.ILogger) *Manager {
	storageDirFS := os.DirFS(storageDir)
	return &Manager{
		storageDir:   storageDir,
		storageDirFS: storageDirFS,
		disk:         newDisk(general, storageDirFS),
		removeAll:    os.RemoveAll,

		logger: log,
	}
}

// RecordingsDir Returns path to recordings diectory.
func (s *Manager) RecordingsDir() string {
	return filepath.Join(s.storageDir, "recordings")
}

// DiskUsageCached returns cached value and its age.
func (s *Manager) DiskUsageCached() (DiskUsage, time.Duration) {
	return s.disk.usageCached()
}

// DiskUsage returns cached value if witin maxAge.
// Will update and return new value if the cached value is too old.
func (s *Manager) DiskUsage(maxAge time.Duration) (DiskUsage, error) {
	return s.disk.usage(maxAge)
}

// purge checks if disk usage is above 99%,
// if true deletes all files from the oldest day.
func (s *Manager) purge() error {
	usage, err := s.DiskUsage(10 * time.Minute)
	if err != nil {
		return fmt.Errorf("update disk usage: %w", err)
	}
	if usage.Percent < 99 {
		return nil
	}

	const dayDepth = 3

	// Find the oldest day.
	path := s.RecordingsDir()
	for depth := 1; depth <= dayDepth; depth++ {
		list, err := fs.ReadDir(s.storageDirFS, ".")
		if err != nil {
			return fmt.Errorf("read directory %v: %w", path, err)
		}

		isDirEmpty := len(list) == 0
		if isDirEmpty {
			if depth <= 2 {
				return nil
			}

			if err := s.removeAll(path); err != nil {
				return fmt.Errorf("remove empty directory: %w", err)
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
		return fmt.Errorf("remove directory: %w", err)
	}
	return nil
}

// PurgeLoop runs Purge on an interval until context is canceled.
func (s *Manager) PurgeLoop(ctx context.Context, duration time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(duration):
			if err := s.purge(); err != nil {
				s.logger.Log(log.Entry{
					Level: log.LevelError,
					Src:   "app",
					Msg:   fmt.Sprintf("could not purge storage: %v", err),
				})
			}
		}
	}
}

// Only used to calculate and cache disk usage.
type disk struct {
	general        *ConfigGeneral
	storageDirFS   fs.FS
	diskUsageBytes func(fs.FS) int64

	cache      DiskUsage
	lastUpdate time.Time
	cacheLock  sync.Mutex

	updateLock sync.Mutex
}

func newDisk(general *ConfigGeneral, storageDirFS fs.FS) *disk {
	return &disk{
		general:        general,
		diskUsageBytes: diskUsageBytes,
		storageDirFS:   storageDirFS,
	}
}

// DiskUsageGet returns cached value if witin maxAge.
// Will update and return new value if the cached value is too old.
func (d *disk) usageCached() (DiskUsage, time.Duration) {
	d.cacheLock.Lock()
	defer d.cacheLock.Unlock()

	return d.cache, time.Since(d.lastUpdate)
}

// usage returns cached value if witin maxAge.
// Will update and return new value if the cached value is too old.
func (d *disk) usage(maxAge time.Duration) (DiskUsage, error) {
	maxTime := time.Now().Add(-maxAge)

	d.cacheLock.Lock()
	if d.lastUpdate.After(maxTime) {
		defer d.cacheLock.Unlock()
		return d.cache, nil
	}
	d.cacheLock.Unlock()

	// Cache is too old, acquire update lock and update it.
	d.updateLock.Lock()
	defer d.updateLock.Unlock()

	// Check if it was updated while we were waiting for the update lock.
	d.cacheLock.Lock()
	if d.lastUpdate.After(maxTime) {
		defer d.cacheLock.Unlock()
		return d.cache, nil
	}
	// Still outdated.
	d.cacheLock.Unlock()

	updatedUsage, err := d.calculateDiskUsage()
	if err != nil {
		return DiskUsage{}, err
	}

	d.cacheLock.Lock()
	d.cache = updatedUsage
	d.lastUpdate = time.Now()
	d.cacheLock.Unlock()

	return updatedUsage, nil
}

func (d *disk) calculateDiskUsage() (DiskUsage, error) {
	used := d.diskUsageBytes(d.storageDirFS)

	diskSpaceBytes, err := d.general.DiskSpace()
	if err != nil {
		return DiskUsage{}, fmt.Errorf("disk space: %w", err)
	}

	percent := func() int {
		if used == 0 || diskSpaceBytes == 0 {
			return 0
		}
		return int((used * 100) / diskSpaceBytes)
	}()

	return DiskUsage{
		Used:      used,
		Percent:   percent,
		Max:       diskSpaceBytes / int64(gigabyte),
		Formatted: formatDiskUsage(float64(used)),
	}, nil
}

// DiskUsage in Bytes.
type DiskUsage struct {
	Used      int64
	Percent   int
	Max       int64
	Formatted string
}

const (
	kilobyte float64 = 1000
	megabyte         = kilobyte * 1000
	gigabyte         = megabyte * 1000
	terabyte         = gigabyte * 1000
)

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

func diskUsageBytes(fileSystem fs.FS) int64 {
	var used int64
	fs.WalkDir(fileSystem, ".", func(_ string, d fs.DirEntry, err error) error { //nolint:errcheck
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		used += info.Size()

		return nil
	})
	return used
}

// ConfigEnv stores system configuration.
type ConfigEnv struct {
	Port           int    `yaml:"port"`
	RTSPPort       int    `yaml:"rtspPort"`
	RTSPPortExpose bool   `yaml:"rtspPortExpose"`
	HLSPort        int    `yaml:"hlsPort"`
	HLSPortExpose  bool   `yaml:"hlsPortExpose"`
	GoBin          string `yaml:"goBin"`
	FFmpegBin      string `yaml:"ffmpegBin"`

	StorageDir string `yaml:"storageDir"`
	TempDir    string

	HomeDir   string `yaml:"homeDir"`
	ConfigDir string
}

// ErrPathNotAbsolute path is not absolute.
var ErrPathNotAbsolute = errors.New("path is not absolute")

// NewConfigEnv return new environment configuration.
func NewConfigEnv(envPath string, envYAML []byte) (*ConfigEnv, error) {
	var env ConfigEnv

	if err := yaml.Unmarshal(envYAML, &env); err != nil {
		return nil, fmt.Errorf("unmarshal env.yaml: %w", err)
	}

	env.ConfigDir = filepath.Dir(envPath)
	env.TempDir = filepath.Join(os.TempDir(), "nvr")

	if env.Port == 0 {
		env.Port = 2020
	}
	if env.RTSPPort == 0 {
		env.RTSPPort = 2021
	}
	if env.HLSPort == 0 {
		env.HLSPort = 2022
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
		env.StorageDir = filepath.Join(env.HomeDir, "storage")
	}

	if !dirExist(env.GoBin) {
		return nil, fmt.Errorf("goBin '%v': %w", env.GoBin, os.ErrNotExist)
	}
	if !dirExist(env.FFmpegBin) {
		return nil, fmt.Errorf("ffmpegBin '%v: %w' does not exist", env.FFmpegBin, os.ErrNotExist)
	}

	if !filepath.IsAbs(env.GoBin) {
		return nil, fmt.Errorf("goBin '%v': %w", env.GoBin, ErrPathNotAbsolute)
	}
	if !filepath.IsAbs(env.FFmpegBin) {
		return nil, fmt.Errorf("ffmpegBin '%v': %w", env.FFmpegBin, ErrPathNotAbsolute)
	}
	if !filepath.IsAbs(env.HomeDir) {
		return nil, fmt.Errorf("homeDir '%v': %w", env.HomeDir, ErrPathNotAbsolute)
	}
	if !filepath.IsAbs(env.StorageDir) {
		return nil, fmt.Errorf("StorageDir '%v': %w", env.StorageDir, ErrPathNotAbsolute)
	}

	return &env, nil
}

// RecordingsDir return recordings directory.
func (env ConfigEnv) RecordingsDir() string {
	return filepath.Join(env.StorageDir, "recordings")
}

// PrepareEnvironment prepares directories.
func (env ConfigEnv) PrepareEnvironment() error {
	err := os.MkdirAll(env.RecordingsDir(), 0o700)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create recordings directory: %v: %w", env.StorageDir, err)
	}

	// Make sure env.TempDir isn't set to "/".
	if len(env.TempDir) <= 4 {
		panic(fmt.Sprintf("tempDir sanity check: %v", env.TempDir))
	}
	err = os.RemoveAll(env.TempDir)
	if err != nil {
		return fmt.Errorf("clear tempDir: %v: %w", env.TempDir, err)
	}

	err = os.MkdirAll(env.TempDir, 0o700)
	if err != nil {
		return fmt.Errorf("create tempDir: %v: %w", env.StorageDir, err)
	}

	return nil
}

// ConfigGeneral stores config and path.
type ConfigGeneral struct {
	Config map[string]string

	path string
	mu   sync.Mutex
}

// NewConfigGeneral return new environment configuration.
func NewConfigGeneral(path string) (*ConfigGeneral, error) {
	general := ConfigGeneral{
		Config: map[string]string{},
		path:   path + "/general.json",
	}

	if !dirExist(general.path) {
		if err := generateGeneralConfig(general.path); err != nil {
			return &ConfigGeneral{}, fmt.Errorf("generate general.yaml: %w", err)
		}
	}

	file, err := os.ReadFile(general.path)
	if err != nil {
		return &ConfigGeneral{}, err
	}

	err = json.Unmarshal(file, &general.Config)
	if err != nil {
		return &ConfigGeneral{}, err
	}

	return &general, nil
}

func generateGeneralConfig(path string) error {
	config := map[string]string{
		"diskSpace": "5",
		"theme":     "default",
	}
	c, _ := json.MarshalIndent(config, "", "    ")

	return os.WriteFile(path, c, 0o600)
}

// Get returns general config.
func (general *ConfigGeneral) Get() map[string]string {
	defer general.mu.Unlock()
	general.mu.Lock()
	return general.Config
}

// Set sets config value and saves file.
func (general *ConfigGeneral) Set(newConfig map[string]string) error {
	general.mu.Lock()

	config, _ := json.MarshalIndent(newConfig, "", "    ")

	if err := os.WriteFile(general.path, config, 0o600); err != nil {
		return err
	}

	general.Config = newConfig

	general.mu.Unlock()
	return nil
}

// DiskSpace returns configured disk space in bytes.
func (general *ConfigGeneral) DiskSpace() (int64, error) {
	defer general.mu.Unlock()
	general.mu.Lock()

	diskSpace := general.Config["diskSpace"]
	if diskSpace == "0" || diskSpace == "" {
		return 0, nil
	}

	diskSpaceGB, err := strconv.ParseFloat(diskSpace, 64)
	if err != nil {
		return 0, fmt.Errorf("parse diskSpace: %w", err)
	}
	diskSpaceByte := diskSpaceGB * gigabyte

	return int64(diskSpaceByte), nil
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
