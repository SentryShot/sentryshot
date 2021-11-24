package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v2"
)

func newTestEnv(t *testing.T) (string, *configEnv, func()) {
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatalf("could not create tempoary directory: %v", err)
	}

	cancelFunc := func() {
		os.RemoveAll(tempDir)
	}

	goBin := tempDir + "/go"
	homeDir := tempDir + "/home"
	configDir := homeDir + "/configs"

	if err := os.WriteFile(goBin, []byte{}, 0o600); err != nil {
		t.Fatalf("could not write goBin: %v", err)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("could not write configDir: %v", err)
	}

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
		if err != nil {
			t.Fatalf("could not marshal env: %v", err)
		}

		env, err := parseEnv(envPath, envYAML)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := fmt.Sprintf("%v", env)

		testEnv.HomeDir = filepath.Dir(filepath.Dir(envPath))
		expected := fmt.Sprintf("%v", testEnv)

		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
		}
	})
	t.Run("maximal", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		envYAML, err := yaml.Marshal(testEnv)
		if err != nil {
			t.Fatalf("could not marshal env: %v", err)
		}

		if err := os.WriteFile(envPath, envYAML, 0o600); err != nil {
			t.Fatalf("could not write env.yaml: %v", err)
		}

		env, err := parseEnv(envPath, envYAML)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := fmt.Sprintf("%v", env)
		expected := fmt.Sprintf("%v", testEnv)

		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
		}
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		if _, err := parseEnv("", []byte("&")); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("goExistErr", func(t *testing.T) {
		if _, err := parseEnv("", []byte{}); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("homeExistErr", func(t *testing.T) {
		envPath, testEnv, cancel := newTestEnv(t)
		defer cancel()

		testEnv.HomeDir = "nil"

		envYAML, err := yaml.Marshal(testEnv)
		if err != nil {
			t.Fatalf("could not marshal env: %v", err)
		}

		if _, err := parseEnv(envPath, envYAML); err == nil {
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

		if _, err := parseEnv(envPath, envYAML); err == nil {
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

		if _, err := parseEnv(envPath, envYAML); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestGenFile(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "")
		defer os.RemoveAll(tempDir)
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}

		path := tempDir + "/main.go"
		addons := []string{"a", "b", "c"}

		if err := genFile(path, addons, "d"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

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
	if err := nvr.Run("d"); err != nil {
		log.Fatal(err)
	}
	os.Exit(0)
}
`

		if actual != expected {
			//for i := range actual {
			//	fmt.Println(i, actual[i], expected[i], string(actual[i]), string(expected[i]))
			//}
			t.Fatalf("\nexpected: \n%v.\ngot: \n%v.", expected, actual)
		}
	})
	t.Run("writeFileErr", func(t *testing.T) {
		if err := genFile("/dev/null/", nil, ""); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}
