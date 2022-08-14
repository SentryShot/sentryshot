package gortsplib

import (
	"nvr/pkg/video/gortsplib/pkg/base"
	"time"

	"github.com/pion/rtp"
)

// ServerHandler is the interface implemented by all the server handlers.
type ServerHandler interface {
	OnConnClose(*ServerHandlerOnConnCloseCtx)
	OnSessionOpen(*ServerHandlerOnSessionOpenCtx)
	OnSessionClose(*ServerHandlerOnSessionCloseCtx)
	OnDescribe(*ServerHandlerOnDescribeCtx) (*base.Response, *ServerStream, error)
	OnAnnounce(*ServerHandlerOnAnnounceCtx) (*base.Response, error)
	OnSetup(*ServerHandlerOnSetupCtx) (*base.Response, *ServerStream, error)
	OnPlay(*ServerHandlerOnPlayCtx) (*base.Response, error)
	OnRecord(*ServerHandlerOnRecordCtx) (*base.Response, error)
	OnPause(*ServerHandlerOnPauseCtx) (*base.Response, error)
	OnPacketRTP(*ServerHandlerOnPacketRTPCtx)
}

// ServerHandlerOnConnCloseCtx is the context of a connection closure.
type ServerHandlerOnConnCloseCtx struct {
	Conn  *ServerConn
	Error error
}

// ServerHandlerOnSessionOpenCtx is the context of a session opening.
type ServerHandlerOnSessionOpenCtx struct {
	Session *ServerSession
	Conn    *ServerConn
}

// ServerHandlerOnSessionCloseCtx is the context of a session closure.
type ServerHandlerOnSessionCloseCtx struct {
	Session *ServerSession
	Error   error
}

// ServerHandlerOnDescribeCtx is the context of a DESCRIBE request.
type ServerHandlerOnDescribeCtx struct {
	Conn    *ServerConn
	Request *base.Request
	Path    string
	Query   string
}

// ServerHandlerOnAnnounceCtx is the context of an ANNOUNCE request.
type ServerHandlerOnAnnounceCtx struct {
	Server  *Server
	Session *ServerSession
	Conn    *ServerConn
	Request *base.Request
	Path    string
	Query   string
	Tracks  Tracks
}

// ServerHandlerOnSetupCtx is the context of a OPTIONS request.
type ServerHandlerOnSetupCtx struct {
	Server  *Server
	Session *ServerSession
	Conn    *ServerConn
	Request *base.Request
	Path    string
	Query   string
	TrackID int
}

// ServerHandlerOnPlayCtx is the context of a PLAY request.
type ServerHandlerOnPlayCtx struct {
	Session *ServerSession
	Conn    *ServerConn
	Request *base.Request
	Path    string
	Query   string
}

// ServerHandlerOnRecordCtx is the context of a RECORD request.
type ServerHandlerOnRecordCtx struct {
	Session *ServerSession
	Conn    *ServerConn
	Request *base.Request
	Path    string
	Query   string
}

// ServerHandlerOnPauseCtx is the context of a PAUSE request.
type ServerHandlerOnPauseCtx struct {
	Session *ServerSession
	Conn    *ServerConn
	Request *base.Request
	Path    string
	Query   string
}

// ServerHandlerOnPacketRTPCtx is the context of a RTP packet.
type ServerHandlerOnPacketRTPCtx struct {
	Session      *ServerSession
	TrackID      int
	Packet       *rtp.Packet
	PTSEqualsDTS bool
	H264NALUs    [][]byte
	H264PTS      time.Duration
}
