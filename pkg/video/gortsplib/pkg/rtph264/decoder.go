package rtph264

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"nvr/pkg/video/gortsplib/pkg/h264"
	"nvr/pkg/video/gortsplib/pkg/rtptimedec"
	"time"

	"github.com/pion/rtp"
)

// PacketConnReader creates a io.Reader around a net.PacketConn.
type PacketConnReader struct {
	net.PacketConn
}

// Read implements io.Reader.
func (r PacketConnReader) Read(p []byte) (int, error) {
	n, _, err := r.PacketConn.ReadFrom(p)
	return n, err
}

// Decoder is a RTP/H264 decoder.
type Decoder struct {
	timeDecoder            *rtptimedec.Decoder
	startingPacketReceived bool
	isDecodingFragmented   bool
	fragmentedBuffer       []byte

	// for DecodeUntilMarker()
	naluBuffer [][]byte
}

// NewDecoder allocates a Decoder.
func NewDecoder() *Decoder {
	return &Decoder{
		timeDecoder: rtptimedec.New(90000),
	}
}

// ErrNonStartingPacketAndNoPrevious is returned when we decoded a non-starting
// packet of a fragmented NALU and we didn't received anything before.
// It's normal to receive this when we are decoding a stream that has been already
// running for some time.
var ErrNonStartingPacketAndNoPrevious = errors.New(
	"decoded a non-starting fragmented packet without any previous starting packet")

// Errors.
var (
	ErrMorePacketsNeeded    = errors.New("need more packets")
	ErrShortPayload         = errors.New("payload is too short")
	ErrSTAPinvalid          = errors.New("invalid STAP-A packet (invalid size)")
	ErrSTAPnaluMissing      = errors.New("STAP-A packet doesn't contain any NALU")
	ErrFUinvalidSize        = errors.New("invalid FU-A packet (invalid size)")
	ErrFUinvalidNonStarting = errors.New("invalid FU-A packet (non-starting)")
	ErrFUinvalidStarting    = errors.New("invalid FU-A packet (decoded two starting packets in a row)")
	ErrTypeUnsupported      = errors.New("packet type not supported")
	ErrWrongType            = errors.New("expected FU-A packet, got another type")
)

// Decode decodes NALUs from a RTP/H264 packet.
func (d *Decoder) Decode(pkt *rtp.Packet) ([][]byte, time.Duration, error) {
	if d.isDecodingFragmented {
		return d.decodeFragmented(pkt)
	}
	return d.decodeUnfragmented(pkt)
}

func (d *Decoder) decodeFragmented(pkt *rtp.Packet) ([][]byte, time.Duration, error) {
	// we are decoding a fragmented NALU
	if len(pkt.Payload) < 2 {
		d.isDecodingFragmented = false
		return nil, 0, ErrFUinvalidSize
	}

	typ := naluType(pkt.Payload[0] & 0x1F)
	if typ != naluTypeFUA {
		d.isDecodingFragmented = false
		return nil, 0, ErrWrongType
	}

	start := pkt.Payload[1] >> 7
	end := (pkt.Payload[1] >> 6) & 0x01

	if start == 1 {
		d.isDecodingFragmented = false
		return nil, 0, ErrFUinvalidStarting
	}

	d.fragmentedBuffer = append(d.fragmentedBuffer, pkt.Payload[2:]...)

	if end != 1 {
		return nil, 0, ErrMorePacketsNeeded
	}

	d.isDecodingFragmented = false
	d.startingPacketReceived = true
	return [][]byte{d.fragmentedBuffer}, d.timeDecoder.Decode(pkt.Timestamp), nil
}

func (d *Decoder) decodeUnfragmented(pkt *rtp.Packet) ([][]byte, time.Duration, error) { //nolint:funlen
	if len(pkt.Payload) < 1 {
		return nil, 0, ErrShortPayload
	}

	typ := naluType(pkt.Payload[0] & 0x1F)

	switch typ {
	case naluTypeSTAPA:
		var nalus [][]byte
		pkt.Payload = pkt.Payload[1:]

		for len(pkt.Payload) > 0 {
			if len(pkt.Payload) < 2 {
				return nil, 0, ErrSTAPinvalid
			}

			size := binary.BigEndian.Uint16(pkt.Payload)
			pkt.Payload = pkt.Payload[2:]

			// avoid final padding
			if size == 0 {
				break
			}

			if int(size) > len(pkt.Payload) {
				return nil, 0, ErrSTAPinvalid
			}

			nalus = append(nalus, pkt.Payload[:size])
			pkt.Payload = pkt.Payload[size:]
		}

		if len(nalus) == 0 {
			return nil, 0, ErrSTAPnaluMissing
		}

		d.startingPacketReceived = true
		return nalus, d.timeDecoder.Decode(pkt.Timestamp), nil

	case naluTypeFUA: // first packet of a fragmented NALU
		if len(pkt.Payload) < 2 {
			return nil, 0, ErrFUinvalidSize
		}

		start := pkt.Payload[1] >> 7
		if start != 1 {
			if !d.startingPacketReceived {
				return nil, 0, ErrNonStartingPacketAndNoPrevious
			}
			return nil, 0, ErrFUinvalidNonStarting
		}

		nri := (pkt.Payload[0] >> 5) & 0x03
		typ := pkt.Payload[1] & 0x1F
		d.fragmentedBuffer = append([]byte{(nri << 5) | typ}, pkt.Payload[2:]...)

		d.isDecodingFragmented = true
		d.startingPacketReceived = true
		return nil, 0, ErrMorePacketsNeeded

	case naluTypeSTAPB, naluTypeMTAP16,
		naluTypeMTAP24, naluTypeFUB:
		return nil, 0, fmt.Errorf("%w (%v)", ErrTypeUnsupported, typ)
	}

	d.startingPacketReceived = true
	return [][]byte{pkt.Payload}, d.timeDecoder.Decode(pkt.Timestamp), nil
}

// DecodeUntilMarker decodes NALUs from a RTP/H264 packet and puts them in a buffer.
// When a packet has the marker flag (meaning that all the NALUs with the same PTS have
// been received), the buffer is returned.
func (d *Decoder) DecodeUntilMarker(pkt *rtp.Packet) ([][]byte, time.Duration, error) {
	nalus, pts, err := d.Decode(pkt)
	if err != nil {
		return nil, 0, err
	}

	d.naluBuffer = append(d.naluBuffer, nalus...)

	if !pkt.Marker {
		return nil, 0, ErrMorePacketsNeeded
	}

	ret := d.naluBuffer
	d.naluBuffer = d.naluBuffer[:0]

	return ret, pts, nil
}

// ReadSPSPPS reads RTP/H264 packets from a reader until SPS and PPS are
// found, and returns them.
func (d *Decoder) ReadSPSPPS(r io.Reader) ([]byte, []byte, error) {
	var sps []byte
	var pps []byte

	buf := make([]byte, 2048)
	for {
		n, err := r.Read(buf)
		if err != nil {
			return nil, nil, err
		}

		var pkt rtp.Packet
		err = pkt.Unmarshal(buf[:n])
		if err != nil {
			return nil, nil, err
		}

		nalus, _, err := d.Decode(&pkt)
		if err != nil {
			if errors.Is(err, ErrMorePacketsNeeded) {
				continue
			}
			return nil, nil, err
		}

		for _, nalu := range nalus {
			switch naluType(nalu[0] & 0x1F) {
			case naluType(h264.NALUTypeSPS):
				sps = append([]byte(nil), nalu...)
				if sps != nil && pps != nil {
					return sps, pps, nil
				}

			case naluType(h264.NALUTypePPS):
				pps = append([]byte(nil), nalu...)
				if sps != nil && pps != nil {
					return sps, pps, nil
				}
			}
		}
	}
}
