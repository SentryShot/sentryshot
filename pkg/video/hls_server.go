package video

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"nvr/pkg/log"
	"nvr/pkg/video/hls"
	gopath "path"
	"strings"
	"sync"
	"time"
)

type hlsServerPathManager interface {
	hlsServerSet(s pathManagerHLSServer)
	readerAdd(req pathReaderAddReq) pathReaderAddRes
}

type hlsServer struct {
	readBufferCount int
	pathManager     hlsServerPathManager
	logger          *log.Logger

	ctx       context.Context
	ctxCancel func()
	wg        *sync.WaitGroup
	muxers    map[string]*HLSMuxer

	muxerByPathNameOnHold map[string]chan *HLSMuxer

	// in
	chPathSourceReady    chan *path
	chPathSourceNotReady chan *path
	chRequest            chan *hlsMuxerRequest
	chMuxerbyPathName    chan muxerByPathNameRequest
	chMuxerClose         chan *HLSMuxer
}

func newHLSServer(
	wg *sync.WaitGroup,
	readBufferCount int,
	pathManager hlsServerPathManager,
	logger *log.Logger,
) *hlsServer {
	s := &hlsServer{
		readBufferCount:       readBufferCount,
		pathManager:           pathManager,
		logger:                logger,
		wg:                    wg,
		muxers:                make(map[string]*HLSMuxer),
		muxerByPathNameOnHold: make(map[string]chan *HLSMuxer),
		chPathSourceReady:     make(chan *path),
		chPathSourceNotReady:  make(chan *path),
		chRequest:             make(chan *hlsMuxerRequest),
		chMuxerbyPathName:     make(chan muxerByPathNameRequest),
		chMuxerClose:          make(chan *HLSMuxer),
	}
	s.pathManager.hlsServerSet(s)
	return s
}

func (s *hlsServer) start(ctx context.Context, address string) error {
	s.ctx, s.ctxCancel = context.WithCancel(ctx)

	ln, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	s.logger.Info().
		Src("app").
		Msgf("HLS: listener opened on %v", address)

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
				s.logger.Error().
					Src("app").
					Msgf("hls: server stopped: %v\nrestarting..", err)

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

func (s *hlsServer) run() {
	defer s.wg.Done()

outer:
	for {
		select {
		case <-s.ctx.Done():
			for _, res := range s.muxerByPathNameOnHold {
				close(res)
			}
			break outer

		case pa := <-s.chPathSourceReady: // TODO: just pass name.
			s.findOrCreateMuxer(pa.Name(), nil)

		case pa := <-s.chPathSourceNotReady: // TODO: just pass name.
			if c, exist := s.muxers[pa.Name()]; exist {
				c.close()
				delete(s.muxers, pa.Name())
			}

		case req := <-s.chRequest:
			s.findOrCreateMuxer(req.path, req)

		case req := <-s.chMuxerbyPathName:
			m, exist := s.muxers[req.pathName]
			if exist {
				req.res <- m
				continue
			}
			s.muxerByPathNameOnHold[req.pathName] = req.res

		case c := <-s.chMuxerClose:
			if c2, ok := s.muxers[c.pathName]; !ok || c2 != c {
				continue
			}
			delete(s.muxers, c.pathName)
		}
	}

	s.ctxCancel()
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

		cres := make(chan func() *hls.MuxerFileResponse)
		hreq := &hlsMuxerRequest{
			path: dir,
			file: fname,
			req:  r,
			res:  cres,
		}

		select {
		case <-s.ctx.Done():
		case s.chRequest <- hreq:
			cb := <-cres

			res := cb()

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

func (s *hlsServer) findOrCreateMuxer(pathName string, req *hlsMuxerRequest) {
	m, exist := s.muxers[pathName]
	if !exist {
		m := newHLSMuxer(
			s.ctx,
			s.readBufferCount,
			req,
			s.wg,
			pathName,
			s.pathManager.readerAdd,
			s.muxerClose,
			s.logger)
		s.muxers[pathName] = m

		res, exist := s.muxerByPathNameOnHold[pathName]
		if exist {
			res <- m
			delete(s.muxerByPathNameOnHold, pathName)
		}
	} else if req != nil {
		m.onRequest(req)
	}
}

// pathSourceReady is called by path manager.
func (s *hlsServer) pathSourceReady(pa *path) {
	select {
	case s.chPathSourceReady <- pa:
	case <-s.ctx.Done():
	}
}

// pathSourceNotReady is called by pathManager.
func (s *hlsServer) pathSourceNotReady(pa *path) {
	select {
	case s.chPathSourceNotReady <- pa:
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
