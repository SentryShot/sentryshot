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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func newTestConfig(t *testing.T) (string, func()) {
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatalf("could not create tempoary directory: %v", err)
	}

	cancelFunc := func() {
		os.RemoveAll(tempDir)
	}

	configPath := tempDir + "/doods.json"

	return configPath, cancelFunc
}

func TestReadConfig(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		configPath, cancel := newTestConfig(t)
		defer cancel()

		file := `{ "ip": "test:8080" }`

		if err := os.WriteFile(configPath, []byte(file), 0o600); err != nil {
			t.Fatalf("could not write test file: %v", err)
		}

		actual, err := readConfig(configPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := "test:8080"
		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
		}
	})
	t.Run("genFile", func(t *testing.T) {
		configPath, cancel := newTestConfig(t)
		defer cancel()

		if _, err := readConfig(configPath); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		file, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("could not read addon file: %v", err)
		}

		actual := string(file)

		file, _ = json.Marshal(defaultConfig)
		expected := string(file)

		if actual != expected {
			t.Errorf("expected: %v, got: %v", expected, actual)
		}
	})
	t.Run("genFileErr", func(t *testing.T) {
		if _, err := readConfig("/dev/null/nil"); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		configPath, cancel := newTestConfig(t)
		defer cancel()

		if err := os.WriteFile(configPath, []byte(""), 0o600); err != nil {
			t.Fatalf("could not write test file: %v", err)
		}

		if _, err := readConfig(configPath); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestNewFetcher(t *testing.T) {
	f := newFetcher("test")
	actual := f.url
	expected := "http://test/detectors"

	if actual != expected {
		t.Fatalf("expected: %v, got: %v", expected, actual)
	}
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
	response, _ := json.Marshal(getDetectorsResponce{testDetectors})

	t.Run("working", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, err := io.WriteString(w, string(response)); err != nil {
				t.Fatalf("could not write response: %v", err)
			}
		}))
		defer ts.Close()

		f := fetcher{url: ts.URL}
		detectors, err := f.fetchDetectors()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		actual := fmt.Sprintf("%v", detectors)
		expected := fmt.Sprintf("%v", testDetectors)

		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
		}
	})
	t.Run("createRequestErr", func(t *testing.T) {
		f := fetcher{url: string(rune(0x7f))}
		if _, err := f.fetchDetectors(); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("sendErr", func(t *testing.T) {
		f := fetcher{url: ""}
		if _, err := f.fetchDetectors(); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, err := io.WriteString(w, "nil"); err != nil {
				t.Fatalf("could not write response: %v", err)
			}
		}))
		defer ts.Close()

		f := fetcher{url: ts.URL}
		if _, err := f.fetchDetectors(); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestDetectorByName(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		addon.detectorList = testDetectors
		d, err := detectorByName("1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		actual := fmt.Sprintf("%v", d)
		expected := fmt.Sprintf("%v", testDetectors[0])

		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
		}
	})
	t.Run("error", func(t *testing.T) {
		addon.detectorList = testDetectors
		if _, err := detectorByName("nil"); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}

func TestClient(t *testing.T) {
	t.Run("singleRequest", func(t *testing.T) {
		ts, cancel := newTestServer(t)
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		client := newClient(ctx, ts.ip)
		if err := client.dial(); err != nil {
			t.Fatal(err)
		}
		go client.start()

		actual, err := client.sendRequest(
			context.Background(),
			detectRequest{DetectorName: "1"},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := detections{Detection{Label: "1"}}
		if !reflect.DeepEqual(*actual, expected) {
			t.Fatalf("expected: %v, got: %v", expected, *actual)
		}
	})
	t.Run("readReconnect", func(t *testing.T) {
		ts, cancel := newTestServer(t)
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		client := newClient(ctx, ts.ip)
		if err := client.dial(); err != nil {
			t.Fatal(err)
		}
		go client.start()

		ts.closeConn()
		time.Sleep(10 * time.Millisecond)

		actual, err := client.sendRequest(
			context.Background(),
			detectRequest{DetectorName: "1"},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := detections{Detection{Label: "1"}}
		if !reflect.DeepEqual(*actual, expected) {
			t.Fatalf("expected: %v, got: %v", expected, *actual)
		}
	})
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel2 := context.WithCancel(context.Background())
		cancel2()

		client := &client{ctx: ctx}

		_, err := client.sendRequest(context.Background(), detectRequest{})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected: %v, got: %v", context.Canceled, err)
		}
	})
	t.Run("canceled2", func(t *testing.T) {
		ts, cancel := newTestServer(t)
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		client := newClient(ctx, ts.ip)
		if err := client.dial(); err != nil {
			t.Fatal(err)
		}
		go client.start()

		ts.sendPause()
		defer ts.sendUnpause()

		go func() {
			time.Sleep(1 * time.Millisecond)
			cancel2()
		}()

		_, err := client.sendRequest(context.Background(), detectRequest{})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected: %v, got: %v", context.Canceled, err)
		}
	})
	t.Run("canceled3", func(t *testing.T) {
		ts, cancel := newTestServer(t)
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		client := newClient(ctx, ts.ip)
		if err := client.dial(); err != nil {
			t.Fatal(err)
		}
		go client.start()

		ts.sendPause()
		defer ts.sendUnpause()

		ctx2, cancel3 := context.WithCancel(context.Background())
		cancel3()

		_, err := client.sendRequest(ctx2, detectRequest{})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected: %v, got: %v", context.Canceled, err)
		}
	})
	t.Run("connectErr", func(t *testing.T) {
		client := newClient(context.Background(), "nil")
		err := client.dial()
		if err == nil {
			t.Fatal("expected: error, got: nil")
		}
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
		if err != nil {
			t.Fatal(err)
		}

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
		close(closeChan)
		close(sendPause)
		close(sendUnpause)
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
