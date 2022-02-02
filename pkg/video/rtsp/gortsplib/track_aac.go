package gortsplib

import (
	"encoding/hex"
	"errors"
	"fmt"
	"nvr/pkg/video/rtsp/gortsplib/pkg/aac"
	"strconv"
	"strings"

	psdp "github.com/pion/sdp/v3"
)

// TrackConfigAAC is the configuration of an AAC track.
type TrackConfigAAC struct {
	Type              int
	SampleRate        int
	ChannelCount      int
	AOTSpecificConfig []byte
}

// NewTrackAAC initializes an AAC track.
func NewTrackAAC(payloadType uint8, conf *TrackConfigAAC) (*Track, error) {
	mpegConf, err := aac.MPEG4AudioConfig{
		Type:              aac.MPEG4AudioType(conf.Type),
		SampleRate:        conf.SampleRate,
		ChannelCount:      conf.ChannelCount,
		AOTSpecificConfig: conf.AOTSpecificConfig,
	}.Encode()
	if err != nil {
		return nil, err
	}

	typ := strconv.FormatInt(int64(payloadType), 10)

	return &Track{
		Media: &psdp.MediaDescription{
			MediaName: psdp.MediaName{
				Media:   "audio",
				Protos:  []string{"RTP", "AVP"},
				Formats: []string{typ},
			},
			Attributes: []psdp.Attribute{
				{
					Key: "rtpmap",
					Value: typ + " mpeg4-generic/" + strconv.FormatInt(int64(conf.SampleRate), 10) +
						"/" + strconv.FormatInt(int64(conf.ChannelCount), 10),
				},
				{
					Key: "fmtp",
					Value: typ + " profile-level-id=1; " +
						"mode=AAC-hbr; " +
						"sizelength=13; " +
						"indexlength=3; " +
						"indexdeltalength=3; " +
						"config=" + hex.EncodeToString(mpegConf),
				},
			},
		},
	}, nil
}

// IsAAC checks whether the track is an AAC track.
func (t *Track) IsAAC() bool {
	if t.Media.MediaName.Media != "audio" {
		return false
	}

	v, ok := t.Media.Attribute("rtpmap")
	if !ok {
		return false
	}

	vals := strings.Split(v, " ")
	if len(vals) != 2 {
		return false
	}

	return strings.HasPrefix(strings.ToLower(vals[1]), "mpeg4-generic/")
}

// Errors.
var (
	ErrAACfmtpMissing   = errors.New("fmtp attribute is missing")
	ErrAACfmtpInvalid   = errors.New("invalid fmtp")
	ErrAACconfigInvalid = errors.New("invalid AAC config")
	ErrAACconfigMissing = errors.New("config is missing")
)

// ExtractConfigAAC extracts the configuration of an AAC track.
func (t *Track) ExtractConfigAAC() (*TrackConfigAAC, error) {
	v, ok := t.Media.Attribute("fmtp")
	if !ok {
		return nil, ErrAACfmtpMissing
	}

	tmp := strings.SplitN(v, " ", 2)
	if len(tmp) != 2 {
		return nil, fmt.Errorf("%w (%v)", ErrAACfmtpInvalid, v)
	}

	for _, kv := range strings.Split(tmp[1], ";") {
		kv = strings.Trim(kv, " ")

		if len(kv) == 0 {
			continue
		}

		tmp := strings.SplitN(kv, "=", 2)
		if len(tmp) != 2 {
			return nil, fmt.Errorf("%w (%v)", ErrAACfmtpInvalid, v)
		}

		if tmp[0] == "config" {
			enc, err := hex.DecodeString(tmp[1])
			if err != nil {
				return nil, fmt.Errorf("%w (%v)", ErrAACconfigInvalid, tmp[1])
			}

			var mpegConf aac.MPEG4AudioConfig
			err = mpegConf.Decode(enc)
			if err != nil {
				return nil, fmt.Errorf("%w (%v)", ErrAACconfigInvalid, tmp[1])
			}

			conf := &TrackConfigAAC{
				Type:              int(mpegConf.Type),
				SampleRate:        mpegConf.SampleRate,
				ChannelCount:      mpegConf.ChannelCount,
				AOTSpecificConfig: mpegConf.AOTSpecificConfig,
			}

			return conf, nil
		}
	}

	return nil, fmt.Errorf("%w (%v)", ErrAACconfigMissing, v)
}
