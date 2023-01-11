package h264

import (
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

// NaluGroupSizeTooBigError .
type NaluGroupSizeTooBigError struct {
	size int
}

// Error .
func (e NaluGroupSizeTooBigError) Error() string {
	return fmt.Sprintf("number of NALUs contained inside a single group (%d)"+
		" is too big (maximum is %d)", e.size, MaxNALUsPerGroup)
}

// AnnexBUnmarshal decodes NALUs from the Annex-B stream format.
func AnnexBUnmarshal(byts []byte) ([][]byte, error) { //nolint:funlen,gocognit
	bl := len(byts)
	initZeroCount := 0

outer:
	for i := 0; i < bl; i++ {
		switch byts[i] {
		case 0:
			initZeroCount++

		case 1:
			break outer

		default:
			return nil, fmt.Errorf("%w: %d", ErrAnnexBUnexpectedByte, byts[i])
		}
	}
	if initZeroCount != 2 && initZeroCount != 3 {
		return nil, ErrAnnexBMissingInitialDelimiter
	}

	start := initZeroCount + 1
	zeroCount := 0
	n := 0

	for i := start; i < bl; i++ {
		switch byts[i] {
		case 0:
			zeroCount++

		case 1:
			if zeroCount == 2 || zeroCount == 3 {
				n++
			}
			zeroCount = 0

		default:
			zeroCount = 0
		}
	}

	if (n + 1) > MaxNALUsPerGroup {
		return nil, NaluGroupSizeTooBigError{size: n + 1}
	}

	ret := make([][]byte, n+1)
	pos := 0
	start = initZeroCount + 1
	zeroCount = 0
	delimStart := 0

	for i := start; i < bl; i++ {
		switch byts[i] {
		case 0:
			if zeroCount == 0 {
				delimStart = i
			}
			zeroCount++

		case 1:
			if zeroCount == 2 || zeroCount == 3 {
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
			zeroCount = 0

		default:
			zeroCount = 0
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
