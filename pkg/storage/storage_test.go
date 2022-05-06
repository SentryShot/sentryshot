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
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"nvr/pkg/log"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestDiskUsage(t *testing.T) {
	require.Equal(t, diskUsage("testdata"), int64(302))
}

func TestUsage(t *testing.T) {
	const mb int64 = 1000000
	cases := []struct {
		name     string
		used     float64 // Byte
		space    string  // GB
		expected string
	}{
		{"formatMB", 10 * megabyte, "0.1", "{10000000 10 0 10MB}"},
		{"formatGB2", 2 * gigabyte, "10", "{2000000000 20 10 2.00GB}"},
		{"formatGB1", 20 * gigabyte, "100", "{20000000000 20 100 20.0GB}"},
		{"formatGB0", 200 * gigabyte, "1000", "{200000000000 20 1000 200GB}"},
		{"formatTB2", 2 * terabyte, "10000", "{2000000000000 20 10000 2.00TB}"},
		{"formatTB1", 20 * terabyte, "100000", "{20000000000000 20 100000 20.0TB}"},
		{"formatDefault", 200 * terabyte, "1000000", "{200000000000000 20 1000000 200TB}"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := Manager{
				path: "testdata",
				general: &ConfigGeneral{
					Config: GeneralConfig{
						DiskSpace: tc.space,
					},
				},
				usage: func(_ string) int64 {
					return int64(tc.used)
				},
			}
			u, err := s.Usage()
			require.NoError(t, err)

			actual := fmt.Sprintf("%v", u)
			require.Equal(t, actual, tc.expected)
		})
	}

	t.Run("diskSpaceZero", func(t *testing.T) {
		s := Manager{
			path: "testdata",
			general: &ConfigGeneral{
				Config: GeneralConfig{
					DiskSpace: "",
				},
			},
			usage: func(_ string) int64 {
				return int64(1000)
			},
		}
		u, err := s.Usage()
		require.NoError(t, err)

		actual := fmt.Sprintf("%v", u)
		require.Equal(t, actual, "{1000 0 0 0MB}")
	})
	t.Run("diskSpace error", func(t *testing.T) {
		s := Manager{
			general: &ConfigGeneral{
				Config: GeneralConfig{
					DiskSpace: "nil",
				},
			},
			usage: func(_ string) int64 {
				return 0
			},
		}
		_, err := s.Usage()
		require.ErrorIs(t, err, strconv.ErrSyntax)
	})
}

var diskSpace1 = &ConfigGeneral{
	Config: GeneralConfig{
		DiskSpace: "1",
	},
}

var diskSpaceErr = &ConfigGeneral{
	Config: GeneralConfig{
		DiskSpace: "nil",
	},
}

var highUsage = func(_ string) int64 {
	return 1000000000
}

func TestPurge(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		cases := []struct {
			name      string
			input     *Manager
			expectErr bool
		}{
			{
				"usage error",
				&Manager{
					general: diskSpaceErr,
					usage: func(string) int64 {
						return 1
					},
				},
				true,
			},
			{
				"below 99%",
				&Manager{
					general: diskSpace1,
					usage: func(string) int64 {
						return 1
					},
				},
				false,
			},
			{
				"readDir error",
				&Manager{
					general: diskSpace1,
					usage:   highUsage,
				},
				true,
			},
			{
				"ok",
				&Manager{
					path:    "testdata",
					general: diskSpace1,
					usage:   highUsage,
					removeAll: func(string) error {
						return nil
					},
				},
				false,
			},
			{
				"removeAll error",
				&Manager{
					path:    "testdata",
					general: diskSpace1,
					usage:   highUsage,
					removeAll: func(string) error {
						return errors.New("")
					},
				},
				true,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				err := tc.input.purge()
				gotError := err != nil
				require.Equal(t, tc.expectErr, gotError)
			})
		}
	})

	t.Run("removeAll", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		testDir := filepath.Join(tempDir, "recordings", "2000", "01", "01")
		err = os.MkdirAll(testDir, 0o700)
		require.NoError(t, err)

		m := &Manager{
			path: tempDir,
			general: &ConfigGeneral{
				Config: GeneralConfig{
					DiskSpace: "1",
				},
			},
			usage:     highUsage,
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
			path:    "testdata",
			general: diskSpace1,
			usage:   highUsage,
			removeAll: func(_ string) error {
				return nil
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		m.PurgeLoop(ctx, 0)
	})
	t.Run("error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		logger := log.NewMockLogger()
		logger.Start(ctx)
		feed, cancel2 := logger.Subscribe()

		m := &Manager{
			path:    "testdata",
			general: diskSpaceErr,
			usage:   highUsage,
			log:     logger,
		}
		ctx, cancel3 := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel3()
		go m.PurgeLoop(ctx, 0)

		actual := <-feed
		cancel2()
		cancel()

		expected := `could not purge storage: strconv.ParseFloat: parsing "nil": invalid syntax`
		require.Equal(t, actual.Msg, expected)
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
		RTSPport:   2021,
		HLSport:    2022,
		GoBin:      goBin,
		FFmpegBin:  ffmpegBin,
		StorageDir: filepath.Join(homeDir, "storage"),
		SHMDir:     filepath.Join(homeDir, "shm"),
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
			RTSPport:   2021,
			HLSport:    2022,
			GoBin:      filepath.Join(homeDir, "go"),
			FFmpegBin:  filepath.Join(homeDir, "ffmpeg"),
			StorageDir: filepath.Join(homeDir, "storage"),
			SHMDir:     "/dev/shm/nvr",
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
	t.Run("shmAbs", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.SHMDir = "."

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

		storageDir := filepath.Join(tempDir, "configs")
		env := &ConfigEnv{
			StorageDir: storageDir,
		}

		err = env.PrepareEnvironment()
		require.NoError(t, err)
		require.DirExists(t, env.RecordingsDir())
	})
	t.Run("storageMkdirErr", func(t *testing.T) {
		env := ConfigEnv{
			StorageDir: "/dev/null",
		}
		err := env.PrepareEnvironment()
		require.Error(t, err)
	})
}

func newTestGeneral(t *testing.T) (string, *ConfigGeneral, func()) {
	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)

	cancelFunc := func() {
		os.RemoveAll(tempDir)
	}

	configPath := filepath.Join(tempDir, "general.json")

	config := GeneralConfig{
		DiskSpace: "1",
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

		config2 := &GeneralConfig{}
		err = json.Unmarshal(file, config2)
		require.NoError(t, err)

		expected := &GeneralConfig{DiskSpace: "5", Theme: "default"}

		require.Equal(t, &config1.Config, expected)
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

		newConfig := GeneralConfig{
			DiskSpace: "1",
		}
		general.Set(newConfig)

		file, err := os.ReadFile(general.path)
		require.NoError(t, err)

		var config GeneralConfig
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

		err = general.Set(GeneralConfig{})
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}
