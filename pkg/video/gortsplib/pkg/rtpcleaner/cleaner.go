// Package rtpcleaner contains a cleaning utility.
package rtpcleaner

import (
	"errors"
	"fmt"
	"nvr/pkg/video/gortsplib/pkg/h264"
	"nvr/pkg/video/gortsplib/pkg/rtph264"
	"time"

	"github.com/pion/rtp"
)

const (
	// 1500 (UDP MTU) - 20 (IP header) - 8 (UDP header).
	maxPacketSize = 1472
)

// Output is the output of Clear().
type Output struct {
	Packet       *rtp.Packet
	PTSEqualsDTS bool
	H264NALUs    [][]byte
	H264PTS      time.Duration
}

// Cleaner is used to clean incoming RTP packets, in order to:
// - remove padding
// - re-encode them if they are bigger than maximum allowed.
type Cleaner struct {
	isH264 bool

	h264Decoder *rtph264.Decoder
	h264Encoder *rtph264.Encoder
}

// New allocates a Cleaner.
func New(isH264 bool) *Cleaner {
	p := &Cleaner{isH264: isH264}

	if isH264 {
		p.h264Decoder = &rtph264.Decoder{}
		p.h264Decoder.Init()
	}

	return p
}

func (p *Cleaner) processH264(pkt *rtp.Packet) ([]*Output, error) { //nolint:funlen
	// check if we need to re-encode.
	if p.h264Encoder == nil && pkt.MarshalSize() > maxPacketSize {
		v1 := pkt.SSRC
		v2 := pkt.SequenceNumber
		v3 := pkt.Timestamp
		p.h264Encoder = &rtph264.Encoder{
			PayloadType:           pkt.PayloadType,
			SSRC:                  &v1,
			InitialSequenceNumber: &v2,
			InitialTimestamp:      &v3,
		}
		p.h264Encoder.Init()
	}

	// decode
	nalus, pts, err := p.h264Decoder.DecodeUntilMarker(pkt)
	if err != nil {
		// ignore decode errors, except for the case in which the
		// encoder is active
		if p.h264Encoder == nil {
			return []*Output{{
				Packet:       pkt,
				PTSEqualsDTS: false,
			}}, nil
		}

		if errors.Is(err, rtph264.ErrNonStartingPacketAndNoPrevious) ||
			errors.Is(err, rtph264.ErrMorePacketsNeeded) {
			return nil, nil
		}

		return nil, err
	}

	ptsEqualsDTS := h264.IDRPresent(nalus)

	// re-encode
	if p.h264Encoder != nil {
		packets, err := p.h264Encoder.Encode(nalus, pts)
		if err != nil {
			return nil, err
		}

		output := make([]*Output, len(packets))

		for i, pkt := range packets {
			if i != len(packets)-1 {
				output[i] = &Output{
					Packet:       pkt,
					PTSEqualsDTS: false,
				}
			} else {
				output[i] = &Output{
					Packet:       pkt,
					PTSEqualsDTS: ptsEqualsDTS,
					H264NALUs:    nalus,
					H264PTS:      pts,
				}
			}
		}

		return output, nil
	}

	return []*Output{{
		Packet:       pkt,
		PTSEqualsDTS: ptsEqualsDTS,
		H264NALUs:    nalus,
		H264PTS:      pts,
	}}, nil
}

// PayloadTooBigError .
type PayloadTooBigError struct {
	PayloadLen int
}

func (e PayloadTooBigError) Error() string {
	return fmt.Sprintf("payload size (%d) greater than maximum allowed (%d)",
		e.PayloadLen, maxPacketSize)
}

// Process processes a RTP packet.
func (p *Cleaner) Process(pkt *rtp.Packet) ([]*Output, error) {
	// remove padding
	pkt.Header.Padding = false
	pkt.PaddingSize = 0

	if p.h264Decoder != nil {
		return p.processH264(pkt)
	}

	if pkt.MarshalSize() > maxPacketSize {
		return nil, PayloadTooBigError{PayloadLen: pkt.MarshalSize()}
	}

	return []*Output{{
		Packet:       pkt,
		PTSEqualsDTS: true,
	}}, nil
}
