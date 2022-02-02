package aac

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/icza/bitio"
)

// Errors.
var (
	ErrConfigDecodeTypeUnsupported    = errors.New("unsupported type")
	ErrConfigDecodeSampleRateInvalid  = errors.New("invalid sample rate index")
	ErrConfigDecodeChannelUnsupported = errors.New("not yet supported")
	ErrConfigDecodeChannelInvalid     = errors.New("invalid channel configuration")
)

// MPEG4AudioConfig is a MPEG-4 Audio configuration.
type MPEG4AudioConfig struct {
	Type              MPEG4AudioType
	SampleRate        int
	ChannelCount      int
	AOTSpecificConfig []byte
}

// Decode decodes an MPEG4AudioConfig.
func (c *MPEG4AudioConfig) Decode(byts []byte) error { //nolint:funlen
	// ref: https://wiki.multimedia.cx/index.php/MPEG-4_Audio

	r := bitio.NewReader(bytes.NewBuffer(byts))

	tmp, err := r.ReadBits(5)
	if err != nil {
		return err
	}
	c.Type = MPEG4AudioType(tmp)

	switch c.Type {
	case MPEG4AudioTypeAACLC:
	default:
		return fmt.Errorf("%w: %d", ErrConfigDecodeTypeUnsupported, c.Type)
	}

	sampleRateIndex, err := r.ReadBits(4)
	if err != nil {
		return err
	}

	switch {
	case sampleRateIndex <= 12:
		c.SampleRate = sampleRates[sampleRateIndex]

	case sampleRateIndex == 15:
		tmp, err := r.ReadBits(24)
		if err != nil {
			return err
		}
		c.SampleRate = int(tmp)

	default:
		return fmt.Errorf("%w (%d)", ErrConfigDecodeSampleRateInvalid, sampleRateIndex)
	}

	channelConfig, err := r.ReadBits(4)
	if err != nil {
		return err
	}

	switch {
	case channelConfig == 0:
		return ErrConfigDecodeChannelUnsupported

	case channelConfig >= 1 && channelConfig <= 7:
		c.ChannelCount = channelCounts[channelConfig-1]

	default:
		return fmt.Errorf("%w (%d)", ErrConfigDecodeChannelInvalid, channelConfig)
	}

	for {
		byt, err := r.ReadBits(8)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		c.AOTSpecificConfig = append(c.AOTSpecificConfig, uint8(byt))
	}

	return nil
}

// ErrConfigEncodeChannelCountInvalid .
var ErrConfigEncodeChannelCountInvalid = errors.New("invalid channel count")

// Encode encodes an MPEG4AudioConfig.
func (c MPEG4AudioConfig) Encode() ([]byte, error) {
	var buf bytes.Buffer
	w := bitio.NewWriter(&buf)

	if err := w.WriteBits(uint64(c.Type), 5); err != nil {
		return nil, err
	}

	sampleRateIndex := func() int {
		for i, s := range sampleRates {
			if s == c.SampleRate {
				return i
			}
		}
		return -1
	}()

	if sampleRateIndex != -1 {
		if err := w.WriteBits(uint64(sampleRateIndex), 4); err != nil {
			return nil, err
		}
	} else {
		if err := w.WriteBits(uint64(15), 4); err != nil {
			return nil, err
		}
		if err := w.WriteBits(uint64(c.SampleRate), 24); err != nil {
			return nil, err
		}
	}

	channelConfig := func() int {
		for i, co := range channelCounts {
			if co == c.ChannelCount {
				return i + 1
			}
		}
		return -1
	}()

	if channelConfig == -1 {
		return nil, fmt.Errorf("%w (%d)",
			ErrConfigEncodeChannelCountInvalid, c.ChannelCount)
	}

	if err := w.WriteBits(uint64(channelConfig), 4); err != nil {
		return nil, err
	}

	for _, b := range c.AOTSpecificConfig {
		if err := w.WriteBits(uint64(b), 8); err != nil {
			return nil, err
		}
	}

	w.Close()

	return buf.Bytes(), nil
}
