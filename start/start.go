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

// Package
// the purpuse of this script is to create a "main.go" with addons from
// env.yaml inserted into the import field. This will include the addons
// with the build. The file will then be run with the same environment and
// file descriptors as this script.

package main

import (
	"bytes"
	"errors"
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

// ErrNotAbsolute path is not absolute.
var ErrNotAbsolute = errors.New("path is not absolute")

func main() {
	err := start()
	if err != nil {
		log.Fatal(fmt.Errorf("start: %w", err))
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
		return fmt.Errorf("--env %v: %w", *envFlag, os.ErrNotExist)
	}

	envPath, err := filepath.Abs(*envFlag)
	if err != nil {
		return fmt.Errorf("could not get absolute path of env.yaml: %w", err)
	}

	envYAML, err := ioutil.ReadFile(envPath)
	if err != nil {
		return fmt.Errorf("could not read env.yaml: %w", err)
	}

	env, err := parseEnv(envPath, envYAML)
	if err != nil {
		return err
	}

	buildDir := env.HomeDir + "/start/build"
	os.Mkdir(buildDir, 0o700) //nolint:errcheck

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
		return nil, fmt.Errorf("could not unmarshal env.yaml: %w", err)
	}

	if env.GoBin == "" {
		env.GoBin = "/usr/bin/go"
	}
	if env.HomeDir == "" {
		env.HomeDir = filepath.Dir(filepath.Dir(envPath))
	}

	if !dirExist(env.GoBin) {
		return nil, fmt.Errorf("goBin '%v': %w", env.GoBin, os.ErrNotExist)
	}
	if !dirExist(env.HomeDir) {
		return nil, fmt.Errorf("homeDir '%v': %w", env.HomeDir, os.ErrNotExist)
	}

	if !filepath.IsAbs(env.GoBin) {
		return nil, fmt.Errorf("goBin '%v': %w", env.GoBin, ErrNotAbsolute)
	}
	if !filepath.IsAbs(env.HomeDir) {
		return nil, fmt.Errorf("homeDir '%v': %w", env.HomeDir, ErrNotAbsolute)
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

	if err := ioutil.WriteFile(path, b.Bytes(), 0o600); err != nil {
		return fmt.Errorf("could not write build file: %w", err)
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
