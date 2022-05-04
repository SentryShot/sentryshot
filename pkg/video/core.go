package video

import (
	"context"
	"net/http"
	"nvr/pkg/log"
	"strconv"
	"sync"
	"time"
)

const (
	readTimeout     = 10 * time.Second
	writeTimeout    = 10 * time.Second
	readBufferSize  = 2048
	readBufferCount = 2048
)

// NewServer allocates a server.
func NewServer(log *log.Logger, wg *sync.WaitGroup, rtspPort int, hlsPort int) *Server {
	// Only allow local connections.
	rtspAddress := "127.0.0.1:" + strconv.Itoa(rtspPort)
	hlsAddress := "127.0.0.1:" + strconv.Itoa(hlsPort)

	pathManager := newPathManager(
		wg,
		rtspAddress,
		readTimeout,
		writeTimeout,
		readBufferCount,
		readBufferSize,
		log,
	)

	rtspServer := newRTSPServer(
		wg,
		rtspAddress,
		readTimeout,
		writeTimeout,
		readBufferCount,
		readBufferSize,
		log,
		pathManager,
	)

	hlsServer := newHLSServer(
		wg,
		hlsAddress,
		readBufferCount,
		pathManager,
		log,
	)

	return &Server{
		rtspAddress: rtspAddress,
		hlsAddress:  hlsAddress,
		pathManager: pathManager,
		rtspServer:  rtspServer,
		hlsServer:   hlsServer,
		wg:          wg,
	}
}

// Server is an instance of rtsp-simple-server.
type Server struct {
	rtspAddress string
	hlsAddress  string
	pathManager *pathManager
	rtspServer  *rtspServer
	hlsServer   *hlsServer
	wg          *sync.WaitGroup
}

// Start server.
func (p *Server) Start(ctx context.Context) error {
	ctx2, cancel := context.WithCancel(ctx)
	_ = cancel

	p.pathManager.start(ctx2)

	if err := p.rtspServer.start(ctx2); err != nil {
		cancel()
		return err
	}

	if err := p.hlsServer.start(ctx2); err != nil {
		cancel()
		return err
	}
	return nil
}

// CancelFunc .
type CancelFunc func()

// NewPath add path.
func (p *Server) NewPath(
	name string, newConf PathConf) (
	string, string, string, WaitForNewHLSsegementFunc, CancelFunc, error,
) {
	err := p.pathManager.AddPath(name, &newConf)
	if err != nil {
		return "", "", "", nil, nil, err
	}

	hlsAddress := "http://" + p.hlsAddress + "/hls/" + name + "/index.m3u8"
	rtspAddress := "rtsp://" + p.rtspAddress + "/" + name
	rtspProtocol := "tcp"
	cancelFunc := func() {
		p.pathManager.RemovePath(name)
	}

	return hlsAddress,
		rtspAddress,
		rtspProtocol,
		newConf.WaitForNewHLSsegment,
		cancelFunc, nil
}

// PathExist returns true if path exist.
func (p *Server) PathExist(name string) bool {
	return p.pathManager.pathExist(name)
}

// HandleHLS handle hls requests.
func (p *Server) HandleHLS() http.HandlerFunc {
	return p.hlsServer.HandleRequest()
}
