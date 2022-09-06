package mpeg4audio

import (
	"errors"
	"fmt"
	"nvr/pkg/video/gortsplib/pkg/bits"
)

// Errors.
var (
	ErrConfigDecodeTypeUnsupported    = errors.New("unsupported object type")
	ErrConfigDecodeSampleRateInvalid  = errors.New("invalid sample rate index")
	ErrConfigDecodeChannelUnsupported = errors.New("not yet supported")
	ErrConfigDecodeChannelInvalid     = errors.New("invalid channel configuration")
	ErrConfigDecodeUnsupported        = errors.New("unsupported")
	ErrConfigDecodeIndexInvalid       = errors.New("invalid extension sample rate index")

	ErrConfigEncodeChannelCountInvalid = errors.New("invalid channel count")
)

// Config is a MPEG-4 Audio configuration.
type Config struct {
	Type         ObjectType
	SampleRate   int
	ChannelCount int

	// AAC-LC specific
	FrameLengthFlag    bool
	DependsOnCoreCoder bool
	CoreCoderDelay     uint16

	// SBR specific
	ExtensionSampleRate int
}

// Unmarshal decodes a Config.
func (c *Config) Unmarshal(buf []byte) error {
	// ref: ISO 14496-3

	pos := 0

	tmp, err := bits.ReadBits(buf, &pos, 5)
	if err != nil {
		return err
	}
	c.Type = ObjectType(tmp)

	switch c.Type {
	case ObjectTypeAACLC:
	case ObjectTypeSBR:
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

	case sampleRateIndex == 0x0F:
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

	if c.Type == ObjectTypeSBR {
		return c.unmarshalSBR(buf, &pos)
	}
	return c.unmarshalAACLC(buf, &pos)
}

func (c *Config) unmarshalSBR(buf []byte, pos *int) error {
	extensionSamplingFrequencyIndex, err := bits.ReadBits(buf, pos, 4)
	if err != nil {
		return err
	}

	switch {
	case extensionSamplingFrequencyIndex <= 12:
		c.ExtensionSampleRate = sampleRates[extensionSamplingFrequencyIndex]

	case extensionSamplingFrequencyIndex == 0x0F:
		tmp, err := bits.ReadBits(buf, pos, 24)
		if err != nil {
			return err
		}
		c.ExtensionSampleRate = int(tmp)

	default:
		return fmt.Errorf("%w (%d)",
			ErrConfigDecodeIndexInvalid, extensionSamplingFrequencyIndex)
	}
	return nil
}

func (c *Config) unmarshalAACLC(buf []byte, pos *int) error {
	var err error
	c.FrameLengthFlag, err = bits.ReadFlag(buf, pos)
	if err != nil {
		return err
	}

	c.DependsOnCoreCoder, err = bits.ReadFlag(buf, pos)
	if err != nil {
		return err
	}

	if c.DependsOnCoreCoder {
		tmp, err := bits.ReadBits(buf, pos, 14)
		if err != nil {
			return err
		}
		c.CoreCoderDelay = uint16(tmp)
	}

	extensionFlag, err := bits.ReadFlag(buf, pos)
	if err != nil {
		return err
	}

	if extensionFlag {
		return ErrConfigDecodeUnsupported
	}
	return nil
}

func (c Config) marshalSize() int {
	n := 5 + 4 + 3

	_, ok := reverseSampleRates[c.SampleRate]
	if !ok {
		n += 28
	} else {
		n += 4
	}

	if c.Type == ObjectTypeSBR {
		_, ok := reverseSampleRates[c.ExtensionSampleRate]
		if !ok {
			n += 28
		} else {
			n += 4
		}
	} else if c.DependsOnCoreCoder {
		n += 14
	}

	ret := n / 8
	if (n % 8) != 0 {
		ret++
	}

	return ret
}

// Marshal encodes a Config.
func (c Config) Marshal() ([]byte, error) {
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

	if c.Type == ObjectTypeSBR { //nolint:nestif
		sampleRateIndex, ok := reverseSampleRates[c.ExtensionSampleRate]
		if !ok {
			bits.WriteBits(buf, &pos, uint64(0x0F), 4)
			bits.WriteBits(buf, &pos, uint64(c.ExtensionSampleRate), 24)
		} else {
			bits.WriteBits(buf, &pos, uint64(sampleRateIndex), 4)
		}
	} else {
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
	}

	return buf, nil
}
