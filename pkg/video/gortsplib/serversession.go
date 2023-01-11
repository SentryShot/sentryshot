package gortsplib

import (
	"context"
	"errors"
	"fmt"
	"nvr/pkg/video/gortsplib/pkg/base"
	"nvr/pkg/video/gortsplib/pkg/headers"
	"nvr/pkg/video/gortsplib/pkg/liberrors"
	"nvr/pkg/video/gortsplib/pkg/ringbuffer"
	"nvr/pkg/video/gortsplib/pkg/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pion/rtp"
)

type rtpPacketMultiBuffer struct {
	count   uint64
	buffers []rtp.Packet
	cur     uint64
}

func newRTPPacketMultiBuffer(count uint64) *rtpPacketMultiBuffer {
	buffers := make([]rtp.Packet, count)
	return &rtpPacketMultiBuffer{
		count:   count,
		buffers: buffers,
	}
}

func (mb *rtpPacketMultiBuffer) next() *rtp.Packet {
	ret := &mb.buffers[mb.cur%mb.count]
	mb.cur++
	return ret
}

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

func setupGetTrackIDPath(
	u *url.URL,
	thMode *headers.TransportMode,
	announcedTracks []*ServerSessionAnnouncedTrack,
	setuppedPath *string,
	setuppedBaseURL *url.URL,
) (int, string, error) {
	path, ok := u.RTSPPath()
	if !ok {
		return 0, "", liberrors.ErrServerInvalidPath
	}

	if thMode != nil && *thMode != headers.TransportModePlay {
		for trackID, track := range announcedTracks {
			u2, _ := track.track.url(setuppedBaseURL)
			if u2.String() == u.String() {
				return trackID, *setuppedPath, nil
			}
		}

		return 0, "", fmt.Errorf("%w (%s)", ErrTrackInvalid, path)
	}

	i := stringsReverseIndex(path, "/trackID=")

	// URL doesn't contain trackID - it's track zero
	if i < 0 {
		if !strings.HasSuffix(path, "/") {
			return 0, "", ErrPathInvalid
		}
		path = path[:len(path)-1]

		// we assume it's track 0
		return 0, path, nil
	}

	tmp, err := strconv.ParseInt(path[i+len("/trackID="):], 10, 64)
	if err != nil || tmp < 0 {
		return 0, "", fmt.Errorf("%w (%v)", ErrTrackParseError, path)
	}
	trackID := int(tmp)
	path = path[:i]

	if setuppedPath != nil && (path != *setuppedPath) {
		return 0, "", ErrTrackPathError
	}

	return trackID, path, nil
}

// ServerSessionState is a state of a ServerSession.
type ServerSessionState int

// States.
const (
	ServerSessionStateInitial ServerSessionState = iota
	ServerSessionStatePrePlay
	ServerSessionStatePlay
	ServerSessionStatePreRecord
	ServerSessionStateRecord
)

// String implements fmt.Stringer.
func (s ServerSessionState) String() string {
	switch s {
	case ServerSessionStateInitial:
		return "initial"
	case ServerSessionStatePrePlay:
		return "prePlay"
	case ServerSessionStatePlay:
		return "play"
	case ServerSessionStatePreRecord:
		return "preRecord"
	case ServerSessionStateRecord:
		return "record"
	}
	return "unknown"
}

// ServerSessionSetuppedTrack is a setupped track of a ServerSession.
type ServerSessionSetuppedTrack struct {
	id         int
	tcpChannel int
}

// ServerSessionAnnouncedTrack is an announced track of a ServerSession.
type ServerSessionAnnouncedTrack struct {
	track Track
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
	setuppedTracks     map[int]*ServerSessionSetuppedTrack
	tcpTracksByChannel map[int]*ServerSessionSetuppedTrack
	IsTransportSetup   bool
	setuppedBaseURL    *url.URL      // publish
	setuppedStream     *ServerStream // read
	setuppedPath       *string
	lastRequestTime    time.Time
	tcpConn            *ServerConn
	announcedTracks    []*ServerSessionAnnouncedTrack // publish
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
	name string,
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
	go ss.run(name)

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
func (ss *ServerSession) SetuppedTracks() map[int]*ServerSessionSetuppedTrack {
	return ss.setuppedTracks
}

// AnnouncedTracks returns the announced tracks.
func (ss *ServerSession) AnnouncedTracks() []*ServerSessionAnnouncedTrack {
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

func (ss *ServerSession) run(name string) {
	defer ss.s.wg.Done()

	ss.s.handler.OnSessionOpen(ss, ss.author, name)

	err := ss.runInner()
	ss.ctxCancel()

	if ss.state == ServerSessionStatePlay {
		ss.setuppedStream.readerSetInactive(ss)
	}

	if ss.setuppedStream != nil {
		ss.setuppedStream.readerRemove(ss)
	}

	if ss.writerRunning {
		ss.writeBuffer.Close()
		<-ss.writerDone
	}

	// close all associated connections except for the ones that called TEARDOWN
	// (that are detached from the session just after the request)
	for sc := range ss.conns {
		sc.Close()

		// make sure that OnFrame() is never called after OnSessionClose()

		<-sc.done

		select {
		case sc.sessionRemove <- ss:
		case <-sc.ctx.Done():
		}
	}

	select {
	case ss.s.sessionClose <- ss:
	case <-ss.s.ctx.Done():
	}

	ss.s.handler.OnSessionClose(ss, err)
}

func (ss *ServerSession) runInner() error { //nolint:gocognit
	for {
		select {
		case req := <-ss.request:
			ss.lastRequestTime = time.Now()

			if _, ok := ss.conns[req.sc]; !ok {
				ss.conns[req.sc] = struct{}{}
			}

			res, err := ss.handleRequest(req.sc, req.req)

			returnedSession := ss

			if err == nil || errors.Is(err, errSwitchReadFunc) {
				// ANNOUNCE responses don't contain the session header.
				if req.req.Method != base.Announce &&
					req.req.Method != base.Teardown {
					if res.Header == nil {
						res.Header = make(base.Header)
					}

					res.Header["Session"] = headers.Session{
						Session: ss.secretID,
					}.Marshal()
				}

				// After a TEARDOWN, session must be unpaired with the connection.
				if req.req.Method == base.Teardown {
					delete(ss.conns, req.sc)
					returnedSession = nil
				}
			}

			savedMethod := req.req.Method

			req.res <- sessionRequestRes{
				res: res,
				err: err,
				ss:  returnedSession,
			}

			if (err == nil || errors.Is(err, errSwitchReadFunc)) && savedMethod == base.Teardown {
				return liberrors.ServerSessionTeardownError{Author: req.sc.NetConn().RemoteAddr()}
			}

		case sc := <-ss.connRemove:
			delete(ss.conns, sc)

			if len(ss.conns) == 0 {
				return context.Canceled
			}

		case <-ss.startWriter:
			if !ss.writerRunning && (ss.state == ServerSessionStateRecord ||
				ss.state == ServerSessionStatePlay) &&
				ss.IsTransportSetup {
				ss.writerRunning = true
				ss.writerDone = make(chan struct{})
				go ss.runWriter()
			}

		case <-ss.ctx.Done():
			return context.Canceled
		}
	}
}

func (ss *ServerSession) handleRequest( //nolint:funlen
	sc *ServerConn,
	req *base.Request,
) (*base.Response, error) {
	if ss.tcpConn != nil && sc != ss.tcpConn {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ErrServerSessionLinkedToOtherConn
	}

	var path string
	switch req.Method {
	case base.Announce, base.Play, base.Record, base.GetParameter, base.SetParameter:
		var ok bool
		path, ok = req.URL.RTSPPath()
		if !ok {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, liberrors.ErrServerInvalidPath
		}

		if req.Method != base.Announce {
			// path can end with a slash due to Content-Base, remove it
			path = strings.TrimSuffix(path, "/")
		}
	}

	switch req.Method {
	case base.Options:
		return ss.handleOptions()

	case base.Announce:
		return ss.handleAnnounce(req, path)

	case base.Setup:
		return ss.handleSetup(req)

	case base.Play:
		return ss.handlePlay(sc, req, path)

	case base.Record:
		return ss.handleRecord(sc, path)

	case base.Teardown:
		var err error
		if ss.state == ServerSessionStatePlay || ss.state == ServerSessionStateRecord {
			ss.tcpConn.readFunc = ss.tcpConn.readFuncStandard
			err = errSwitchReadFunc
		}

		return &base.Response{
			StatusCode: base.StatusOK,
		}, err

	case base.GetParameter:
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
		StatusCode: base.StatusNotImplemented,
	}, nil
}

func (ss *ServerSession) handleOptions() (*base.Response, error) {
	return &base.Response{
		StatusCode: base.StatusOK,
		Header: base.Header{
			"Public": base.HeaderValue{strings.Join(supportedMethods, ", ")},
		},
	}, nil
}

// Errors.
var (
	ErrTrackGenURL      = errors.New("unable to generate track URL")
	ErrTrackInvalidURL  = errors.New("invalid track URL")
	ErrTrackInvalidPath = errors.New("invalid track path")
)

func (ss *ServerSession) handleAnnounce( //nolint:funlen
	req *base.Request,
	path string,
) (*base.Response, error) {
	err := ss.checkState(map[ServerSessionState]struct{}{
		ServerSessionStateInitial: {},
	})
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

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

	var tracks Tracks
	_, err = tracks.Unmarshal(req.Body)
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ServerSDPinvalidError{Err: err}
	}

	for _, track := range tracks {
		trackURL, err := track.url(req.URL)
		if err != nil {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, ErrTrackGenURL
		}

		trackPath, ok := trackURL.RTSPPath()
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

	res, err := ss.s.handler.OnAnnounce(ss, path, tracks)

	if res.StatusCode != base.StatusOK {
		return res, err
	}

	ss.state = ServerSessionStatePreRecord
	ss.setuppedPath = &path
	ss.setuppedBaseURL = req.URL

	ss.announcedTracks = make([]*ServerSessionAnnouncedTrack, len(tracks))
	for trackID, track := range tracks {
		ss.announcedTracks[trackID] = &ServerSessionAnnouncedTrack{
			track: track,
		}
	}

	return res, err
}

func (ss *ServerSession) handleSetup(req *base.Request) (*base.Response, error) { //nolint:funlen,gocognit
	err := ss.checkState(map[ServerSessionState]struct{}{
		ServerSessionStateInitial:   {},
		ServerSessionStatePrePlay:   {},
		ServerSessionStatePreRecord: {},
	})
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	var inTH headers.Transport
	err = inTH.Unmarshal(req.Header["Transport"])
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ServerTransportHeaderInvalidError{Err: err}
	}

	trackID, path, err := setupGetTrackIDPath(
		req.URL,
		inTH.Mode,
		ss.announcedTracks,
		ss.setuppedPath,
		ss.setuppedBaseURL,
	)
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
	case ServerSessionStateInitial, ServerSessionStatePrePlay: // play
		if inTH.Mode != nil && *inTH.Mode != headers.TransportModePlay {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, liberrors.ServerTransportHeaderInvalidModeError{Mode: *inTH.Mode}
		}

	default: // record
		if inTH.Mode == nil || *inTH.Mode != headers.TransportModeRecord {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, liberrors.ServerTransportHeaderInvalidModeError{Mode: *inTH.Mode}
		}
	}

	res, stream, err := ss.s.handler.OnSetup(ss, path, trackID)

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
		if err := stream.readerAdd(ss); err != nil {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, err
		}

		ss.state = ServerSessionStatePrePlay
		ss.setuppedPath = &path
		ss.setuppedStream = stream
	}

	th := headers.Transport{}

	if ss.state == ServerSessionStatePrePlay {
		ssrc := stream.ssrc(trackID)
		if ssrc != 0 {
			th.SSRC = &ssrc
		}
	}

	ss.IsTransportSetup = true

	if res.Header == nil {
		res.Header = make(base.Header)
	}

	sst := &ServerSessionSetuppedTrack{id: trackID}

	if ss.tcpTracksByChannel == nil {
		ss.tcpTracksByChannel = make(map[int]*ServerSessionSetuppedTrack)
	}

	ss.tcpTracksByChannel[inTH.InterleavedIDs[0]] = sst

	th.InterleavedIDs = inTH.InterleavedIDs

	if ss.setuppedTracks == nil {
		ss.setuppedTracks = make(map[int]*ServerSessionSetuppedTrack)
	}
	ss.setuppedTracks[trackID] = sst

	res.Header["Transport"] = th.Marshal()

	return res, err
}

func (ss *ServerSession) handlePlay( //nolint:funlen
	sc *ServerConn,
	req *base.Request,
	path string,
) (*base.Response, error) {
	// play can be sent twice, allow calling it even if we're already playing
	err := ss.checkState(map[ServerSessionState]struct{}{
		ServerSessionStatePrePlay: {},
		ServerSessionStatePlay:    {},
	})
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	if ss.State() == ServerSessionStatePrePlay &&
		path != *ss.setuppedPath {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ServerPathHasChangedError{Prev: *ss.setuppedPath, Cur: path}
	}

	// allocate writeBuffer before calling OnPlay().
	// in this way it's possible to call ServerSession.WritePacket*()
	// inside the callback.
	if ss.state != ServerSessionStatePlay {
		ss.writeBuffer, _ = ringbuffer.New(uint64(ss.s.writeBufferCount))
	}

	res, err := sc.s.handler.OnPlay(ss)

	if res.StatusCode != base.StatusOK {
		if ss.State() == ServerSessionStatePrePlay {
			ss.writeBuffer = nil
		}
		return res, err
	}

	if ss.state == ServerSessionStatePlay {
		return res, err
	}

	ss.state = ServerSessionStatePlay

	ss.tcpConn = sc
	ss.tcpConn.readFunc = ss.tcpConn.readFuncTCP
	err = errSwitchReadFunc

	ss.writeBuffer, _ = ringbuffer.New(uint64(ss.s.readBufferCount))
	// runWriter() is called by ServerConn after the response has been sent

	ss.setuppedStream.readerSetActive(ss)

	var trackIDs []int
	for trackID := range ss.setuppedTracks {
		trackIDs = append(trackIDs, trackID)
	}

	sort.Slice(trackIDs, func(a, b int) bool {
		return trackIDs[a] < trackIDs[b]
	})

	var ri headers.RTPinfo
	now := time.Now()

	for _, trackID := range trackIDs {
		seqNum, ts, ok := ss.setuppedStream.rtpInfo(trackID, now)
		if !ok {
			continue
		}

		u := &url.URL{
			Scheme: req.URL.Scheme,
			User:   req.URL.User,
			Host:   req.URL.Host,
			Path:   "/" + *ss.setuppedPath + "/trackID=" + strconv.FormatInt(int64(trackID), 10),
		}

		ri = append(ri, &headers.RTPInfoEntry{
			URL:            u.String(),
			SequenceNumber: &seqNum,
			Timestamp:      &ts,
		})
	}
	if len(ri) > 0 {
		if res.Header == nil {
			res.Header = make(base.Header)
		}
		res.Header["RTP-Info"] = ri.Marshal()
	}

	return res, err
}

func (ss *ServerSession) handleRecord(
	sc *ServerConn,
	path string,
) (*base.Response, error) {
	err := ss.checkState(map[ServerSessionState]struct{}{
		ServerSessionStatePreRecord: {},
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

	if path != *ss.setuppedPath {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, liberrors.ServerPathHasChangedError{Prev: *ss.setuppedPath, Cur: path}
	}

	// allocate writeBuffer before calling OnRecord().
	// in this way it's possible to call ServerSession.WritePacket*()
	// inside the callback.
	ss.writeBuffer, _ = ringbuffer.New(uint64(8))

	res, err := ss.s.handler.OnRecord(ss)

	if res.StatusCode != base.StatusOK {
		ss.writeBuffer = nil
		return res, err
	}

	ss.state = ServerSessionStateRecord

	ss.tcpConn = sc
	ss.tcpConn.readFunc = ss.tcpConn.readFuncTCP
	err = errSwitchReadFunc

	// runWriter() is called by conn after sending the response
	return res, err
}

func (ss *ServerSession) runWriter() {
	defer close(ss.writerDone)

	rtpFrames := make(map[int]*base.InterleavedFrame, len(ss.setuppedTracks))

	for trackID, sst := range ss.setuppedTracks {
		rtpFrames[trackID] = &base.InterleavedFrame{Channel: sst.tcpChannel}
	}

	buf := make([]byte, maxPacketSize+4)

	writeFunc := func(trackID int, payload []byte) {
		fr := rtpFrames[trackID]
		fr.Payload = payload

		ss.tcpConn.nconn.SetWriteDeadline(time.Now().Add(ss.s.writeTimeout)) //nolint:errcheck
		ss.tcpConn.conn.WriteInterleavedFrame(fr, buf)                       //nolint:errcheck
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

func (ss *ServerSession) writePacketRTP(trackID int, byts []byte) {
	if _, ok := ss.setuppedTracks[trackID]; !ok {
		return
	}

	ss.writeBuffer.Push(trackTypePayload{
		trackID: trackID,
		payload: byts,
	})
}

// WritePacketRTP writes a RTP packet to the session.
func (ss *ServerSession) WritePacketRTP(trackID int, pkt *rtp.Packet) {
	byts, err := pkt.Marshal()
	if err != nil {
		return
	}

	ss.writePacketRTP(trackID, byts)
}
