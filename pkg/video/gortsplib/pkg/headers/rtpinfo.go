package headers

import (
	"errors"
	"fmt"
	"nvr/pkg/video/gortsplib/pkg/base"
	"strconv"
	"strings"
)

// RTPInfoEntry is an entry of a RTP-Info header.
type RTPInfoEntry struct {
	URL            string
	SequenceNumber *uint16
	Timestamp      *uint32
}

// RTPinfo errors.
var (
	ErrRTPinfoValueMissing   = errors.New("value not provided")
	ErrRTPinfoMultipleValues = errors.New("value provided multiple times")
	ErrRTPinfoURLmissing     = errors.New("URL is missing")
)

// RTPinfo is a RTP-Info header.
type RTPinfo []*RTPInfoEntry

// Unmarshal decodes a RTP-Info header.
func (h *RTPinfo) Unmarshal(v base.HeaderValue) error {
	if len(v) == 0 {
		return ErrRTPinfoValueMissing
	}

	if len(v) > 1 {
		return fmt.Errorf("%w (%v)", ErrRTPinfoMultipleValues, v)
	}

	for _, part := range strings.Split(v[0], ",") {
		e := &RTPInfoEntry{}

		// remove leading spaces
		part = strings.TrimLeft(part, " ")

		kvs, err := keyValParse(part, ';')
		if err != nil {
			return err
		}

		for k, v := range kvs {
			switch k {
			case "url":
				e.URL = v

			case "seq":
				vi, err := strconv.ParseUint(v, 10, 16)
				if err != nil {
					return err
				}
				vi2 := uint16(vi)
				e.SequenceNumber = &vi2

			case "rtptime":
				vi, err := strconv.ParseUint(v, 10, 32)
				if err != nil {
					return err
				}
				vi2 := uint32(vi)
				e.Timestamp = &vi2

			default:
				// ignore non-standard keys
			}
		}

		if e.URL == "" {
			return ErrRTPinfoURLmissing
		}

		*h = append(*h, e)
	}

	return nil
}

// Marshal encodes a RTP-Info header.
func (h RTPinfo) Marshal() base.HeaderValue {
	rets := make([]string, len(h))

	for i, e := range h {
		var tmp []string
		tmp = append(tmp, "url="+e.URL)

		if e.SequenceNumber != nil {
			tmp = append(tmp, "seq="+strconv.FormatUint(uint64(*e.SequenceNumber), 10))
		}

		if e.Timestamp != nil {
			tmp = append(tmp, "rtptime="+strconv.FormatUint(uint64(*e.Timestamp), 10))
		}

		rets[i] = strings.Join(tmp, ";")
	}

	return base.HeaderValue{strings.Join(rets, ",")}
}
