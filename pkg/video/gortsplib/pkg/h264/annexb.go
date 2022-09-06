package h264

import (
	"bytes"
	"errors"
	"fmt"
)

const (
	// MaxNALUSize is the maximum size of a NALU.
	// with a 250 Mbps H264 video, the maximum NALU size is 2.2MB.
	MaxNALUSize = 3 * 1024 * 1024

	// MaxNALUsPerGroup is the maximum number of NALUs per group.
	MaxNALUsPerGroup = 20
)

// AnnexB errors.
var (
	ErrAnnexBUnexpectedByte          = errors.New("unexpected byte")
	ErrAnnexBMissingInitialDelimiter = errors.New("initial delimiter not found")
	ErrAnnexBEmptyNalu               = errors.New("empty NALU")
)

// NaluSizeTooBigError .
type NaluSizeTooBigError struct {
	size int
}

// Error .
func (e NaluSizeTooBigError) Error() string {
	return fmt.Sprintf("NALU size (%d) is too big (maximum is %d)", e.size, MaxNALUSize)
}

// AnnexBUnmarshal decodes NALUs from the Annex-B stream format.
func AnnexBUnmarshal(byts []byte) ([][]byte, error) { //nolint:funlen
	bl := len(byts)
	zeroCount := 0

outer:
	for i := 0; i < bl; i++ {
		switch byts[i] {
		case 0:
			zeroCount++

		case 1:
			break outer

		default:
			return nil, fmt.Errorf("%w: %d", ErrAnnexBUnexpectedByte, byts[i])
		}
	}
	if zeroCount != 2 && zeroCount != 3 {
		return nil, ErrAnnexBMissingInitialDelimiter
	}

	start := zeroCount + 1

	var n int
	if zeroCount == 2 {
		n = bytes.Count(byts[start:], []byte{0x00, 0x00, 0x01})
	} else {
		n = bytes.Count(byts[start:], []byte{0x00, 0x00, 0x00, 0x01})
	}

	ret := make([][]byte, n+1)
	pos := 0

	curZeroCount := 0
	delimStart := 0

	for i := start; i < bl; i++ {
		switch byts[i] {
		case 0:
			if curZeroCount == 0 {
				delimStart = i
			}
			curZeroCount++

		case 1:
			if curZeroCount == zeroCount {
				if (delimStart - start) > MaxNALUSize {
					return nil, NaluSizeTooBigError{size: delimStart - start}
				}

				nalu := byts[start:delimStart]
				if len(nalu) == 0 {
					return nil, ErrAnnexBEmptyNalu
				}

				ret[pos] = nalu
				pos++
				start = i + 1
			}
			curZeroCount = 0

		default:
			curZeroCount = 0
		}
	}

	if (bl - start) > MaxNALUSize {
		return nil, NaluSizeTooBigError{size: bl - start}
	}

	nalu := byts[start:bl]
	if len(nalu) == 0 {
		return nil, ErrAnnexBEmptyNalu
	}
	ret[pos] = nalu

	return ret, nil
}

func annexBMarshalSize(nalus [][]byte) int {
	n := 0
	for _, nalu := range nalus {
		n += 4 + len(nalu)
	}
	return n
}

// AnnexBMarshal encodes NALUs into the Annex-B stream format.
func AnnexBMarshal(nalus [][]byte) ([]byte, error) {
	buf := make([]byte, annexBMarshalSize(nalus))
	pos := 0

	for _, nalu := range nalus {
		pos += copy(buf[pos:], []byte{0x00, 0x00, 0x00, 0x01})
		pos += copy(buf[pos:], nalu)
	}

	return buf, nil
}
