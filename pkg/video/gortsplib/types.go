package gortsplib

import (
	"nvr/pkg/video/gortsplib/pkg/base"
	"time"

	"github.com/pion/rtp"
)

// ServerHandler is the interface implemented by all the server handlers.
type ServerHandler interface {
	OnConnClose(*ServerConn, error)
	OnSessionOpen(*ServerSession, *ServerConn, string)
	OnSessionClose(*ServerSession, error)
	OnDescribe(pathName string) (*base.Response, *ServerStream, error)
	OnAnnounce(*ServerSession, string, Tracks) (*base.Response, error)
	OnSetup(*ServerSession, string, int) (*base.Response, *ServerStream, error)
	OnPlay(*ServerSession) (*base.Response, error)
	OnRecord(*ServerSession) (*base.Response, error)
	OnPacketRTP(*PacketRTPCtx)
}

// PacketRTPCtx is the context of a RTP packet.
type PacketRTPCtx struct {
	Session      *ServerSession
	TrackID      int
	Packet       *rtp.Packet
	PTSEqualsDTS bool
	H264NALUs    [][]byte
	H264PTS      time.Duration
}
