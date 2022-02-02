package rtsp

import (
	"context"
	"nvr/pkg/log"
	"strconv"
	"sync"
	"time"
)

const (
	readBufferSize  = 2048
	readTimeout     = 10 * time.Second
	writeTimeout    = 10 * time.Second
	readBufferCount = 512
)

// NewServer allocates a server.
func NewServer(log *log.Logger, wg *sync.WaitGroup, port int) *Server {
	// Only allow local connections.
	address := "127.0.0.1:" + strconv.Itoa(port)

	pathManager := newPathManager(
		wg,
		address,
		readTimeout,
		writeTimeout,
		readBufferCount,
		readBufferSize,
		log,
	)

	rtspServer := newRTSPServer(
		wg,
		address,
		readTimeout,
		writeTimeout,
		readBufferCount,
		readBufferSize,
		log,
		pathManager,
	)

	return &Server{
		address:     address,
		pathManager: pathManager,
		rtspServer:  rtspServer,
	}
}

// Server is an instance of rtsp-simple-server.
type Server struct {
	address     string
	pathManager *pathManager
	rtspServer  *rtspServer
}

// Start server.
func (p *Server) Start(ctx context.Context) error {
	p.pathManager.start(ctx)
	return p.rtspServer.start(ctx)
}

// CancelFunc .
type CancelFunc func()

// NewPath add path.
func (p *Server) NewPath(name string, newConf PathConf) (string, string, CancelFunc, error) {
	err := p.pathManager.AddPath(name, newConf)
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
