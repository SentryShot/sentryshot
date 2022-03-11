package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func newTestEnv(t *testing.T) (string, *configEnv, func()) {
	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)

	cancelFunc := func() {
		os.RemoveAll(tempDir)
	}

	goBin := tempDir + "/go"
	homeDir := tempDir + "/home"
	configDir := homeDir + "/configs"

	err = os.WriteFile(goBin, []byte{}, 0o600)
	require.NoError(t, err)

	err = os.MkdirAll(configDir, 0o700)
	require.NoError(t, err)

	envPath := configDir + "/env.yaml"

	testEnv := &configEnv{
		Addons:  []string{"a"},
		GoBin:   goBin,
		HomeDir: homeDir,
	}

	return envPath, testEnv, cancelFunc
}

func TestParseEnv(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.HomeDir = ""

		envYAML, err := yaml.Marshal(testEnv)
		require.NoError(t, err)

		testEnv.HomeDir = filepath.Dir(filepath.Dir(envPath))

		env, err := parseEnv(envPath, envYAML)
		require.NoError(t, err)
		require.Equal(t, env, testEnv)
	})
	t.Run("maximal", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		envYAML, err := yaml.Marshal(testEnv)
		require.NoError(t, err)

		err = os.WriteFile(envPath, envYAML, 0o600)
		require.NoError(t, err)

		env, err := parseEnv(envPath, envYAML)
		require.NoError(t, err)
		require.Equal(t, env, testEnv)
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		_, err := parseEnv("", []byte("&"))
		require.Error(t, err)
	})
	t.Run("goExistErr", func(t *testing.T) {
		_, err := parseEnv("", []byte{})
		require.Error(t, err)
	})
	t.Run("homeExistErr", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.HomeDir = "nil"

		envYAML, err := yaml.Marshal(testEnv)
		require.NoError(t, err)

		_, err = parseEnv(envPath, envYAML)
		require.Error(t, err)
	})
	t.Run("goBinAbs", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.GoBin = "."

		envYAML, err := yaml.Marshal(testEnv)
		require.NoError(t, err)

		_, err = parseEnv(envPath, envYAML)
		require.ErrorIs(t, err, ErrPathNotAbsolute)
	})
	t.Run("homeDirAbs", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.HomeDir = "."

		envYAML, err := yaml.Marshal(testEnv)
		require.NoError(t, err)

		_, err = parseEnv(envPath, envYAML)
		require.ErrorIs(t, err, ErrPathNotAbsolute)
	})
}

func TestGenMainFile(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		path := tempDir + "/main.go"
		addons := []string{"a", "b", "c"}

		err = genBuildFile(path, addons)
		require.NoError(t, err)

		file, err := os.ReadFile(path)
		actual := string(file)
		expected := `package main

import (
	"log"
	"nvr"
	"os"

	_ "a"
	_ "b"
	_ "c"
)

func main() {
	if err := nvr.Run(); err != nil {
		log.Fatalf("\n\nERROR: %v\n\n", err)
	}
	os.Exit(0)
}
`
		require.Equal(t, actual, expected)
	})
	t.Run("writeFileErr", func(t *testing.T) {
		err := genBuildFile("/dev/null/", nil)
		require.Error(t, err)
	})
}
