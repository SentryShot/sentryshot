package gortsplib

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	psdp "github.com/pion/sdp/v3"
)

// H264 errors.
var (
	ErrH264SPSinvalid   = errors.New("invalid SPS")
	ErrH264fmtpMissing  = errors.New("fmtp attribute is missing")
	ErrH264fmtpInvalid  = errors.New("invalid fmtp attribute")
	ErrH264spropInvalid = errors.New("invalid sprop-parameter-sets")
	ErrH264spropMissing = errors.New("sprop-parameter-sets is missing")
)

func trackH264GetSPSPPS(md *psdp.MediaDescription) ([]byte, []byte, error) {
	v, ok := md.Attribute("fmtp")
	if !ok {
		return nil, nil, ErrH264fmtpMissing
	}

	tmp := strings.SplitN(v, " ", 2)
	if len(tmp) != 2 {
		return nil, nil, fmt.Errorf("%w (%v)", ErrH264fmtpInvalid, v)
	}

	for _, kv := range strings.Split(tmp[1], ";") {
		kv = strings.Trim(kv, " ")

		if len(kv) == 0 {
			continue
		}

		tmp := strings.SplitN(kv, "=", 2)
		if len(tmp) != 2 {
			return nil, nil, fmt.Errorf("%w (%v)", ErrH264fmtpInvalid, v)
		}

		if tmp[0] == "sprop-parameter-sets" {
			tmp := strings.SplitN(tmp[1], ",", 3)
			if len(tmp) < 2 {
				return nil, nil, fmt.Errorf("%w (%v)", ErrH264spropInvalid, v)
			}

			sps, err := base64.StdEncoding.DecodeString(tmp[0])
			if err != nil {
				return nil, nil, fmt.Errorf("%w (%v)", ErrH264spropInvalid, v)
			}

			pps, err := base64.StdEncoding.DecodeString(tmp[1])
			if err != nil {
				return nil, nil, fmt.Errorf("%w (%v)", ErrH264spropInvalid, v)
			}

			return sps, pps, nil
		}
	}

	return nil, nil, fmt.Errorf("%w (%v)", ErrH264spropMissing, v)
}

// TrackH264 is a H264 track.
type TrackH264 struct {
	trackBase
	payloadType uint8
	sps         []byte
	pps         []byte
	extradata   []byte

	mu sync.RWMutex
}

// NewTrackH264 allocates a TrackH264.
func NewTrackH264(payloadType uint8, sps []byte, pps []byte, extradata []byte) (*TrackH264, error) {
	return &TrackH264{
		payloadType: payloadType,
		sps:         sps,
		pps:         pps,
		extradata:   extradata,
	}, nil
}

func newTrackH264FromMediaDescription(
	control string,
	payloadType uint8,
	md *psdp.MediaDescription,
) *TrackH264 {
	t := &TrackH264{
		trackBase: trackBase{
			control: control,
		},
		payloadType: payloadType,
	}

	sps, pps, err := trackH264GetSPSPPS(md)
	if err == nil {
		t.sps = sps
		t.pps = pps
	}

	return t
}

// ClockRate returns the track clock rate.
func (t *TrackH264) ClockRate() int {
	return 90000
}

func (t *TrackH264) clone() Track {
	return &TrackH264{
		trackBase:   t.trackBase,
		payloadType: t.payloadType,
		sps:         t.sps,
		pps:         t.pps,
		extradata:   t.extradata,
	}
}

// SPS returns the track SPS.
func (t *TrackH264) SPS() []byte {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.sps
}

// PPS returns the track PPS.
func (t *TrackH264) PPS() []byte {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.pps
}

// ExtraData returns the track extra data.
func (t *TrackH264) ExtraData() []byte {
	return t.extradata
}

// SetSPS sets the track SPS.
func (t *TrackH264) SetSPS(v []byte) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	t.sps = v
}

// SetPPS sets the track PPS.
func (t *TrackH264) SetPPS(v []byte) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	t.pps = v
}

// MediaDescription returns the track media description in SDP format.
func (t *TrackH264) MediaDescription() *psdp.MediaDescription {
	typ := strconv.FormatInt(int64(t.payloadType), 10)

	fmtp := typ + " packetization-mode=1"

	var tmp []string
	if t.sps != nil {
		tmp = append(tmp, base64.StdEncoding.EncodeToString(t.sps))
	}
	if t.pps != nil {
		tmp = append(tmp, base64.StdEncoding.EncodeToString(t.pps))
	}
	if t.extradata != nil {
		tmp = append(tmp, base64.StdEncoding.EncodeToString(t.extradata))
	}
	fmtp += "; sprop-parameter-sets=" + strings.Join(tmp, ",")

	if len(t.sps) >= 4 {
		fmtp += "; profile-level-id=" + strings.ToUpper(hex.EncodeToString(t.sps[1:4]))
	}

	return &psdp.MediaDescription{
		MediaName: psdp.MediaName{
			Media:   "video",
			Protos:  []string{"RTP", "AVP"},
			Formats: []string{typ},
		},
		Attributes: []psdp.Attribute{
			{
				Key:   "rtpmap",
				Value: typ + " H264/90000",
			},
			{
				Key:   "fmtp",
				Value: fmtp,
			},
			{
				Key:   "control",
				Value: t.control,
			},
		},
	}
}
