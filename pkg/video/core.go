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
func NewServer(log *log.Logger, wg *sync.WaitGroup, rtspPort int) *Server {
	// Only allow local connections.
	rtspAddress := "127.0.0.1:" + strconv.Itoa(rtspPort)

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
		":2022",
		readBufferCount,
		pathManager,
		log,
	)

	return &Server{
		address:     rtspAddress,
		pathManager: pathManager,
		rtspServer:  rtspServer,
		hlsServer:   hlsServer,
		wg:          wg,
	}
}

// Server is an instance of rtsp-simple-server.
type Server struct {
	address     string
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
func (p *Server) NewPath(name string, newConf PathConf) (string, string, CancelFunc, error) {
	err := p.pathManager.AddPath(name, &newConf)
	if err != nil {
		return "", "", nil, err
	}

	address := "rtsp://" + p.address + "/" + name
	protocol := "tcp"
	cancelFunc := func() {
		p.pathManager.RemovePath(name)
	}

	return address, protocol, cancelFunc, nil
}

// PathExist returns true if path exist.
func (p *Server) PathExist(name string) bool {
	return p.pathManager.pathExist(name)
}

// HandleHLS handle hls requests.
func (p *Server) HandleHLS() http.HandlerFunc {
	return p.hlsServer.HandleRequest()
}
