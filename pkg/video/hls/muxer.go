package hls

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib"
	"sync"
	"time"
)

// MuxerFileResponse is a response of the Muxer's File() func.
type MuxerFileResponse struct {
	Status int
	Header map[string]string
	Body   io.Reader
}

// Muxer is a HLS muxer.
type Muxer struct {
	playlist   *playlist
	segmenter  *segmenter
	logf       log.Func
	videoTrack *gortsplib.TrackH264
	audioTrack *gortsplib.TrackMPEG4Audio

	mutex        sync.Mutex
	videoLastSPS []byte
	videoLastPPS []byte
	initContent  []byte
}

// ErrTrackInvalid invalid H264 track: SPS or PPS not provided into the SDP.
var ErrTrackInvalid = errors.New("invalid H264 track: SPS or PPS not provided into the SDP")

// NewMuxer allocates a Muxer.
func NewMuxer(
	ctx context.Context,
	segmentCount int,
	segmentDuration time.Duration,
	partDuration time.Duration,
	segmentMaxSize uint64,
	logf log.Func,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackMPEG4Audio,
) *Muxer {
	playlist := newPlaylist(ctx, segmentCount)
	go playlist.start()

	m := &Muxer{
		playlist:   playlist,
		logf:       logf,
		videoTrack: videoTrack,
	}

	m.segmenter = newSegmenter(
		time.Now().UnixNano(),
		segmentDuration,
		partDuration,
		segmentMaxSize,
		videoTrack,
		audioTrack,
		m.playlist.onSegmentFinalized,
		m.playlist.partFinalized,
	)
	return m
}

// OnSegmentFinalizedFunc is injected by core.
type OnSegmentFinalizedFunc func([]SegmentOrGap)

// WriteH264 writes H264 NALUs, grouped by timestamp.
func (m *Muxer) WriteH264(ntp time.Time, pts time.Duration, nalus [][]byte) error {
	return m.segmenter.writeH264(ntp, pts, nalus)
}

// WriteAAC writes AAC AUs, grouped by timestamp.
func (m *Muxer) WriteAAC(pts time.Duration, au []byte) error {
	return m.segmenter.writeAAC(pts, au)
}

// File returns a file reader.
func (m *Muxer) File(
	name string,
	msn string,
	part string,
	skip string,
) *MuxerFileResponse {
	if name == "index.m3u8" {
		return primaryPlaylist(m.videoTrack, m.audioTrack)
	}

	if name == "init.mp4" {
		m.mutex.Lock()
		defer m.mutex.Unlock()

		sps := m.videoTrack.SPS

		if m.initContent == nil ||
			(!bytes.Equal(m.videoLastSPS, sps) ||
				!bytes.Equal(m.videoLastPPS, m.videoTrack.PPS)) {
			initContent, err := generateInit(m.videoTrack, m.audioTrack)
			if err != nil {
				m.logf(log.LevelError, "generate init.mp4: %w", err)
				return &MuxerFileResponse{Status: http.StatusInternalServerError}
			}
			m.videoLastSPS = m.videoTrack.SPS
			m.videoLastPPS = m.videoTrack.PPS
			m.initContent = initContent
		}

		return &MuxerFileResponse{
			Status: http.StatusOK,
			Header: map[string]string{
				"Content-Type": "video/mp4",
			},
			Body: bytes.NewReader(m.initContent),
		}
	}

	return m.playlist.file(name, msn, part, skip)
}

// VideoTrack returns the stream video track.
func (m *Muxer) VideoTrack() *gortsplib.TrackH264 {
	return m.videoTrack
}

// AudioTrack returns the stream audio track.
func (m *Muxer) AudioTrack() *gortsplib.TrackMPEG4Audio {
	return m.audioTrack
}

// WaitForSegFinalized blocks until a new segment has been finalized.
func (m *Muxer) WaitForSegFinalized() {
	m.playlist.waitForSegFinalized()
}

// NextSegment returns the first segment with a ID greater than prevID.
// Will wait for new segments if the next segment isn't cached.
func (m *Muxer) NextSegment(prevID uint64) (*Segment, error) {
	return m.playlist.nextSegment(prevID)
}

// VideoTimescale the number of time units that pass per second.
const VideoTimescale = 90000

// Sample .
type Sample interface {
	private()
}

// VideoSample Timestamps are in UnixNano.
type VideoSample struct {
	PTS        int64
	DTS        int64
	AVCC       []byte
	IdrPresent bool

	Duration time.Duration
}

func (VideoSample) private() {}

// AudioSample Timestamps are in UnixNano.
type AudioSample struct {
	AU  []byte
	PTS int64

	NextPTS int64
}

// Duration sample duration.
func (s AudioSample) Duration() time.Duration {
	return time.Duration(s.NextPTS - s.PTS)
}

func (AudioSample) private() {}
