package gortsplib

import (
	"errors"
	"sync"
	"time"

	"github.com/pion/rtp"
)

type trackTypePayload struct {
	trackID int
	payload []byte
}

type serverStreamTrack struct {
	lastSequenceNumber uint16
	lastSSRC           uint32
	lastTimeFilled     bool
	lastTimeRTP        uint32
	lastTimeNTP        time.Time
}

// ServerStream represents a single stream.
// This is in charge of
// - distributing the stream to each reader
// - gathering infos about the stream to generate SSRC and RTP-Info.
type ServerStream struct {
	tracks Tracks

	mutex          sync.RWMutex
	s              *Server
	readersUnicast map[*ServerSession]struct{}
	readers        map[*ServerSession]struct{}
	streamTracks   []*serverStreamTrack
	closed         bool
}

// NewServerStream allocates a ServerStream.
func NewServerStream(tracks Tracks) *ServerStream {
	tracks = tracks.clone()
	tracks.setControls()

	st := &ServerStream{
		tracks:         tracks,
		readersUnicast: make(map[*ServerSession]struct{}),
		readers:        make(map[*ServerSession]struct{}),
	}

	st.streamTracks = make([]*serverStreamTrack, len(tracks))
	for i := range st.streamTracks {
		st.streamTracks[i] = &serverStreamTrack{}
	}

	return st
}

// Close closes a ServerStream.
func (st *ServerStream) Close() error {
	st.mutex.Lock()
	st.closed = true
	st.mutex.Unlock()

	for ss := range st.readers {
		ss.Close()
	}

	return nil
}

// Tracks returns the tracks of the stream.
func (st *ServerStream) Tracks() Tracks {
	return st.tracks
}

func (st *ServerStream) ssrc(trackID int) uint32 {
	st.mutex.Lock()
	defer st.mutex.Unlock()
	return st.streamTracks[trackID].lastSSRC
}

func (st *ServerStream) rtpInfo(trackID int, now time.Time) (uint16, uint32, bool) {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	track := st.streamTracks[trackID]

	if !track.lastTimeFilled {
		return 0, 0, false
	}

	clockRate := st.tracks[trackID].ClockRate()
	if clockRate == 0 {
		return 0, 0, false
	}

	// sequence number of the first packet of the stream
	seq := track.lastSequenceNumber + 1

	// RTP timestamp corresponding to the time value in
	// the Range response header.
	// remove a small quantity in order to avoid DTS > PTS
	ts := uint32(uint64(track.lastTimeRTP) +
		uint64(now.Sub(track.lastTimeNTP).Seconds()*float64(clockRate)) -
		uint64(clockRate)/10)

	return seq, ts, true
}

// ErrClosedStream stream is closed.
var ErrClosedStream = errors.New("stream is closed")

func (st *ServerStream) readerAdd(ss *ServerSession) error {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	if st.closed {
		return ErrClosedStream
	}

	if st.s == nil {
		st.s = ss.s
	}

	st.readers[ss] = struct{}{}

	return nil
}

func (st *ServerStream) readerRemove(ss *ServerSession) {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	if st.closed {
		return
	}

	delete(st.readers, ss)
}

func (st *ServerStream) readerSetActive(ss *ServerSession) {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	if st.closed {
		return
	}

	st.readersUnicast[ss] = struct{}{}
}

func (st *ServerStream) readerSetInactive(ss *ServerSession) {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	if st.closed {
		return
	}

	delete(st.readersUnicast, ss)
}

// WritePacketRTP writes a RTP packet to all the readers of the stream.
func (st *ServerStream) WritePacketRTP(trackID int, pkt *rtp.Packet) {
	st.WritePacketRTPWithNTP(trackID, pkt, time.Now())
}

// WritePacketRTPWithNTP writes a RTP packet to all the readers of the stream.
// ntp is the absolute time of the packet, and is needed to generate RTCP sender reports
// that allows the receiver to reconstruct the absolute time of the packet.
func (st *ServerStream) WritePacketRTPWithNTP(trackID int, pkt *rtp.Packet, ntp time.Time) {
	byts := make([]byte, maxPacketSize)
	n, err := pkt.MarshalTo(byts)
	if err != nil {
		return
	}
	byts = byts[:n]

	st.mutex.RLock()
	defer st.mutex.RUnlock()

	if st.closed {
		return
	}

	track := st.streamTracks[trackID]
	ptsEqualsDTS := ptsEqualsDTS(st.tracks[trackID], pkt)

	if ptsEqualsDTS {
		track.lastTimeFilled = true
		track.lastTimeRTP = pkt.Header.Timestamp
		track.lastTimeNTP = ntp
	}

	track.lastSequenceNumber = pkt.Header.SequenceNumber
	track.lastSSRC = pkt.Header.SSRC

	// send unicast
	for r := range st.readersUnicast {
		r.writePacketRTP(trackID, byts)
	}
}
