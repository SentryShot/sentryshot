// Package
// the purpuse of this script is to create a "main.go" with addons from
// addons.conf inserted into the import field. This will include the addons
// with the build. The file will then be run with the same environment and
// file descriptors as this script.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"gopkg.in/yaml.v2"
)

func main() {
	err := start()
	if err != nil {
		log.Fatal(fmt.Errorf("start: %v", err))
	}
}

type configEnv struct {
	Addons  []string `yaml:"addons"`
	GoBin   string   `yaml:"goBin"`
	HomeDir string   `yaml:"homeDir"`
}

func start() error {
	envFlag := flag.String("env", "/home/_nvr/os-nvr/configs/env.yaml", "path to env.yaml")
	flag.Parse()

	if !dirExist(*envFlag) {
		return fmt.Errorf("--env %v does not exist", *envFlag)
	}

	envPath, err := filepath.Abs(*envFlag)
	if err != nil {
		return fmt.Errorf("could not get absolute path of env.yaml: %v", err)
	}

	envYAML, err := ioutil.ReadFile(envPath)
	if err != nil {
		return fmt.Errorf("could not read env.yaml: %v", err)
	}

	env, err := parseEnv(envPath, envYAML)
	if err != nil {
		return err
	}

	buildDir := env.HomeDir + "/start/build"
	os.Mkdir(buildDir, 0700) //nolint:errcheck

	main := buildDir + "/main.go"

	if err := genFile(main, env.Addons, envPath); err != nil {
		return err
	}

	cmd := exec.Command(env.GoBin, "run", main)
	cmd.Dir = env.HomeDir

	// Give parrents file descriptors and environment to child process.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	fmt.Println("starting..")
	return cmd.Run()
}

func parseEnv(envPath string, envYAML []byte) (*configEnv, error) {
	var env configEnv

	err := yaml.Unmarshal(envYAML, &env)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal env.yaml: %v", err)
	}

	if env.GoBin == "" {
		env.GoBin = "/usr/bin/go"
	}
	if env.HomeDir == "" {
		env.HomeDir = filepath.Dir(filepath.Dir(envPath))
	}

	if !dirExist(env.GoBin) {
		return nil, fmt.Errorf("goBin '%v' does not exist", env.GoBin)
	}
	if !dirExist(env.HomeDir) {
		return nil, fmt.Errorf("homeDir '%v' does not exist", env.HomeDir)
	}

	if !filepath.IsAbs(env.GoBin) {
		return nil, fmt.Errorf("goBin '%v' is not absolute path", env.GoBin)
	}
	if !filepath.IsAbs(env.HomeDir) {
		return nil, fmt.Errorf("homeDir '%v' is not absolute path", env.HomeDir)
	}

	return &env, nil
}

// genFile inserts addons into "main.go" template and writes to file.
func genFile(path string, addons []string, envPath string) error {
	const file = `package main

import (
	"log"
	"nvr"
	"os"
{{ range .addons }}
	_ "{{ . }}"{{ end }}
)

func main() {
	if err := nvr.Run("{{.envPath}}"); err != nil {
		log.Fatal(err)
	}
	os.Exit(0)
}
`

	t := template.New("file")
	t, _ = t.Parse(file)
	data := template.FuncMap{
		"addons":  addons,
		"envPath": envPath,
	}

	var b bytes.Buffer
	t.Execute(&b, data) //nolint:errcheck

	if err := ioutil.WriteFile(path, b.Bytes(), 0600); err != nil {
		return fmt.Errorf("could not write build file: %v", err)
	}
	return nil
}

func dirExist(path string) bool {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}
