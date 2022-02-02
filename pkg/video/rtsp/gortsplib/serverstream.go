package gortsplib

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"
)

type trackTypePayload struct {
	trackID int
	payload []byte
}

type trackInfo struct {
	lastSequenceNumber uint32
	lastTimeRTP        uint32
	lastTimeNTP        int64
	lastSSRC           uint32
}

// ServerStream represents a single stream.
// This is in charge of
// - distributing the stream to each reader
// - gathering infos about the stream to generate SSRC and RTP-Info.
type ServerStream struct {
	s      *Server
	tracks Tracks

	mutex          sync.RWMutex
	readersUnicast map[*ServerSession]struct{}
	readers        map[*ServerSession]struct{}
	trackInfos     []*trackInfo
}

// NewServerStream allocates a ServerStream.
func NewServerStream(tracks Tracks) *ServerStream {
	st := &ServerStream{
		readersUnicast: make(map[*ServerSession]struct{}),
		readers:        make(map[*ServerSession]struct{}),
	}

	st.tracks = cloneAndClearTracks(tracks)

	st.trackInfos = make([]*trackInfo, len(tracks))
	for i := range st.trackInfos {
		st.trackInfos[i] = &trackInfo{}
	}

	return st
}

// Close closes a ServerStream.
func (st *ServerStream) Close() error {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	if st.s != nil {
		select {
		case st.s.streamRemove <- st:
		case <-st.s.ctx.Done():
		}
	}

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
	return atomic.LoadUint32(&st.trackInfos[trackID].lastSSRC)
}

func (st *ServerStream) timestamp(trackID int) uint32 {
	lastTimeRTP := atomic.LoadUint32(&st.trackInfos[trackID].lastTimeRTP)
	lastTimeNTP := atomic.LoadInt64(&st.trackInfos[trackID].lastTimeNTP)
	clockRate, _ := st.tracks[trackID].ClockRate()

	if lastTimeRTP == 0 || lastTimeNTP == 0 {
		return 0
	}

	return uint32(uint64(lastTimeRTP) +
		uint64(time.Since(time.Unix(lastTimeNTP, 0)).Seconds()*float64(clockRate)))
}

func (st *ServerStream) lastSequenceNumber(trackID int) uint16 {
	return uint16(atomic.LoadUint32(&st.trackInfos[trackID].lastSequenceNumber))
}

func (st *ServerStream) readerAdd(ss *ServerSession) {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	if st.s == nil {
		st.s = ss.s
		select {
		case st.s.streamAdd <- st:
		case <-st.s.ctx.Done():
		}
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
func (st *ServerStream) WritePacketRTP(trackID int, payload []byte) {
	if len(payload) >= 8 {
		track := st.trackInfos[trackID]

		sequenceNumber := binary.BigEndian.Uint16(payload[2:4])
		atomic.StoreUint32(&track.lastSequenceNumber, uint32(sequenceNumber))

		timestamp := binary.BigEndian.Uint32(payload[4:8])
		atomic.StoreUint32(&track.lastTimeRTP, timestamp)
		atomic.StoreInt64(&track.lastTimeNTP, time.Now().Unix())

		ssrc := binary.BigEndian.Uint32(payload[8:12])
		atomic.StoreUint32(&track.lastSSRC, ssrc)
	}

	st.mutex.RLock()
	defer st.mutex.RUnlock()

	// send unicast
	for r := range st.readersUnicast {
		r.WritePacketRTP(trackID, payload)
	}
}
