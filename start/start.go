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
	"strings"
	"text/template"
)

func main() {
	err := start()
	if err != nil {
		log.Fatal(fmt.Errorf("start: %v", err))
	}
}

type configEnv struct {
	homeDir   string
	Addons    []string
	GoBin     string
	ConfigDir string
}

func start() error {
	env, err := parseFlags()
	if err != nil {
		return err
	}

	if !dirExist(env.ConfigDir) {
		if err := os.MkdirAll(env.ConfigDir, 0744); err != nil {
			return fmt.Errorf("could not create configDir: %v", err)
		}
	}

	addons, err := getAddons(env.ConfigDir + "/addons.conf")
	if err != nil {
		return err
	}
	env.Addons = addons

	main := "start/build/main.go"
	filePath := env.homeDir + "/" + main

	os.Mkdir(env.homeDir+"/start/build", 0700) //nolint:errcheck

	if err := genFile(filePath, env); err != nil {
		return err
	}

	cmd := exec.Command(env.GoBin, "run", main)
	cmd.Dir = env.homeDir

	// Give parrents file descriptors and environment to child process.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	fmt.Println("running..")
	return cmd.Run()
}

func parseFlags() (*configEnv, error) {
	goBin := flag.String("goBin", "go", "golang binary")
	homeDirectory := flag.String("homeDir", "/home/_nvr/os-nvr", "project home directory")
	configDirectory := flag.String("configDir", *homeDirectory+"/configs", "configuration directory")
	flag.Parse()

	homeDir, _ := filepath.Abs(*homeDirectory)
	configDir, _ := filepath.Abs(*configDirectory)

	if !dirExist(*goBin) {
		return nil, fmt.Errorf("--goBin '%v' does not exist", *goBin)
	}
	if !dirExist(homeDir) {
		return nil, fmt.Errorf("--homeDir '%v' does not exist", homeDir)
	}

	env := configEnv{
		homeDir:   homeDir,
		GoBin:     *goBin,
		ConfigDir: configDir,
	}

	return &env, nil
}

const addonFile = `# Uncomment to enable.

# Motion detection. Multiple zones, cannot handle shadows.
#nvr/addons/motion

# Object detection. https://github.com/snowzach/doods
#nvr/addons/doods`

// getAddons reads and parses "addons.conf"
func getAddons(addonPath string) ([]string, error) {
	if !dirExist(addonPath) {
		ioutil.WriteFile(addonPath, []byte(addonFile), 0600) //nolint:errcheck
	}

	file, err := ioutil.ReadFile(addonPath)
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
func genFile(path string, env *configEnv) error {
	const file = `package main

import (
	"log"
	"nvr"
	"os"
{{ range .Addons }}
	_ "{{ . }}"{{ end }}
)

func main() {
	if err := nvr.Run("{{.GoBin}}", "{{.ConfigDir}}"); err != nil {
		log.Fatal(err)
	}
	os.Exit(0)
}
`

	t := template.New("file")
	t, _ = t.Parse(file)
	data := env

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
