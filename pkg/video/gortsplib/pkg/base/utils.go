package base

import (
	"bufio"
	"errors"
	"fmt"
)

// ErrByteNotNewLine byte is not '\n'.
var ErrByteNotNewLine = errors.New(`byte is not '\n'`)

func readNewLine(rb *bufio.Reader) error {
	byt, err := rb.ReadByte()
	if err != nil {
		return err
	}

	cmp := byte('\n')
	if byt != cmp {
		return ErrByteNotNewLine
	}

	return nil
}

// ErrBufLenToBig buffer length exceeds.
var ErrBufLenToBig = errors.New("buffer length exceeds")

func readBytesLimited(rb *bufio.Reader, delim byte, n int) ([]byte, error) {
	for i := 1; i <= n; i++ {
		byts, err := rb.Peek(i)
		if err != nil {
			return nil, err
		}

		if byts[len(byts)-1] == delim {
			if _, err := rb.Discard(len(byts)); err != nil {
				return nil, err
			}
			return byts, nil
		}
	}
	return nil, fmt.Errorf("%w: %d", ErrBufLenToBig, n)
}
