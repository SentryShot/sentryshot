package rtph264

import (
	"bytes"
	"errors"
	"fmt"
	"nvr/pkg/video/gortsplib/pkg/h264"
	"nvr/pkg/video/gortsplib/pkg/rtptimedec"
	"time"

	"github.com/pion/rtp"
)

// ErrNonStartingPacketAndNoPrevious is returned when we received a non-starting
// packet of a fragmented NALU and we didn't received anything before.
// It's normal to receive this when we are decoding a stream that has been already
// running for some time.
var ErrNonStartingPacketAndNoPrevious = errors.New(
	"received a non-starting FU-A packet without any previous FU-A starting packet")

// Errors.
var (
	ErrMorePacketsNeeded    = errors.New("need more packets")
	ErrShortPayload         = errors.New("payload is too short")
	ErrSTAPinvalid          = errors.New("invalid STAP-A packet (invalid size)")
	ErrSTAPnaluMissing      = errors.New("STAP-A packet doesn't contain any NALU")
	ErrFUinvalidSize        = errors.New("invalid FU-A packet (invalid size)")
	ErrFUinvalidNonStarting = errors.New("invalid FU-A packet (non-starting)")
	ErrFUinvalidStarting    = errors.New("invalid FU-A packet (decoded two starting packets in a row)")
	ErrFUinvalidStartAndEnd = errors.New("invalid FU-A packet (can't contain both a start and end bit)")
	ErrTypeUnsupported      = errors.New("packet type not supported")
	ErrMultipleAnnexBNalus  = errors.New("multiple NALUs in Annex-B mode are not supported")
	ErrModeUnsupported      = errors.New("PacketizationMode >= 2 is not supported")
)

// WrongTypeError .
type WrongTypeError struct {
	typ naluType
}

func (e WrongTypeError) Error() string {
	return fmt.Sprintf("expected FU-A packet, got %s packet", e.typ)
}

// NALUToBigError .
type NALUToBigError struct {
	NALUsize int
}

func (e NALUToBigError) Error() string {
	return fmt.Sprintf("NALU size (%d) is too big (maximum is %d)", e.NALUsize, h264.MaxNALUSize)
}

// MaxNALUsError .
type MaxNALUsError struct {
	count int
}

func (e MaxNALUsError) Error() string {
	return fmt.Sprintf("number of NALUs contained inside a single group (%d)"+
		" is too big (maximum is %d)", e.count, h264.MaxNALUsPerGroup)
}

// Decoder is a RTP/H264 decoder.
type Decoder struct {
	PacketizationMode int

	timeDecoder         *rtptimedec.Decoder
	firstPacketReceived bool
	fragmentedSize      int
	fragments           [][]byte
	firstNALUParsed     bool
	annexBMode          bool

	// for DecodeUntilMarker()
	naluBuffer [][]byte
}

// Init initializes the decoder.
func (d *Decoder) Init() {
	d.timeDecoder = rtptimedec.New(rtpClockRate)
}

// Decode decodes NALUs from a RTP/H264 packet.
func (d *Decoder) Decode(pkt *rtp.Packet) ([][]byte, time.Duration, error) { //nolint:funlen,gocognit
	if d.PacketizationMode >= 2 {
		return nil, 0, ErrModeUnsupported
	}

	if len(pkt.Payload) < 1 {
		d.fragments = d.fragments[:0] // discard pending fragmented packets
		return nil, 0, ErrShortPayload
	}

	typ := naluType(pkt.Payload[0] & 0x1F)
	var nalus [][]byte

	switch typ {
	case naluTypeFUA:
		if len(pkt.Payload) < 2 {
			return nil, 0, ErrFUinvalidSize
		}

		start := pkt.Payload[1] >> 7
		end := (pkt.Payload[1] >> 6) & 0x01

		if start == 1 {
			d.fragments = d.fragments[:0] // discard pending fragmented packets

			if end != 0 {
				return nil, 0, ErrFUinvalidStartAndEnd
			}

			nri := (pkt.Payload[0] >> 5) & 0x03
			typ := pkt.Payload[1] & 0x1F
			d.fragmentedSize = len(pkt.Payload[1:])
			d.fragments = append(d.fragments, []byte{(nri << 5) | typ}, pkt.Payload[2:])
			d.firstPacketReceived = true

			return nil, 0, ErrMorePacketsNeeded
		}

		if len(d.fragments) == 0 {
			if !d.firstPacketReceived {
				return nil, 0, ErrNonStartingPacketAndNoPrevious
			}

			return nil, 0, ErrFUinvalidNonStarting
		}

		d.fragmentedSize += len(pkt.Payload[2:])
		if d.fragmentedSize > h264.MaxNALUSize {
			d.fragments = d.fragments[:0]
			return nil, 0, NALUToBigError{NALUsize: d.fragmentedSize}
		}

		d.fragments = append(d.fragments, pkt.Payload[2:])

		if end != 1 {
			return nil, 0, ErrMorePacketsNeeded
		}

		nalu := make([]byte, d.fragmentedSize)
		pos := 0

		for _, frag := range d.fragments {
			pos += copy(nalu[pos:], frag)
		}

		d.fragments = d.fragments[:0]
		nalus = [][]byte{nalu}

	case naluTypeSTAPA:
		d.fragments = d.fragments[:0] // discard pending fragmented packets

		payload := pkt.Payload[1:]

		for len(payload) > 0 {
			if len(payload) < 2 {
				return nil, 0, ErrSTAPinvalid
			}

			size := uint16(payload[0])<<8 | uint16(payload[1])
			payload = payload[2:]

			// avoid final padding
			if size == 0 {
				break
			}

			if int(size) > len(payload) {
				return nil, 0, ErrSTAPinvalid
			}

			nalus = append(nalus, payload[:size])
			payload = payload[size:]
		}

		if nalus == nil {
			return nil, 0, ErrSTAPnaluMissing
		}

		d.firstPacketReceived = true

	case naluTypeSTAPB, naluTypeMTAP16,
		naluTypeMTAP24, naluTypeFUB:
		d.fragments = d.fragments[:0] // discard pending fragmented packets
		d.firstPacketReceived = true
		return nil, 0, fmt.Errorf("%w (%v)", ErrTypeUnsupported, typ)

	default:
		d.fragments = d.fragments[:0] // discard pending fragmented packets
		d.firstPacketReceived = true
		nalus = [][]byte{pkt.Payload}
	}

	nalus, err := d.removeAnnexB(nalus)
	if err != nil {
		return nil, 0, err
	}

	return nalus, d.timeDecoder.Decode(pkt.Timestamp), nil
}

// DecodeUntilMarker decodes NALUs from a RTP/H264 packet and puts them in a buffer.
// When a packet has the marker flag (meaning that all the NALUs with the same PTS have
// been received), the buffer is returned.
func (d *Decoder) DecodeUntilMarker(pkt *rtp.Packet) ([][]byte, time.Duration, error) {
	nalus, pts, err := d.Decode(pkt)
	if err != nil {
		return nil, 0, err
	}

	if (len(d.naluBuffer) + len(nalus)) > h264.MaxNALUsPerGroup {
		return nil, 0, MaxNALUsError{count: len(d.naluBuffer) + len(nalus)}
	}

	d.naluBuffer = append(d.naluBuffer, nalus...)

	if !pkt.Marker {
		return nil, 0, ErrMorePacketsNeeded
	}

	ret := d.naluBuffer
	d.naluBuffer = d.naluBuffer[:0]

	return ret, pts, nil
}

func (d *Decoder) removeAnnexB(nalus [][]byte) ([][]byte, error) {
	// some cameras / servers wrap NALUs into Annex-B
	if !d.firstNALUParsed {
		d.firstNALUParsed = true

		if len(nalus) != 1 {
			return nalus, nil
		}

		nalu := nalus[0]

		i := bytes.Index(nalu, []byte{0x00, 0x00, 0x00, 0x01})
		if i >= 0 {
			d.annexBMode = true

			if !bytes.HasPrefix(nalu, []byte{0x00, 0x00, 0x00, 0x01}) {
				nalu = append([]byte{0x00, 0x00, 0x00, 0x01}, nalu...)
			}

			return h264.AnnexBUnmarshal(nalu)
		}
	} else if d.annexBMode {
		if len(nalus) != 1 {
			return nil, ErrMultipleAnnexBNalus
		}

		nalu := nalus[0]

		if !bytes.HasPrefix(nalu, []byte{0x00, 0x00, 0x00, 0x01}) {
			nalu = append([]byte{0x00, 0x00, 0x00, 0x01}, nalu...)
		}

		return h264.AnnexBUnmarshal(nalu)
	}

	return nalus, nil
}
