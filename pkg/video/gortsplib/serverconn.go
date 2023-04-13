package gortsplib

import (
	"context"
	"errors"
	"fmt"
	"net"
	"nvr/pkg/video/gortsplib/pkg/base"
	"nvr/pkg/video/gortsplib/pkg/conn"
	"nvr/pkg/video/gortsplib/pkg/liberrors"
	"strings"
	"time"
)

func getSessionID(header base.Header) string {
	if h, ok := header["Session"]; ok && len(h) == 1 {
		return h[0]
	}
	return ""
}

type readReq struct {
	req *base.Request
	res chan error
}

// ServerConn is a server-side RTSP connection.
type ServerConn struct {
	s     *Server
	nconn net.Conn

	ctx        context.Context
	ctxCancel  func()
	remoteAddr *net.TCPAddr
	conn       *conn.Conn
	session    *ServerSession
	readFunc   func(readRequest chan readReq) error

	// in
	sessionRemove chan *ServerSession

	// out
	done chan struct{}
}

func newServerConn(
	s *Server,
	nconn net.Conn,
) *ServerConn {
	ctx, ctxCancel := context.WithCancel(s.ctx)
	sc := &ServerConn{ //nolint:forcetypeassert
		s:             s,
		nconn:         nconn,
		ctx:           ctx,
		ctxCancel:     ctxCancel,
		remoteAddr:    nconn.RemoteAddr().(*net.TCPAddr),
		sessionRemove: make(chan *ServerSession),
		done:          make(chan struct{}),
	}

	sc.readFunc = sc.readFuncStandard

	s.wg.Add(1)
	go sc.run()

	return sc
}

// Close closes the ServerConn.
func (sc *ServerConn) Close() error {
	sc.ctxCancel()
	return nil
}

// NetConn returns the underlying net.Conn.
func (sc *ServerConn) NetConn() net.Conn {
	return sc.nconn
}

func (sc *ServerConn) Session() *ServerSession {
	return sc.session
}

func (sc *ServerConn) ip() net.IP {
	return sc.remoteAddr.IP
}

func (sc *ServerConn) zone() string {
	return sc.remoteAddr.Zone
}

func (sc *ServerConn) run() {
	defer sc.s.wg.Done()
	defer close(sc.done)

	sc.conn = conn.NewConn(sc.nconn)

	readRequest := make(chan readReq)
	readErr := make(chan error)
	readDone := make(chan struct{})
	go sc.runReader(readRequest, readErr, readDone)

	err := func() error {
		for {
			select {
			case req := <-readRequest:
				req.res <- sc.handleRequestOuter(req.req)

			case err := <-readErr:
				return fmt.Errorf("read: %w", err)

			case ss := <-sc.sessionRemove:
				if sc.session == ss {
					sc.session = nil
				}

			case <-sc.ctx.Done():
				return context.Canceled
			}
		}
	}()

	sc.ctxCancel()

	sc.nconn.Close()
	<-readDone

	if sc.session != nil {
		select {
		case sc.session.connRemove <- sc:
		case <-sc.session.ctx.Done():
		}
	}

	select {
	case sc.s.connClose <- sc:
	case <-sc.s.ctx.Done():
	}

	sc.s.handler.OnConnClose(sc, err)
}

var errSwitchReadFunc = errors.New("switch read function")

func (sc *ServerConn) runReader(readRequest chan readReq, readErr chan error, readDone chan struct{}) {
	defer close(readDone)

	for {
		err := sc.readFunc(readRequest)

		if errors.Is(err, errSwitchReadFunc) {
			continue
		}

		select {
		case readErr <- err:
		case <-sc.ctx.Done():
		}
		break
	}
}

func (sc *ServerConn) readFuncStandard(readRequest chan readReq) error {
	// reset deadline
	sc.nconn.SetReadDeadline(time.Time{}) //nolint:errcheck

	for {
		value, err := sc.conn.ReadInterleavedFrameOrRequest()
		if err != nil {
			return err
		}

		switch what := value.(type) {
		case *base.Request:
			cres := make(chan error)
			select {
			case readRequest <- readReq{req: what, res: cres}:
				err = <-cres
				if err != nil {
					return err
				}

			case <-sc.ctx.Done():
				return context.Canceled
			}
		default:
			return liberrors.ErrServerUnexpectedFrame
		}
	}
}

func (sc *ServerConn) readFuncTCP(readRequest chan readReq) error {
	// reset deadline
	sc.nconn.SetReadDeadline(time.Time{}) //nolint:errcheck

	select {
	case sc.session.startWriter <- struct{}{}:
	case <-sc.session.ctx.Done():
	}

	processFunc := func(*ServerSessionSetuppedTrack, []byte) error {
		return nil
	}

	if sc.session.state != ServerSessionStatePlay {
		tcpRTPPacketBuffer := newRTPPacketMultiBuffer(uint64(sc.s.readBufferCount))

		processFunc = func(track *ServerSessionSetuppedTrack, payload []byte) error {
			pkt := tcpRTPPacketBuffer.next()
			err := pkt.Unmarshal(payload)
			if err != nil {
				return fmt.Errorf("unmarshal packet: %w", err)
			}

			sc.s.handler.OnPacketRTP(sc.session, track.id, pkt)

			return nil
		}
	}

	for {
		if sc.session.state == ServerSessionStateRecord {
			sc.nconn.SetReadDeadline(time.Now().Add(sc.s.readTimeout)) //nolint:errcheck
		}

		what, err := sc.conn.ReadInterleavedFrameOrRequest()
		if err != nil {
			return err
		}

		switch twhat := what.(type) {
		case *base.InterleavedFrame:
			// forward frame only if it has been set up
			if track, ok := sc.session.tcpTracksByChannel[twhat.Channel]; ok {
				err := processFunc(track, twhat.Payload)
				if err != nil {
					return err
				}
			}

		case *base.Request:
			cres := make(chan error)
			select {
			case readRequest <- readReq{req: twhat, res: cres}:
				err := <-cres
				if err != nil {
					return err
				}

			case <-sc.ctx.Done():
				return context.Canceled
			}
		}
	}
}

var supportedMethods = []string{
	string(base.Describe),
	string(base.Announce),
	string(base.Setup),
	string(base.Play),
	string(base.Record),
	string(base.Teardown),
}

func (sc *ServerConn) handleRequest(req *base.Request) (*base.Response, error) { //nolint:funlen
	if cseq, ok := req.Header["CSeq"]; !ok || len(cseq) != 1 {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
			Header:     base.Header{},
		}, liberrors.ErrServerCSeqMissing
	}

	sxID := getSessionID(req.Header)

	switch req.Method {
	case base.Options:
		if sxID != "" {
			return sc.handleRequestInSession(sxID, req, false)
		}

		return &base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Public": base.HeaderValue{strings.Join(supportedMethods, ", ")},
			},
		}, nil

	case base.Describe:
		path, ok := req.URL.RTSPPath()
		if !ok {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, liberrors.ErrServerInvalidPath
		}

		h := sc.s.handler
		res, stream, err := h.OnDescribe(path)

		if res.StatusCode == base.StatusOK {
			if res.Header == nil {
				res.Header = make(base.Header)
			}

			res.Header["Content-Base"] = base.HeaderValue{req.URL.String() + "/"}
			res.Header["Content-Type"] = base.HeaderValue{"application/sdp"}

			if stream != nil {
				res.Body = stream.Tracks().Marshal()
			}
		}

		return res, err

	case base.Announce:
		return sc.handleRequestInSession(sxID, req, true)

	case base.Setup:
		return sc.handleRequestInSession(sxID, req, true)

	case base.Play:
		if sxID != "" {
			return sc.handleRequestInSession(sxID, req, false)
		}

	case base.Record:
		if sxID != "" {
			return sc.handleRequestInSession(sxID, req, false)
		}

	case base.Teardown:
		if sxID != "" {
			return sc.handleRequestInSession(sxID, req, false)
		}
	}

	return &base.Response{
		StatusCode: base.StatusNotImplemented,
	}, nil
}

func (sc *ServerConn) handleRequestOuter(req *base.Request) error {
	res, err := sc.handleRequest(req)

	if res.Header == nil {
		res.Header = make(base.Header)
	}

	// add cseq
	if !errors.Is(err, liberrors.ErrServerCSeqMissing) {
		res.Header["CSeq"] = req.Header["CSeq"]
	}

	// add server
	res.Header["Server"] = base.HeaderValue{"gortsplib"}

	sc.nconn.SetWriteDeadline(time.Now().Add(sc.s.writeTimeout)) //nolint:errcheck
	sc.conn.WriteResponse(res)                                   //nolint:errcheck

	return err
}

func (sc *ServerConn) handleRequestInSession(
	sxID string,
	req *base.Request,
	create bool,
) (*base.Response, error) {
	// handle directly in Session
	if sc.session != nil {
		// session ID is optional in SETUP and ANNOUNCE requests, since
		// client may not have received the session ID yet due to multiple reasons:
		// * requests can be retries after code 301
		// * SETUP requests comes after ANNOUNCE response, that don't contain the session ID
		if sxID != "" {
			// the connection can't communicate with two sessions at once.
			if sxID != sc.session.secretID {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, liberrors.ErrServerLinkedToOtherSession
			}
		}

		cres := make(chan sessionRequestRes)
		sreq := sessionRequestReq{
			sc:     sc,
			req:    req,
			id:     sxID,
			create: create,
			res:    cres,
		}

		select {
		case sc.session.request <- sreq:
			res := <-cres
			sc.session = res.ss
			return res.res, res.err

		case <-sc.session.ctx.Done():
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, context.Canceled
		}
	}

	// otherwise, pass through Server
	cres := make(chan sessionRequestRes)
	sreq := sessionRequestReq{
		sc:     sc,
		req:    req,
		id:     sxID,
		create: create,
		res:    cres,
	}

	select {
	case sc.s.sessionRequest <- sreq:
		res := <-cres
		sc.session = res.ss

		return res.res, res.err

	case <-sc.s.ctx.Done():
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, context.Canceled
	}
}
