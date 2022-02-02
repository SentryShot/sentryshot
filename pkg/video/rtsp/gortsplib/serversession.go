package gortsplib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"nvr/pkg/video/rtsp/gortsplib/pkg/base"
	"nvr/pkg/video/rtsp/gortsplib/pkg/headers"
	"nvr/pkg/video/rtsp/gortsplib/pkg/liberrors"
	"nvr/pkg/video/rtsp/gortsplib/pkg/multibuffer"
	"nvr/pkg/video/rtsp/gortsplib/pkg/ringbuffer"
	"sort"
	"strconv"
	"strings"
	"time"
)

func stringsReverseIndex(s, substr string) int {
	for i := len(s) - 1 - len(substr); i >= 0; i-- {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// Errors.
var (
	ErrTrackInvalid = errors.New("invalid track path")
	ErrPathInvalid  = errors.New(
		"path of a SETUP request must end with a slash." +
			" This typically happens when VLC fails a request," +
			" and then switches to an unsupported RTSP dialect")
	ErrTrackParseError = errors.New("unable to parse track id")
	ErrTrackPathError  = errors.New("can't setup tracks with different paths")
)

func setupGetTrackIDPathQuery(
	url *base.URL,
	thMode *headers.TransportMode,
	announcedTracks []ServerSessionAnnouncedTrack,
	setuppedPath *string,
	setuppedQuery *string,
	setuppedBaseURL *base.URL,
) (int, string, string, error) {
	pathAndQuery, ok := url.RTSPPathAndQuery()
	if !ok {
		return 0, "", "", liberrors.ErrServerInvalidPath
	}

	if thMode != nil && *thMode != headers.TransportModePlay {
		for trackID, track := range announcedTracks {
			u, _ := track.track.URL(setuppedBaseURL)
			if u.String() == url.String() {
				return trackID, *setuppedPath, *setuppedQuery, nil
			}
		}

		return 0, "", "", fmt.Errorf("%w (%s)", ErrTrackInvalid, pathAndQuery)
	}

	i := stringsReverseIndex(pathAndQuery, "/trackID=")

	// URL doesn't contain trackID - it's track zero
	if i < 0 {
		if !strings.HasSuffix(pathAndQuery, "/") {
			return 0, "", "", ErrPathInvalid
		}
		pathAndQuery = pathAndQuery[:len(pathAndQuery)-1]

		path, query := base.PathSplitQuery(pathAndQuery)

		// we assume it's track 0
		return 0, path, query, nil
	}

	tmp, err := strconv.ParseInt(pathAndQuery[i+len("/trackID="):], 10, 64)
	if err != nil || tmp < 0 {
		return 0, "", "", fmt.Errorf("%w (%v)", ErrTrackParseError, pathAndQuery)
	}
	trackID := int(tmp)
	pathAndQuery = pathAndQuery[:i]

	path, query := base.PathSplitQuery(pathAndQuery)

	if setuppedPath != nil && (path != *setuppedPath || query != *setuppedQuery) {
		return 0, "", "", ErrTrackPathError
	}

	return trackID, path, query, nil
}

// ServerSessionState is a state of a ServerSession.
type ServerSessionState int

// standard states.
const (
	ServerSessionStateInitial ServerSessionState = iota
	ServerSessionStatePreRead
	ServerSessionStateRead
	ServerSessionStatePrePublish
	ServerSessionStatePublish
)

// String implements fmt.Stringer.
func (s ServerSessionState) String() string {
	switch s {
	case ServerSessionStateInitial:
		return "initial"
	case ServerSessionStatePreRead:
		return "prePlay"
	case ServerSessionStateRead:
		return "play"
	case ServerSessionStatePrePublish:
		return "preRecord"
	case ServerSessionStatePublish:
		return "record"
	}
	return "unknown"
}

// ServerSessionSetuppedTrack is a setupped track of a ServerSession.
type ServerSessionSetuppedTrack struct {
	tcpChannel  int
	tcpRTPFrame *base.InterleavedFrame
}

// ServerSessionAnnouncedTrack is an announced track of a ServerSession.
type ServerSessionAnnouncedTrack struct {
	track *Track
}

// ServerSession is a server-side RTSP session.
type ServerSession struct {
	s        *Server
	secretID string // must not be shared, allows to take ownership of the session
	author   *ServerConn

	ctx                context.Context
	ctxCancel          func()
	conns              map[*ServerConn]struct{}
	state              ServerSessionState
	setuppedTracks     map[int]ServerSessionSetuppedTrack
	tcpTracksByChannel map[int]int
	IsTransportSetup   bool
	setuppedBaseURL    *base.URL     // publish
	setuppedStream     *ServerStream // read
	setuppedPath       *string
	setuppedQuery      *string
	lastRequestTime    time.Time
	tcpConn            *ServerConn
	announcedTracks    []ServerSessionAnnouncedTrack // publish
	writerRunning      bool
	writeBuffer        *ringbuffer.RingBuffer

	// writer channels
	writerDone chan struct{}

	// in
	request     chan sessionRequestReq
	connRemove  chan *ServerConn
	startWriter chan struct{}
}

func newServerSession(
	s *Server,
	secretID string,
	author *ServerConn,
) *ServerSession {
	ctx, ctxCancel := context.WithCancel(s.ctx)

	ss := &ServerSession{
		s:               s,
		secretID:        secretID,
		author:          author,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
		conns:           make(map[*ServerConn]struct{}),
		lastRequestTime: time.Now(),
		request:         make(chan sessionRequestReq),
		connRemove:      make(chan *ServerConn),
		startWriter:     make(chan struct{}),
	}

	s.wg.Add(1)
	go ss.run()

	return ss
}

// Close closes the ServerSession.
func (ss *ServerSession) Close() error {
	ss.ctxCancel()
	return nil
}

// State returns the state of the session.
func (ss *ServerSession) State() ServerSessionState {
	return ss.state
}

// SetuppedTracks returns the setupped tracks.
func (ss *ServerSession) SetuppedTracks() map[int]ServerSessionSetuppedTrack {
	return ss.setuppedTracks
}

// AnnouncedTracks returns the announced tracks.
func (ss *ServerSession) AnnouncedTracks() []ServerSessionAnnouncedTrack {
	return ss.announcedTracks
}

func (ss *ServerSession) checkState(allowed map[ServerSessionState]struct{}) error {
	if _, ok := allowed[ss.state]; ok {
		return nil
	}

	allowedList := make([]fmt.Stringer, len(allowed))
	i := 0
	for a := range allowed {
		allowedList[i] = a
		i++
	}
	return liberrors.ServerInvalidStateError{AllowedList: allowedList, State: ss.state}
}

func (ss *ServerSession) run() {
	defer ss.s.wg.Done()

	if h, ok := ss.s.Handler.(ServerHandlerOnSessionOpen); ok {
		h.OnSessionOpen(&ServerHandlerOnSessionOpenCtx{
			Session: ss,
			Conn:    ss.author,
		})
	}

	err := ss.runLoop()
	ss.ctxCancel()

	if ss.state == ServerSessionStateRead {
		ss.setuppedStream.readerSetInactive(ss)
	}

	if ss.setuppedStream != nil {
		ss.setuppedStream.readerRemove(ss)
	}

	if ss.writerRunning {
		ss.writeBuffer.Close()
		<-ss.writerDone
		ss.writerRunning = false
	}

	for sc := range ss.conns {
		if sc == ss.tcpConn {
			sc.Close()

			// make sure that OnFrame() is never called after OnSessionClose()
			<-sc.done
		}

		select {
		case sc.sessionRemove <- ss:
		case <-sc.ctx.Done():
		}
	}

	select {
	case ss.s.sessionClose <- ss:
	case <-ss.s.ctx.Done():
	}

	if h, ok := ss.s.Handler.(ServerHandlerOnSessionClose); ok {
		h.OnSessionClose(&ServerHandlerOnSessionCloseCtx{
			Session: ss,
			Error:   err,
		})
	}
}

func (ss *ServerSession) runLoop() error { //nolint:funlen,gocognit
	for {
		select {
		case req := <-ss.request:
			ss.lastRequestTime = time.Now()

			if _, ok := ss.conns[req.sc]; !ok {
				ss.conns[req.sc] = struct{}{}
			}

			res, err := ss.handleRequest(req.sc, req.req)

			if res.StatusCode == base.StatusOK {
				if res.Header == nil {
					res.Header = make(base.Header)
				}
				res.Header["Session"] = headers.Session{
					Session: ss.secretID,
					Timeout: func() *uint {
						v := uint(ss.s.sessionTimeout / time.Second)
						return &v
					}(),
				}.Write()
			}

			if errors.Is(err, liberrors.ServerSessionTeardownError{}) {
				req.res <- sessionRequestRes{res: res, err: nil}
				return err
			}

			req.res <- sessionRequestRes{
				res: res,
				err: err,
				ss:  ss,
			}

		case sc := <-ss.connRemove:
			if _, ok := ss.conns[sc]; ok {
				delete(ss.conns, sc)

				select {
				case sc.sessionRemove <- ss:
				case <-sc.ctx.Done():
				}
			}

			// if session is not in state RECORD or PLAY, or transport is TCP
			if (ss.state != ServerSessionStatePublish &&
				ss.state != ServerSessionStateRead) ||
				ss.IsTransportSetup {
				// close if there are no associated connections
				if len(ss.conns) == 0 {
					return liberrors.ErrServerSessionNotInUse
				}
			}

		case <-ss.startWriter:
			if !ss.writerRunning && (ss.state == ServerSessionStatePublish ||
				ss.state == ServerSessionStateRead) &&
				ss.IsTransportSetup {
				ss.writerRunning = true
				ss.writerDone = make(chan struct{})
				go ss.runWriter()
			}

		case <-ss.ctx.Done():
			return liberrors.ErrServerTerminated
		}
	}
}

func (ss *ServerSession) handleRequest(sc *ServerConn, req *base.Request) (*base.Response, error) { //nolint:funlen
	if ss.tcpConn != nil && sc != ss.tcpConn {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ErrServerSessionLinkedToOtherConn
	}

	switch req.Method {
	case base.Options:
		return ss.handleOptions(sc)

	case base.Announce:
		return ss.handleAnnounce(sc, req)

	case base.Setup:
		return ss.handleSetup(sc, req)

	case base.Play:
		return ss.handlePlay(sc, req)

	case base.Record:
		return ss.handleRecord(sc, req)

	case base.Pause:
		return ss.handlePause(sc, req)

	case base.Teardown:
		return &base.Response{
			StatusCode: base.StatusOK,
		}, liberrors.ServerSessionTeardownError{Author: sc.NetConn().RemoteAddr()}

	case base.GetParameter:
		if h, ok := sc.s.Handler.(ServerHandlerOnGetParameter); ok {
			pathAndQuery, ok := req.URL.RTSPPathAndQuery()
			if !ok {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, liberrors.ErrServerInvalidPath
			}

			path, query := base.PathSplitQuery(pathAndQuery)

			return h.OnGetParameter(&ServerHandlerOnGetParameterCtx{
				Session: ss,
				Conn:    sc,
				Req:     req,
				Path:    path,
				Query:   query,
			})
		}

		// GET_PARAMETER is used like a ping when reading, and sometimes
		// also when publishing; reply with 200
		return &base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Content-Type": base.HeaderValue{"text/parameters"},
			},
			Body: []byte{},
		}, nil
	}

	return &base.Response{
		StatusCode: base.StatusBadRequest,
	}, liberrors.ServerUnhandledRequestError{Req: req}
}

func (ss *ServerSession) handleOptions(sc *ServerConn) (*base.Response, error) {
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
}

// Errors.
var (
	ErrTrackGenURL      = errors.New("unable to generate track URL")
	ErrTrackInvalidURL  = errors.New("invalid track URL")
	ErrTrackInvalidPath = errors.New("invalid track path")
)

func (ss *ServerSession) handleAnnounce(sc *ServerConn, req *base.Request) (*base.Response, error) { //nolint:funlen
	err := ss.checkState(map[ServerSessionState]struct{}{
		ServerSessionStateInitial: {},
	})
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	pathAndQuery, ok := req.URL.RTSPPathAndQuery()
	if !ok {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ErrServerInvalidPath
	}

	path, query := base.PathSplitQuery(pathAndQuery)

	ct, ok := req.Header["Content-Type"]
	if !ok || len(ct) != 1 {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ErrServerContentTypeMissing
	}

	if ct[0] != "application/sdp" {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ErrServerContentTypeUnsupportedError{CT: ct}
	}

	tracks, err := ReadTracks(req.Body)
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ServerSDPinvalidError{Err: err}
	}

	if len(tracks) == 0 {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ErrServerSDPnoTracksDefined
	}

	for _, track := range tracks {
		trackURL, err := track.URL(req.URL)
		if err != nil {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, ErrTrackGenURL
		}

		trackPath, ok := trackURL.RTSPPathAndQuery()
		if !ok {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("%w (%v)", ErrTrackInvalidURL, trackURL)
		}

		if !strings.HasPrefix(trackPath, path) {
			return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("%w: must begin with '%s', but is '%s'",
					ErrTrackInvalidPath, path, trackPath)
		}
	}

	res, err := ss.s.Handler.(ServerHandlerOnAnnounce).OnAnnounce(&ServerHandlerOnAnnounceCtx{
		Server:  ss.s,
		Session: ss,
		Conn:    sc,
		Req:     req,
		Path:    path,
		Query:   query,
		Tracks:  tracks,
	})

	if res.StatusCode != base.StatusOK {
		return res, err
	}

	ss.state = ServerSessionStatePrePublish
	ss.setuppedPath = &path
	ss.setuppedQuery = &query
	ss.setuppedBaseURL = req.URL

	ss.announcedTracks = make([]ServerSessionAnnouncedTrack, len(tracks))
	for trackID, track := range tracks {
		ss.announcedTracks[trackID] = ServerSessionAnnouncedTrack{
			track: track,
		}
	}

	return res, err
}

func (ss *ServerSession) handleSetup(sc *ServerConn, //nolint:funlen,gocognit
	req *base.Request) (*base.Response, error) {
	err := ss.checkState(map[ServerSessionState]struct{}{
		ServerSessionStateInitial:    {},
		ServerSessionStatePreRead:    {},
		ServerSessionStatePrePublish: {},
	})
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	var inTH headers.Transport
	err = inTH.Read(req.Header["Transport"])
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ServerTransportHeaderInvalidError{Err: err}
	}

	if inTH.Protocol != headers.TransportProtocolTCP {
		return &base.Response{
			StatusCode: base.StatusUnsupportedTransport,
		}, nil
	}

	trackID, path, query, err := setupGetTrackIDPathQuery(req.URL, inTH.Mode,
		ss.announcedTracks, ss.setuppedPath, ss.setuppedQuery, ss.setuppedBaseURL)
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	if _, ok := ss.setuppedTracks[trackID]; ok {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ServerTrackAlreadySetupError{TrackID: trackID}
	}

	if inTH.InterleavedIDs == nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ErrServerTransportHeaderNoInterleavedIDs
	}

	if (inTH.InterleavedIDs[0]%2) != 0 ||
		(inTH.InterleavedIDs[0]+1) != inTH.InterleavedIDs[1] {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ErrServerTransportHeaderInvalidInterleavedIDs
	}

	if _, ok := ss.tcpTracksByChannel[inTH.InterleavedIDs[0]]; ok {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ErrServerTransportHeaderInterleavedIDsAlreadyUsed
	}

	switch ss.state {
	case ServerSessionStateInitial, ServerSessionStatePreRead: // play
		if inTH.Mode != nil && *inTH.Mode != headers.TransportModePlay {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, liberrors.ServerTransportHeaderInvalidModeError{Mode: inTH.Mode}
		}

	default: // record

		if inTH.Mode == nil || *inTH.Mode != headers.TransportModeRecord {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, liberrors.ServerTransportHeaderInvalidModeError{Mode: inTH.Mode}
		}
	}

	res, stream, err := ss.s.Handler.(ServerHandlerOnSetup).OnSetup(&ServerHandlerOnSetupCtx{
		Server:  ss.s,
		Session: ss,
		Conn:    sc,
		Req:     req,
		Path:    path,
		Query:   query,
		TrackID: trackID,
	})

	// workaround to prevent a bug in rtspclientsink
	// that makes impossible for the client to receive the response
	// and send frames.
	// this was causing problems during unit tests.
	if ua, ok := req.Header["User-Agent"]; ok && len(ua) == 1 &&
		strings.HasPrefix(ua[0], "GStreamer") {
		select {
		case <-time.After(1 * time.Second):
		case <-ss.ctx.Done():
		}
	}

	if res.StatusCode != base.StatusOK {
		return res, err
	}

	if ss.state == ServerSessionStateInitial {
		stream.readerAdd(ss)

		ss.state = ServerSessionStatePreRead
		ss.setuppedPath = &path
		ss.setuppedQuery = &query
		ss.setuppedStream = stream
	}

	th := headers.Transport{}

	if ss.state == ServerSessionStatePreRead {
		ssrc := stream.ssrc(trackID)
		if ssrc != 0 {
			th.SSRC = &ssrc
		}
	}

	ss.IsTransportSetup = true

	if res.Header == nil {
		res.Header = make(base.Header)
	}

	sst := ServerSessionSetuppedTrack{}

	sst.tcpChannel = inTH.InterleavedIDs[0]

	sst.tcpRTPFrame = &base.InterleavedFrame{
		Channel: sst.tcpChannel,
	}

	if ss.tcpTracksByChannel == nil {
		ss.tcpTracksByChannel = make(map[int]int)
	}

	ss.tcpTracksByChannel[inTH.InterleavedIDs[0]] = trackID

	th.Protocol = headers.TransportProtocolTCP
	th.InterleavedIDs = inTH.InterleavedIDs

	if ss.setuppedTracks == nil {
		ss.setuppedTracks = make(map[int]ServerSessionSetuppedTrack)
	}

	ss.setuppedTracks[trackID] = sst

	res.Header["Transport"] = th.Write()

	return res, err
}

func (ss *ServerSession) handlePlay(sc *ServerConn, req *base.Request) (*base.Response, error) { //nolint:funlen
	// play can be sent twice, allow calling it even if we're already playing
	err := ss.checkState(map[ServerSessionState]struct{}{
		ServerSessionStatePreRead: {},
		ServerSessionStateRead:    {},
	})
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	pathAndQuery, ok := req.URL.RTSPPathAndQuery()
	if !ok {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ErrServerInvalidPath
	}

	// path can end with a slash due to Content-Base, remove it
	pathAndQuery = strings.TrimSuffix(pathAndQuery, "/")

	path, query := base.PathSplitQuery(pathAndQuery)

	if ss.State() == ServerSessionStatePreRead &&
		path != *ss.setuppedPath {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ServerPathHasChangedError{Prev: *ss.setuppedPath, Cur: path}
	}

	res, err := sc.s.Handler.(ServerHandlerOnPlay).OnPlay(&ServerHandlerOnPlayCtx{
		Session: ss,
		Conn:    sc,
		Req:     req,
		Path:    path,
		Query:   query,
	})

	if res.StatusCode != base.StatusOK {
		if ss.State() == ServerSessionStatePreRead {
			ss.writeBuffer = nil
		}
		return res, err
	}

	if ss.state == ServerSessionStateRead {
		return res, err
	}

	ss.state = ServerSessionStateRead

	ss.tcpConn = sc
	ss.tcpConn.tcpSession = ss
	ss.tcpConn.tcpFrameEnabled = true
	ss.tcpConn.tcpFrameTimeout = false
	// decrease RAM consumption by allocating less buffers.
	ss.tcpConn.tcpReadBuffer = multibuffer.New(8, uint64(sc.s.ReadBufferSize))

	ss.writeBuffer = ringbuffer.New(uint64(ss.s.ReadBufferCount))
	// run writer after sending the response
	ss.tcpConn.tcpWriterRunning = false

	// add RTP-Info
	var trackIDs []int
	for trackID := range ss.setuppedTracks {
		trackIDs = append(trackIDs, trackID)
	}
	sort.Slice(trackIDs, func(a, b int) bool {
		return trackIDs[a] < trackIDs[b]
	})
	var ri headers.RTPinfo
	for _, trackID := range trackIDs {
		ts := ss.setuppedStream.timestamp(trackID)
		if ts == 0 {
			continue
		}

		u := &base.URL{
			Scheme: req.URL.Scheme,
			User:   req.URL.User,
			Host:   req.URL.Host,
			Path:   "/" + *ss.setuppedPath + "/trackID=" + strconv.FormatInt(int64(trackID), 10),
		}

		lsn := ss.setuppedStream.lastSequenceNumber(trackID)

		ri = append(ri, &headers.RTPInfoEntry{
			URL:            u.String(),
			SequenceNumber: &lsn,
			Timestamp:      &ts,
		})
	}
	if len(ri) > 0 {
		if res.Header == nil {
			res.Header = make(base.Header)
		}
		res.Header["RTP-Info"] = ri.Write()
	}

	ss.setuppedStream.readerSetActive(ss)

	return res, err
}

func (ss *ServerSession) handleRecord(sc *ServerConn, req *base.Request) (*base.Response, error) {
	err := ss.checkState(map[ServerSessionState]struct{}{
		ServerSessionStatePrePublish: {},
	})
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	if len(ss.setuppedTracks) != len(ss.announcedTracks) {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ErrServerNotAllAnnouncedTracksSetup
	}

	pathAndQuery, ok := req.URL.RTSPPathAndQuery()
	if !ok {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ErrServerInvalidPath
	}

	// path can end with a slash due to Content-Base, remove it
	pathAndQuery = strings.TrimSuffix(pathAndQuery, "/")

	path, query := base.PathSplitQuery(pathAndQuery)

	if path != *ss.setuppedPath {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ServerPathHasChangedError{Prev: *ss.setuppedPath, Cur: path}
	}

	res, err := ss.s.Handler.(ServerHandlerOnRecord).OnRecord(&ServerHandlerOnRecordCtx{
		Session: ss,
		Conn:    sc,
		Req:     req,
		Path:    path,
		Query:   query,
	})

	if res.StatusCode != base.StatusOK {
		return res, err
	}

	ss.state = ServerSessionStatePublish

	ss.tcpConn = sc
	ss.tcpConn.tcpSession = ss
	ss.tcpConn.tcpFrameEnabled = true
	ss.tcpConn.tcpFrameTimeout = true
	ss.tcpConn.tcpReadBuffer = multibuffer.New(uint64(sc.s.ReadBufferCount), uint64(sc.s.ReadBufferSize))
	ss.tcpConn.tcpProcessFunc = sc.tcpProcessRecord

	// decrease RAM consumption by allocating less buffers.
	ss.writeBuffer = ringbuffer.New(uint64(8))
	// run writer after sending the response
	ss.tcpConn.tcpWriterRunning = false

	return res, err
}

func (ss *ServerSession) handlePause(sc *ServerConn, req *base.Request) (*base.Response, error) { //nolint:funlen
	err := ss.checkState(map[ServerSessionState]struct{}{
		ServerSessionStatePreRead:    {},
		ServerSessionStateRead:       {},
		ServerSessionStatePrePublish: {},
		ServerSessionStatePublish:    {},
	})
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	pathAndQuery, ok := req.URL.RTSPPathAndQuery()
	if !ok {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ErrServerInvalidPath
	}

	// path can end with a slash due to Content-Base, remove it
	pathAndQuery = strings.TrimSuffix(pathAndQuery, "/")

	path, query := base.PathSplitQuery(pathAndQuery)

	res, err := ss.s.Handler.(ServerHandlerOnPause).OnPause(&ServerHandlerOnPauseCtx{
		Session: ss,
		Conn:    sc,
		Req:     req,
		Path:    path,
		Query:   query,
	})

	if res.StatusCode != base.StatusOK {
		return res, err
	}

	if ss.writerRunning {
		ss.writeBuffer.Close()
		<-ss.writerDone
		ss.writerRunning = false
	}

	switch ss.state {
	case ServerSessionStateRead:
		ss.setuppedStream.readerSetInactive(ss)

		ss.state = ServerSessionStatePreRead

		ss.tcpConn.tcpSession = nil
		ss.tcpConn.tcpFrameEnabled = false
		ss.tcpConn.tcpReadBuffer = nil
		ss.tcpConn = nil

	case ServerSessionStatePublish:
		ss.state = ServerSessionStatePrePublish

		ss.tcpConn.tcpSession = nil
		ss.tcpConn.tcpFrameEnabled = false
		ss.tcpConn.tcpReadBuffer = nil
		err := ss.tcpConn.conn.SetReadDeadline(time.Time{})
		if err != nil {
			return nil, err
		}
		ss.tcpConn = nil
	}

	return res, err
}

func (ss *ServerSession) runWriter() {
	defer close(ss.writerDone)

	var writeFunc func(int, []byte)

	var buf bytes.Buffer

	writeFunc = func(trackID int, payload []byte) {
		f := ss.setuppedTracks[trackID].tcpRTPFrame
		f.Payload = payload
		f.Write(&buf)

		ss.tcpConn.conn.SetWriteDeadline(time.Now().Add(ss.s.WriteTimeout)) //nolint:errcheck
		ss.tcpConn.conn.Write(buf.Bytes())                                  //nolint:errcheck
	}

	for {
		tmp, ok := ss.writeBuffer.Pull()
		if !ok {
			return
		}
		data := tmp.(trackTypePayload) //nolint:forcetypeassert

		writeFunc(data.trackID, data.payload)
	}
}

// WritePacketRTP writes a RTP packet to the session.
func (ss *ServerSession) WritePacketRTP(trackID int, payload []byte) {
	if _, ok := ss.setuppedTracks[trackID]; !ok {
		return
	}

	ss.writeBuffer.Push(trackTypePayload{
		trackID: trackID,
		payload: payload,
	})
}
