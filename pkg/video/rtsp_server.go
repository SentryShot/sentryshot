package video

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/base"
	"strconv"
	"sync"
	"time"
)

type rtspServer struct {
	address     string
	readTimeout time.Duration
	pathManager *pathManager
	logger      *log.Logger

	ctx      context.Context
	wg       *sync.WaitGroup
	srv      *gortsplib.Server
	mutex    sync.RWMutex
	conns    map[*gortsplib.ServerConn]*rtspConn
	sessions map[*gortsplib.ServerSession]*rtspSession
}

func newRTSPServer(
	wg *sync.WaitGroup,
	address string,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	readBufferSize int,
	logger *log.Logger,
	pathManager *pathManager) *rtspServer {
	s := &rtspServer{
		wg:          wg,
		address:     address,
		readTimeout: readTimeout,
		logger:      logger,
		pathManager: pathManager,
		conns:       make(map[*gortsplib.ServerConn]*rtspConn),
		sessions:    make(map[*gortsplib.ServerSession]*rtspSession),
	}

	s.srv = &gortsplib.Server{
		Handler:         s,
		ReadTimeout:     readTimeout,
		WriteTimeout:    writeTimeout,
		ReadBufferCount: readBufferCount,
		ReadBufferSize:  readBufferSize,
		RTSPaddress:     address,
	}

	return s
}

func (s *rtspServer) start(ctx context.Context) error {
	s.ctx = ctx

	err := s.srv.Start()
	if err != nil {
		return err
	}

	s.logger.Info().
		Src("app").
		Msgf("RTSP: listener opened on %v", s.address)

	s.wg.Add(1)
	go s.run()

	return nil
}

func (s *rtspServer) run() {
	defer s.wg.Done()

	serverErr := make(chan error)
	go func() {
		serverErr <- s.srv.Wait()
	}()

	select {
	case err := <-serverErr:
		s.logger.Error().Src("app").Msgf("rtps: server error: %s", err)
		return

	case <-s.ctx.Done():
		s.srv.Close()
		<-serverErr
		return
	}
}

func (s *rtspServer) newSessionID() (string, error) {
	for {
		b := make([]byte, 4)
		_, err := rand.Read(b)
		if err != nil {
			return "", err
		}

		u := binary.LittleEndian.Uint32(b)
		u %= 899999999
		u += 100000000

		id := strconv.FormatUint(uint64(u), 10)

		alreadyPresent := func() bool {
			for _, s := range s.sessions {
				if s.ID() == id {
					return true
				}
			}
			return false
		}()
		if !alreadyPresent {
			return id, nil
		}
	}
}

// OnConnOpen implements gortsplib.ServerHandlerOnConnOpen.
func (s *rtspServer) OnConnOpen(ctx *gortsplib.ServerHandlerOnConnOpenCtx) {
	c := newRTSPConn(
		s.address,
		s.readTimeout,
		s.pathManager,
		ctx.Conn,
		s.logger)

	s.mutex.Lock()
	s.conns[ctx.Conn] = c
	s.mutex.Unlock()
}

// OnConnClose implements gortsplib.ServerHandlerOnConnClose.
func (s *rtspServer) OnConnClose(ctx *gortsplib.ServerHandlerOnConnCloseCtx) {
	s.mutex.Lock()
	c := s.conns[ctx.Conn]
	delete(s.conns, ctx.Conn)
	s.mutex.Unlock()

	c.onClose(ctx.Error)
}

// OnRequest implements gortsplib.ServerHandlerOnRequest.
func (s *rtspServer) OnRequest(sc *gortsplib.ServerConn, req *base.Request) {
	s.mutex.Lock()
	c := s.conns[sc]
	s.mutex.Unlock()

	c.onRequest(req)
}

// OnResponse implements gortsplib.ServerHandlerOnResponse.
func (s *rtspServer) OnResponse(sc *gortsplib.ServerConn, res *base.Response) {
	s.mutex.Lock()
	c := s.conns[sc]
	s.mutex.Unlock()

	c.OnResponse(res)
}

// OnSessionOpen implements gortsplib.ServerHandlerOnSessionOpen.
func (s *rtspServer) OnSessionOpen(ctx *gortsplib.ServerHandlerOnSessionOpenCtx) {
	s.mutex.Lock()

	id, _ := s.newSessionID()

	se := newRTSPSession(
		id,
		ctx.Session,
		ctx.Conn,
		s.pathManager,
		s.logger)

	s.sessions[ctx.Session] = se
	s.mutex.Unlock()
}

// OnSessionClose implements gortsplib.ServerHandlerOnSessionClose.
func (s *rtspServer) OnSessionClose(ctx *gortsplib.ServerHandlerOnSessionCloseCtx) {
	s.mutex.Lock()
	se := s.sessions[ctx.Session]
	delete(s.sessions, ctx.Session)
	s.mutex.Unlock()

	if se != nil {
		se.onClose(*se.path.conf, ctx.Error)
	}
}

// OnDescribe implements gortsplib.ServerHandlerOnDescribe.
func (s *rtspServer) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	s.mutex.RLock()
	c := s.conns[ctx.Conn]
	s.mutex.RUnlock()
	return c.onDescribe(ctx)
}

// OnAnnounce implements gortsplib.ServerHandlerOnAnnounce.
func (s *rtspServer) OnAnnounce(ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.onAnnounce(ctx)
}

// OnSetup implements gortsplib.ServerHandlerOnSetup.
func (s *rtspServer) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.onSetup(ctx)
}

// OnPlay implements gortsplib.ServerHandlerOnPlay.
func (s *rtspServer) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.onPlay()
}

// OnRecord implements gortsplib.ServerHandlerOnRecord.
func (s *rtspServer) OnRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.onRecord()
}

// OnPause implements gortsplib.ServerHandlerOnPause.
func (s *rtspServer) OnPause(ctx *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.onPause()
}

// OnPacketRTP implements gortsplib.ServerHandlerOnPacket.
func (s *rtspServer) OnPacketRTP(ctx *gortsplib.ServerHandlerOnPacketRTPCtx) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	se.onPacketRTP(ctx)
}
