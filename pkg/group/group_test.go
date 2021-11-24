package group

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type cancelFunc func()

func prepareDir(t *testing.T) (string, cancelFunc) {
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatal(err)
	}
	configDir := tempDir + "/groups"

	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}

	err = filepath.Walk("./testdata/groups/", func(path string, info os.FileInfo, _ error) error {
		if !info.IsDir() {
			file, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if err := os.WriteFile(configDir+"/"+info.Name(), file, 0o600); err != nil {
				return err
			}

		}
		return nil
	})
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatal(err)
	}

	cancel := func() {
		os.RemoveAll(tempDir)
	}
	return configDir, cancel
}

func newTestManager(t *testing.T) (string, *Manager, context.CancelFunc) {
	configDir, cancel := prepareDir(t)

	cancelFunc := func() {
		cancel()
	}

	manager, err := NewManager(
		configDir,
	)
	if err != nil {
		t.Fatal(err)
	}

	return configDir, manager, cancelFunc
}

func readConfig(path string) (Config, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if json.Unmarshal(file, &config); err != nil {
		return nil, err
	}
	return config, nil
}

func TestNewManager(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		configDir, manager, cancel := newTestManager(t)
		defer cancel()

		config, err := readConfig(configDir + "/1.json")
		if err != nil {
			t.Fatalf("%v", err)
		}

		expected := fmt.Sprintf("%v", config)
		actual := fmt.Sprintf("%v", manager.Groups["1"].Config)

		if expected != actual {
			t.Fatalf("expected: %v, got %v", expected, actual)
		}
	})
	t.Run("readFileErr", func(t *testing.T) {
		_, err := NewManager(
			"/dev/null/nil.json",
		)
		if err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		configDir, cancel := prepareDir(t)
		defer cancel()

		data := []byte("{")
		if err := os.WriteFile(configDir+"/1.json", data, 0o600); err != nil {
			t.Fatalf("%v", err)
		}

		_, err := NewManager(
			configDir,
		)

		if err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestGroupSet(t *testing.T) {
	t.Run("createNew", func(t *testing.T) {
		configDir, manager, cancel := newTestManager(t)
		defer cancel()

		config := manager.Groups["1"].Config
		config["name"] = "new"
		err := manager.GroupSet("new", config)
		if err != nil {
			t.Fatalf("%v", err)
		}

		newName := manager.Groups["new"].Config["name"]
		if newName != "new" {
			t.Fatalf("expected: new, got: %v", newName)
		}

		// Check if changes were saved to file.
		config, err = readConfig(configDir + "/new.json")
		if err != nil {
			t.Fatalf("%v", err)
		}

		expected := fmt.Sprintf("%v", manager.Groups["new"].Config)
		actual := fmt.Sprintf("%v", config)

		if expected != actual {
			t.Fatalf("expected: %v, got %v", expected, actual)
		}
	})
	t.Run("setOld", func(t *testing.T) {
		configDir, manager, cancel := newTestManager(t)
		defer cancel()

		oldGroup := manager.Groups["1"]

		oldname := oldGroup.Config["name"]
		if oldname != "one" {
			t.Fatalf("expected: one, got: %v", oldname)
		}

		config := oldGroup.Config
		config["name"] = "two"
		err := manager.GroupSet("1", config)
		if err != nil {
			t.Fatalf("%v", err)
		}

		newName := manager.Groups["1"].Config["name"]
		if newName != "two" {
			t.Fatalf("expected: two, got: %v", newName)
		}

		// Check if changes were saved to file.
		config, err = readConfig(configDir + "/1.json")
		if err != nil {
			t.Fatalf("%v", err)
		}

		expected := fmt.Sprintf("%v", manager.Groups["1"].Config)
		actual := fmt.Sprintf("%v", config)

		if expected != actual {
			t.Fatalf("expected: %v, got %v", expected, actual)
		}
	})
	t.Run("writeFileErr", func(t *testing.T) {
		_, manager, cancel := newTestManager(t)
		defer cancel()

		manager.path = "/dev/null"
		if err := manager.GroupSet("1", Config{}); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestGroupDelete(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		_, manager, cancel := newTestManager(t)
		defer cancel()

		if _, exists := manager.Groups["1"]; !exists {
			t.Fatal("test group does not exist")
		}

		if err := manager.GroupDelete("1"); err != nil {
			t.Fatalf("%v", err)
		}

		if _, exists := manager.Groups["1"]; exists {
			t.Fatal("group was not deleted")
		}
	})
	t.Run("existErr", func(t *testing.T) {
		_, manager, cancel := newTestManager(t)
		defer cancel()

		if err := manager.GroupDelete("nil"); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("removeErr", func(t *testing.T) {
		_, manager, cancel := newTestManager(t)
		defer cancel()

		manager.path = "/dev/null"

		if err := manager.GroupDelete("1"); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestGroupConfigs(t *testing.T) {
	_, manager, cancel := newTestManager(t)
	defer cancel()

	expected := "map[1:map[id:1 monitors:[\"1\"] name:one] 2:map[id:2 monitors:[\"2\"] name:two]]"

	actual := fmt.Sprintf("%v", manager.Configs())
	if actual != expected {
		t.Fatalf("\nexpected:\n%v.\ngot:\n%v", expected, actual)
	}
}
