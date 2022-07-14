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

// TrackH264 is a H264 track.
type TrackH264 struct {
	PayloadType uint8
	SPS         []byte
	PPS         []byte

	trackBase
	mu sync.RWMutex
}

func newTrackH264FromMediaDescription(
	control string,
	payloadType uint8,
	md *psdp.MediaDescription,
) (*TrackH264, error) { //nolint:unparam
	t := &TrackH264{
		PayloadType: payloadType,
		trackBase: trackBase{
			control: control,
		},
	}

	t.fillParamsFromMediaDescription(md) //nolint:errcheck

	return t, nil
}

func (t *TrackH264) fillParamsFromMediaDescription(md *psdp.MediaDescription) error {
	v, ok := md.Attribute("fmtp")
	if !ok {
		return ErrH264fmtpMissing
	}

	tmp := strings.SplitN(v, " ", 2)
	if len(tmp) != 2 {
		return fmt.Errorf("%w (%v)", ErrH264fmtpInvalid, v)
	}

	for _, kv := range strings.Split(tmp[1], ";") {
		kv = strings.Trim(kv, " ")

		if len(kv) == 0 {
			continue
		}

		tmp := strings.SplitN(kv, "=", 2)
		if len(tmp) != 2 {
			return fmt.Errorf("%w (%v)", ErrH264fmtpInvalid, v)
		}

		if tmp[0] != "sprop-parameter-sets" {
			continue
		}

		tmp = strings.SplitN(tmp[1], ",", 3)
		if len(tmp) < 2 {
			return fmt.Errorf("%w (%v)", ErrH264spropInvalid, v)
		}

		sps, err := base64.StdEncoding.DecodeString(tmp[0])
		if err != nil {
			return fmt.Errorf("%w (%v)", ErrH264spropInvalid, v)
		}

		pps, err := base64.StdEncoding.DecodeString(tmp[1])
		if err != nil {
			return fmt.Errorf("%w (%v)", ErrH264spropInvalid, v)
		}

		t.SPS = sps
		t.PPS = pps
		return nil
	}

	return fmt.Errorf("%w (%v)", ErrH264spropMissing, v)
}

// ClockRate returns the track clock rate.
func (t *TrackH264) ClockRate() int {
	return 90000
}

func (t *TrackH264) clone() Track {
	return &TrackH264{
		PayloadType: t.PayloadType,
		SPS:         t.SPS,
		PPS:         t.PPS,
		trackBase:   t.trackBase,
	}
}

// SafeSPS returns the track SafeSPS.
func (t *TrackH264) SafeSPS() []byte {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.SPS
}

// SafePPS returns the track SafePPS.
func (t *TrackH264) SafePPS() []byte {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.PPS
}

// SafeSetSPS sets the track SPS.
func (t *TrackH264) SafeSetSPS(v []byte) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	t.SPS = v
}

// SafeSetPPS sets the track PPS.
func (t *TrackH264) SafeSetPPS(v []byte) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	t.PPS = v
}

// MediaDescription returns the track media description in SDP format.
func (t *TrackH264) MediaDescription() *psdp.MediaDescription {
	typ := strconv.FormatInt(int64(t.PayloadType), 10)

	fmtp := typ + " packetization-mode=1"

	var tmp []string
	if t.SPS != nil {
		tmp = append(tmp, base64.StdEncoding.EncodeToString(t.SPS))
	}
	if t.PPS != nil {
		tmp = append(tmp, base64.StdEncoding.EncodeToString(t.PPS))
	}
	fmtp += "; sprop-parameter-sets=" + strings.Join(tmp, ",")

	if len(t.SPS) >= 4 {
		fmtp += "; profile-level-id=" + strings.ToUpper(hex.EncodeToString(t.SPS[1:4]))
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
