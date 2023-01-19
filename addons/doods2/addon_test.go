// SPDX-License-Identifier: GPL-2.0-or-later

package doods

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"nvr/pkg/log"

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

func logf(log.Level, string, ...interface{}) {}

func TestClient(t *testing.T) {
	t.Run("singleRequest", func(t *testing.T) {
		ts, cancel := newTestServer(t)
		defer cancel()

		client, wg, cancel2 := ts.newTestClient()

		wg.Add(1)
		go client.start()

		go func() { ts.respond("") }()

		d, err := client.sendRequest(
			context.Background(),
			detectRequest{DetectorName: "1"},
		)
		require.NoError(t, err)
		require.Equal(t, d, &detections{Detection{Label: "1"}})

		cancel2()
		wg.Wait()
	})
	t.Run("crashed", func(t *testing.T) {
		ts, cancel := newTestServer(t)
		defer cancel()

		client, wg, cancel2 := ts.newTestClient()
		client.logf = func(_ log.Level, format string, a ...interface{}) {
			fmt.Println("err", fmt.Sprintf(format, a...))
		}

		wg.Add(1)
		go client.start()

		ts.closeConn()
		time.Sleep(10 * time.Millisecond)

		go func() { ts.respond("") }()

		d, err := client.sendRequest(
			context.Background(),
			detectRequest{DetectorName: "1"},
		)
		require.NoError(t, err)
		require.Equal(t, d, &detections{Detection{Label: "1"}})

		cancel2()
		wg.Wait()
	})
}

func TestSendRequest(t *testing.T) {
	t.Run("canceledRequest", func(t *testing.T) {
		ctx, cancel2 := context.WithCancel(context.Background())
		cancel2()

		c := client{ctx: context.Background()}
		_, err := c.sendRequest(ctx, detectRequest{})
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("canceledClient", func(t *testing.T) {
		ctx, cancel2 := context.WithCancel(context.Background())
		cancel2()

		c := client{ctx: ctx}
		_, err := c.sendRequest(context.Background(), detectRequest{})
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("canceledRequest2", func(t *testing.T) {
		ts, cancel := newTestServer(t)
		defer cancel()

		client, wg, cancel2 := ts.newTestClient()

		wg.Add(1)
		go client.start()

		ctx, cancel3 := context.WithCancel(context.Background())
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel3()
		}()

		_, err := client.sendRequest(ctx, detectRequest{})
		require.ErrorIs(t, err, context.Canceled)
		cancel2()
		wg.Wait()
	})
	t.Run("canceledClient2", func(t *testing.T) {
		ts, cancel := newTestServer(t)
		defer cancel()

		client, wg, cancel2 := ts.newTestClient()

		wg.Add(1)
		go client.start()

		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel2()
		}()

		_, err := client.sendRequest(context.Background(), detectRequest{})
		require.ErrorIs(t, err, context.Canceled)
		wg.Wait()
	})
	t.Run("serverErr", func(t *testing.T) {
		ts, cancel := newTestServer(t)
		defer cancel()

		client, wg, cancel2 := ts.newTestClient()

		wg.Add(1)
		go client.start()

		go func() {
			time.Sleep(10 * time.Millisecond)
			ts.respond("error")
		}()

		_, err := client.sendRequest(context.Background(), detectRequest{})
		require.ErrorIs(t, err, errDoods)

		cancel2()
		wg.Wait()
	})
}

type cancelFunc func()

type testServer struct {
	ip string

	closeConn func()
	respond   func(serverError string)
}

func newTestServer(t *testing.T) (*testServer, cancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	requestChan := make(chan detectRequest, 10)
	closeChan := make(chan struct{})
	chRespond := make(chan string)

	detect := func(w http.ResponseWriter, r *http.Request) {
		ctx2, cancel2 := context.WithCancel(ctx)

		conn, err := new(websocket.Upgrader).Upgrade(w, r, nil)
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
				case request := <-requestChan:
					serverError := <-chRespond

					response := detectResponse{
						ID: request.ID,
						Detections: detections{
							Detection{Label: request.DetectorName},
						},
						ServerError: serverError,
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
		server.Close()
	}

	ip := strings.TrimPrefix(server.URL, "http://")
	ts := &testServer{
		ip:        ip,
		closeConn: func() { closeChan <- struct{}{} },
		respond:   func(serverError string) { chRespond <- serverError },
	}

	return ts, cancelFunc
}

func (ts *testServer) newTestClient() (*client, *sync.WaitGroup, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	c := &client{
		wg:         &wg,
		ctx:        ctx,
		logf:       logf,
		url:        "ws://" + ts.ip + "/detect",
		warmup:     0,
		timeout:    1000 * time.Millisecond,
		retrySleep: 0,

		pendingRequests: make(map[string]chan detectResponse),
		requestChan:     make(chan clientRequest),
		responseChan:    make(chan detectResponse),
	}
	return c, &wg, cancel
}
