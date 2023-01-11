package gortsplib

import (
	"errors"
	"nvr/pkg/video/gortsplib/pkg/url"
	"strconv"
	"strings"

	psdp "github.com/pion/sdp/v3"
)

// Track is a RTSP track.
type Track interface {
	// ClockRate returns the track clock rate.
	ClockRate() int

	// GetControl returns the track control attribute.
	GetControl() string

	// SetControl sets the track control attribute.
	SetControl(string)

	// MediaDescription returns the track media description in SDP format.
	MediaDescription() *psdp.MediaDescription
	clone() Track
	url(*url.URL) (*url.URL, error)
}

// Track errors.
var (
	ErrTrackContentBaseMissing = errors.New("Content-Base header not provided")
	ErrTrackNoFormats          = errors.New("no media formats found")
)

func getControlAttribute(attributes []psdp.Attribute) string {
	for _, attr := range attributes {
		if attr.Key == "control" {
			return attr.Value
		}
	}
	return ""
}

func getRtpmapAttribute(attributes []psdp.Attribute, payloadType uint8) string {
	for _, attr := range attributes {
		if attr.Key == "rtpmap" {
			v := strings.TrimSpace(attr.Value)
			if parts := strings.SplitN(v, " ", 2); len(parts) == 2 {
				if tmp, err := strconv.ParseInt(parts[0], 10, 8); err == nil && uint8(tmp) == payloadType {
					return parts[1]
				}
			}
		}
	}
	return ""
}

func getCodecAndClock(attributes []psdp.Attribute, payloadType uint8) (string, string) {
	rtpmap := getRtpmapAttribute(attributes, payloadType)
	if rtpmap == "" {
		return "", ""
	}

	parts2 := strings.SplitN(rtpmap, "/", 2)
	if len(parts2) != 2 {
		return "", ""
	}

	return parts2[0], parts2[1]
}

func newTrackFromMediaDescription(md *psdp.MediaDescription) (Track, error) {
	if len(md.MediaName.Formats) == 0 {
		return nil, ErrTrackNoFormats
	}

	control := getControlAttribute(md.Attributes)

	if len(md.MediaName.Formats) == 1 {
		tmp, err := strconv.ParseInt(md.MediaName.Formats[0], 10, 8)
		if err != nil {
			return nil, err
		}
		payloadType := uint8(tmp)

		codec, clock := getCodecAndClock(md.Attributes, payloadType)
		codec = strings.ToLower(codec)

		if md.MediaName.Media == "video" && codec == "h264" && clock == "90000" {
			return newTrackH264FromMediaDescription(control, payloadType, md)
		} else if md.MediaName.Media == "audio" && strings.ToLower(codec) == "mpeg4-generic" {
			return newTrackMPEG4AudioFromMediaDescription(control, payloadType, md)
		}
	}

	return nil, nil
}

type trackBase struct {
	control string
}

// GetControl gets the track control attribute.
func (t *trackBase) GetControl() string {
	return t.control
}

// SetControl sets the track control attribute.
func (t *trackBase) SetControl(c string) {
	t.control = c
}

func (t *trackBase) url(contentBase *url.URL) (*url.URL, error) {
	if contentBase == nil {
		return nil, ErrTrackContentBaseMissing
	}

	control := t.GetControl()

	// no control attribute, use base URL
	if control == "" {
		return contentBase, nil
	}

	// control attribute contains an absolute path
	if strings.HasPrefix(control, "rtsp://") {
		ur, err := url.Parse(control)
		if err != nil {
			return nil, err
		}

		// copy host and credentials
		ur.Host = contentBase.Host
		ur.User = contentBase.User
		return ur, nil
	}

	// control attribute contains a relative control attribute
	// insert the control attribute at the end of the URL
	// if there's a query, insert it after the query
	// otherwise insert it after the path
	strURL := contentBase.String()
	if control[0] != '?' && !strings.HasSuffix(strURL, "/") {
		strURL += "/"
	}

	ur, _ := url.Parse(strURL + control)
	return ur, nil
}
