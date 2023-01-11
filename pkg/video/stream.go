package video

import (
	"bytes"
	"errors"
	"fmt"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/h264"
	"nvr/pkg/video/gortsplib/pkg/rtph264"
	"nvr/pkg/video/gortsplib/pkg/rtpmpeg4audio"

	"github.com/pion/rtp"
)

type streamTrack interface {
	onData(data) error
}

func newStreamTrack(track gortsplib.Track) streamTrack {
	switch ttrack := track.(type) {
	case *gortsplib.TrackH264:
		return newStreamTrackH264(ttrack)

	case *gortsplib.TrackMPEG4Audio:
		return newStreamTrackMPEG4Audio(ttrack)

	default:
		return nil
	}
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
		s.streamTracks[i] = newStreamTrack(track)
	}

	return s
}

func (s *stream) close() {
	s.rtspStream.Close()
}

func (s *stream) tracks() gortsplib.Tracks {
	return s.rtspStream.Tracks()
}

func (s *stream) writeData(data data) error {
	err := s.streamTracks[data.getTrackID()].onData(data)
	if err != nil {
		return fmt.Errorf("on data: %w", err)
	}

	// Forward to rtsp stream.
	for _, pkt := range data.getRTPPackets() {
		s.rtspStream.WritePacketRTPWithNTP(data.getTrackID(), pkt, data.getNTP())
	}

	// Forward to hls muxer.
	s.hlsMuxer.readerData(data)

	return nil
}

const (
	// 1500 (UDP MTU) - 20 (IP header) - 8 (UDP header).
	maxPacketSize = 1472
)

func rtpH264ExtractSPSPPS(pkt *rtp.Packet) ([]byte, []byte) {
	if len(pkt.Payload) == 0 {
		return nil, nil
	}

	typ := h264.NALUType(pkt.Payload[0] & 0x1F)

	switch typ {
	case h264.NALUTypeSPS:
		return pkt.Payload, nil

	case h264.NALUTypePPS:
		return nil, pkt.Payload

	case 24: // STAP-A
		payload := pkt.Payload[1:]
		var sps []byte
		var pps []byte

		for len(payload) > 0 {
			if len(payload) < 2 {
				break
			}

			size := uint16(payload[0])<<8 | uint16(payload[1])
			payload = payload[2:]

			if size == 0 || int(size) > len(payload) {
				break
			}

			nalu := payload[:size]
			payload = payload[size:]

			typ = h264.NALUType(nalu[0] & 0x1F)

			switch typ {
			case h264.NALUTypeSPS:
				sps = nalu

			case h264.NALUTypePPS:
				pps = nalu
			}
		}

		return sps, pps

	default:
		return nil, nil
	}
}

type streamTrackH264 struct {
	track   *gortsplib.TrackH264
	encoder *rtph264.Encoder
	decoder *rtph264.Decoder
}

func newStreamTrackH264(track *gortsplib.TrackH264) *streamTrackH264 {
	return &streamTrackH264{
		track:   track,
		decoder: track.CreateDecoder(),
	}
}

func (t *streamTrackH264) updateTrackParametersFromRTPPacket(pkt *rtp.Packet) {
	sps, pps := rtpH264ExtractSPSPPS(pkt)

	if sps != nil && !bytes.Equal(sps, t.track.SafeSPS()) {
		t.track.SafeSetSPS(sps)
	}

	if pps != nil && !bytes.Equal(pps, t.track.SafePPS()) {
		t.track.SafeSetPPS(pps)
	}
}

func (t *streamTrackH264) updateTrackParametersFromNALUs(nalus [][]byte) {
	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case h264.NALUTypeSPS:
			if !bytes.Equal(nalu, t.track.SafeSPS()) {
				t.track.SafeSetSPS(nalu)
			}

		case h264.NALUTypePPS:
			if !bytes.Equal(nalu, t.track.SafePPS()) {
				t.track.SafeSetPPS(nalu)
			}
		}
	}
}

// remux is needed to fix corrupted streams and make streams
// compatible with all protocols.
func (t *streamTrackH264) remuxNALUs(nalus [][]byte) [][]byte {
	addSPSPPS := false
	n := 0
	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS:
			continue
		case h264.NALUTypeAccessUnitDelimiter:
			continue
		case h264.NALUTypeIDR:
			// prepend SPS and PPS to the group if there's at least an IDR
			if !addSPSPPS {
				addSPSPPS = true
				n += 2
			}
		}
		n++
	}

	if n == 0 {
		return nil
	}

	filteredNALUs := make([][]byte, n)
	i := 0

	if addSPSPPS {
		filteredNALUs[0] = t.track.SafeSPS()
		filteredNALUs[1] = t.track.SafePPS()
		i = 2
	}

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS:
			// remove since they're automatically added
			continue

		case h264.NALUTypeAccessUnitDelimiter:
			// remove since it is not needed
			continue
		}

		filteredNALUs[i] = nalu
		i++
	}

	return filteredNALUs
}

func (t *streamTrackH264) generateRTPPackets(tdata *dataH264) error {
	pkts, err := t.encoder.Encode(tdata.nalus, tdata.pts)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	tdata.rtpPackets = pkts
	return nil
}

func (t *streamTrackH264) onData(dat data) error {
	tdata := dat.(*dataH264) //nolint:forcetypeassert

	if tdata.rtpPackets == nil {
		t.updateTrackParametersFromNALUs(tdata.nalus)
		tdata.nalus = t.remuxNALUs(tdata.nalus)

		return t.generateRTPPackets(tdata)
	}

	pkt := tdata.rtpPackets[0]
	t.updateTrackParametersFromRTPPacket(pkt)

	if t.encoder == nil {
		// remove padding
		pkt.Header.Padding = false
		pkt.PaddingSize = 0

		// we need to re-encode since RTP packets exceed maximum size
		if pkt.MarshalSize() > maxPacketSize {
			v1 := pkt.SSRC
			v2 := pkt.SequenceNumber
			v3 := pkt.Timestamp
			t.encoder = &rtph264.Encoder{
				PayloadType:           pkt.PayloadType,
				SSRC:                  &v1,
				InitialSequenceNumber: &v2,
				InitialTimestamp:      &v3,
				PacketizationMode:     1,
			}
			t.encoder.Init()
		}
	}

	nalus, pts, err := t.decoder.Decode(pkt)
	if err != nil {
		if errors.Is(err, rtph264.ErrNonStartingPacketAndNoPrevious) ||
			errors.Is(err, rtph264.ErrMorePacketsNeeded) {
			return nil
		}
		return fmt.Errorf("decode: %w", err)
	}

	tdata.nalus = nalus
	tdata.pts = pts

	tdata.nalus = t.remuxNALUs(tdata.nalus)

	// route packet as is
	if t.encoder == nil {
		return nil
	}

	return t.generateRTPPackets(tdata)
}

type streamTrackMPEG4Audio struct {
	track   *gortsplib.TrackMPEG4Audio
	encoder *rtpmpeg4audio.Encoder
	decoder *rtpmpeg4audio.Decoder
}

func newStreamTrackMPEG4Audio(track *gortsplib.TrackMPEG4Audio) *streamTrackMPEG4Audio {
	return &streamTrackMPEG4Audio{
		track:   track,
		encoder: track.CreateEncoder(),
		decoder: track.CreateDecoder(),
	}
}

func (t *streamTrackMPEG4Audio) generateRTPPackets(tdata *dataMPEG4Audio) error {
	pkts, err := t.encoder.Encode(tdata.aus, tdata.pts)
	if err != nil {
		return err
	}

	tdata.rtpPackets = pkts
	return nil
}

// PayloadTooBigError .
type PayloadTooBigError struct {
	size int
}

func (e PayloadTooBigError) Error() string {
	return fmt.Sprintf("payload size (%d) is greater than maximum allowed (%d)",
		e.size, maxPacketSize)
}

func (t *streamTrackMPEG4Audio) onData(dat data) error {
	tdata := dat.(*dataMPEG4Audio) //nolint:forcetypeassert

	if tdata.rtpPackets == nil {
		return t.generateRTPPackets(tdata)
	}

	pkt := tdata.rtpPackets[0]

	// remove padding
	pkt.Header.Padding = false
	pkt.PaddingSize = 0

	if pkt.MarshalSize() > maxPacketSize {
		return PayloadTooBigError{size: pkt.MarshalSize()}
	}

	aus, pts, err := t.decoder.Decode(pkt)
	if err != nil {
		if errors.Is(err, rtpmpeg4audio.ErrMorePacketsNeeded) {
			return nil
		}
		return err
	}

	tdata.aus = aus
	tdata.pts = pts

	return nil
}
