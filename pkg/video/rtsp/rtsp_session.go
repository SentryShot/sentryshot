package rtsp

import (
	"errors"
	"fmt"
	"net"
	"nvr/pkg/log"
	"nvr/pkg/video/rtsp/gortsplib"
	"nvr/pkg/video/rtsp/gortsplib/pkg/base"
	"sync"
)

type rtspSession struct {
	id          string
	ss          *gortsplib.ServerSession
	author      *gortsplib.ServerConn
	pathManager *pathManager
	logger      *log.Logger

	path            *path
	state           gortsplib.ServerSessionState
	stateMutex      sync.Mutex
	setuppedTracks  map[int]*gortsplib.Track // read
	announcedTracks gortsplib.Tracks         // publish
	stream          *stream                  // publish
}

func newRTSPSession(
	id string,
	ss *gortsplib.ServerSession,
	sc *gortsplib.ServerConn,
	pathManager *pathManager,
	logger *log.Logger) *rtspSession {
	s := &rtspSession{
		id:          id,
		ss:          ss,
		author:      sc,
		pathManager: pathManager,
		logger:      logger,
		path:        &path{conf: &PathConf{}},
	}

	return s
}

// close closes a Session.
func (s *rtspSession) close() {
	s.ss.Close()
}

// IsRTSPSession implements pathRTSPSession.
func (s *rtspSession) IsRTSPSession() {}

// ID returns the public ID of the session.
func (s *rtspSession) ID() string {
	return s.id
}

// RemoteAddr returns the remote address of the author of the session.
func (s *rtspSession) RemoteAddr() net.Addr {
	return s.author.NetConn().RemoteAddr()
}

func (s *rtspSession) logf(level log.Level, conf PathConf, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	sendLog(s.logger, conf, level, fmt.Sprintf("S:%s %s", s.id, msg))
}

// onClose is called by rtspServer.
func (s *rtspSession) onClose(conf PathConf, err error) {
	switch s.ss.State() {
	case gortsplib.ServerSessionStatePreRead, gortsplib.ServerSessionStateRead:
		s.path.onReaderRemove(pathReaderRemoveReq{Author: s})
		s.path = nil

	case gortsplib.ServerSessionStatePrePublish, gortsplib.ServerSessionStatePublish:
		s.path.onPublisherRemove(pathPublisherRemoveReq{Author: s})
		s.path = nil
	}

	s.logf(log.LevelDebug, conf, "closed (%v)", err)
}

// Errors .
var (
	ErrTrackInvalidH264 = errors.New("h264 track is not valid")
	ErrTrackInvalidAAC  = errors.New("aac track is not valid")
	ErrTrackInvalidOpus = errors.New("opus track is not valid")
)

// onAnnounce is called by rtspServer.
func (s *rtspSession) onAnnounce(ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	for i, track := range ctx.Tracks {
		if track.IsH264() {
			_, err := track.ExtractConfigH264()
			if err != nil {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("%w %d: %v", ErrTrackInvalidH264, i+1, err)
			}
		}

		if track.IsAAC() {
			_, err := track.ExtractConfigAAC()
			if err != nil {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("%w %d: %v", ErrTrackInvalidAAC, i+1, err)
			}
		}

		if track.IsOpus() {
			_, err := track.ExtractConfigOpus()
			if err != nil {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("%w %d: %v", ErrTrackInvalidOpus, i+1, err)
			}
		}
	}

	res := s.pathManager.onPublisherAnnounce(pathPublisherAnnounceReq{
		Author:   s,
		PathName: ctx.Path,
	})

	if res.Err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, res.Err
	}

	s.path = res.Path
	s.announcedTracks = ctx.Tracks

	s.stateMutex.Lock()
	s.state = gortsplib.ServerSessionStatePrePublish
	s.stateMutex.Unlock()

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// ErrTrackNotExist Track does not exist.
var ErrTrackNotExist = errors.New("track does not exist")

// onSetup is called by rtspServer.
func (s *rtspSession) onSetup(ctx *gortsplib.ServerHandlerOnSetupCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	state := s.ss.State()

	// record
	if state != gortsplib.ServerSessionStateInitial &&
		state != gortsplib.ServerSessionStatePreRead {
		return &base.Response{
			StatusCode: base.StatusOK,
		}, nil, nil
	}

	// play
	res := s.pathManager.onReaderSetupPlay(pathReaderSetupPlayReq{
		Author:   s,
		PathName: ctx.Path,
	})

	if res.Err != nil {
		if errors.Is(res.Err, ErrPathNoOnePublishing) {
			return &base.Response{
				StatusCode: base.StatusNotFound,
			}, nil, res.Err
		}
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, nil, res.Err
	}

	s.path = res.Path

	if ctx.TrackID >= len(res.Stream.tracks()) {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, nil, fmt.Errorf("%w (%d)", ErrTrackNotExist, ctx.TrackID)
	}

	if s.setuppedTracks == nil {
		s.setuppedTracks = make(map[int]*gortsplib.Track)
	}
	s.setuppedTracks[ctx.TrackID] = res.Stream.tracks()[ctx.TrackID]

	s.stateMutex.Lock()
	s.state = gortsplib.ServerSessionStatePreRead
	s.stateMutex.Unlock()

	return &base.Response{
		StatusCode: base.StatusOK,
	}, res.Stream.rtspStream, nil
}

// onPlay is called by rtspServer.
func (s *rtspSession) onPlay() (*base.Response, error) {
	h := make(base.Header)

	if s.ss.State() == gortsplib.ServerSessionStatePreRead {
		s.path.onReaderPlay(pathReaderPlayReq{Author: s})

		s.stateMutex.Lock()
		s.state = gortsplib.ServerSessionStateRead
		s.stateMutex.Unlock()
	}

	return &base.Response{
		StatusCode: base.StatusOK,
		Header:     h,
	}, nil
}

// onRecord is called by rtspServer.
func (s *rtspSession) onRecord() (*base.Response, error) {
	res := s.path.onPublisherRecord(pathPublisherRecordReq{
		Author: s,
		Tracks: s.announcedTracks,
	})
	if res.Err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, res.Err
	}

	s.stream = res.Stream

	s.stateMutex.Lock()
	s.state = gortsplib.ServerSessionStatePublish
	s.stateMutex.Unlock()

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// onPause is called by rtspServer.
func (s *rtspSession) onPause() (*base.Response, error) {
	switch s.ss.State() { //nolint:exhaustive
	case gortsplib.ServerSessionStateRead:
		s.path.onReaderPause(pathReaderPauseReq{Author: s})

		s.stateMutex.Lock()
		s.state = gortsplib.ServerSessionStatePreRead
		s.stateMutex.Unlock()

	case gortsplib.ServerSessionStatePublish:
		s.path.onPublisherPause(pathPublisherPauseReq{Author: s})

		s.stateMutex.Lock()
		s.state = gortsplib.ServerSessionStatePrePublish
		s.stateMutex.Unlock()
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

func formatTracksLen(tracksLen int) string {
	if tracksLen == 1 {
		return fmt.Sprintf("%d track", tracksLen)
	}
	return fmt.Sprintf("%d tracks", tracksLen)
}

// onReaderAccepted implements reader.
func (s *rtspSession) onReaderAccepted() {
	tracksLen := len(s.ss.SetuppedTracks())

	s.logf(log.LevelDebug,
		*s.path.conf,
		"is reading %s",
		formatTracksLen(tracksLen),
	)
}

// onReaderPacketRTP implements reader.
func (s *rtspSession) onReaderPacketRTP(trackID int, payload []byte) {
	// packets are routed to the session by gortsplib.ServerStream.
}

// onPublisherAccepted implements publisher.
func (s *rtspSession) onPublisherAccepted(tracksLen int) {
	s.logf(
		log.LevelDebug,
		*s.path.conf,
		"is publishing %v",
		formatTracksLen(tracksLen),
	)
}

// onPacketRTP is called by rtspServer.
func (s *rtspSession) onPacketRTP(ctx *gortsplib.ServerHandlerOnPacketRTPCtx) {
	if s.ss.State() != gortsplib.ServerSessionStatePublish {
		return
	}

	s.stream.onPacketRTP(ctx.TrackID, ctx.Payload)
}
