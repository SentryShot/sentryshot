package video

import (
	"bytes"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/h264"
	"time"

	"github.com/pion/rtp"
)

// data is the data unit routed across the server.
// it must contain one or more of the following:
// - a single RTP packet
// - a group of H264 NALUs (grouped by timestamp)
// - a single AAC AU.
type data struct {
	trackID   int
	rtpPacket *rtp.Packet

	ptsEqualsDTS bool
	pts          time.Duration
	h264NALUs    [][]byte
}

type stream struct {
	rtspStream   *gortsplib.ServerStream
	hlsMuxer     *HLSMuxer
	streamTracks []streamTrack
}

func newStream(tracks gortsplib.Tracks, hlsMuxer *HLSMuxer) *stream {
	s := &stream{
		rtspStream: gortsplib.NewServerStream(tracks),
		hlsMuxer:   hlsMuxer,
	}

	s.streamTracks = make([]streamTrack, len(s.rtspStream.Tracks()))
	for i, track := range s.rtspStream.Tracks() {
		s.streamTracks[i] = newStreamTrack(track, s.writeDataInner)
	}

	return s
}

func (s *stream) close() {
	s.rtspStream.Close()
}

func (s *stream) tracks() gortsplib.Tracks {
	return s.rtspStream.Tracks()
}

func (s *stream) writeData(data *data) {
	s.streamTracks[data.trackID](data)
}

func (s *stream) writeDataInner(data *data) {
	// forward to RTSP readers
	s.rtspStream.WritePacketRTP(data.trackID, data.rtpPacket, data.ptsEqualsDTS)

	// forward to hls muxer.
	s.hlsMuxer.readerData(data)
}

type streamTrack func(*data)

func newStreamTrack(track gortsplib.Track, writeDataInner func(*data)) streamTrack {
	switch ttrack := track.(type) {
	case *gortsplib.TrackH264:
		return newStreamTrackH264(ttrack, writeDataInner)

	case *gortsplib.TrackMPEG4Audio:
		return newStreamTrackMPEG4Audio(writeDataInner)

	default:
		return newStreamTrackGeneric(writeDataInner)
	}
}

// Generic.
func newStreamTrackGeneric(writeDataInner func(*data)) streamTrack {
	return func(dat *data) {
		writeDataInner(dat)
	}
}

// MPEG4Audio.
func newStreamTrackMPEG4Audio(writeDataInner func(*data)) streamTrack {
	return func(dat *data) {
		if dat.rtpPacket != nil {
			writeDataInner(dat)
		}
	}
}

// H264.
func newStreamTrackH264(
	track *gortsplib.TrackH264,
	writeDataInner func(*data),
) streamTrack {
	return func(dat *data) {
		if dat.h264NALUs != nil {
			updateH264TrackParameters(track, dat.h264NALUs)
			dat.h264NALUs = remuxH264NALUs(track, dat.h264NALUs)
		}

		if dat.rtpPacket != nil {
			writeDataInner(dat)
		}
	}
}

func updateH264TrackParameters(track *gortsplib.TrackH264, nalus [][]byte) {
	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case h264.NALUTypeSPS:
			if !bytes.Equal(nalu, track.SafeSPS()) {
				track.SafeSetSPS(nalu)
			}

		case h264.NALUTypePPS:
			if !bytes.Equal(nalu, track.SafePPS()) {
				track.SafeSetPPS(nalu)
			}
		}
	}
}

// remux is needed to
// - fix corrupted streams
// - make streams compatible with all protocols.
func remuxH264NALUs(track *gortsplib.TrackH264, nalus [][]byte) [][]byte {
	n := 0
	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS:
			continue
		case h264.NALUTypeAccessUnitDelimiter:
			continue
		case h264.NALUTypeIDR:
			n += 2
		}
		n++
	}

	if n == 0 {
		return nil
	}

	filteredNALUs := make([][]byte, n)
	i := 0

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS:
			// remove since they're automatically added before every IDR
			continue

		case h264.NALUTypeAccessUnitDelimiter:
			// remove since it is not needed
			continue

		case h264.NALUTypeIDR:
			// add SPS and PPS before every IDR
			filteredNALUs[i] = track.SafeSPS()
			i++
			filteredNALUs[i] = track.SafePPS()
			i++
		}

		filteredNALUs[i] = nalu
		i++
	}

	return filteredNALUs
}
