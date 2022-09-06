package mpeg4audio

import (
	"errors"
	"fmt"
)

// ADTS decode errors.
var (
	ErrADTSdecodeLengthInvalid     = errors.New("invalid length")
	ErrADTSdecodeSyncwordInvalid   = errors.New("invalid syncword")
	ErrADTSdecodeCRCunsupported    = errors.New("CRC is not supported")
	ErrADTSdecodeTypeUnsupported   = errors.New("unsupported audio type")
	ErrADTSdecodeSampleRateInvalid = errors.New("invalid sample rate index")
	ErrADTSdecodeChannelInvalid    = errors.New("invalid channel configuration")

	ErrADTSdecodeMultipleFramesUnsupported = errors.New(
		"frame count greater than 1 is not supported")
	ErrADTSdecodeFrameLengthInvalid = errors.New(
		"invalid frame length")
)

// ADTSdecodeAUsizeToBigError .
type ADTSdecodeAUsizeToBigError struct {
	AUsize int
}

func (e ADTSdecodeAUsizeToBigError) Error() string {
	return fmt.Sprintf("AU size (%d) is too big (maximum is %d)", e.AUsize, MaxAccessUnitSize)
}

// ADTSPacket is an ADTS packet.
type ADTSPacket struct {
	Type         ObjectType
	SampleRate   int
	ChannelCount int
	AU           []byte
}

// ADTSPackets is a group od ADTS packets.
type ADTSPackets []*ADTSPacket

// Unmarshal decodes an ADTS stream into ADTS packets.
func (ps *ADTSPackets) Unmarshal(buf []byte) error { //nolint:funlen
	// refs: https://wiki.multimedia.cx/index.php/ADTS

	bl := len(buf)
	pos := 0

	for {
		if (bl - pos) < 8 {
			return ErrADTSdecodeLengthInvalid
		}

		syncWord := (uint16(buf[pos]) << 4) | (uint16(buf[pos+1]) >> 4)
		if syncWord != 0xfff {
			return ErrADTSdecodeSyncwordInvalid
		}

		protectionAbsent := buf[pos+1] & 0x01
		if protectionAbsent != 1 {
			return ErrADTSdecodeCRCunsupported
		}

		pkt := &ADTSPacket{}

		pkt.Type = ObjectType((buf[pos+2] >> 6) + 1)
		if pkt.Type != ObjectTypeAACLC {
			return fmt.Errorf("%w: %d", ErrADTSdecodeTypeUnsupported, pkt.Type)
		}

		sampleRateIndex := (buf[pos+2] >> 2) & 0x0F
		if sampleRateIndex <= 12 {
			pkt.SampleRate = sampleRates[sampleRateIndex]
		} else {
			return fmt.Errorf("%w: %d", ErrADTSdecodeSampleRateInvalid, sampleRateIndex)
		}

		channelConfig := ((buf[pos+2] & 0x01) << 2) | ((buf[pos+3] >> 6) & 0x03)
		switch {
		case channelConfig >= 1 && channelConfig <= 6:
			pkt.ChannelCount = int(channelConfig)

		case channelConfig == 7:
			pkt.ChannelCount = 8

		default:
			return fmt.Errorf("%w: %d", ErrADTSdecodeChannelInvalid, channelConfig)
		}

		frameLen := int(((uint16(buf[pos+3])&0x03)<<11)|
			(uint16(buf[pos+4])<<3)|
			((uint16(buf[pos+5])>>5)&0x07)) - 7
		if frameLen > MaxAccessUnitSize {
			return ADTSdecodeAUsizeToBigError{AUsize: frameLen}
		}

		frameCount := buf[pos+6] & 0x03
		if frameCount != 0 {
			return ErrADTSdecodeMultipleFramesUnsupported
		}

		if len(buf[pos+7:]) < frameLen {
			return ErrADTSdecodeFrameLengthInvalid
		}

		pkt.AU = buf[pos+7 : pos+7+frameLen]
		pos += 7 + frameLen

		*ps = append(*ps, pkt)

		if (bl - pos) == 0 {
			break
		}
	}

	return nil
}

func (ps ADTSPackets) marshalSize() int {
	n := 0
	for _, pkt := range ps {
		n += 7 + len(pkt.AU)
	}
	return n
}

// ADTS encode errors.
var (
	ErrADTSencodeSampleRateInvalid   = errors.New("invalid sample rate")
	ErrADTSencodeChannelCountInvalid = errors.New("invalid channel count")
)

// Marshal encodes ADTS packets into an ADTS stream.
func (ps ADTSPackets) Marshal() ([]byte, error) {
	buf := make([]byte, ps.marshalSize())
	pos := 0

	for _, pkt := range ps {
		sampleRateIndex, ok := reverseSampleRates[pkt.SampleRate]
		if !ok {
			return nil, fmt.Errorf("%w: %d",
				ErrADTSencodeSampleRateInvalid, pkt.SampleRate)
		}

		var channelConfig int
		switch {
		case pkt.ChannelCount >= 1 && pkt.ChannelCount <= 6:
			channelConfig = pkt.ChannelCount

		case pkt.ChannelCount == 8:
			channelConfig = 7

		default:
			return nil, fmt.Errorf("%w (%d)",
				ErrADTSencodeChannelCountInvalid, pkt.ChannelCount)
		}

		frameLen := len(pkt.AU) + 7

		fullness := 0x07FF // like ffmpeg does

		buf[pos+0] = 0xFF
		buf[pos+1] = 0xF1
		buf[pos+2] = uint8((int(pkt.Type-1) << 6) | (sampleRateIndex << 2) | ((channelConfig >> 2) & 0x01))
		buf[pos+3] = uint8((channelConfig&0x03)<<6 | (frameLen>>11)&0x03)
		buf[pos+4] = uint8((frameLen >> 3) & 0xFF)
		buf[pos+5] = uint8((frameLen&0x07)<<5 | ((fullness >> 6) & 0x1F))
		buf[pos+6] = uint8((fullness & 0x3F) << 2)
		pos += 7

		pos += copy(buf[pos:], pkt.AU)
	}

	return buf, nil
}
