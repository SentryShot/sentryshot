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

package doods

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	log2 "log"
	"net/http"
	"nvr"
	"nvr/pkg/log"
	"nvr/pkg/storage"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

var addon = struct {
	doodsIP      string
	detectorList detectors

	sendRequest sendRequestFunc

	log *log.Logger
}{}

func init() {
	nvr.RegisterEnvHook(onEnv)
	nvr.RegisterLogHook(func(log *log.Logger) {
		addon.log = log
	})
	nvr.RegisterLogSource([]string{"doods"})
	nvr.RegisterAppRunHook(onAppRun)
}

func onEnv(env *storage.ConfigEnv) {
	configPath := env.ConfigDir + "/doods.json"
	var err error
	addon.doodsIP, err = readConfig(configPath)
	if err != nil {
		log2.Fatalf("doods: config: %v, %v\n", err, configPath)
		return
	}

	for {
		addon.detectorList, err = newFetcher(addon.doodsIP).fetchDetectors()
		if err != nil {
			fmt.Printf("could not fetch detectors: %v %v\nretrying..\n", addon.doodsIP, err)
			time.Sleep(3 * time.Second)
			continue
		}
		return
	}
}

func onAppRun(ctx context.Context) error {
	client := newClient(ctx, addon.doodsIP)
	client.onError = func(err error) {
		addon.log.Error().Src("doods").Msg(err.Error())
	}
	fmt.Printf("starting doods client: %v\n", client.url)
	if err := client.dial(); err != nil {
		return err
	}
	go client.start()

	addon.sendRequest = client.sendRequest

	return nil
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

	file, err := os.ReadFile(configPath)
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
	if err := os.WriteFile(path, data, 0o600); err != nil {
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

func (f *fetcher) fetchDetectors() (detectors, error) {
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

	body, err := io.ReadAll(response.Body)
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
	Detectors detectors `json:"detectors"`
}

func detectorByName(name string) (detector, error) {
	for _, detector := range addon.detectorList {
		if detector.Name == name {
			return detector, nil
		}
	}
	return detector{}, fmt.Errorf("%v: %w", name, os.ErrNotExist)
}

type detectors []detector

type detector struct {
	Name string `json:"name"`
	// Type string `json:"type"`
	Model  string   `json:"model"`
	Labels []string `json:"labels"`
	Width  int32    `json:"width"`
	Height int32    `json:"height"`
}

type detectRequest struct {
	ID           string  `json:"id"`
	DetectorName string  `json:"detector_name"`
	Data         *[]byte `json:"data"`
	// Preprocess   []string   `json:"preprocess"`
	Detect thresholds `json:"detect"`
}

type (
	thresholds map[string]float64
	detections []Detection
)

type detectResponse struct {
	ID         string     `json:"id"`
	Detections detections `json:"detections"`
	Error      string     `json:"error"`
}

// Detection .
type Detection struct {
	Top        float32 `json:"top"`
	Left       float32 `json:"left"`
	Bottom     float32 `json:"bottom"`
	Right      float32 `json:"right"`
	Label      string  `json:"label"`
	Confidence float32 `json:"confidence"`
}

func newClient(ctx context.Context, doodsIP string) *client {
	return &client{
		ctx:     ctx,
		url:     "ws://" + doodsIP + "/detect",
		timeout: 1000 * time.Millisecond,

		pendingRequests: make(map[string]retChan),
		requestChan:     make(chan clientRequest),
		closedChan:      make(chan struct{}),
	}
}

type client struct {
	ctx     context.Context
	url     string
	timeout time.Duration

	conn            *websocket.Conn
	pendingRequests map[string]retChan
	requestChan     chan clientRequest
	responseChan    chan detectResponse
	closedChan      chan struct{}

	onError func(err error)
}

func (c *client) dial() error {
	ctx2, cancel := context.WithTimeout(c.ctx, c.timeout)
	defer cancel()

	var err error
	c.conn, _, err = websocket.DefaultDialer.DialContext(ctx2, c.url, nil) //nolint: bodyclose
	if err != nil {
		return fmt.Errorf("could not connect: %v %w", c.url, err)
	}
	return nil
}

func (c *client) reconnect() {
	c.conn.Close()

	for id, ret := range c.pendingRequests {
		close(ret)
		delete(c.pendingRequests, id)
	}
	for {
		if err := c.dial(); err != nil {
			if c.onError != nil {
				c.onError(err)
			}
			select {
			case <-time.After(c.timeout):
			case <-c.ctx.Done():
				return
			}
			continue
		}
		break
	}
	c.startReader()
}

func (c *client) start() {
	c.startReader()

	count := 0
	for {
		select {
		case r := <-c.requestChan:
			count++
			r.request.ID = strconv.Itoa(count)
			for {
				if err := c.conn.WriteJSON(r.request); err != nil {
					if c.ctx.Err() != nil {
						close(r.ret)
						break
					}
					if c.onError != nil {
						c.onError(err)
					}
					<-c.closedChan
					c.reconnect()
					continue
				}
				c.pendingRequests[r.request.ID] = r.ret
				break
			}

		case response := <-c.responseChan:
			if response.ID == "" {
				return
			}

			c.pendingRequests[response.ID] <- response
			delete(c.pendingRequests, response.ID)

		case <-c.closedChan:
			c.reconnect()

		case <-c.ctx.Done():
			close(c.requestChan)
			close(c.closedChan)
			for _, ret := range c.pendingRequests {
				close(ret)
			}
			return
		}
	}
}

func (c *client) startReader() {
	c.responseChan = make(chan detectResponse)
	go func() {
		for {
			var response detectResponse
			if err := c.conn.ReadJSON(&response); err != nil {
				// Reset on all read errors to avoid leaking pending requests.
				if c.onError != nil {
					c.onError(err)
				}
				if c.ctx.Err() != nil {
					close(c.responseChan)
					return
				}
				c.closedChan <- struct{}{}
				return
			}
			c.responseChan <- response
		}
	}()
}

type sendRequestFunc func(context.Context, detectRequest) (*detections, error)

var errDoods = errors.New("doods error")

func (c *client) sendRequest(ctx context.Context, request detectRequest) (*detections, error) {
	if c.ctx.Err() != nil {
		return nil, context.Canceled
	}

	ret := make(retChan)
	c.requestChan <- clientRequest{
		request: request,
		ret:     ret,
	}

	select {
	case <-ctx.Done():
		go func() { <-ret }()
		return nil, context.Canceled
	case response, ok := <-ret:
		if !ok {
			return nil, context.Canceled
		}
		if response.Error != "" {
			return nil, fmt.Errorf("%w: %v", errDoods, response.Error)
		}
		return &response.Detections, nil
	}
}

type clientRequest struct {
	request detectRequest
	ret     retChan
}

type retChan chan detectResponse

func dirExist(path string) bool {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false
		}
		return false
	}
	return true
}
