package base

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// Errors.
var (
	ErrBodyContentLengthInvalid = errors.New("invalid Content-Length")
	ErrBodyContentLengthToBig   = fmt.Errorf(
		"Content-Length exceeds %d", rtspMaxContentLength)
)

type body []byte

func (b *body) read(header Header, rb *bufio.Reader) error {
	cls, ok := header["Content-Length"]
	if !ok || len(cls) != 1 {
		*b = nil
		return nil
	}

	cl, err := strconv.ParseInt(cls[0], 10, 64)
	if err != nil {
		return ErrBodyContentLengthInvalid
	}

	if cl > rtspMaxContentLength {
		return fmt.Errorf("%w (it's %d)", ErrBodyContentLengthToBig, cl)
	}

	*b = make([]byte, cl)
	n, err := io.ReadFull(rb, *b)
	if err != nil && n != len(*b) {
		return err
	}

	return nil
}

func (b body) writeSize() int {
	return len(b)
}

func (b body) writeTo(buf []byte) int {
	return copy(buf, b)
}

func (b body) write() []byte { //nolint:unused
	buf := make([]byte, b.writeSize())
	b.writeTo(buf)
	return buf
}
