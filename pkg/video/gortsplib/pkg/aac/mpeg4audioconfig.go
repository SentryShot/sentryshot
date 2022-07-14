package aac

import (
	"errors"
	"fmt"
	"nvr/pkg/video/gortsplib/pkg/bits"
)

// Errors.
var (
	ErrConfigDecodeTypeUnsupported    = errors.New("unsupported type")
	ErrConfigDecodeSampleRateInvalid  = errors.New("invalid sample rate index")
	ErrConfigDecodeChannelUnsupported = errors.New("not yet supported")
	ErrConfigDecodeChannelInvalid     = errors.New("invalid channel configuration")
	ErrConfigDecodeUnsupported        = errors.New("unsupported")
)

// MPEG4AudioConfig is a MPEG-4 Audio configuration.
type MPEG4AudioConfig struct {
	Type         MPEG4AudioType
	SampleRate   int
	ChannelCount int

	// AAC-LC specific
	FrameLengthFlag    bool
	DependsOnCoreCoder bool
	CoreCoderDelay     uint16
}

// Unmarshal decodes an MPEG4AudioConfig.
func (c *MPEG4AudioConfig) Unmarshal(buf []byte) error { //nolint:funlen
	// ref: ISO 14496-3

	pos := 0

	tmp, err := bits.ReadBits(buf, &pos, 5)
	if err != nil {
		return err
	}
	c.Type = MPEG4AudioType(tmp)

	switch c.Type {
	case MPEG4AudioTypeAACLC:
	default:
		return fmt.Errorf("%w: %d", ErrConfigDecodeTypeUnsupported, c.Type)
	}

	sampleRateIndex, err := bits.ReadBits(buf, &pos, 4)
	if err != nil {
		return err
	}

	switch {
	case sampleRateIndex <= 12:
		c.SampleRate = sampleRates[sampleRateIndex]

	case sampleRateIndex == 15:
		tmp, err := bits.ReadBits(buf, &pos, 24)
		if err != nil {
			return err
		}
		c.SampleRate = int(tmp)

	default:
		return fmt.Errorf("%w (%d)", ErrConfigDecodeSampleRateInvalid, sampleRateIndex)
	}

	channelConfig, err := bits.ReadBits(buf, &pos, 4)
	if err != nil {
		return err
	}

	switch {
	case channelConfig == 0:
		return ErrConfigDecodeChannelUnsupported

	case channelConfig >= 1 && channelConfig <= 6:
		c.ChannelCount = int(channelConfig)

	case channelConfig == 7:
		c.ChannelCount = 8

	default:
		return fmt.Errorf("%w (%d)", ErrConfigDecodeChannelInvalid, channelConfig)
	}

	tmp, err = bits.ReadBits(buf, &pos, 1)
	if err != nil {
		return err
	}
	c.FrameLengthFlag = (tmp == 1)

	c.DependsOnCoreCoder, err = bits.ReadFlag(buf, &pos)
	if err != nil {
		return err
	}

	if c.DependsOnCoreCoder {
		tmp, err := bits.ReadBits(buf, &pos, 14)
		if err != nil {
			return err
		}
		c.CoreCoderDelay = uint16(tmp)
	}

	extensionFlag, err := bits.ReadFlag(buf, &pos)
	if err != nil {
		return err
	}

	if extensionFlag {
		return ErrConfigDecodeUnsupported
	}

	return nil
}

func (c MPEG4AudioConfig) marshalSize() int {
	n := 5 + 4 + 3

	_, ok := reverseSampleRates[c.SampleRate]
	if !ok {
		n += 28
	} else {
		n += 4
	}

	if c.DependsOnCoreCoder {
		n += 14
	}

	ret := n / 8
	if (n % 8) != 0 {
		ret++
	}

	return ret
}

// ErrConfigEncodeChannelCountInvalid .
var ErrConfigEncodeChannelCountInvalid = errors.New("invalid channel count")

// Marshal encodes an MPEG4AudioConfig.
func (c MPEG4AudioConfig) Marshal() ([]byte, error) {
	buf := make([]byte, c.marshalSize())
	pos := 0

	bits.WriteBits(buf, &pos, uint64(c.Type), 5)

	sampleRateIndex, ok := reverseSampleRates[c.SampleRate]
	if !ok {
		bits.WriteBits(buf, &pos, uint64(15), 4)
		bits.WriteBits(buf, &pos, uint64(c.SampleRate), 24)
	} else {
		bits.WriteBits(buf, &pos, uint64(sampleRateIndex), 4)
	}

	var channelConfig int
	switch {
	case c.ChannelCount >= 1 && c.ChannelCount <= 6:
		channelConfig = c.ChannelCount

	case c.ChannelCount == 8:
		channelConfig = 7

	default:
		return nil, fmt.Errorf("%w (%d)",
			ErrConfigEncodeChannelCountInvalid, c.ChannelCount)
	}

	bits.WriteBits(buf, &pos, uint64(channelConfig), 4)

	if c.FrameLengthFlag {
		bits.WriteBits(buf, &pos, 1, 1)
	} else {
		bits.WriteBits(buf, &pos, 0, 1)
	}

	if c.DependsOnCoreCoder {
		bits.WriteBits(buf, &pos, 1, 1)
	} else {
		bits.WriteBits(buf, &pos, 0, 1)
	}

	if c.DependsOnCoreCoder {
		bits.WriteBits(buf, &pos, uint64(c.CoreCoderDelay), 14)
	}

	return buf, nil
}
