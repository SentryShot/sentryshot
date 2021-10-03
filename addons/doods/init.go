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

package doods

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"nvr"
	"nvr/addons/doods/odrpc"
	"nvr/pkg/storage"
	"os"
	"time"
)

var (
	detectors []odrpc.Detector
	doodsIP   string
)

func init() {
	nvr.RegisterEnvHook(onEnvLoad)
}

func onEnvLoad(env *storage.ConfigEnv) {
	ip, err := readConfig(env.ConfigDir + "/doods.json")
	if err != nil {
		log.Fatalf("doods: config: %v", err)
	}
	doodsIP = ip

	d, err := newFetcher(ip).fetchDetectors()
	if err != nil {
		log.Fatalf("doods: could not fetch detectors: %v %v", ip, err)
	}
	detectors = d
}

// Config doods global configuration.
type Config struct {
	IP string `json:"ip"`
}

func readConfig(configPath string) (string, error) {
	if !dirExist(configPath) {
		if err := genConfig(configPath); err != nil {
			return "", fmt.Errorf("could not generate config: %w", err)
		}
	}

	file, err := ioutil.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("could not read config: %w", err)
	}

	var config Config
	if err := json.Unmarshal(file, &config); err != nil {
		return "", fmt.Errorf("could not unmarshal config: %w", err)
	}

	return config.IP, nil
}

var defaultConfig = Config{
	IP: "127.0.0.1:8080",
}

func genConfig(path string) error {
	data, _ := json.Marshal(defaultConfig)
	if err := ioutil.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return nil
}

func newFetcher(ip string) *fetcher {
	return &fetcher{
		url: "http://" + ip + "/detectors",
	}
}

type fetcher struct {
	url string
}

func (f *fetcher) fetchDetectors() ([]odrpc.Detector, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create request: %w", err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("could not send request: %w", err)
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read body: %w", err)
	}

	var d getDetectorsResponce
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("could not unmarshal response: %v %w", body, err)
	}

	return d.Detectors, nil
}

type getDetectorsResponce struct {
	Detectors []odrpc.Detector `json:"detectors"`
}

func detectorByName(name string) (odrpc.Detector, error) {
	for _, detector := range detectors {
		if detector.Name == name {
			return detector, nil
		}
	}
	return odrpc.Detector{}, fmt.Errorf("%v: %w", name, os.ErrNotExist)
}

func dirExist(path string) bool {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false
		}
		return false
	}
	return true
}
