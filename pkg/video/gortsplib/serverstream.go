package gortsplib

import (
	"sync"
	"time"

	"github.com/pion/rtp"
)

type trackTypePayload struct {
	trackID int
	payload []byte
}

type serverStreamTrack struct {
	firstPacketSent    bool
	lastSequenceNumber uint16
	lastSSRC           uint32
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
	stTracks       []*serverStreamTrack
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

	st.stTracks = make([]*serverStreamTrack, len(tracks))
	for i := range st.stTracks {
		st.stTracks[i] = &serverStreamTrack{}
	}

	return st
}

// Close closes a ServerStream.
func (st *ServerStream) Close() error {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	for ss := range st.readers {
		ss.Close()
	}

	st.readers = nil
	st.readersUnicast = nil

	return nil
}

// Tracks returns the tracks of the stream.
func (st *ServerStream) Tracks() Tracks {
	return st.tracks
}

func (st *ServerStream) ssrc(trackID int) uint32 {
	st.mutex.Lock()
	defer st.mutex.Unlock()
	return st.stTracks[trackID].lastSSRC
}

func (st *ServerStream) rtpInfo(trackID int, now time.Time) (uint16, uint32, bool) {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	track := st.stTracks[trackID]

	if !track.firstPacketSent {
		return 0, 0, false
	}

	// sequence number of the first packet of the stream
	seq := track.lastSequenceNumber + 1

	// RTP timestamp corresponding to the time value in
	// the Range response header.
	// remove a small quantity in order to avoid DTS > PTS
	cr := st.tracks[trackID].ClockRate()
	ts := uint32(uint64(track.lastTimeRTP) +
		uint64(now.Sub(track.lastTimeNTP).Seconds()*float64(cr)) -
		uint64(cr)/10)

	return seq, ts, true
}

func (st *ServerStream) readerAdd(ss *ServerSession) {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	if st.s == nil {
		st.s = ss.s
	}

	st.readers[ss] = struct{}{}
}

func (st *ServerStream) readerRemove(ss *ServerSession) {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	delete(st.readers, ss)
}

func (st *ServerStream) readerSetActive(ss *ServerSession) {
	st.mutex.Lock()
	st.readersUnicast[ss] = struct{}{}
	st.mutex.Unlock()
}

func (st *ServerStream) readerSetInactive(ss *ServerSession) {
	st.mutex.Lock()
	delete(st.readersUnicast, ss)
	st.mutex.Unlock()
}

// WritePacketRTP writes a RTP packet to all the readers of the stream.
func (st *ServerStream) WritePacketRTP(trackID int, pkt *rtp.Packet, ptsEqualsDTS bool) {
	byts := make([]byte, maxPacketSize)
	n, err := pkt.MarshalTo(byts)
	if err != nil {
		return
	}
	byts = byts[:n]

	st.mutex.RLock()
	defer st.mutex.RUnlock()

	track := st.stTracks[trackID]
	now := time.Now()

	if !track.firstPacketSent ||
		ptsEqualsDTS ||
		pkt.Header.SequenceNumber > track.lastSequenceNumber ||
		(track.lastSequenceNumber-pkt.Header.SequenceNumber) > 0xFFF {
		if !track.firstPacketSent || ptsEqualsDTS {
			track.lastTimeRTP = pkt.Header.Timestamp
			track.lastTimeNTP = now
		}

		track.firstPacketSent = true
		track.lastSequenceNumber = pkt.Header.SequenceNumber
		track.lastSSRC = pkt.Header.SSRC
	}

	// send unicast
	for r := range st.readersUnicast {
		r.writePacketRTP(trackID, byts)
	}
}
