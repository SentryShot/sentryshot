package video

import (
	"context"
	"net/http"
	"nvr/pkg/log"
	"nvr/pkg/video/hls"
	"strconv"
	"sync"
	"time"
)

const (
	readTimeout     = 10 * time.Second
	writeTimeout    = 10 * time.Second
	readBufferCount = 2048
)

// Server is an instance of rtsp-simple-server.
type Server struct {
	rtspAddress string
	hlsAddress  string
	pathManager *pathManager
	rtspServer  *rtspServer
	hlsServer   *hlsServer
	wg          *sync.WaitGroup
}

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
		log,
	)

	rtspServer := newRTSPServer(
		wg,
		rtspAddress,
		readTimeout,
		writeTimeout,
		readBufferCount,
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

// Start server.
func (s *Server) Start(ctx context.Context) error {
	ctx2, cancel := context.WithCancel(ctx)
	_ = cancel

	s.pathManager.start(ctx2)

	if err := s.rtspServer.start(ctx2); err != nil {
		cancel()
		return err
	}

	if err := s.hlsServer.start(ctx2); err != nil {
		cancel()
		return err
	}
	return nil
}

// CancelFunc .
type CancelFunc func()

// ServerPath .
type ServerPath struct {
	HlsAddress           string
	RtspAddress          string
	RtspProtocol         string
	StreamInfo           hls.StreamInfoFunc
	WaitForNewHLSsegment WaitForNewHLSsegementFunc
}

// NewPath add path.
func (s *Server) NewPath(name string, newConf PathConf) (*ServerPath, CancelFunc, error) {
	err := s.pathManager.AddPath(name, &newConf)
	if err != nil {
		return nil, nil, err
	}

	cancelFunc := func() {
		s.pathManager.RemovePath(name)
	}

	return &ServerPath{
		HlsAddress:           "http://" + s.hlsAddress + "/hls/" + name + "/index.m3u8",
		RtspAddress:          "rtsp://" + s.rtspAddress + "/" + name,
		RtspProtocol:         "tcp",
		StreamInfo:           newConf.streamInfo,
		WaitForNewHLSsegment: newConf.WaitForNewHLSsegment,
	}, cancelFunc, nil
}

// PathExist returns true if path exist.
func (s *Server) PathExist(name string) bool {
	return s.pathManager.pathExist(name)
}

// HandleHLS handle hls requests.
func (s *Server) HandleHLS() http.HandlerFunc {
	return s.hlsServer.HandleRequest()
}
