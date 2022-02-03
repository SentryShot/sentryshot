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
	"nvr/pkg/log"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"gopkg.in/yaml.v2"
)

func TestNewManager(t *testing.T) {
	m := NewManager("", &ConfigGeneral{}, &log.Logger{})
	if m == nil {
		t.Fatal("nil")
	}
}

func TestDiskUsage(t *testing.T) {
	var expected int64 = 302

	actual := diskUsage("testdata")
	if actual != expected {
		t.Fatalf("expected %v, got %v", expected, actual)
	}
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
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			actual := fmt.Sprintf("%v", u)
			if actual != tc.expected {
				t.Fatalf("\nexpected %v\n     got %v", tc.expected, actual)
			}
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		actual := fmt.Sprintf("%v", u)
		expected := "{1000 0 0 0MB}"
		if actual != expected {
			t.Fatalf("\nexpected %v\n     got %v", expected, actual)
		}
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
		if !errors.Is(err, strconv.ErrSyntax) {
			t.Fatalf("expected: %v, got: %v", strconv.ErrSyntax, err)
		}
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
	t.Run("working", func(t *testing.T) {
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
				"working",
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
				if tc.expectErr != gotError {
					t.Fatalf("\nexpected error %v\n     got %v", tc.expectErr, err)
				}
			})
		}
	})

	t.Run("removeAll", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "")
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		testDir := filepath.Join(tempDir, "recordings", "2000", "01", "01")
		if err := os.MkdirAll(testDir, 0o700); err != nil {
			t.Fatalf("could not create test directory: %v", err)
		}

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
		if err := m.purge(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := m.purge(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if exist(filepath.Join(tempDir, "recordings", "1000")) {
			t.Fatalf("empty year directory was not deleted")
		}

		if !exist(filepath.Join(tempDir, "recordings")) {
			t.Fatalf("recordings directory was deleted")
		}
	})
}

func exist(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func TestPurgeLoop(t *testing.T) {
	t.Run("working", func(t *testing.T) {
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
		go logger.Start(ctx)
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

		expected := `failed to purge storage: strconv.ParseFloat: parsing "nil": invalid syntax`
		if actual.Msg != expected {
			t.Fatalf("\nexpected:\n%v.\ngot:\n%v.", expected, actual.Msg)
		}
	})
}

func newTestEnv(t *testing.T) (string, *ConfigEnv, func()) {
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatalf("could not create tempoary directory: %v", err)
	}

	cancelFunc := func() {
		os.RemoveAll(tempDir)
	}

	homeDir := filepath.Join(tempDir, "home")
	goBin := filepath.Join(homeDir, "go")
	ffmpegBin := filepath.Join(homeDir, "ffmpeg")
	configDir := filepath.Join(homeDir, "configs")
	envPath := filepath.Join(configDir, "env.yaml")

	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatalf("could not write homeDir: %v", err)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("could not write configDir: %v", err)
	}
	if err := os.WriteFile(goBin, []byte{}, 0o600); err != nil {
		t.Fatalf("could not write goBin: %v", err)
	}
	if err := os.WriteFile(ffmpegBin, []byte{}, 0o600); err != nil {
		t.Fatalf("could not write ffmpegBin: %v", err)
	}

	env := &ConfigEnv{
		Port:       2020,
		RTSPport:   2021,
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
		if err != nil {
			t.Fatalf("could not marshal env.yaml: %v", err)
		}

		env, err := NewConfigEnv(envPath, envYAML)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := fmt.Sprintf("%v", env)

		expected := fmt.Sprintf("%v", &ConfigEnv{
			Port:       2020,
			RTSPport:   2021,
			GoBin:      filepath.Join(homeDir, "go"),
			FFmpegBin:  filepath.Join(homeDir, "ffmpeg"),
			StorageDir: filepath.Join(homeDir, "storage"),
			SHMDir:     "/dev/shm/nvr",
			HomeDir:    homeDir,
			ConfigDir:  filepath.Join(homeDir, "configs"),
		})

		if actual != expected {
			t.Fatalf("\nexpected:\n%v.\ngot:\n%v.", expected, actual)
		}
	})
	t.Run("maximal", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		envYAML, err := yaml.Marshal(testEnv)
		if err != nil {
			t.Fatalf("could not marshal env.yaml: %v", err)
		}

		env, err := NewConfigEnv(envPath, envYAML)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := fmt.Sprintf("%v", env)
		expected := fmt.Sprintf("%v", testEnv)

		if actual != expected {
			t.Fatalf("\nexpected:\n%v.\ngot:\n%v.", expected, actual)
		}
	})
	t.Run("unmarshal error", func(t *testing.T) {
		if _, err := NewConfigEnv("", []byte("&")); err == nil {
			t.Fatalf("expected: error, got: nil")
		}
	})
	t.Run("shmHls", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		envYAML, err := yaml.Marshal(testEnv)
		if err != nil {
			t.Fatalf("could not marshal env.yaml: %v", err)
		}

		env, err := NewConfigEnv(envPath, envYAML)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := fmt.Sprintf("%v", env.SHMhls())
		expected := filepath.Join(env.SHMDir, "hls")

		if actual != expected {
			t.Fatalf("expected: %v got: %v", expected, actual)
		}
	})
	t.Run("goBinExist", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.GoBin = "/dev/null/nil"

		envYAML, err := yaml.Marshal(testEnv)
		if err != nil {
			t.Fatalf("could not marshal env.yaml: %v", err)
		}

		_, err = NewConfigEnv(envPath, envYAML)
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected: %v, got: %v", os.ErrNotExist, err)
		}
	})
	t.Run("ffmpegBinExist", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.FFmpegBin = "/dev/null/nil"

		envYAML, err := yaml.Marshal(testEnv)
		if err != nil {
			t.Fatalf("could not marshal env.yaml: %v", err)
		}

		_, err = NewConfigEnv(envPath, envYAML)
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected: %v, got: %v", os.ErrNotExist, err)
		}
	})
	t.Run("storageAbs", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.StorageDir = "."

		envYAML, err := yaml.Marshal(testEnv)
		if err != nil {
			t.Fatalf("could not marshal env.yaml: %v", err)
		}

		_, err = NewConfigEnv(envPath, envYAML)
		if !errors.Is(err, ErrNotAbsPath) {
			t.Fatalf("expected: %v, got: %v", ErrNotAbsPath, err)
		}
	})
	t.Run("goBinAbs", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.GoBin = "."

		envYAML, err := yaml.Marshal(testEnv)
		if err != nil {
			t.Fatalf("could not marshal env.yaml: %v", err)
		}

		_, err = NewConfigEnv(envPath, envYAML)
		if !errors.Is(err, ErrNotAbsPath) {
			t.Fatalf("expected: %v, got: %v", ErrNotAbsPath, err)
		}
	})
	t.Run("ffmpegBinAbs", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.FFmpegBin = "."

		envYAML, err := yaml.Marshal(testEnv)
		if err != nil {
			t.Fatalf("could not marshal env.yaml: %v", err)
		}

		_, err = NewConfigEnv(envPath, envYAML)
		if !errors.Is(err, ErrNotAbsPath) {
			t.Fatalf("expected: %v, got: %v", ErrNotAbsPath, err)
		}
	})
	t.Run("homeDirAbs", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.HomeDir = "."

		envYAML, err := yaml.Marshal(testEnv)
		if err != nil {
			t.Fatalf("could not marshal env.yaml: %v", err)
		}

		_, err = NewConfigEnv(envPath, envYAML)
		if !errors.Is(err, ErrNotAbsPath) {
			t.Fatalf("expected: %v, got: %v", ErrNotAbsPath, err)
		}
	})
	t.Run("shmAbs", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.SHMDir = "."

		envYAML, err := yaml.Marshal(testEnv)
		if err != nil {
			t.Fatalf("could not marshal env.yaml: %v", err)
		}

		_, err = NewConfigEnv(envPath, envYAML)
		if !errors.Is(err, ErrNotAbsPath) {
			t.Fatalf("expected: %v, got: %v", ErrNotAbsPath, err)
		}
	})
}

func TestPrepareEnvironment(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "")
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		storageDir := filepath.Join(tempDir, "configs")
		env := &ConfigEnv{
			StorageDir: storageDir,
		}

		if err := env.PrepareEnvironment(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !dirExist(env.RecordingsDir()) {
			t.Fatal("recordingsDir wasn't created")
		}
	})
	t.Run("storageMkdirErr", func(t *testing.T) {
		env := ConfigEnv{
			StorageDir: "/dev/null",
		}

		if err := env.PrepareEnvironment(); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func newTestGeneral(t *testing.T) (string, *ConfigGeneral, func()) {
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatalf("could not create tempoary directory: %v", err)
	}
	cancelFunc := func() {
		os.RemoveAll(tempDir)
	}

	configPath := filepath.Join(tempDir, "general.json")

	config := GeneralConfig{
		DiskSpace: "1",
	}
	data, _ := json.MarshalIndent(config, "", "    ")

	if err := os.WriteFile(configPath, data, 0o660); err != nil {
		t.Fatalf("could not write config file: %v", err)
	}

	general := ConfigGeneral{
		Config: config,
		path:   configPath,
	}

	return tempDir, &general, cancelFunc
}

func TestNewConfigGeneral(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		tempDir, testGeneral, cancel := newTestGeneral(t)
		defer cancel()

		general, _ := NewConfigGeneral(tempDir)

		actual := fmt.Sprintf("%v", general)
		expected := fmt.Sprintf("%v", testGeneral)

		if actual != expected {
			t.Fatalf("\nexpected: %v\n    got: %v", expected, actual)
		}
	})
	t.Run("genConfig", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "")
		defer os.RemoveAll(tempDir)
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}
		configDir := tempDir
		configFile := filepath.Join(configDir, "general.json")

		if dirExist(configFile) {
			t.Fatal("configFile should not already exist")
		}

		expected := "&{10000 default}"

		config1, err := NewConfigGeneral(configDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		file, err := os.ReadFile(configFile)
		if err != nil {
			t.Fatalf("could not read configFile: %v", err)
		}

		config2 := &GeneralConfig{}
		if err := json.Unmarshal(file, config2); err != nil {
			t.Fatalf("could not unmarshal config: %v", err)
		}

		actual1 := fmt.Sprintf("%v", &config1.Config)
		actual2 := fmt.Sprintf("%v", config2)

		if actual1 != expected {
			t.Fatalf("expected: %v got: %v", expected, actual1)
		}
		if actual2 != expected {
			t.Fatalf("expected: %v got: %v", expected, actual2)
		}
	})
	t.Run("genConfigErr", func(t *testing.T) {
		if _, err := NewConfigGeneral("/dev/null"); err == nil {
			t.Fatalf("expected: error, got: nil")
		}
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		tempDir, _, cancel := newTestGeneral(t)
		defer cancel()

		configPath := filepath.Join(tempDir, "general.json")
		if err := os.WriteFile(configPath, []byte{}, 0o660); err != nil {
			t.Fatalf("could not write configPath: %v", err)
		}

		if _, err := NewConfigGeneral(tempDir); err == nil {
			t.Fatalf("expected: error, got: nil")
		}
	})
}

func TestGeneral(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		tempDir, testGeneral, cancel := newTestGeneral(t)
		defer cancel()

		general, _ := NewConfigGeneral(tempDir)

		actual := fmt.Sprintf("%v", general.Get())
		expected := fmt.Sprintf("%v", testGeneral.Config)

		if actual != expected {
			t.Fatalf("expected: %v got: %v", expected, actual)
		}
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
		if err != nil {
			t.Fatalf("could not read config file: %v", err)
		}

		var config GeneralConfig
		if err := json.Unmarshal(file, &config); err != nil {
			t.Fatalf("could not unmarshal config file: %v", err)
		}

		actual1 := fmt.Sprintf("%v", general.Get())
		actual2 := fmt.Sprintf("%v", config)

		expected := fmt.Sprintf("%v", newConfig)

		if actual1 != expected {
			t.Fatalf("expected: %v got: %v", expected, actual1)
		}
		if actual2 != expected {
			t.Fatalf("expected: %v got: %v", expected, actual2)
		}
	})
	t.Run("setWriteFileErr", func(t *testing.T) {
		tempDir, _, cancel := newTestGeneral(t)
		defer cancel()

		general, _ := NewConfigGeneral(tempDir)
		os.RemoveAll(tempDir)

		err := general.Set(GeneralConfig{})
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected: %v, got: %v", os.ErrNotExist, err)
		}
	})
}
