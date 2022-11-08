package gortsplib

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"nvr/pkg/video/gortsplib/pkg/base"
	"nvr/pkg/video/gortsplib/pkg/liberrors"
	"strconv"
	"sync"
	"time"
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
	// The RTSP address of the server, to accept connections and send and receive
	// packets with the TCP transport.
	rtspAddress string

	// Timeout of read operations.
	readTimeout time.Duration

	// Timeout of write operations.
	writeTimeout time.Duration

	// Read buffer count.
	// If greater than 1, allows to pass buffers to routines different than the one
	// that is reading frames.
	// It also allows to buffer routed frames and mitigate network fluctuations.
	readBufferCount int

	// Write buffer count.
	// It allows to queue packets before sending them.
	writeBufferCount int

	handler ServerHandler

	// Function used to initialize the TCP listener.
	// It defaults to net.listen.
	listen func(network string, address string) (net.Listener, error)

	sessionTimeout    time.Duration
	checkStreamPeriod time.Duration

	ctx         context.Context
	ctxCancel   func()
	wg          sync.WaitGroup
	tcpListener net.Listener
	sessions    map[string]*ServerSession
	conns       map[*ServerConn]struct{}
	closeError  error

	// in
	connClose      chan *ServerConn
	sessionRequest chan sessionRequestReq
	sessionClose   chan *ServerSession
}

// NewServer creates a new RTSP server.
func NewServer(
	handler ServerHandler,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	writeBufferCount int,
	address string,
) *Server {
	return &Server{
		handler:          handler,
		readTimeout:      readTimeout,
		writeTimeout:     writeTimeout,
		readBufferCount:  readBufferCount,
		writeBufferCount: writeBufferCount,
		rtspAddress:      address,
	}
}

// Errors.
var (
	ErrServerMissingRTSPaddress = errors.New("RTSPAddress not provided")
	ErrWriteBufferSize          = errors.New("WriteBufferCount must be a power of two")
)

// Start starts the server.
func (s *Server) Start() error {
	// RTSP parameters
	if s.readTimeout == 0 {
		s.readTimeout = 10 * time.Second
	}
	if s.writeTimeout == 0 {
		s.writeTimeout = 10 * time.Second
	}
	if s.readBufferCount == 0 {
		s.readBufferCount = 256
	}
	if s.writeBufferCount == 0 {
		s.writeBufferCount = 256
	}
	if (s.writeBufferCount & (s.writeBufferCount - 1)) != 0 {
		return ErrWriteBufferSize
	}

	// system functions
	if s.listen == nil {
		s.listen = net.Listen
	}

	// private
	if s.sessionTimeout == 0 {
		s.sessionTimeout = 1 * 60 * time.Second
	}
	if s.checkStreamPeriod == 0 {
		s.checkStreamPeriod = 1 * time.Second
	}

	if s.rtspAddress == "" {
		return ErrServerMissingRTSPaddress
	}

	var err error
	s.tcpListener, err = s.listen("tcp", s.rtspAddress)
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
	s.connClose = make(chan *ServerConn)
	s.sessionRequest = make(chan sessionRequestReq)
	s.sessionClose = make(chan *ServerSession)

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

					select {
					case ss.request <- req:
					case <-ss.ctx.Done():
						req.res <- sessionRequestRes{
							res: &base.Response{
								StatusCode: base.StatusBadRequest,
							},
							err: context.Canceled,
						}
					}
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

					name, ok := req.req.URL.RTSPPath()
					if !ok {
						req.res <- sessionRequestRes{
							res: &base.Response{
								StatusCode: base.StatusBadRequest,
							},
							err: ErrServerInternalError,
						}
						continue
					}

					ss := newServerSession(s, secretID, req.sc, name)
					s.sessions[secretID] = ss

					select {
					case ss.request <- req:
					case <-ss.ctx.Done():
						req.res <- sessionRequestRes{
							res: &base.Response{
								StatusCode: base.StatusBadRequest,
							},
							err: context.Canceled,
						}
					}
				}

			case ss := <-s.sessionClose:
				if sss, ok := s.sessions[ss.secretID]; !ok || sss != ss {
					continue
				}
				delete(s.sessions, ss.secretID)
				ss.Close()

			case <-s.ctx.Done():
				return context.Canceled
			}
		}
	}()

	s.ctxCancel()

	s.tcpListener.Close()
}

// StartAndWait starts the server and waits until a fatal error.
func (s *Server) StartAndWait() error {
	err := s.Start()
	if err != nil {
		return err
	}

	return s.Wait()
}
