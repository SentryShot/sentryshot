package gortsplib

import (
	"bufio"
	"context"
	"errors"
	"net"
	"nvr/pkg/video/gortsplib/pkg/base"
	"nvr/pkg/video/gortsplib/pkg/liberrors"
	"nvr/pkg/video/gortsplib/pkg/url"
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
	s    *Server
	conn net.Conn

	ctx        context.Context
	ctxCancel  func()
	remoteAddr *net.TCPAddr
	br         *bufio.Reader
	session    *ServerSession
	readFunc   func(readRequest chan readReq) error

	// in
	sessionRemove chan *ServerSession

	// out
	done chan struct{}
}

func newServerConn(
	s *Server,
	conn net.Conn,
) *ServerConn {
	ctx, ctxCancel := context.WithCancel(s.ctx)
	sc := &ServerConn{ //nolint:forcetypeassert
		s:             s,
		conn:          conn,
		ctx:           ctx,
		ctxCancel:     ctxCancel,
		remoteAddr:    conn.RemoteAddr().(*net.TCPAddr),
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
	return sc.conn
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

	if h, ok := sc.s.Handler.(ServerHandlerOnConnOpen); ok {
		h.OnConnOpen(&ServerHandlerOnConnOpenCtx{
			Conn: sc,
		})
	}

	sc.br = bufio.NewReaderSize(sc.conn, tcpReadBufferSize)

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
				return err

			case ss := <-sc.sessionRemove:
				if sc.session == ss {
					sc.session = nil
				}

			case <-sc.ctx.Done():
				return liberrors.ErrServerTerminated
			}
		}
	}()

	sc.ctxCancel()

	sc.conn.Close()
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

	if h, ok := sc.s.Handler.(ServerHandlerOnConnClose); ok {
		h.OnConnClose(&ServerHandlerOnConnCloseCtx{
			Conn:  sc,
			Error: err,
		})
	}
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
	sc.conn.SetReadDeadline(time.Time{}) //nolint:errcheck

	var req base.Request

	for {
		err := req.Read(sc.br)
		if err != nil {
			return err
		}

		cres := make(chan error)
		select {
		case readRequest <- readReq{req: &req, res: cres}:
			err = <-cres
			if err != nil {
				return err
			}

		case <-sc.ctx.Done():
			return liberrors.ErrServerTerminated
		}
	}
}

func (sc *ServerConn) readFuncTCP(readRequest chan readReq) error { //nolint:funlen,gocognit
	// reset deadline
	sc.conn.SetReadDeadline(time.Time{}) // nolint:errcheck

	select {
	case sc.session.startWriter <- struct{}{}:
	case <-sc.session.ctx.Done():
	}

	var processFunc func(int, []byte) error

	if sc.session.state == ServerSessionStatePlay {
		processFunc = func(trackID int, payload []byte) error {
			return nil
		}
	} else {
		tcpRTPPacketBuffer := newRTPPacketMultiBuffer(uint64(sc.s.ReadBufferCount))

		processFunc = func(trackID int, payload []byte) error {
			pkt := tcpRTPPacketBuffer.next()
			err := pkt.Unmarshal(payload)
			if err != nil {
				return err
			}

			out, err := sc.session.announcedTracks[trackID].cleaner.Process(pkt)
			if err != nil {
				return err
			}
			if h, ok := sc.s.Handler.(ServerHandlerOnPacketRTP); ok {
				for _, entry := range out {
					h.OnPacketRTP(&ServerHandlerOnPacketRTPCtx{
						Session:      sc.session,
						TrackID:      trackID,
						Packet:       entry.Packet,
						PTSEqualsDTS: entry.PTSEqualsDTS,
						H264NALUs:    entry.H264NALUs,
						H264PTS:      entry.H264PTS,
					})
				}
			}
			return nil
		}
	}

	var req base.Request
	var frame base.InterleavedFrame

	for {
		if sc.session.state == ServerSessionStateRecord {
			sc.conn.SetReadDeadline(time.Now().Add(sc.s.ReadTimeout)) //nolint:errcheck
		}

		what, err := base.ReadInterleavedFrameOrRequest(&frame, tcpMaxFramePayloadSize, &req, sc.br)
		if err != nil {
			return err
		}

		switch what.(type) {
		case *base.InterleavedFrame:
			channel := frame.Channel

			// forward frame only if it has been set up
			if trackID, ok := sc.session.tcpTracksByChannel[channel]; ok {
				err := processFunc(trackID, frame.Payload)
				if err != nil {
					return err
				}
			}

		case *base.Request:
			cres := make(chan error)
			select {
			case readRequest <- readReq{req: &req, res: cres}:
				err := <-cres
				if err != nil {
					return err
				}

			case <-sc.ctx.Done():
				return liberrors.ErrServerTerminated
			}
		}
	}
}

func (sc *ServerConn) handleRequest(req *base.Request) (*base.Response, error) { //nolint:funlen,gocognit,gocyclo
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

		// handle request here
		var methods []string
		if _, ok := sc.s.Handler.(ServerHandlerOnDescribe); ok {
			methods = append(methods, string(base.Describe))
		}
		if _, ok := sc.s.Handler.(ServerHandlerOnAnnounce); ok {
			methods = append(methods, string(base.Announce))
		}
		if _, ok := sc.s.Handler.(ServerHandlerOnSetup); ok {
			methods = append(methods, string(base.Setup))
		}
		if _, ok := sc.s.Handler.(ServerHandlerOnPlay); ok {
			methods = append(methods, string(base.Play))
		}
		if _, ok := sc.s.Handler.(ServerHandlerOnRecord); ok {
			methods = append(methods, string(base.Record))
		}
		if _, ok := sc.s.Handler.(ServerHandlerOnPause); ok {
			methods = append(methods, string(base.Pause))
		}
		methods = append(methods, string(base.GetParameter))
		if _, ok := sc.s.Handler.(ServerHandlerOnSetParameter); ok {
			methods = append(methods, string(base.SetParameter))
		}
		methods = append(methods, string(base.Teardown))

		return &base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Public": base.HeaderValue{strings.Join(methods, ", ")},
			},
		}, nil

	case base.Describe:
		if h, ok := sc.s.Handler.(ServerHandlerOnDescribe); ok { //nolint:nestif
			pathAndQuery, ok := req.URL.RTSPPathAndQuery()
			if !ok {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, liberrors.ErrServerInvalidPath
			}

			path, query := url.PathSplitQuery(pathAndQuery)

			res, stream, err := h.OnDescribe(&ServerHandlerOnDescribeCtx{
				Conn:    sc,
				Request: req,
				Path:    path,
				Query:   query,
			})

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
		}

	case base.Announce:
		if _, ok := sc.s.Handler.(ServerHandlerOnAnnounce); ok {
			return sc.handleRequestInSession(sxID, req, true)
		}

	case base.Setup:
		if _, ok := sc.s.Handler.(ServerHandlerOnSetup); ok {
			return sc.handleRequestInSession(sxID, req, true)
		}

	case base.Play:
		if sxID != "" {
			if _, ok := sc.s.Handler.(ServerHandlerOnPlay); ok {
				return sc.handleRequestInSession(sxID, req, false)
			}
		}

	case base.Record:
		if sxID != "" {
			if _, ok := sc.s.Handler.(ServerHandlerOnRecord); ok {
				return sc.handleRequestInSession(sxID, req, false)
			}
		}

	case base.Pause:
		if sxID != "" {
			if _, ok := sc.s.Handler.(ServerHandlerOnPause); ok {
				return sc.handleRequestInSession(sxID, req, false)
			}
		}

	case base.Teardown:
		if sxID != "" {
			return sc.handleRequestInSession(sxID, req, false)
		}

	case base.GetParameter:
		if sxID != "" {
			return sc.handleRequestInSession(sxID, req, false)
		}

		// handle request here
		if h, ok := sc.s.Handler.(ServerHandlerOnGetParameter); ok {
			pathAndQuery, ok := req.URL.RTSPPathAndQuery()
			if !ok {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, liberrors.ErrServerInvalidPath
			}

			path, query := url.PathSplitQuery(pathAndQuery)

			return h.OnGetParameter(&ServerHandlerOnGetParameterCtx{
				Conn:    sc,
				Request: req,
				Path:    path,
				Query:   query,
			})
		}

	case base.SetParameter:
		if h, ok := sc.s.Handler.(ServerHandlerOnSetParameter); ok {
			pathAndQuery, ok := req.URL.RTSPPathAndQuery()
			if !ok {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, liberrors.ErrServerInvalidPath
			}

			path, query := url.PathSplitQuery(pathAndQuery)

			return h.OnSetParameter(&ServerHandlerOnSetParameterCtx{
				Conn:    sc,
				Request: req,
				Path:    path,
				Query:   query,
			})
		}
	}

	return &base.Response{
		StatusCode: base.StatusBadRequest,
	}, liberrors.ServerUnhandledRequestError{Request: req}
}

func (sc *ServerConn) handleRequestOuter(req *base.Request) error {
	if h, ok := sc.s.Handler.(ServerHandlerOnRequest); ok {
		h.OnRequest(sc, req)
	}

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

	if h, ok := sc.s.Handler.(ServerHandlerOnResponse); ok {
		h.OnResponse(sc, res)
	}

	byts, _ := res.Marshal()

	sc.conn.SetWriteDeadline(time.Now().Add(sc.s.WriteTimeout)) //nolint:errcheck
	sc.conn.Write(byts)                                         //nolint:errcheck

	return err
}

func (sc *ServerConn) handleRequestInSession( //nolint:funlen
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
			}, liberrors.ErrServerTerminated
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
		}, liberrors.ErrServerTerminated
	}
}
