package main

import (
	"fmt"
	"io/ioutil"
	"testing"
)

func TestGetEnv(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}

		file := `{"goBin":"test1", "homeDir": "test2"}`
		if err := ioutil.WriteFile(tempDir+"/env.json", []byte(file), 0600); err != nil {
			t.Fatalf("could not write test file: %v", err)
		}

		env, err := getEnv(tempDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		actual := fmt.Sprintf("%v", env)

		expected := "{test1 test2}"
		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
		}
	})
	t.Run("readFileErr", func(t *testing.T) {
		if _, err := getEnv("nil"); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}

		file := "nil"
		if err := ioutil.WriteFile(tempDir+"/env.json", []byte(file), 0600); err != nil {
			t.Fatalf("could not write test file: %v", err)
		}

		if _, err := getEnv(tempDir); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("missingErr", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}

		file := "{}"
		if err := ioutil.WriteFile(tempDir+"/env.json", []byte(file), 0600); err != nil {
			t.Fatalf("could not write test file: %v", err)
		}

		if _, err := getEnv(tempDir); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})

}

func TestGetAddons(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}

		file := `
#comment
addon
 #comment
 ignoreSpace`
		if err := ioutil.WriteFile(tempDir+"/addons.conf", []byte(file), 0600); err != nil {
			t.Fatalf("could not write test file: %v", err)
		}

		addons, err := getAddons(tempDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := fmt.Sprintf("%v", addons)
		expected := "[addon ignoreSpace]"
		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
		}
	})
	t.Run("empty", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}

		file := ``
		if err := ioutil.WriteFile(tempDir+"/addons.conf", []byte(file), 0600); err != nil {
			t.Fatalf("could not write test file: %v", err)
		}

		if _, err = getAddons(tempDir); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("readFileErr", func(t *testing.T) {
		if _, err := getAddons("nil"); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("spaceErr", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}

		file := "sp ace"
		if err := ioutil.WriteFile(tempDir+"/addons.conf", []byte(file), 0600); err != nil {
			t.Fatalf("could not write test file: %v", err)
		}

		if _, err := getAddons(tempDir); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestGenFile(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("could not create tempoary directory: %v", err)
		}

		path := tempDir + "/main.go"
		addons := []string{"a", "b", "c"}
		if err := genFile(path, addons, "d"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		file, err := ioutil.ReadFile(path)
		actual := string(file)
		expected := `package main

import (
	"flag"
	"log"
	"nvr"
	"os"

	_ "a"
	_ "b"
	_ "c"
)

func main() {
	configDir := flag.String("configDir", "d", "configuration directory")
	if err := nvr.Run(*configDir); err != nil {
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
		if err := genFile("/dev/null/", []string{}, ""); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}
