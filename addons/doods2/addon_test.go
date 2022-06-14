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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func newTestConfig(t *testing.T) (string, func()) {
	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)

	cancelFunc := func() {
		os.RemoveAll(tempDir)
	}

	configPath := tempDir + "/doods.json"

	return configPath, cancelFunc
}

func TestReadConfig(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		configPath, cancel := newTestConfig(t)
		defer cancel()

		file := `{ "ip": "test:8080" }`

		err := os.WriteFile(configPath, []byte(file), 0o600)
		require.NoError(t, err)

		ip, err := readConfig(configPath)
		require.NoError(t, err)
		require.Equal(t, ip, "test:8080")
	})
	t.Run("genFile", func(t *testing.T) {
		configPath, cancel := newTestConfig(t)
		defer cancel()

		_, err := readConfig(configPath)
		require.NoError(t, err)

		file, err := os.ReadFile(configPath)
		require.NoError(t, err)

		defaultConfigJSON, _ := json.Marshal(defaultConfig)
		require.Equal(t, file, defaultConfigJSON)
	})
	t.Run("genFileErr", func(t *testing.T) {
		_, err := readConfig("/dev/null/nil")
		var e *os.PathError
		require.ErrorAs(t, err, &e)
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		configPath, cancel := newTestConfig(t)
		defer cancel()

		err := os.WriteFile(configPath, []byte(""), 0o600)
		require.NoError(t, err)

		_, err = readConfig(configPath)
		var e *json.SyntaxError
		require.ErrorAs(t, err, &e)
	})
}

func TestNewFetcher(t *testing.T) {
	f := newFetcher("test")
	require.Equal(t, f.url, "http://test/detectors")
}

var testDetectors = detectors{
	{
		Name:   "1",
		Model:  "3",
		Labels: []string{"4"},
		Width:  5,
		Height: 6,
	},
	{
		Name: "1x",
	},
}

func TestFetchDetectors(t *testing.T) {
	response, err := json.Marshal(getDetectorsResponce{testDetectors})
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := io.WriteString(w, string(response))
			require.NoError(t, err)
		}))
		defer ts.Close()

		f := fetcher{url: ts.URL}

		detectors, err := f.fetchDetectors()
		require.NoError(t, err)
		require.Equal(t, detectors, testDetectors)
	})
	t.Run("createRequestErr", func(t *testing.T) {
		f := fetcher{url: string(rune(0x7f))}
		_, err := f.fetchDetectors()
		require.Error(t, err)
	})
	t.Run("sendErr", func(t *testing.T) {
		f := fetcher{url: ""}
		_, err := f.fetchDetectors()
		require.Error(t, err)
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := io.WriteString(w, "nil")
			require.NoError(t, err)
		}))
		defer ts.Close()

		f := fetcher{url: ts.URL}

		_, err := f.fetchDetectors()
		var e *json.SyntaxError
		require.ErrorAs(t, err, &e)
	})
}

func TestDetectorByName(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		addon.detectorList = testDetectors

		detector, err := detectorByName("1")
		require.NoError(t, err)
		require.Equal(t, detector, testDetectors[0])
	})
	t.Run("existError", func(t *testing.T) {
		addon.detectorList = testDetectors

		_, err := detectorByName("nil")
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestClient(t *testing.T) {
	t.Run("singleRequest", func(t *testing.T) {
		ts, cancel := newTestServer(t)
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		client := newClient(ctx, ts.ip)
		err := client.dial()
		require.NoError(t, err)

		go client.start()

		d, err := client.sendRequest(
			context.Background(),
			detectRequest{DetectorName: "1"},
		)
		require.NoError(t, err)
		require.Equal(t, d, &detections{Detection{Label: "1"}})
	})
	t.Run("readReconnect", func(t *testing.T) {
		ts, cancel := newTestServer(t)
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		client := newClient(ctx, ts.ip)
		err := client.dial()
		require.NoError(t, err)

		go client.start()

		ts.closeConn()
		time.Sleep(10 * time.Millisecond)

		d, err := client.sendRequest(
			context.Background(),
			detectRequest{DetectorName: "1"},
		)
		require.NoError(t, err)
		require.Equal(t, d, &detections{Detection{Label: "1"}})
	})
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel2 := context.WithCancel(context.Background())
		cancel2()

		client := &client{ctx: ctx}

		_, err := client.sendRequest(context.Background(), detectRequest{})
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("canceled2", func(t *testing.T) {
		ts, cancel := newTestServer(t)
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		client := newClient(ctx, ts.ip)
		err := client.dial()
		require.NoError(t, err)

		go client.start()

		ts.sendPause()
		defer ts.sendUnpause()

		go func() {
			time.Sleep(1 * time.Millisecond)
			cancel2()
		}()

		_, err = client.sendRequest(context.Background(), detectRequest{})
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("canceled3", func(t *testing.T) {
		ts, cancel := newTestServer(t)
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		client := newClient(ctx, ts.ip)
		err := client.dial()
		require.NoError(t, err)

		go client.start()

		ts.sendPause()
		defer ts.sendUnpause()

		ctx2, cancel3 := context.WithCancel(context.Background())
		cancel3()

		_, err = client.sendRequest(ctx2, detectRequest{})
		require.ErrorIs(t, err, context.Canceled)
	})
}

type cancelFunc func()

func newTestServer(t *testing.T) (*testServer, cancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	upgrader := websocket.Upgrader{}
	requestChan := make(chan detectRequest, 10)
	closeChan := make(chan struct{})
	sendPause := make(chan struct{})
	sendUnpause := make(chan struct{})

	detect := func(w http.ResponseWriter, r *http.Request) {
		if ctx.Err() != nil {
			return
		}
		ctx2, cancel2 := context.WithCancel(ctx)

		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)

		// Reciver.
		go func() {
			for {
				var request detectRequest
				if err := conn.ReadJSON(&request); err != nil {
					cancel2()
					return
				}
				requestChan <- request
			}
		}()

		// Sender.
		go func() {
			for {
				select {
				case <-sendPause:
					<-sendUnpause
				case request := <-requestChan:
					response := detectResponse{
						ID: request.ID,
						Detections: detections{
							Detection{Label: request.DetectorName},
						},
					}
					conn.WriteJSON(response)
				case <-closeChan:
					cancel2()
				case <-ctx2.Done():
					conn.Close()
					return
				}
			}
		}()
	}

	mux := http.NewServeMux()
	mux.Handle("/detect", http.HandlerFunc(detect))

	server := httptest.NewServer(mux)
	cancelFunc := func() {
		cancel()
		time.Sleep(10 * time.Millisecond)
		server.Close()
	}

	ip := strings.TrimPrefix(server.URL, "http://")
	ts := &testServer{
		ip:          ip,
		closeConn:   func() { closeChan <- struct{}{} },
		sendPause:   func() { sendPause <- struct{}{} },
		sendUnpause: func() { sendUnpause <- struct{}{} },
	}

	return ts, cancelFunc
}

type testServer struct {
	ip string

	closeConn   func()
	sendPause   func()
	sendUnpause func()
}
