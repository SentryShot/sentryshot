package hls

import (
	"errors"
	"io"
	"nvr/pkg/video/gortsplib"
	"time"
)

// Muxer is a HLS muxer.
type Muxer struct {
	primaryPlaylist *muxerPrimaryPlaylist
	streamPlaylist  *muxerStreamPlaylist
	tsGenerator     *muxerTSGenerator
}

// ErrTrackInvalid invalid H264 track: SPS or PPS not provided into the SDP.
var ErrTrackInvalid = errors.New("invalid H264 track: SPS or PPS not provided into the SDP")

// NewMuxer allocates a Muxer.
func NewMuxer(
	hlsSegmentCount int,
	hlsSegmentDuration time.Duration,
	hlsSegmentMaxSize uint64,
	onNewSegement chan<- Segments,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
) (*Muxer, error) {
	primaryPlaylist := newMuxerPrimaryPlaylist(videoTrack, audioTrack)

	streamPlaylist := newMuxerStreamPlaylist(hlsSegmentCount, onNewSegement)

	tsGenerator, err := newMuxerTSGenerator(
		hlsSegmentCount,
		hlsSegmentDuration,
		hlsSegmentMaxSize,
		videoTrack,
		audioTrack,
		streamPlaylist)
	if err != nil {
		return nil, err
	}

	m := &Muxer{
		primaryPlaylist: primaryPlaylist,
		streamPlaylist:  streamPlaylist,
		tsGenerator:     tsGenerator,
	}

	return m, nil
}

// Close closes a Muxer.
func (m *Muxer) Close() {
	m.streamPlaylist.close()
}

// WriteH264 writes H264 NALUs, grouped by timestamp, into the muxer.
func (m *Muxer) WriteH264(pts time.Duration, nalus [][]byte) error {
	return m.tsGenerator.writeH264(pts, nalus)
}

// WriteAAC writes AAC AUs, grouped by timestamp, into the muxer.
func (m *Muxer) WriteAAC(pts time.Duration, aus [][]byte) error {
	return m.tsGenerator.writeAAC(pts, aus)
}

// PrimaryPlaylist returns a reader to read the primary playlist.
func (m *Muxer) PrimaryPlaylist() io.Reader {
	return m.primaryPlaylist.reader()
}

// StreamPlaylist returns a reader to read the stream playlist.
func (m *Muxer) StreamPlaylist() io.Reader {
	return m.streamPlaylist.reader()
}

// Segment returns a reader to read a segment listed in the stream playlist.
func (m *Muxer) Segment(fname string) io.Reader {
	return m.streamPlaylist.segment(fname)
}
