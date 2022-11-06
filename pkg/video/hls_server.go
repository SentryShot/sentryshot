package video

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/hls"
	gopath "path"
	"strings"
	"sync"
	"time"
)

type hlsServer struct {
	readBufferCount int
	logger          *log.Logger

	ctx       context.Context
	ctxCancel func()
	wg        *sync.WaitGroup
	muxers    map[string]*HLSMuxer

	// in
	chPathSourceReady    chan pathSourceReadyRequest
	chPathSourceNotReady chan string
	chRequest            chan *hlsMuxerRequest
	chMuxerbyPathName    chan muxerByPathNameRequest
	chMuxerClose         chan *HLSMuxer
}

func newHLSServer(
	wg *sync.WaitGroup,
	readBufferCount int,
	logger *log.Logger,
) *hlsServer {
	return &hlsServer{
		readBufferCount:      readBufferCount,
		logger:               logger,
		wg:                   wg,
		muxers:               make(map[string]*HLSMuxer),
		chPathSourceReady:    make(chan pathSourceReadyRequest),
		chPathSourceNotReady: make(chan string),
		chRequest:            make(chan *hlsMuxerRequest),
		chMuxerbyPathName:    make(chan muxerByPathNameRequest),
		chMuxerClose:         make(chan *HLSMuxer),
	}
}

func (s *hlsServer) start(ctx context.Context, address string) error {
	s.ctx, s.ctxCancel = context.WithCancel(ctx)

	ln, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	s.logger.Log(log.Entry{
		Level: log.LevelInfo,
		Src:   "app",
		Msg:   fmt.Sprintf("HLS: listener opened on %v", address),
	})

	s.wg.Add(2)
	s.startServer(ln)
	go s.run()

	return nil
}

// Log is the main logging function.
/*func (s *hlsServer) logf(level log.Level, format string, args ...interface{}) {
	_ = level
	fmt.Printf("[HLS] "+format+"\n", append([]interface{}{}, args...)...)
}*/

func (s *hlsServer) startServer(ln net.Listener) {
	mux := http.NewServeMux()
	mux.Handle("/hls/", s.HandleRequest())
	server := http.Server{Handler: mux}

	go func() {
		for {
			err := server.Serve(ln)
			if !errors.Is(err, http.ErrServerClosed) {
				s.logger.Log(log.Entry{
					Level: log.LevelError,
					Src:   "app",
					Msg:   fmt.Sprintf("hls: server stopped: %v\nrestarting..", err),
				})
				time.Sleep(3 * time.Second)
			}
			if s.ctx.Err() != nil {
				return
			}
		}
	}()

	go func() {
		<-s.ctx.Done()
		server.Close()
		s.wg.Done()
	}()
}

// ErrMuxerAleadyExists muxer already exists.
var ErrMuxerAleadyExists = errors.New("muxer already exists")

func (s *hlsServer) run() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			s.ctxCancel()
			return

		case req := <-s.chPathSourceReady:
			if _, exist := s.muxers[req.path.name]; exist {
				req.res <- pathSourceReadyResponse{err: ErrMuxerAleadyExists}
			}

			m := newHLSMuxer(
				s.ctx,
				s.readBufferCount,
				s.wg,
				req.path,
				s.muxerClose,
				s.logger,
			)

			if err := m.start(req.tracks); err != nil {
				req.res <- pathSourceReadyResponse{
					err: fmt.Errorf("start hls muxer: %w", err),
				}
				continue
			}
			s.muxers[req.path.name] = m
			req.res <- pathSourceReadyResponse{muxer: m}

		case pathName := <-s.chPathSourceNotReady:
			if c, exist := s.muxers[pathName]; exist {
				c.close()
				delete(s.muxers, pathName)
			}

		case req := <-s.chRequest:
			m, exist := s.muxers[req.path]
			if exist {
				m.onRequest(req)
				continue
			}
			req.res <- &hls.MuxerFileResponse{Status: http.StatusNotFound}

		case req := <-s.chMuxerbyPathName:
			m, exist := s.muxers[req.pathName]
			if exist {
				req.res <- m
				continue
			}
			req.res <- nil

		case c := <-s.chMuxerClose:
			_, exist := s.muxers[c.path.name]
			if exist {
				delete(s.muxers, c.path.name)
			}
		}
	}
}

func (s *hlsServer) HandleRequest() http.HandlerFunc { //nolint:funlen
	return func(w http.ResponseWriter, r *http.Request) {
		// s.logf(log.LevelInfo, "[conn %v] %s %s", r.RemoteAddr, r.Method, r.URL.Path)

		w.Header().Set("Server", "rtsp-simple-server")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		switch r.Method {
		case http.MethodGet:

		case http.MethodOptions:
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
			w.WriteHeader(http.StatusOK)
			return

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Remove leading prefix "/hls/"
		if len(r.URL.Path) <= 5 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		pa := r.URL.Path[5:]

		dir, fname := func() (string, string) {
			if strings.HasSuffix(pa, ".ts") ||
				strings.HasSuffix(pa, ".m3u8") ||
				strings.HasSuffix(pa, ".mp4") {
				return gopath.Dir(pa), gopath.Base(pa)
			}
			return pa, ""
		}()

		if fname == "" && !strings.HasSuffix(dir, "/") {
			w.Header().Set("Location", "/hls/"+dir+"/")
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}

		dir = strings.TrimSuffix(dir, "/")

		cres := make(chan *hls.MuxerFileResponse)
		hreq := &hlsMuxerRequest{
			path: dir,
			file: fname,
			req:  r,
			res:  cres,
		}

		select {
		case <-s.ctx.Done():
		case s.chRequest <- hreq:
			res := <-cres

			for k, v := range res.Header {
				w.Header().Set(k, v)
			}
			w.WriteHeader(res.Status)

			if res.Body != nil {
				io.Copy(w, res.Body) //nolint:errcheck
			}
		}
	}
}

type pathSourceReadyRequest struct {
	path   *path
	tracks gortsplib.Tracks
	res    chan pathSourceReadyResponse
}

type pathSourceReadyResponse struct {
	muxer *HLSMuxer
	err   error
}

// pathSourceReady is called by path manager.
func (s *hlsServer) pathSourceReady(pa *path, tracks gortsplib.Tracks) (*HLSMuxer, error) {
	pathSourceRes := make(chan pathSourceReadyResponse)
	pathSourceReq := pathSourceReadyRequest{
		path:   pa,
		tracks: tracks,
		res:    pathSourceRes,
	}
	select {
	case s.chPathSourceReady <- pathSourceReq:
		res := <-pathSourceRes
		return res.muxer, res.err
	case <-s.ctx.Done():
		return nil, context.Canceled
	}
}

// pathSourceNotReady is called by pathManager.
func (s *hlsServer) pathSourceNotReady(pathName string) {
	select {
	case s.chPathSourceNotReady <- pathName:
	case <-s.ctx.Done():
	}
}

// muxerClose is called by hlsMuxer.
func (s *hlsServer) muxerClose(c *HLSMuxer) {
	select {
	case s.chMuxerClose <- c:
	case <-s.ctx.Done():
	}
}

type muxerByPathNameRequest struct {
	pathName string
	res      chan *HLSMuxer
}

// MuxerByPathName .
func (s *hlsServer) MuxerByPathName(pathName string) (*hls.Muxer, error) {
	muxerByPathNameRes := make(chan *HLSMuxer)
	muxerByPathNameReq := muxerByPathNameRequest{
		pathName: pathName,
		res:      muxerByPathNameRes,
	}
	select {
	case <-s.ctx.Done():
		return nil, context.Canceled
	case s.chMuxerbyPathName <- muxerByPathNameReq:
		res := <-muxerByPathNameRes
		if res == nil || res.muxer == nil {
			return nil, context.Canceled
		}
		return res.muxer, nil
	}
}
