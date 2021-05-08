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
	"testing"
	"time"
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
		logger := log.NewLogger(ctx)
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

func TestNewConfigEnv(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "")
	defer os.RemoveAll(tempDir)
	if err != nil {
		t.Fatalf("could not create tempoary directory: %v", err)
	}
	configPath := tempDir + "/env.json"

	workingConfig := func() *ConfigEnv {
		wd, _ := os.Getwd()
		return &ConfigEnv{
			FFmpegBin:  wd + "/1",
			GoBin:      wd + "/2",
			StorageDir: wd + "/3",
			SHMDir:     wd + "/4",
			HomeDir:    wd + "/5",
		}
	}
	writeWorkingFile := func() {
		data, _ := json.MarshalIndent(workingConfig(), "", "    ")
		ioutil.WriteFile(configPath, data, 0600)
	}

	t.Run("working", func(t *testing.T) {
		writeWorkingFile()
		env, _ := NewConfigEnv(tempDir)

		env.configDir = ""
		actual := fmt.Sprintf("%v", env)
		expected := fmt.Sprintf("%v", workingConfig())

		if actual != expected {
			t.Fatalf("\nexpected:\n%v.\ngot:\n%v.", expected, actual)
		}
	})

	t.Run("readFile error", func(t *testing.T) {
		_, err := NewConfigEnv("nil")
		if err == nil {
			t.Fatalf("expected error, got: nil")
		}
	})

	t.Run("unmarshal error", func(t *testing.T) {
		ioutil.WriteFile(configPath, []byte{}, 0660)
		_, err := NewConfigEnv(tempDir)
		if err == nil {
			t.Fatalf("expected error, got: nil")
		}
	})

	t.Run("shmHls", func(t *testing.T) {
		writeWorkingFile()
		env, _ := NewConfigEnv(tempDir)

		actual := fmt.Sprintf("%v", env.SHMhls())
		expected := workingConfig().SHMDir + "/hls"

		if actual != expected {
			t.Fatalf("expected: %v got: %v", expected, actual)
		}
	})
}

func TestPrepareEnvironment(t *testing.T) {
	dirExists := func(path string) bool {
		if _, err := os.Stat(path); err != nil {
			return !os.IsNotExist(err)
		}
		return true
	}

	t.Run("working", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		defer os.RemoveAll(tempDir)
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}
		configDir := tempDir + "/configs"

		env := ConfigEnv{
			SHMDir:    tempDir,
			configDir: configDir,
		}

		hlsDir := tempDir + "/" + env.SHMhls()

		testDir := hlsDir + "/test"
		os.MkdirAll(testDir, 0600)

		if err := env.PrepareEnvironment(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !dirExists(testDir) {
			t.Fatal("hlsDir was not reset")
		}

		if !dirExists(configDir + "/monitors") {
			t.Fatal("configs/monitors was not created")
		}
	})

	t.Run("monitorsMkdirErr", func(t *testing.T) {
		env := ConfigEnv{
			configDir: "/dev/null",
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
			configDir: configDir,
		}

		err = env.PrepareEnvironment()
		if err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestNewConfigGeneral(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "")
	defer os.RemoveAll(tempDir)
	if err != nil {
		t.Fatalf("could not create tempoary directory: %v", err)
	}
	configPath := tempDir + "/general.json"

	workingConfig := GeneralConfig{
		DiskSpace: "1",
	}
	writeWorkingFile := func() {
		data, _ := json.MarshalIndent(workingConfig, "", "    ")
		ioutil.WriteFile(configPath, data, 0660)
	}

	t.Run("working", func(t *testing.T) {
		writeWorkingFile()
		output, _ := NewConfigGeneral(tempDir)

		actual := fmt.Sprintf("%v", output)
		expected := fmt.Sprintf("%v", &ConfigGeneral{
			Config: workingConfig,
			path:   configPath,
		})

		if actual != expected {
			t.Fatalf("\nexpected: %v\n    got: %v", expected, actual)
		}
	})

	t.Run("readFile error", func(t *testing.T) {
		_, err := NewConfigGeneral("nil")
		if err == nil {
			t.Fatalf("expected error, got: nil")
		}
	})

	t.Run("unmarshal error", func(t *testing.T) {
		ioutil.WriteFile(configPath, []byte{}, 0660)
		_, err := NewConfigGeneral(tempDir)
		if err == nil {
			t.Fatalf("expected error, got: nil")
		}
	})

	t.Run("get", func(t *testing.T) {
		writeWorkingFile()
		g, _ := NewConfigGeneral(tempDir)

		actual := fmt.Sprintf("%v", g.Get())
		expected := fmt.Sprintf("%v", workingConfig)

		if actual != expected {
			t.Fatalf("expected: %v got: %v", expected, actual)
		}
	})

	t.Run("set", func(t *testing.T) {
		writeWorkingFile()
		g, _ := NewConfigGeneral(tempDir)

		newConfig := GeneralConfig{
			DiskSpace: "1",
		}
		g.Set(newConfig)

		actual := fmt.Sprintf("%v", g.Get())
		expected := fmt.Sprintf("%v", newConfig)

		if actual != expected {
			t.Fatalf("expected: %v got: %v", expected, actual)
		}
	})

	t.Run("set writeFile error", func(t *testing.T) {
		g, _ := NewConfigGeneral(tempDir)
		os.RemoveAll(tempDir)

		err := g.Set(GeneralConfig{})

		if err == nil {
			t.Fatalf("expected error, got: nil")
		}
	})
}
