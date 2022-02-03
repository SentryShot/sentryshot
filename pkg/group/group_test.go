package group

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type cancelFunc func()

func prepareDir(t *testing.T) (string, cancelFunc) {
	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)

	configDir := tempDir + "/groups"

	err = os.Mkdir(configDir, 0o700)
	require.NoError(t, err)

	fileSystem := os.DirFS("./testdata/groups/")
	err = fs.WalkDir(fileSystem, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		file, err := fs.ReadFile(fileSystem, path)
		if err != nil {
			return err
		}
		newFilePath := filepath.Join(configDir, d.Name())
		if err := os.WriteFile(newFilePath, file, 0o600); err != nil {
			return err
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

func newTestManager(t *testing.T) (string, *Manager, cancelFunc) {
	configDir, cancel := prepareDir(t)

	manager, err := NewManager(
		configDir,
	)
	require.NoError(t, err)

	return configDir, manager, cancel
}

func readConfig(t *testing.T, path string) Config {
	file, err := os.ReadFile(path)
	require.NoError(t, err)

	var config Config
	err = json.Unmarshal(file, &config)
	require.NoError(t, err)

	return config
}

func TestNewManager(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		configDir, manager, cancel := newTestManager(t)
		defer cancel()

		config := readConfig(t, configDir+"/1.json")

		require.Equal(t, config, manager.Groups["1"].Config)
	})
	t.Run("mkDirErr", func(t *testing.T) {
		_, err := NewManager("/dev/null/nil")
		require.Error(t, err)
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		configDir, cancel := prepareDir(t)
		defer cancel()

		data := []byte("{")
		err := os.WriteFile(configDir+"/1.json", data, 0o600)
		require.NoError(t, err)

		_, err = NewManager(configDir)
		var e *json.SyntaxError
		require.ErrorAs(t, err, &e)
	})
}

func TestGroupSet(t *testing.T) {
	t.Run("createNew", func(t *testing.T) {
		configDir, manager, cancel := newTestManager(t)
		defer cancel()

		config := manager.Groups["1"].Config
		config["name"] = "new"

		err := manager.GroupSet("new", config)
		require.NoError(t, err)

		newName := manager.Groups["new"].Config["name"]
		require.Equal(t, newName, "new")

		// Check if changes were saved to file.
		config = readConfig(t, configDir+"/new.json")
		require.Equal(t, config, manager.Groups["new"].Config)
	})
	t.Run("setOld", func(t *testing.T) {
		configDir, manager, cancel := newTestManager(t)
		defer cancel()

		oldGroup := manager.Groups["1"]

		oldname := oldGroup.Config["name"]
		require.Equal(t, oldname, "one")

		config := oldGroup.Config
		config["name"] = "two"

		err := manager.GroupSet("1", config)
		require.NoError(t, err)

		newName := manager.Groups["1"].Config["name"]
		require.Equal(t, newName, "two")

		// Check if changes were saved to file.
		config = readConfig(t, configDir+"/1.json")
		require.Equal(t, config, manager.Groups["1"].Config)
	})
	t.Run("writeFileErr", func(t *testing.T) {
		_, manager, cancel := newTestManager(t)
		defer cancel()

		manager.path = "/dev/null"

		err := manager.GroupSet("1", Config{})
		require.Error(t, err)
	})
}

func TestGroupDelete(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		_, manager, cancel := newTestManager(t)
		defer cancel()

		require.NotNil(t, manager.Groups["1"])

		err := manager.GroupDelete("1")
		require.NoError(t, err)

		require.Nil(t, manager.Groups["1"])
	})
	t.Run("existErr", func(t *testing.T) {
		_, manager, cancel := newTestManager(t)
		defer cancel()

		err := manager.GroupDelete("nil")
		require.ErrorIs(t, err, ErrGroupNotExist)
	})
	t.Run("removeErr", func(t *testing.T) {
		_, manager, cancel := newTestManager(t)
		defer cancel()

		manager.path = "/dev/null"

		err := manager.GroupDelete("1")
		require.Error(t, err)
	})
}

func TestGroupConfigs(t *testing.T) {
	_, manager, cancel := newTestManager(t)
	defer cancel()

	actual := fmt.Sprintf("%v", manager.Configs())
	expected := "map[1:map[id:1 monitors:[\"1\"] name:one] 2:map[id:2 monitors:[\"2\"] name:two]]"
	require.Equal(t, actual, expected)
}
