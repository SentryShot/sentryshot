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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"testing/fstest"
	"time"

	"nvr/pkg/log"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

var recordingTestFS = fstest.MapFS{
	"recordings": {Data: bytes.Repeat([]byte{0}, 302)},
}

func TestDiskUsage(t *testing.T) {
	usage := diskUsageBytes(recordingTestFS)
	require.Equal(t, int64(302), usage)
}

func TestDisk(t *testing.T) {
	du := func(used int64, percent int, max int64, formatted string) DiskUsage {
		return DiskUsage{
			Used:      used,
			Percent:   percent,
			Max:       max,
			Formatted: formatted,
		}
	}

	cases := map[string]struct {
		used     float64 // Bytes
		space    string  // GB
		expected DiskUsage
	}{
		"formatMB":      {11 * megabyte, "0.1", du(11000000, 11, 0, "11MB")},
		"formatGB2":     {2345 * megabyte, "10", du(2345000000, 23, 10, "2.35GB")},
		"formatGB1":     {22 * gigabyte, "100", du(22000000000, 22, 100, "22.0GB")},
		"formatGB0":     {234 * gigabyte, "1000", du(234000000000, 23, 1000, "234GB")},
		"formatTB2":     {2345 * gigabyte, "10000", du(2345000000000, 23, 10000, "2.35TB")},
		"formatTB1":     {22 * terabyte, "100000", du(22000000000000, 22, 100000, "22.0TB")},
		"formatDefault": {234 * terabyte, "1000000", du(234000000000000, 23, 1000000, "234TB")},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := &disk{
				general: &ConfigGeneral{
					Config: map[string]string{
						"diskSpace": tc.space,
					},
				},
				diskUsageBytes: func(fs.FS) int64 {
					return int64(tc.used)
				},
			}
			actual, err := d.usage(0)
			require.NoError(t, err)
			require.Equal(t, actual, tc.expected)
		})
	}

	t.Run("cached", func(t *testing.T) {
		usage := DiskUsage{Used: 1}
		d := &disk{
			cache:      usage,
			lastUpdate: time.Now(),
		}
		actual, age := d.usageCached()
		require.Equal(t, usage, actual)
		require.Less(t, age, 1*time.Second)
	})
	t.Run("updatedDuringLock", func(t *testing.T) {
		d := &disk{}
		d.updateLock.Lock()

		result := make(chan DiskUsage)
		go func() {
			usage, err := d.usage(1 * time.Hour)
			require.NoError(t, err)
			result <- usage
		}()
		time.Sleep(10 * time.Millisecond)

		usage := DiskUsage{Used: 1}

		d.cacheLock.Lock()
		d.cache = usage
		d.lastUpdate = time.Now()
		d.cacheLock.Unlock()

		d.updateLock.Unlock()
		require.Equal(t, usage, <-result)
	})
	t.Run("diskSpaceZero", func(t *testing.T) {
		d := &disk{
			general: &ConfigGeneral{
				Config: map[string]string{},
			},
			diskUsageBytes: func(fs.FS) int64 {
				return int64(1000)
			},
		}
		actual, err := d.usage(0)
		require.NoError(t, err)

		expected := DiskUsage{
			Used:      1000,
			Percent:   0,
			Max:       0,
			Formatted: "0MB",
		}
		require.Equal(t, expected, actual)
	})
	t.Run("diskSpace error", func(t *testing.T) {
		d := &disk{
			general: &ConfigGeneral{
				Config: map[string]string{
					"diskSpace": "nil",
				},
			},
			diskUsageBytes: func(fs.FS) int64 {
				return 0
			},
		}
		_, err := d.usage(0)
		require.ErrorIs(t, err, strconv.ErrSyntax)
	})
}

var diskSpace1 = &ConfigGeneral{
	Config: map[string]string{
		"diskSpace": "1",
	},
}

var diskSpaceErr = &ConfigGeneral{
	Config: map[string]string{
		"diskSpace": "nil",
	},
}

var highUsage = func(fs.FS) int64 {
	return 1000000000
}

func TestPurge(t *testing.T) {
	cases := map[string]struct {
		input     *Manager
		expectErr bool
	}{
		"usageErr": {
			&Manager{
				storageDirFS: recordingTestFS,
				disk: &disk{
					storageDirFS: recordingTestFS,
					general:      diskSpaceErr,
					diskUsageBytes: func(fs.FS) int64 {
						return 1
					},
				},
			},
			true,
		},
		"below99%": {
			&Manager{
				storageDirFS: recordingTestFS,
				disk: &disk{
					storageDirFS: recordingTestFS,
					general:      diskSpace1,
					diskUsageBytes: func(fs.FS) int64 {
						return 1
					},
				},
			},
			false,
		},
		"ok": {
			&Manager{
				storageDirFS: recordingTestFS,
				disk: &disk{
					storageDirFS:   recordingTestFS,
					general:        diskSpace1,
					diskUsageBytes: highUsage,
				},
				removeAll: func(string) error {
					return nil
				},
			},
			false,
		},
		"removeAllErr": {
			&Manager{
				storageDirFS: recordingTestFS,
				disk: &disk{
					storageDirFS:   recordingTestFS,
					general:        diskSpace1,
					diskUsageBytes: highUsage,
				},
				removeAll: func(string) error {
					return errors.New("")
				},
			},
			true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := tc.input.purge()
			gotError := err != nil
			require.Equal(t, tc.expectErr, gotError, err)
		})
	}

	t.Run("removeAll", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		testDir := filepath.Join(tempDir, "recordings", "2000", "01", "01")
		err = os.MkdirAll(testDir, 0o700)
		require.NoError(t, err)

		m := &Manager{
			storageDirFS: os.DirFS(tempDir),
			disk: &disk{
				storageDirFS: os.DirFS(tempDir),
				general: &ConfigGeneral{
					Config: map[string]string{
						"diskSpace": "1",
					},
				},
				diskUsageBytes: highUsage,
			},
			removeAll: os.RemoveAll,
		}
		err = m.purge()
		require.NoError(t, err)

		err = m.purge()
		require.NoError(t, err)

		yearDir := filepath.Join(tempDir, "recordings", "1000")
		require.NoDirExists(t, yearDir, "empty year directory was not deleted")

		recordingsDir := filepath.Join(tempDir, "recordings")
		require.DirExists(t, recordingsDir, "recordings directory was deleted")
	})
}

func TestPurgeLoop(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		m := &Manager{
			storageDirFS: recordingTestFS,
			disk: &disk{
				storageDirFS:   recordingTestFS,
				general:        diskSpace1,
				diskUsageBytes: highUsage,
			},
			removeAll: func(_ string) error {
				return nil
			},
			logger: log.NewDummyLogger(),
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		m.PurgeLoop(ctx, 0)
	})
	t.Run("error", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		logger, logs := log.NewMockLogger()

		m := &Manager{
			disk: &disk{
				general:        diskSpaceErr,
				diskUsageBytes: highUsage,
			},
			logger: logger,
		}

		go m.PurgeLoop(ctx, 0)

		expected := `could not purge storage: update disk usage: disk space: parse diskSpace: strconv.ParseFloat: parsing "nil": invalid syntax`
		require.Equal(t, expected, <-logs)
	})
}

func newTestEnv(t *testing.T) (string, *ConfigEnv, func()) {
	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)

	cancelFunc := func() {
		os.RemoveAll(tempDir)
	}

	homeDir := filepath.Join(tempDir, "home")
	goBin := filepath.Join(homeDir, "go")
	ffmpegBin := filepath.Join(homeDir, "ffmpeg")
	configDir := filepath.Join(homeDir, "configs")
	envPath := filepath.Join(configDir, "env.yaml")

	err = os.MkdirAll(homeDir, 0o700)
	require.NoError(t, err)

	err = os.MkdirAll(configDir, 0o700)
	require.NoError(t, err)

	err = os.WriteFile(goBin, []byte{}, 0o600)
	require.NoError(t, err)

	err = os.WriteFile(ffmpegBin, []byte{}, 0o600)
	require.NoError(t, err)

	env := &ConfigEnv{
		Port:       2020,
		RTSPPort:   2021,
		HLSPort:    2022,
		GoBin:      goBin,
		FFmpegBin:  ffmpegBin,
		StorageDir: filepath.Join(homeDir, "storage"),
		TempDir:    filepath.Join(homeDir, "nvr"),
		HomeDir:    homeDir,
		ConfigDir:  configDir,
	}

	return envPath, env, cancelFunc
}

func TestNewConfigEnv(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		envPath, _, cancel := newTestEnv(t)
		defer cancel()

		homeDir := filepath.Dir(filepath.Dir(envPath))

		envYAML, err := yaml.Marshal(ConfigEnv{
			GoBin:     filepath.Join(homeDir, "go"),
			FFmpegBin: filepath.Join(homeDir, "ffmpeg"),
		})
		require.NoError(t, err)

		env, err := NewConfigEnv(envPath, envYAML)
		require.NoError(t, err)

		expected := ConfigEnv{
			Port:       2020,
			RTSPPort:   2021,
			HLSPort:    2022,
			GoBin:      filepath.Join(homeDir, "go"),
			FFmpegBin:  filepath.Join(homeDir, "ffmpeg"),
			StorageDir: filepath.Join(homeDir, "storage"),
			TempDir:    env.TempDir,
			HomeDir:    homeDir,
			ConfigDir:  filepath.Join(homeDir, "configs"),
		}
		require.Equal(t, *env, expected)
	})
	t.Run("maximal", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		envYAML, err := yaml.Marshal(testEnv)
		require.NoError(t, err)

		env, err := NewConfigEnv(envPath, envYAML)
		require.NoError(t, err)

		env.TempDir = testEnv.TempDir
		require.Equal(t, env, testEnv)
	})
	t.Run("unmarshal error", func(t *testing.T) {
		_, err := NewConfigEnv("", []byte("&"))
		require.Error(t, err)
	})
	t.Run("goBinExist", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.GoBin = "/dev/null/nil"

		envYAML, err := yaml.Marshal(testEnv)
		require.NoError(t, err)

		_, err = NewConfigEnv(envPath, envYAML)
		require.ErrorIs(t, err, os.ErrNotExist)
	})
	t.Run("ffmpegBinExist", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.FFmpegBin = "/dev/null/nil"

		envYAML, err := yaml.Marshal(testEnv)
		require.NoError(t, err)

		_, err = NewConfigEnv(envPath, envYAML)
		require.ErrorIs(t, err, os.ErrNotExist)
	})
	t.Run("storageAbs", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.StorageDir = "."

		envYAML, err := yaml.Marshal(testEnv)
		require.NoError(t, err)

		_, err = NewConfigEnv(envPath, envYAML)
		require.ErrorIs(t, err, ErrPathNotAbsolute)
	})
	t.Run("goBinAbs", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.GoBin = "."

		envYAML, err := yaml.Marshal(testEnv)
		require.NoError(t, err)

		_, err = NewConfigEnv(envPath, envYAML)
		require.ErrorIs(t, err, ErrPathNotAbsolute)
	})
	t.Run("ffmpegBinAbs", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.FFmpegBin = "."

		envYAML, err := yaml.Marshal(testEnv)
		require.NoError(t, err)

		_, err = NewConfigEnv(envPath, envYAML)
		require.ErrorIs(t, err, ErrPathNotAbsolute)
	})
	t.Run("homeDirAbs", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.HomeDir = "."

		envYAML, err := yaml.Marshal(testEnv)
		require.NoError(t, err)

		_, err = NewConfigEnv(envPath, envYAML)
		require.ErrorIs(t, err, ErrPathNotAbsolute)
	})
}

func TestPrepareEnvironment(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		env := &ConfigEnv{
			StorageDir: filepath.Join(tempDir, "configs"),
			TempDir:    filepath.Join(tempDir, "temp"),
		}

		// Create test file.
		err = os.Mkdir(env.TempDir, 0o700)
		require.NoError(t, err)
		testFile := filepath.Join(env.TempDir, "test")
		file, err := os.Create(testFile)
		require.NoError(t, err)
		file.Close()

		err = env.PrepareEnvironment()
		require.NoError(t, err)
		require.DirExists(t, env.RecordingsDir())
		require.NoFileExists(t, testFile)
	})
}

func newTestGeneral(t *testing.T) (string, *ConfigGeneral, func()) {
	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)

	cancelFunc := func() {
		os.RemoveAll(tempDir)
	}

	configPath := filepath.Join(tempDir, "general.json")

	config := map[string]string{
		"diskSpace": "1",
	}
	data, err := json.MarshalIndent(config, "", "    ")
	require.NoError(t, err)

	err = os.WriteFile(configPath, data, 0o660)
	require.NoError(t, err)

	general := ConfigGeneral{
		Config: config,
		path:   configPath,
	}

	return tempDir, &general, cancelFunc
}

func TestNewConfigGeneral(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		tempDir, testGeneral, cancel := newTestGeneral(t)
		defer cancel()

		general, _ := NewConfigGeneral(tempDir)
		require.Equal(t, general, testGeneral)
	})
	t.Run("genConfig", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "")
		defer os.RemoveAll(tempDir)
		require.NoError(t, err)

		configDir := tempDir
		configFile := filepath.Join(configDir, "general.json")

		require.NoFileExists(t, configFile, "configFile should not already exist")

		config1, err := NewConfigGeneral(configDir)
		require.NoError(t, err)

		file, err := os.ReadFile(configFile)
		require.NoError(t, err)

		config2 := map[string]string{}
		err = json.Unmarshal(file, &config2)
		require.NoError(t, err)

		expected := map[string]string{"diskSpace": "5", "theme": "default"}

		require.Equal(t, config1.Config, expected)
		require.Equal(t, config2, expected)
	})
	t.Run("genConfigErr", func(t *testing.T) {
		_, err := NewConfigGeneral("/dev/null")
		require.Error(t, err)
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		tempDir, _, cancel := newTestGeneral(t)
		defer cancel()

		configPath := filepath.Join(tempDir, "general.json")
		err := os.WriteFile(configPath, []byte{}, 0o660)
		require.NoError(t, err)

		_, err = NewConfigGeneral(tempDir)
		var e *json.SyntaxError
		require.ErrorAs(t, err, &e)
	})
}

func TestGeneral(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		tempDir, testGeneral, cancel := newTestGeneral(t)
		defer cancel()

		general, _ := NewConfigGeneral(tempDir)
		require.Equal(t, general.Get(), testGeneral.Config)
	})
	t.Run("set", func(t *testing.T) {
		tempDir, _, cancel := newTestGeneral(t)
		defer cancel()

		general, _ := NewConfigGeneral(tempDir)

		newConfig := map[string]string{
			"diskSpace": "1",
		}
		general.Set(newConfig)

		file, err := os.ReadFile(general.path)
		require.NoError(t, err)

		var config map[string]string
		err = json.Unmarshal(file, &config)
		require.NoError(t, err)

		require.Equal(t, general.Get(), newConfig)
		require.Equal(t, config, newConfig)
	})
	t.Run("setWriteFileErr", func(t *testing.T) {
		tempDir, _, cancel := newTestGeneral(t)
		defer cancel()

		general, err := NewConfigGeneral(tempDir)
		require.NoError(t, err)
		os.RemoveAll(tempDir)

		err = general.Set(map[string]string{})
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}
