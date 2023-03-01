package video

import (
	"context"
	"net/http"
	"nvr/pkg/log"
	"nvr/pkg/storage"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/hls"
	"strconv"
	"sync"
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

const readBufferCount = 2048

// NewServer allocates a server.
func NewServer(log *log.Logger, wg *sync.WaitGroup, env storage.ConfigEnv) *Server {
	rtspAddress := func() string {
		if env.RTSPPortExpose {
			return ":" + strconv.Itoa(env.RTSPPort)
		}
		return "127.0.0.1:" + strconv.Itoa(env.RTSPPort)
	}()
	hlsAddress := func() string {
		if env.HLSPortExpose {
			return ":" + strconv.Itoa(env.HLSPort)
		}
		return "127.0.0.1:" + strconv.Itoa(env.HLSPort)
	}()

	hlsServer := newHLSServer(wg, readBufferCount, log)
	pathManager := newPathManager(wg, log, hlsServer)
	rtspServer := newRTSPServer(wg, rtspAddress, readBufferCount, pathManager, log)

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

	if err := s.rtspServer.start(ctx2); err != nil {
		cancel()
		return err
	}

	if err := s.hlsServer.start(ctx2, s.hlsAddress); err != nil {
		cancel()
		return err
	}
	return nil
}

// CancelFunc .
type CancelFunc func()

// HlsMuxerFunc .
type HlsMuxerFunc func(context.Context) (IHLSMuxer, error)

// IHLSMuxer HLS muxer interface.
type IHLSMuxer interface {
	VideoTrack() *gortsplib.TrackH264
	AudioTrack() *gortsplib.TrackMPEG4Audio
	WaitForSegFinalized()
	NextSegment(prevID uint64) (*hls.Segment, error)
}

// ServerPath .
type ServerPath struct {
	HlsAddress   string
	RtspAddress  string
	RtspProtocol string
	HLSMuxer     HlsMuxerFunc
}

// NewPath add path.
func (s *Server) NewPath(ctx context.Context, name string, newConf PathConf) (*ServerPath, error) {
	hlsMuxer, err := s.pathManager.AddPath(ctx, name, newConf)
	if err != nil {
		return nil, err
	}

	return &ServerPath{
		HlsAddress:   "http://" + s.hlsAddress + "/hls/" + name + "/index.m3u8",
		RtspAddress:  "rtsp://" + s.rtspAddress + "/" + name,
		RtspProtocol: "tcp",
		HLSMuxer:     hlsMuxer,
	}, nil
}

// PathExist returns true if path exist.
func (s *Server) PathExist(name string) bool {
	return s.pathManager.pathExist(name)
}

// HandleHLS handle hls requests.
func (s *Server) HandleHLS() http.HandlerFunc {
	return s.hlsServer.HandleRequest()
}
