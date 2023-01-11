package video

import (
	"context"
	"errors"
	"fmt"
	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/base"
	"sync"
	"time"

	"github.com/pion/rtp"
)

type rtspSessionPathManager interface {
	publisherAdd(name string, session *rtspSession) (*path, error)
	readerAdd(name string, session *rtspSession) (*path, *stream, error)
}

type rtspSession struct {
	id          string
	ss          *gortsplib.ServerSession
	author      *gortsplib.ServerConn
	pathManager rtspSessionPathManager

	path            *path
	pathLogf        log.Func
	stream          *stream
	state           gortsplib.ServerSessionState
	stateMutex      sync.Mutex
	announcedTracks gortsplib.Tracks
}

func newRTSPSession(
	id string,
	ss *gortsplib.ServerSession,
	sc *gortsplib.ServerConn,
	pathManager rtspSessionPathManager,
	pathLogf log.Func,
) *rtspSession {
	s := &rtspSession{
		id:          id,
		ss:          ss,
		author:      sc,
		pathManager: pathManager,
		pathLogf:    pathLogf,
	}

	return s
}

// close closes the session.
func (s *rtspSession) close() {
	s.ss.Close()
}

// ID returns the public ID of the session.
func (s *rtspSession) ID() string {
	return s.id
}

func (s *rtspSession) logf(level log.Level, format string, a ...interface{}) {
	if s.pathLogf != nil {
		msg := fmt.Sprintf(format, a...)
		s.pathLogf(level, "RTSP: S:%s %s", s.id, msg)
	}
}

// onConnClose is called by rtspServer.
func (s *rtspSession) onConnClose(err error) {
	if err != nil && !errors.Is(err, context.Canceled) {
		s.logf(log.LevelError, "closed: %v", err)
	} else {
		s.logf(log.LevelDebug, "closed")
	}
}

// onClose is called by rtspServer.
func (s *rtspSession) onClose(err error) {
	switch s.ss.State() {
	case gortsplib.ServerSessionStatePrePlay, gortsplib.ServerSessionStatePlay:
		s.path.readerRemove(s)
		s.path = nil

	case gortsplib.ServerSessionStatePreRecord, gortsplib.ServerSessionStateRecord:
		s.path.close()
		s.path = nil
	}

	if s.pathLogf != nil {
		if err != nil && !errors.Is(err, context.Canceled) {
			s.logf(log.LevelError, "destroyed: %v", err)
		} else {
			s.logf(log.LevelDebug, "destroyed")
		}
	}
}

// Errors .
var (
	ErrTrackInvalidH264 = errors.New("h264 SPS or PPS not provided into the SDP")
	ErrTrackInvalidAAC  = errors.New("aac track is not valid")
	ErrTrackInvalidOpus = errors.New("opus track is not valid")
)

// onAnnounce is called by rtspServer.
func (s *rtspSession) onAnnounce(
	pathName string,
	tracks gortsplib.Tracks,
) (*base.Response, error) {
	path, err := s.pathManager.publisherAdd(pathName, s)
	if err != nil {
		return &base.Response{StatusCode: base.StatusBadRequest}, err
	}

	s.path = path
	s.announcedTracks = tracks

	s.stateMutex.Lock()
	s.state = gortsplib.ServerSessionStatePreRecord
	s.stateMutex.Unlock()

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// ErrTrackNotExist Track does not exist.
var ErrTrackNotExist = errors.New("track does not exist")

// onSetup is called by rtspServer.
func (s *rtspSession) onSetup(
	pathName string,
	trackID int,
) (*base.Response, *gortsplib.ServerStream, error) {
	state := s.ss.State()

	// record
	if state != gortsplib.ServerSessionStateInitial &&
		state != gortsplib.ServerSessionStatePrePlay {
		return &base.Response{StatusCode: base.StatusOK}, nil, nil
	}

	// play
	path, stream, err := s.pathManager.readerAdd(pathName, s)
	if err != nil {
		if errors.Is(err, ErrPathNoOnePublishing) {
			return &base.Response{StatusCode: base.StatusNotFound}, nil, err
		}
		return &base.Response{StatusCode: base.StatusBadRequest}, nil, err
	}

	s.path = path

	if trackID >= len(stream.tracks()) {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, nil, fmt.Errorf("%w (%d)", ErrTrackNotExist, trackID)
	}

	s.stateMutex.Lock()
	s.state = gortsplib.ServerSessionStatePrePlay
	s.stateMutex.Unlock()

	return &base.Response{StatusCode: base.StatusOK}, stream.rtspStream, nil
}

// onPlay is called by rtspServer.
func (s *rtspSession) onPlay() (*base.Response, error) {
	h := make(base.Header)

	if s.ss.State() == gortsplib.ServerSessionStatePrePlay {
		s.path.readerStart(s)

		s.stateMutex.Lock()
		s.state = gortsplib.ServerSessionStatePlay
		s.stateMutex.Unlock()
	}

	return &base.Response{
		StatusCode: base.StatusOK,
		Header:     h,
	}, nil
}

// onRecord is called by rtspServer.
func (s *rtspSession) onRecord() (*base.Response, error) {
	stream, err := s.path.publisherStart(s.announcedTracks)
	if err != nil {
		return &base.Response{StatusCode: base.StatusBadRequest}, err
	}

	s.stream = stream

	s.stateMutex.Lock()
	s.state = gortsplib.ServerSessionStateRecord
	s.stateMutex.Unlock()

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// onPacketRTP is called by rtspServer.
func (s *rtspSession) onPacketRTP(trackID int, packet *rtp.Packet) {
	var err error

	switch s.announcedTracks[trackID].(type) {
	case *gortsplib.TrackH264:
		err = s.stream.writeData(&dataH264{
			trackID:    trackID,
			rtpPackets: []*rtp.Packet{packet},
			ntp:        time.Now(),
		})

	case *gortsplib.TrackMPEG4Audio:
		err = s.stream.writeData(&dataMPEG4Audio{
			trackID:    trackID,
			rtpPackets: []*rtp.Packet{packet},
			ntp:        time.Now(),
		})
	}

	if err != nil {
		s.logf(log.LevelWarning, "write data: %v", err)
	}
}

func (s *rtspSession) onDecodeError(err error) {
	s.logf(log.LevelWarning, "decode: %v", err)
}
