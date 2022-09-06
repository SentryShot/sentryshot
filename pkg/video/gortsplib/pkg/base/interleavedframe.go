package base

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	// InterleavedFrameMagicByte is the first byte of an interleaved frame.
	InterleavedFrameMagicByte = 0x24
)

// InterleavedFrame is an interleaved frame, and allows to transfer binary data
// within RTSP/TCP connections. It is used to send and receive RTP packets with TCP.
type InterleavedFrame struct {
	// Channel ID.
	Channel int
	Payload []byte
}

// ErrInvalidMagicByte invalid magic byte.
var ErrInvalidMagicByte = errors.New("invalid magic byte")

// PayloadToBigError .
/*type PayloadToBigError struct {
	PayloadLen     int
	MaxPayloadSize int
}

func (e PayloadToBigError) Error() string {
	return fmt.Sprintf("payload size (%d) greater than maximum allowed (%d)",
		e.PayloadLen, e.MaxPayloadSize)
}*/

// Read decodes an interleaved frame.
func (f *InterleavedFrame) Read(br *bufio.Reader) error {
	var header [4]byte
	_, err := io.ReadFull(br, header[:])
	if err != nil {
		return err
	}

	if header[0] != InterleavedFrameMagicByte {
		return fmt.Errorf("%w (0x%.2x)", ErrInvalidMagicByte, header[0])
	}

	payloadLen := int(binary.BigEndian.Uint16(header[2:]))

	f.Channel = int(header[1])
	f.Payload = make([]byte, payloadLen)

	_, err = io.ReadFull(br, f.Payload)
	if err != nil {
		return err
	}
	return nil
}

// MarshalSize returns the size of an InterleavedFrame.
func (f InterleavedFrame) MarshalSize() int {
	return 4 + len(f.Payload)
}

// MarshalTo writes an InterleavedFrame.
func (f InterleavedFrame) MarshalTo(buf []byte) (int, error) {
	pos := 0

	pos += copy(buf[pos:], []byte{0x24, byte(f.Channel)})

	binary.BigEndian.PutUint16(buf[pos:], uint16(len(f.Payload)))
	pos += 2

	pos += copy(buf[pos:], f.Payload)

	return pos, nil
}

// Marshal writes an InterleavedFrame.
func (f InterleavedFrame) Marshal() ([]byte, error) {
	buf := make([]byte, f.MarshalSize())
	_, err := f.MarshalTo(buf)
	return buf, err
}
