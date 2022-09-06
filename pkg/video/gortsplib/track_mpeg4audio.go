package gortsplib

import (
	"encoding/hex"
	"errors"
	"fmt"
	"nvr/pkg/video/gortsplib/pkg/mpeg4audio"
	"strconv"
	"strings"

	psdp "github.com/pion/sdp/v3"
)

// TrackMPEG4Audio is a MPEG-4 audio track.
type TrackMPEG4Audio struct {
	PayloadType      uint8
	Config           *mpeg4audio.Config
	SizeLength       int
	IndexLength      int
	IndexDeltaLength int

	trackBase
}

// AAC errors.
var (
	ErrAACfmtpMissing             = errors.New("fmtp attribute is missing")
	ErrACCfmtpInvalid             = errors.New("invalid fmtp")
	ErrACCconfigInvalid           = errors.New("invalid AAC config")
	ErrACCconfigMissing           = errors.New("config is missing")
	ErrACCsizelengthInvalid       = errors.New("invalid AAC SizeLength")
	ErrACCsizelengthMissing       = errors.New("sizelength is missing")
	ErrACCindexLengthInvalid      = errors.New("invalid AAC IndexLength")
	ErrACCindexDeltaLengthInvalid = errors.New("invalid AAC IndexDeltaLength")
)

func newTrackMPEG4AudioFromMediaDescription( //nolint:funlen
	control string,
	payloadType uint8,
	md *psdp.MediaDescription,
) (*TrackMPEG4Audio, error) {
	v, ok := md.Attribute("fmtp")
	if !ok {
		return nil, ErrAACfmtpMissing
	}

	tmp := strings.SplitN(v, " ", 2)
	if len(tmp) != 2 {
		return nil, fmt.Errorf("%w (%v)", ErrACCfmtpInvalid, v)
	}

	t := &TrackMPEG4Audio{
		PayloadType: payloadType,
		trackBase: trackBase{
			control: control,
		},
	}

	for _, kv := range strings.Split(tmp[1], ";") {
		kv = strings.Trim(kv, " ")

		if len(kv) == 0 {
			continue
		}

		tmp := strings.SplitN(kv, "=", 2)
		if len(tmp) != 2 {
			return nil, fmt.Errorf("%w (%v)", ErrACCfmtpInvalid, v)
		}
		switch strings.ToLower(tmp[0]) {
		case "config":
			enc, err := hex.DecodeString(tmp[1])
			if err != nil {
				return nil, fmt.Errorf("%w (%v)", ErrACCconfigInvalid, tmp[1])
			}

			t.Config = &mpeg4audio.Config{}
			err = t.Config.Unmarshal(enc)
			if err != nil {
				return nil, fmt.Errorf("%w (%v)", ErrACCconfigInvalid, tmp[1])
			}

		case "sizelength":
			val, err := strconv.ParseUint(tmp[1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%w (%v)", ErrACCsizelengthMissing, tmp[1])
			}
			t.SizeLength = int(val)

		case "indexlength":
			val, err := strconv.ParseUint(tmp[1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%w (%v)", ErrACCindexLengthInvalid, tmp[1])
			}
			t.IndexLength = int(val)

		case "indexdeltalength":
			val, err := strconv.ParseUint(tmp[1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%w (%v)", ErrACCindexDeltaLengthInvalid, tmp[1])
			}
			t.IndexDeltaLength = int(val)
		}
	}

	if t.Config == nil {
		return nil, fmt.Errorf("%w (%v)", ErrACCconfigMissing, v)
	}

	if t.SizeLength == 0 {
		return nil, fmt.Errorf("%w (%v)", ErrACCsizelengthMissing, v)
	}

	return t, nil
}

// ClockRate returns the track clock rate.
func (t *TrackMPEG4Audio) ClockRate() int {
	return t.Config.SampleRate
}

func (t *TrackMPEG4Audio) clone() Track {
	return &TrackMPEG4Audio{
		PayloadType:      t.PayloadType,
		Config:           t.Config,
		SizeLength:       t.SizeLength,
		IndexLength:      t.IndexLength,
		IndexDeltaLength: t.IndexDeltaLength,
		trackBase:        t.trackBase,
	}
}

// MediaDescription returns the track media description in SDP format.
func (t *TrackMPEG4Audio) MediaDescription() *psdp.MediaDescription {
	enc, err := t.Config.Marshal()
	if err != nil {
		return nil
	}

	typ := strconv.FormatInt(int64(t.PayloadType), 10)

	sampleRate := t.Config.SampleRate
	if t.Config.ExtensionSampleRate != 0 {
		sampleRate = t.Config.ExtensionSampleRate
	}

	return &psdp.MediaDescription{
		MediaName: psdp.MediaName{
			Media:   "audio",
			Protos:  []string{"RTP", "AVP"},
			Formats: []string{typ},
		},
		Attributes: []psdp.Attribute{
			{
				Key: "rtpmap",
				Value: typ + " mpeg4-generic/" + strconv.FormatInt(int64(sampleRate), 10) +
					"/" + strconv.FormatInt(int64(t.Config.ChannelCount), 10),
			},
			{
				Key: "fmtp",
				Value: typ + " profile-level-id=1; " +
					"mode=AAC-hbr; " +
					func() string {
						if t.SizeLength > 0 {
							return fmt.Sprintf("sizelength=%d; ", t.SizeLength)
						}
						return ""
					}() +
					func() string {
						if t.IndexLength > 0 {
							return fmt.Sprintf("indexlength=%d; ", t.IndexLength)
						}
						return ""
					}() +
					func() string {
						if t.IndexDeltaLength > 0 {
							return fmt.Sprintf("indexdeltalength=%d; ", t.IndexDeltaLength)
						}
						return ""
					}() +
					"config=" + hex.EncodeToString(enc),
			},
			{
				Key:   "control",
				Value: t.control,
			},
		},
	}
}
