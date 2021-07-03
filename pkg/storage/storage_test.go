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
	"errors"
	"fmt"
	"io/ioutil"
	"nvr/pkg/log"
	"os"
	"path/filepath"
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
	var expected int64 = 2

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
		if err == nil {
			t.Fatalf("expected error, got %v", err)
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
	cases := []struct {
		name      string
		input     *Manager
		expectErr bool
	}{
		{
			"usage error",
			&Manager{
				general: diskSpaceErr,
				usage: func(_ string) int64 {
					return 1
				},
			},
			true,
		},
		{
			"below 99%",
			&Manager{
				general: diskSpace1,
				usage: func(_ string) int64 {
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
				removeAll: func(_ string) error {
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
				removeAll: func(_ string) error {
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
		logger := log.NewLogger()
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
		fmt.Println(actual)
		cancel2()
		cancel()

		expected := `failed to purge storage: strconv.ParseFloat: parsing "nil": invalid syntax`
		if actual != expected {
			t.Fatalf("\nexpected:\n%v.\ngot:\n%v.", expected, actual)
		}
	})
}

func newTestEnv(t *testing.T) (string, *ConfigEnv, func()) {
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("could not create tempoary directory: %v", err)
	}

	cancelFunc := func() {
		os.RemoveAll(tempDir)
	}

	homeDir := tempDir + "/home"
	goBin := homeDir + "/go"
	ffmpegBin := homeDir + "/ffmpeg"
	configDir := homeDir + "/configs"
	envPath := configDir + "/env.yaml"

	if err := os.MkdirAll(homeDir, 0700); err != nil {
		t.Fatalf("could not write homeDir: %v", err)
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("could not write configDir: %v", err)
	}
	if err := ioutil.WriteFile(goBin, []byte{}, 0600); err != nil {
		t.Fatalf("could not write goBin: %v", err)
	}
	if err := ioutil.WriteFile(ffmpegBin, []byte{}, 0600); err != nil {
		t.Fatalf("could not write ffmpegBin: %v", err)
	}

	env := &ConfigEnv{
		Port:       "2020",
		GoBin:      goBin,
		FFmpegBin:  ffmpegBin,
		StorageDir: homeDir + "/storage",
		SHMDir:     homeDir + "/shm",
		HomeDir:    homeDir,
		WebDir:     homeDir + "/web",
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
			GoBin:     homeDir + "/go",
			FFmpegBin: homeDir + "/ffmpeg",
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
			Port:       "2020",
			GoBin:      homeDir + "/go",
			FFmpegBin:  homeDir + "/ffmpeg",
			StorageDir: homeDir + "/storage",
			SHMDir:     "/dev/shm/nvr",
			HomeDir:    homeDir,
			WebDir:     homeDir + "/web",
			ConfigDir:  homeDir + "/configs",
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
			t.Fatalf("expected error, got: nil")
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
		expected := env.SHMDir + "/hls"

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

		if _, err := NewConfigEnv(envPath, envYAML); err == nil {
			t.Fatal("expected: error, got: nil")
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

		if _, err := NewConfigEnv(envPath, envYAML); err == nil {
			t.Fatal("expected: error, got: nil")
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

		if _, err := NewConfigEnv(envPath, envYAML); err == nil {
			t.Fatal("expected: error, got: nil")
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

		if _, err := NewConfigEnv(envPath, envYAML); err == nil {
			t.Fatal("expected: error, got: nil")
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

		if _, err := NewConfigEnv(envPath, envYAML); err == nil {
			t.Fatal("expected: error, got: nil")
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

		if _, err := NewConfigEnv(envPath, envYAML); err == nil {
			t.Fatal("expected: error, got: nil")
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

		if _, err := NewConfigEnv(envPath, envYAML); err == nil {
			t.Fatal("expected: error, got: nil")
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

		if _, err := NewConfigEnv(envPath, envYAML); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("webDirAbs", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.WebDir = "."

		envYAML, err := yaml.Marshal(testEnv)
		if err != nil {
			t.Fatalf("could not marshal env.yaml: %v", err)
		}

		if _, err := NewConfigEnv(envPath, envYAML); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestPrepareEnvironment(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		defer os.RemoveAll(tempDir)
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}
		configDir := tempDir + "/configs"

		env := &ConfigEnv{
			SHMDir:    tempDir,
			ConfigDir: configDir,
		}

		testDir := env.SHMhls() + "/test"
		if err := os.MkdirAll(testDir, 0744); err != nil {
			t.Fatalf("could not create temp directory: %v", err)
		}

		if err := env.PrepareEnvironment(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if dirExist(testDir) {
			t.Fatal("testDir wasn't reset")
		}

		if !dirExist(configDir + "/monitors") {
			t.Fatal("configs/monitors wasn't created")
		}
	})
	t.Run("monitorsMkdirErr", func(t *testing.T) {
		env := ConfigEnv{
			ConfigDir: "/dev/null",
		}

		if err := env.PrepareEnvironment(); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("hlsMkdirErr", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		defer os.RemoveAll(tempDir)
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}

		configDir := tempDir + "/configs"
		if err := os.MkdirAll(configDir, 0700); err != nil {
			t.Fatal(err)
		}

		env := ConfigEnv{
			SHMDir:    "/dev/null",
			ConfigDir: configDir,
		}

		err = env.PrepareEnvironment()
		if err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func newTestGeneral(t *testing.T) (string, *ConfigGeneral, func()) {
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("could not create tempoary directory: %v", err)
	}
	cancelFunc := func() {
		os.RemoveAll(tempDir)
	}

	configPath := tempDir + "/general.json"

	config := GeneralConfig{
		DiskSpace: "1",
	}
	data, _ := json.MarshalIndent(config, "", "    ")

	if err := ioutil.WriteFile(configPath, data, 0660); err != nil {
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
		tempDir, err := ioutil.TempDir("", "")
		defer os.RemoveAll(tempDir)
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}
		configDir := tempDir
		configFile := configDir + "/general.json"

		if dirExist(configFile) {
			t.Fatal("configFile should not already exist")
		}

		expected := "&{10000 default}"

		config1, err := NewConfigGeneral(configDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		file, err := ioutil.ReadFile(configFile)
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
			t.Fatalf("expected error, got: nil")
		}
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		tempDir, _, cancel := newTestGeneral(t)
		defer cancel()

		configPath := tempDir + "/general.json"
		if err := ioutil.WriteFile(configPath, []byte{}, 0660); err != nil {
			t.Fatalf("could not write configPath: %v", err)
		}

		_, err := NewConfigGeneral(tempDir)
		if err == nil {
			t.Fatalf("expected error, got: nil")
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

		file, err := ioutil.ReadFile(general.path)
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

		if err := general.Set(GeneralConfig{}); err == nil {
			t.Fatalf("expected error, got: nil")
		}
	})
}
