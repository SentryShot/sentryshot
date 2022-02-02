package gortsplib

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"nvr/pkg/video/rtsp/gortsplib/pkg/base"
	"nvr/pkg/video/rtsp/gortsplib/pkg/liberrors"
	"strconv"
	"sync"
	"time"
)

const (
	serverReadBufferSize = 4096
)

func newSessionSecretID(sessions map[string]*ServerSession) (string, error) {
	for {
		b := make([]byte, 4)
		_, err := rand.Read(b)
		if err != nil {
			return "", err
		}

		id := strconv.FormatUint(uint64(binary.LittleEndian.Uint32(b)), 10)

		if _, ok := sessions[id]; !ok {
			return id, nil
		}
	}
}

type sessionRequestRes struct {
	ss  *ServerSession
	res *base.Response
	err error
}

type sessionRequestReq struct {
	sc     *ServerConn
	req    *base.Request
	id     string
	create bool
	res    chan sessionRequestRes
}

// Server is a RTSP server.
type Server struct {
	//
	// handler
	//
	// an handler to handle server events.
	Handler ServerHandler

	//
	// RTSP parameters
	//
	// timeout of read operations.
	// It defaults to 10 seconds
	ReadTimeout time.Duration
	// timeout of write operations.
	// It defaults to 10 seconds
	WriteTimeout time.Duration
	// the RTSP address of the server, to accept connections and send and receive
	// packets with the TCP transport.
	RTSPaddress string
	// read buffer count.
	// If greater than 1, allows to pass buffers to routines different than the one
	// that is reading frames.
	// It also allows to buffer routed frames and mitigate network fluctuations
	// that are particularly high when using UDP.
	// It defaults to 512
	ReadBufferCount int
	// read buffer size.
	// This must be touched only when the server reports errors about buffer sizes.
	// It defaults to 2048.
	ReadBufferSize int

	//
	// system functions
	//
	// function used to initialize the TCP listener.
	// It defaults to net.Listen.
	Listen func(network string, address string) (net.Listener, error)

	//
	// private
	//

	sessionTimeout    time.Duration
	checkStreamPeriod time.Duration

	ctx         context.Context
	ctxCancel   func()
	wg          sync.WaitGroup
	tcpListener net.Listener
	sessions    map[string]*ServerSession
	conns       map[*ServerConn]struct{}
	closeError  error
	streams     map[*ServerStream]struct{}

	// in
	connClose      chan *ServerConn
	sessionRequest chan sessionRequestReq
	sessionClose   chan *ServerSession
	streamAdd      chan *ServerStream
	streamRemove   chan *ServerStream
}

// ErrServerMissingRTSPaddress RTSPAddress not provided.
var ErrServerMissingRTSPaddress = errors.New("RTSPAddress not provided")

// Start starts the server.
func (s *Server) Start() error {
	// RTSP parameters
	if s.ReadTimeout == 0 {
		s.ReadTimeout = 10 * time.Second
	}
	if s.WriteTimeout == 0 {
		s.WriteTimeout = 10 * time.Second
	}
	if s.ReadBufferCount == 0 {
		s.ReadBufferCount = 512
	}
	if s.ReadBufferSize == 0 {
		s.ReadBufferSize = 2048
	}

	// system functions
	if s.Listen == nil {
		s.Listen = net.Listen
	}

	// private
	if s.sessionTimeout == 0 {
		s.sessionTimeout = 1 * 60 * time.Second
	}
	if s.checkStreamPeriod == 0 {
		s.checkStreamPeriod = 1 * time.Second
	}

	if s.RTSPaddress == "" {
		return ErrServerMissingRTSPaddress
	}

	var err error
	s.tcpListener, err = s.Listen("tcp", s.RTSPaddress)
	if err != nil {
		return err
	}

	s.ctx, s.ctxCancel = context.WithCancel(context.Background())

	s.wg.Add(1)
	go s.run()

	return nil
}

// Close closes all the server resources and waits for them to close.
func (s *Server) Close() error {
	s.ctxCancel()
	s.wg.Wait()
	return s.closeError
}

// Wait waits until all server resources are closed.
// This can happen when a fatal error occurs or when Close() is called.
func (s *Server) Wait() error {
	s.wg.Wait()
	return s.closeError
}

// ErrServerInternalError internal error.
var ErrServerInternalError = errors.New("internal error")

func (s *Server) run() { //nolint:funlen,gocognit
	defer s.wg.Done()

	s.sessions = make(map[string]*ServerSession)
	s.conns = make(map[*ServerConn]struct{})
	s.streams = make(map[*ServerStream]struct{})
	s.connClose = make(chan *ServerConn)
	s.sessionRequest = make(chan sessionRequestReq)
	s.sessionClose = make(chan *ServerSession)
	s.streamAdd = make(chan *ServerStream)
	s.streamRemove = make(chan *ServerStream)

	s.wg.Add(1)
	connNew := make(chan net.Conn)
	acceptErr := make(chan error)
	go func() {
		defer s.wg.Done()
		err := func() error {
			for {
				nconn, err := s.tcpListener.Accept()
				if err != nil {
					return err
				}

				select {
				case connNew <- nconn:
				case <-s.ctx.Done():
					nconn.Close()
				}
			}
		}()

		select {
		case acceptErr <- err:
		case <-s.ctx.Done():
		}
	}()

	s.closeError = func() error {
		for {
			select {
			case err := <-acceptErr:
				return err

			case nconn := <-connNew:
				sc := newServerConn(s, nconn)
				s.conns[sc] = struct{}{}

			case sc := <-s.connClose:
				if _, ok := s.conns[sc]; !ok {
					continue
				}
				delete(s.conns, sc)
				sc.Close()

			case req := <-s.sessionRequest:
				if ss, ok := s.sessions[req.id]; ok {
					if !req.sc.ip().Equal(ss.author.ip()) ||
						req.sc.zone() != ss.author.zone() {
						req.res <- sessionRequestRes{
							res: &base.Response{
								StatusCode: base.StatusBadRequest,
							},
							err: liberrors.ErrServerCannotUseSessionCreatedByOtherIP,
						}
						continue
					}

					ss.request <- req
				} else {
					if !req.create {
						req.res <- sessionRequestRes{
							res: &base.Response{
								StatusCode: base.StatusSessionNotFound,
							},
							err: liberrors.ErrServerSessionNotFound,
						}
						continue
					}

					secretID, err := newSessionSecretID(s.sessions)
					if err != nil {
						req.res <- sessionRequestRes{
							res: &base.Response{
								StatusCode: base.StatusBadRequest,
							},
							err: ErrServerInternalError,
						}
						continue
					}

					ss := newServerSession(s, secretID, req.sc)
					s.sessions[secretID] = ss

					select {
					case ss.request <- req:
					case <-ss.ctx.Done():
						req.res <- sessionRequestRes{
							res: &base.Response{
								StatusCode: base.StatusBadRequest,
							},
							err: liberrors.ErrServerTerminated,
						}
					}
				}

			case ss := <-s.sessionClose:
				if sss, ok := s.sessions[ss.secretID]; !ok || sss != ss {
					continue
				}
				delete(s.sessions, ss.secretID)
				ss.Close()

			case st := <-s.streamAdd:
				s.streams[st] = struct{}{}

			case st := <-s.streamRemove:
				delete(s.streams, st)

			case <-s.ctx.Done():
				return liberrors.ErrServerTerminated
			}
		}
	}()

	s.ctxCancel()

	s.tcpListener.Close()

	for st := range s.streams {
		st.Close()
	}
}

// StartAndWait starts the server and waits until a fatal error.
func (s *Server) StartAndWait() error {
	err := s.Start()
	if err != nil {
		return err
	}

	return s.Wait()
}
