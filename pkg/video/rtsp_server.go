package video

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/base"
	"strconv"
	"sync"
	"time"

	"github.com/pion/rtp"
)

type rtspServer struct {
	address     string
	readTimeout time.Duration
	pathManager *pathManager
	logger      *log.Logger

	ctx      context.Context
	wg       *sync.WaitGroup
	srv      *gortsplib.Server
	mu       sync.RWMutex
	sessions map[*gortsplib.ServerSession]*rtspSession
}

const (
	readTimeout  = 10 * time.Second
	writeTimeout = 10 * time.Second
)

func newRTSPServer(
	wg *sync.WaitGroup,
	address string,
	readBufferCount int,
	pathManager *pathManager,
	logger *log.Logger,
) *rtspServer {
	s := &rtspServer{
		wg:          wg,
		address:     address,
		readTimeout: readTimeout,
		pathManager: pathManager,
		logger:      logger,
		sessions:    make(map[*gortsplib.ServerSession]*rtspSession),
	}

	s.srv = gortsplib.NewServer(
		s,
		readTimeout,
		writeTimeout,
		readBufferCount,
		readBufferCount,
		address,
	)

	return s
}

func (s *rtspServer) start(ctx context.Context) error {
	s.ctx = ctx

	err := s.srv.Start()
	if err != nil {
		return err
	}

	s.logger.Log(log.Entry{
		Level: log.LevelInfo,
		Src:   "app",
		Msg:   fmt.Sprintf("RTSP: listener opened on %v", s.address),
	})
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
		s.logger.Log(log.Entry{
			Level: log.LevelError,
			Src:   "app",
			Msg:   fmt.Sprintf("RTSP: server error: %s", err),
		})
		return

	case <-s.ctx.Done():
		s.srv.Close()
		<-serverErr
		return
	}
}

func (s *rtspServer) newSessionID() string {
	for {
		b := make([]byte, 4)
		_, err := rand.Read(b)
		if err != nil {
			panic(err)
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
			return id
		}
	}
}

// OnConnClose implements gortsplib.ServerHandler.
func (s *rtspServer) OnConnClose(sc *gortsplib.ServerConn, err error) {
	session := sc.Session()
	s.mu.Lock()
	se := s.sessions[session]
	s.mu.Unlock()

	if se != nil {
		se.onConnClose(err)
	}
}

// OnSessionOpen implements gortsplib.ServerHandler.
func (s *rtspServer) OnSessionOpen(
	session *gortsplib.ServerSession,
	conn *gortsplib.ServerConn,
	name string,
) {
	s.mu.Lock()

	pathLogf := s.pathManager.pathLogfByName(name)
	id := s.newSessionID()

	se := newRTSPSession(
		id,
		session,
		conn,
		s.pathManager,
		pathLogf,
	)

	s.sessions[session] = se
	s.mu.Unlock()
}

// OnSessionClose implements gortsplib.ServerHandler.
func (s *rtspServer) OnSessionClose(session *gortsplib.ServerSession, err error) {
	s.mu.Lock()
	se := s.sessions[session]
	delete(s.sessions, session)
	s.mu.Unlock()

	if se != nil {
		se.onClose(err)
	}
}

// OnDescribe implements gortsplib.ServerHandler.
func (s *rtspServer) OnDescribe(
	pathName string,
) (*base.Response, *gortsplib.ServerStream, error) {
	return s.pathManager.onDescribe(pathName)
}

// OnAnnounce implements gortsplib.ServerHandler.
func (s *rtspServer) OnAnnounce(
	session *gortsplib.ServerSession,
	path string,
	tracks gortsplib.Tracks,
) (*base.Response, error) {
	s.mu.RLock()
	se := s.sessions[session]
	s.mu.RUnlock()
	return se.onAnnounce(path, tracks)
}

// OnSetup implements gortsplib.ServerHandler.
func (s *rtspServer) OnSetup(
	session *gortsplib.ServerSession,
	path string,
	trackID int,
) (*base.Response, *gortsplib.ServerStream, error) {
	s.mu.RLock()
	se := s.sessions[session]
	s.mu.RUnlock()
	return se.onSetup(path, trackID)
}

// OnPlay implements gortsplib.ServerHandler.
func (s *rtspServer) OnPlay(
	session *gortsplib.ServerSession,
) (*base.Response, error) {
	s.mu.RLock()
	se := s.sessions[session]
	s.mu.RUnlock()
	return se.onPlay()
}

// OnRecord implements gortsplib.ServerHandler.
func (s *rtspServer) OnRecord(
	session *gortsplib.ServerSession,
) (*base.Response, error) {
	s.mu.RLock()
	se := s.sessions[session]
	s.mu.RUnlock()
	return se.onRecord()
}

// OnPacketRTP implements gortsplib.ServerHandler.
func (s *rtspServer) OnPacketRTP(
	session *gortsplib.ServerSession,
	trackID int,
	packet *rtp.Packet,
) {
	s.mu.RLock()
	se := s.sessions[session]
	s.mu.RUnlock()
	se.onPacketRTP(trackID, packet)
}

// OnDecodeError implements gortsplib.ServerHandler.
func (s *rtspServer) OnDecodeError(
	session *gortsplib.ServerSession,
	err error,
) {
	s.mu.Lock()
	se := s.sessions[session]
	s.mu.Unlock()

	if se != nil {
		se.onDecodeError(err)
	}
}
