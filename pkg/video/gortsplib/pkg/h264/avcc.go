package h264

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// AVCC errors.
var (
	ErrAVCCInvalidLength = errors.New("invalid length")
)

// AVCCnaluSizeTooBigError .
type AVCCnaluSizeTooBigError struct {
	NALUSize int
}

func (e AVCCnaluSizeTooBigError) Error() string {
	return fmt.Sprintf("NALU size (%d) is too big (maximum is %d)", e.NALUSize, MaxNALUSize)
}

// AVCCUnmarshal decodes NALUs from the AVCC stream format.
func AVCCUnmarshal(buf []byte) ([][]byte, error) {
	bl := len(buf)
	pos := 0
	var ret [][]byte

	for {
		if (bl - pos) < 4 {
			return nil, ErrAVCCInvalidLength
		}

		le := int(binary.BigEndian.Uint32(buf[pos:]))
		pos += 4

		if (bl - pos) < le {
			return nil, ErrAVCCInvalidLength
		}

		if (bl - pos) > MaxNALUSize {
			return nil, AVCCnaluSizeTooBigError{NALUSize: bl - pos}
		}

		ret = append(ret, buf[pos:pos+le])
		pos += le

		if (bl - pos) == 0 {
			break
		}
	}

	return ret, nil
}

func avccMarshalSize(nalus [][]byte) int {
	n := 0
	for _, nalu := range nalus {
		n += 4 + len(nalu)
	}
	return n
}

// AVCCMarshal encodes NALUs into the AVCC stream format.
func AVCCMarshal(nalus [][]byte) []byte {
	buf := make([]byte, avccMarshalSize(nalus))
	pos := 0
	for _, nalu := range nalus {
		binary.BigEndian.PutUint32(buf[pos:], uint32(len(nalu)))
		pos += 4

		pos += copy(buf[pos:], nalu)
	}
	return buf
}
