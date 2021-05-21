package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

func newTestFlag(t *testing.T) (*configEnv, func()) {
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("could not create tempoary directory: %v", err)
	}

	cancelFunc := func() {
		os.RemoveAll(tempDir)
	}

	goBin := tempDir + "/a"
	homeDir := tempDir + "/b"
	configDir := tempDir + "/c"

	if err := ioutil.WriteFile(goBin, []byte{}, 0600); err != nil {
		t.Fatalf("could not write goBin: %v", err)
	}
	if err := ioutil.WriteFile(homeDir, []byte{}, 0600); err != nil {
		t.Fatalf("could not write homeDir: %v", err)
	}

	env := &configEnv{
		GoBin:     goBin,
		homeDir:   homeDir,
		ConfigDir: configDir,
	}

	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError) // Reset flags.
	os.Args = []string{"", "--goBin", goBin, "--homeDir", homeDir, "--configDir", configDir}

	return env, cancelFunc
}

func TestParseFlags(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		testEnv, cancel := newTestFlag(t)
		defer cancel()

		env, err := parseFlags()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := fmt.Sprintf("%v", env)
		expected := fmt.Sprintf("%v", testEnv)

		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
		}
	})
	t.Run("goExistErr", func(t *testing.T) {
		_, cancel := newTestFlag(t)
		defer cancel()

		os.Args[2] = "nil"

		if _, err := parseFlags(); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("homeExistErr", func(t *testing.T) {
		_, cancel := newTestFlag(t)
		defer cancel()

		os.Args[4] = "nil"

		if _, err := parseFlags(); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})

}

func newTestAddons(t *testing.T) (string, func()) {
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("could not create tempoary directory: %v", err)
	}

	cancelFunc := func() {
		os.RemoveAll(tempDir)
	}

	addonPath := tempDir + "/addons.conf"

	return addonPath, cancelFunc
}

func TestGetAddons(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		addonPath, cancel := newTestAddons(t)
		defer cancel()

		file := `
#comment
addon
 #comment
 ignoreSpace`

		if err := ioutil.WriteFile(addonPath, []byte(file), 0600); err != nil {
			t.Fatalf("could not write test file: %v", err)
		}

		addons, err := getAddons(addonPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := fmt.Sprintf("%v", addons)
		expected := "[addon ignoreSpace]"
		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
		}
	})
	t.Run("genFile", func(t *testing.T) {
		addonPath, cancel := newTestAddons(t)
		defer cancel()

		if _, err := getAddons(addonPath); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		file, err := ioutil.ReadFile(addonPath)
		if err != nil {
			t.Fatalf("could not read addon file: %v", err)
		}

		actual := string(file)
		expected := addonFile

		if actual != expected {
			t.Errorf("expected: %v, got: %v", expected, actual)
		}
	})
	t.Run("empty", func(t *testing.T) {
		addonPath, cancel := newTestAddons(t)
		defer cancel()

		if err := ioutil.WriteFile(addonPath, []byte(""), 0600); err != nil {
			t.Fatalf("could not write test file: %v", err)
		}

		if _, err := getAddons(addonPath); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("readFileErr", func(t *testing.T) {
		if _, err := getAddons("/dev/null/nil"); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("spaceErr", func(t *testing.T) {
		addonPath, cancel := newTestAddons(t)
		defer cancel()

		file := "sp ace"
		if err := ioutil.WriteFile(addonPath, []byte(file), 0600); err != nil {
			t.Fatalf("could not write test file: %v", err)
		}

		if _, err := getAddons(addonPath); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestGenFile(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		defer os.RemoveAll(tempDir)
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}

		path := tempDir + "/main.go"
		env := &configEnv{
			Addons:    []string{"a", "b", "c"},
			GoBin:     "d",
			ConfigDir: "e",
		}
		if err := genFile(path, env); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		file, err := ioutil.ReadFile(path)
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
	if err := nvr.Run("d", "e"); err != nil {
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
		if err := genFile("/dev/null/", &configEnv{}); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}
