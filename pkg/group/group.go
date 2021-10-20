package group

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Config Monitor configuration.
type Config map[string]string

// Configs Monitor configurations.
type Configs map[string]Config

// Group .
type Group struct {
	Config Config
	mu     sync.Mutex
}
type groups map[string]*Group

// Manager for the groups.
type Manager struct {
	Groups groups
	path   string
	mu     sync.Mutex
}

// NewManager return new group manager.
func NewManager(configPath string) (*Manager, error) {
	configFiles, err := readConfigs(configPath)
	if err != nil {
		return nil, fmt.Errorf("could not read configuration files: %w", err)
	}

	manager := &Manager{path: configPath}

	groups := make(groups)
	for _, file := range configFiles {
		var config Config
		if err := json.Unmarshal(file, &config); err != nil {
			return nil, fmt.Errorf("could not unmarshal config: %w: %v", err, file)
		}
		groups[config["id"]] = manager.newGroup(config)
	}
	manager.Groups = groups

	return manager, nil
}

func readConfigs(path string) ([][]byte, error) {
	var files [][]byte
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if strings.Contains(path, ".json") {
			file, err := ioutil.ReadFile(path)
			if err != nil {
				return fmt.Errorf("could not read file: %v %w", path, err)
			}
			files = append(files, file)
		}
		return nil
	})
	return files, err
}

// GroupSet sets config for specified group.
func (m *Manager) GroupSet(id string, c Config) error {
	defer m.mu.Unlock()
	m.mu.Lock()

	group, exist := m.Groups[id]
	if exist {
		group.mu.Lock()
		group.Config = c
		group.mu.Unlock()
	} else {
		group = m.newGroup(c)
		m.Groups[id] = group
	}

	// Update file.
	group.mu.Lock()
	config, _ := json.MarshalIndent(group.Config, "", "    ")

	if err := ioutil.WriteFile(m.configPath(id), config, 0o600); err != nil {
		return err
	}
	group.mu.Unlock()

	return nil
}

// ErrGroupNotExist group does not exist.
var ErrGroupNotExist = errors.New("group does not exist")

// GroupDelete deletes group by id.
func (m *Manager) GroupDelete(id string) error {
	defer m.mu.Unlock()
	m.mu.Lock()
	groups := m.Groups

	_, exists := groups[id]
	if !exists {
		return ErrGroupNotExist
	}

	delete(m.Groups, id)

	if err := os.Remove(m.configPath(id)); err != nil {
		return err
	}

	return nil
}

func (m *Manager) configPath(id string) string {
	return m.path + "/" + id + ".json"
}

// Configs returns configurations for all groups.
func (m *Manager) Configs() map[string]Config {
	configs := make(map[string]Config)

	m.mu.Lock()
	for _, group := range m.Groups {
		group.mu.Lock()
		configs[group.Config["id"]] = group.Config
		group.mu.Unlock()
	}
	m.mu.Unlock()

	return configs
}

func (m *Manager) newGroup(config Config) *Group {
	return &Group{
		Config: config,
	}
}
