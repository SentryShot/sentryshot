// Copyright 2020-2022 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; either version 2 of the License, or
// (at your option) any later version.
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
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"text/template"

	"gopkg.in/yaml.v2"
)

// ErrPathNotAbsolute path is not absolute.
var ErrPathNotAbsolute = errors.New("path is not absolute")

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
	envFlag := flag.String("env", "", "path to env.yaml")
	flag.Parse()

	if *envFlag == "" {
		flag.Usage()
		return nil
	}

	envPath, err := filepath.Abs(*envFlag)
	if err != nil {
		return fmt.Errorf("could not get absolute path of env.yaml: %w", err)
	}

	if !dirExist(envPath) {
		// Generate config and exit.
		if err := genConfigFile(envPath); err != nil {
			return fmt.Errorf("could not generate config file: %w", err)
		}
		return nil
	}

	envYAML, err := os.ReadFile(envPath)
	if err != nil {
		return fmt.Errorf("could not read env.yaml: %w", err)
	}

	env, err := parseEnv(envPath, envYAML)
	if err != nil {
		return err
	}

	buildDir := filepath.Join(env.HomeDir, "start", "build")
	err = os.Mkdir(buildDir, 0o700)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("could not create build directory: %w", err)
	}

	main := buildDir + "/nvr.go"

	if err := genBuildFile(main, env.Addons); err != nil {
		return err
	}

	return startMain(*env, main, envPath)
}

func startMain(env configEnv, main string, envPath string) error {
	// go run ./start/build/nvr.go -env ./config/env.yaml
	cmd := exec.Command(env.GoBin, "run", main, "-env", envPath)
	cmd.Dir = env.HomeDir

	// Give parent file descriptors and environment to child process.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	fmt.Println("starting..")
	if err := cmd.Start(); err != nil {
		return err
	}

	// Redirect interrupt to child process.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop)
	go func() {
		s := <-stop
		cmd.Process.Signal(s) //nolint:errcheck
	}()

	return cmd.Wait()
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
		return nil, fmt.Errorf("goBin '%v': %w", env.GoBin, ErrPathNotAbsolute)
	}
	if !filepath.IsAbs(env.HomeDir) {
		return nil, fmt.Errorf("homeDir '%v': %w", env.HomeDir, ErrPathNotAbsolute)
	}

	return &env, nil
}

// genBuildFile inserts addons into "main.go" template and writes to file.
func genBuildFile(path string, addons []string) error {
	const file = `package main

import (
	"log"
	"nvr"
	"os"
{{ range .addons }}
	_ "{{ . }}"{{ end }}
)

func main() {
	if err := nvr.Run(); err != nil {
		log.Fatalf("\n\nERROR: %v\n\n", err)
	}
	os.Exit(0)
}
`

	t := template.New("file")
	t, _ = t.Parse(file)
	data := template.FuncMap{
		"addons": addons,
	}

	var b bytes.Buffer
	t.Execute(&b, data) //nolint:errcheck

	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		return fmt.Errorf("could not write build file: %w", err)
	}
	return nil
}

func fileExist(path string) bool {
	if info, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) || info.IsDir() {
			return false
		}
	}
	return true
}

func dirExist(path string) bool {
	if info, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) || !info.IsDir() {
			return false
		}
	}
	return true
}

func genConfigFile(envPath string) error {
	fmt.Printf("\nGenerating `config/env.yaml` and exiting.\n\n")

	t := template.New("file")
	t, _ = t.Parse(configTemplate)

	var b bytes.Buffer
	data := configData(envPath)

	err := t.Execute(&b, data)
	if err != nil {
		return fmt.Errorf("could not execute template: %w", err)
	}

	err = os.WriteFile(envPath, b.Bytes(), 0o600)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf( //nolint:goerr113
				"the specified directory doesn't exist '%v':"+
					" please create it manually or use the './start/start.sh' script",
				filepath.Dir(envPath))
		}
		return fmt.Errorf("could not write config file: %w", err)
	}

	return nil
}

func configData(envPath string) template.FuncMap {
	ffmpegBin := "/usr/bin/ffmpeg"
	if fileExist("/usr/local/bin/ffmpeg") {
		ffmpegBin = "/usr/local/bin/ffmpeg"
	}
	if fileExist("/usr/bin/ffmpeg") {
		ffmpegBin = "/usr/bin/ffmpeg"
	}

	goBin := "/usr/go/bin/go"
	findGoBin(&goBin, "/usr/local")
	findGoBin(&goBin, "/lib/local")
	findGoBin(&goBin, "/usr")
	findGoBin(&goBin, "/lib")

	homeDir := filepath.Dir(filepath.Dir(envPath))

	return template.FuncMap{
		"goBin":     goBin,
		"ffmpegBin": ffmpegBin,
		"homeDir":   homeDir,
	}
}

func findGoBin(goBin *string, path string) {
	dirs, err := os.ReadDir(path)
	if err != nil {
		return
	}

	for _, dir := range dirs {
		// Check if directory name starts with `go`.
		if dir.Name()[:2] != "go" || !dir.IsDir() {
			continue
		}

		binPath := filepath.Join(path, dir.Name(), "bin", "go")
		if fileExist(binPath) {
			*goBin = binPath
		}
	}
}

var configTemplate = `
# Port app will be served on.
port: 2020

# Internal ports.
rtspPort: 2021
hlsPort: 2022

# Path to golang binary.
goBin: {{ .goBin }}

# Path to ffmpeg binary.
ffmpegBin: {{ .ffmpegBin }}

# Project home.
homeDir: {{ .homeDir }}

# Directory where recordings will be stored.
storageDir: {{ .homeDir }}/storage


addons: # Uncomment to enable.

  # Authentication. One must be enabled.
  #
  # Basic Auth.
  #- nvr/addons/auth/basic
  #
  # No authentication.
  #- nvr/addons/auth/none

  # Object detection. https://github.com/snowzach/doods2
  # Documentation is located at ../addons/doods2/README.md
  #- nvr/addons/doods2

  # Thumbnail downscaling.
  # Downscale video thumbnails to improve loading times and data usage.
  #- nvr/addons/thumbscale

  # System status.
  # Show system status in the web interface. CPU, RAM, disk usage.
  #- nvr/addons/status

  # Watchdog.
  # Detect and restart frozen processes.
  #- nvr/addons/watchdog

  # Timeline.
  # Works best with a Chromium based browser.
  #- nvr/addons/timeline
`
