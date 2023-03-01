package customformat

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/mpeg4audio"
)

// Header meta file header.
type Header struct {
	VideoSPS    []byte
	VideoPPS    []byte
	AudioConfig []byte
	StartTime   int64 // UnixNano.
}

// Size marshaled size.
func (h *Header) Size() int {
	return 15 + len(h.VideoSPS) + len(h.VideoPPS) + len(h.AudioConfig)
}

// Marshal header.
func (h Header) Marshal() []byte {
	out := make([]byte, h.Size())
	pos := 0

	const version = 0
	out[pos] = version
	pos++

	// Video sps.
	marshalArray(out, &pos, h.VideoSPS)

	// Video pps.
	marshalArray(out, &pos, h.VideoPPS)

	// Audio config.
	marshalArray(out, &pos, h.AudioConfig)

	// Start time.
	binary.BigEndian.PutUint64(out[pos:pos+8], uint64(h.StartTime))
	pos += 8

	return out
}

func marshalArray(out []byte, pos *int, value []byte) {
	size := len(value)
	binary.BigEndian.PutUint16(out[*pos:*pos+2], uint16(size))
	*pos += 2

	copy(out[*pos:*pos+size], value)
	*pos += size
}

// ErrUnsupportedVersion unsupported version.
var ErrUnsupportedVersion = errors.New("unsupported version")

// Unmarshal header from reader.
func (h *Header) Unmarshal(r io.Reader) (int, error) {
	read := 0

	version := make([]byte, 1)
	n, err := io.ReadFull(r, version)
	if err != nil {
		return 0, err
	}
	if version[0] != 0 {
		return 0, fmt.Errorf("%w: %d", ErrUnsupportedVersion, version[0])
	}
	read += n

	// Video sps.
	n, err = unmarshalArray(r, &h.VideoSPS)
	if err != nil {
		return 0, err
	}
	read += n

	// Video pps.
	n, err = unmarshalArray(r, &h.VideoPPS)
	if err != nil {
		return 0, err
	}
	read += n

	// Audio config.
	n, err = unmarshalArray(r, &h.AudioConfig)
	if err != nil {
		return 0, err
	}
	read += n

	// Start time.
	startTime := make([]byte, 8)
	n, err = io.ReadFull(r, startTime)
	if err != nil {
		return 0, err
	}
	h.StartTime = int64(binary.BigEndian.Uint64(startTime))
	read += n

	return read, nil
}

func unmarshalArray(r io.Reader, value *[]byte) (int, error) {
	read := 0

	sizeBuf := make([]byte, 2)
	n, err := io.ReadFull(r, sizeBuf)
	if err != nil {
		return 0, err
	}
	size := binary.BigEndian.Uint16(sizeBuf)
	read += n

	*value = make([]byte, size)
	n, err = io.ReadFull(r, *value)
	if err != nil {
		return 0, err
	}
	read += n

	return read, nil
}

// GetTracks from header.
func (h Header) GetTracks() (
	*gortsplib.TrackH264,
	*gortsplib.TrackMPEG4Audio,
	error,
) {
	videoTrack := &gortsplib.TrackH264{SPS: h.VideoSPS, PPS: h.VideoPPS}

	var audioTrack *gortsplib.TrackMPEG4Audio

	audioTrackExist := len(h.AudioConfig) != 0
	if audioTrackExist {
		var config mpeg4audio.Config
		err := config.Unmarshal(h.AudioConfig)
		if err != nil {
			return nil, nil, fmt.Errorf("unmarshal audio config: %w", err)
		}
		audioTrack = &gortsplib.TrackMPEG4Audio{Config: &config}
	}

	return videoTrack, audioTrack, nil
}
