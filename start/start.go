// Package
// the purpuse of this script is to create a "main.go" with addons from
// addons.conf inserted into the import field. This will include the addons
// with the build. The file will then be run with the same environment and
// file descriptors as this script.

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

func main() {
	err := start()
	if err != nil {
		log.Fatal(err)
	}
}

type configEnv struct {
	GoBin   string `json:"goBin"`
	HomeDir string `json:"homeDir"`
}

func start() error {
	configDirectory := flag.String("configDir", "/home/_nvr/nvr/configs", "configuration directory")
	flag.Parse()

	configDir, err := filepath.Abs(*configDirectory)
	if err != nil {
		return fmt.Errorf("could not get absolute path of configDir: %v", err)
	}

	if _, err := os.Stat(configDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("--configDir '%v' does not exist", configDir)
		}
	}

	env, err := getEnv(configDir)
	if err != nil {
		return err
	}

	addons, err := getAddons(configDir)
	if err != nil {
		return err
	}

	main := "start/build/main.go"
	filePath := env.HomeDir + "/" + main

	os.Mkdir(env.HomeDir+"/start/build", 0700) //nolint:errcheck

	if err := genFile(filePath, addons); err != nil {
		return err
	}

	cmd := exec.Command(env.GoBin, "run", main, "--configDir", configDir)
	cmd.Dir = env.HomeDir

	// Give parrents file descriptors and environment to child process.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	fmt.Println("running..")
	return cmd.Run()
}

// getEnv reads environment configuration from "env.json"
func getEnv(configDir string) (configEnv, error) {
	var env configEnv

	file, err := ioutil.ReadFile(configDir + "/env.json")
	if err != nil {
		return configEnv{}, fmt.Errorf("could not read env.json: %v", err)
	}

	err = json.Unmarshal(file, &env)
	if err != nil {
		return configEnv{}, fmt.Errorf("could not unmarshal env.json: %v", err)
	}

	if env.HomeDir == "" {
		return configEnv{}, fmt.Errorf("'homeDir' missing in env.json")
	}

	return env, nil
}

// getAddons reads and parses "addons.conf"
func getAddons(configDir string) ([]string, error) {
	file, err := ioutil.ReadFile(configDir + "/addons.conf")
	if err != nil {
		return nil, fmt.Errorf("could not read addons.json: %v", err)
	}

	var addons []string
	lines := strings.Split(strings.TrimSpace(string(file)), "\n")
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Ignore lines starting with "#"
		if len(trimmedLine) == 0 || string(trimmedLine[0]) == "#" {
			continue
		}

		// Return error if trimmedLine contains spaces
		if strings.Contains(trimmedLine, " ") {
			return nil, fmt.Errorf("one addon per line: %v", trimmedLine)
		}

		addons = append(addons, trimmedLine)
	}
	return addons, nil
}

// genFile inserts addons into "main.go" template and writes to file.
func genFile(path string, addons []string) error {
	const file = `package main

import (
	"flag"
	"log"
	"nvr"
	"os"
{{ range . }}
	_ "{{ . }}"{{ end }}
)

func main() {
	configDir := flag.String("configDir", "/home/_nvr/nvr/configs", "configuration directory")
	if err := nvr.Run(*configDir); err != nil {
		log.Fatal(err)
	}
	os.Exit(0)
}
`

	t := template.New("file")
	t, _ = t.Parse(file)

	var b bytes.Buffer
	t.Execute(&b, addons) //nolint:errcheck

	if err := ioutil.WriteFile(path, b.Bytes(), 0600); err != nil {
		return fmt.Errorf("could not write build file: %v", err)
	}
	return nil
}
