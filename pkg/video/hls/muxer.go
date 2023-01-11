package hls

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib/pkg/h264"
	"nvr/pkg/video/gortsplib/pkg/mpeg4audio"
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
	logf       logFunc
	streamInfo StreamInfo

	mutex        sync.Mutex
	videoLastSPS []byte
	videoLastPPS []byte
	initContent  []byte
}

type logFunc func(log.Level, string, ...interface{})

// ErrTrackInvalid invalid H264 track: SPS or PPS not provided into the SDP.
var ErrTrackInvalid = errors.New("invalid H264 track: SPS or PPS not provided into the SDP")

// NewMuxer allocates a Muxer.
func NewMuxer(
	ctx context.Context,
	segmentCount int,
	segmentDuration time.Duration,
	partDuration time.Duration,
	segmentMaxSize uint64,
	logf logFunc,
	videoTrackExist bool,
	videoSps videoSPSFunc,
	audioTrackExist bool,
	audioClockRate audioClockRateFunc,
	streamInfo StreamInfo,
) *Muxer {
	playlist := newPlaylist(ctx, segmentCount)
	go playlist.start()

	m := &Muxer{
		playlist:   playlist,
		logf:       logf,
		streamInfo: streamInfo,
	}

	m.segmenter = newSegmenter(
		time.Now().UnixNano(),
		segmentDuration,
		partDuration,
		segmentMaxSize,
		videoTrackExist,
		videoSps,
		audioTrackExist,
		audioClockRate,
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
func (m *Muxer) WriteAAC(ntp time.Time, pts time.Duration, au []byte) error {
	return m.segmenter.writeAAC(ntp, pts, au)
}

// StreamInfo Stream information required for decoding.
type StreamInfo struct {
	VideoTrackExist bool
	VideoTrackID    int
	VideoSPS        []byte
	VideoPPS        []byte
	VideoSPSP       h264.SPS
	VideoWidth      int
	VideoHeight     int

	AudioTrackExist   bool
	AudioTrackID      int
	AudioTrackConfig  []byte
	AudioChannelCount int
	AudioClockRate    int
	AudioType         mpeg4audio.ObjectType
}

// File returns a file reader.
func (m *Muxer) File(
	name string,
	msn string,
	part string,
	skip string,
) *MuxerFileResponse {
	if name == "index.m3u8" {
		return primaryPlaylist(m.streamInfo)
	}

	if name == "init.mp4" {
		m.mutex.Lock()
		defer m.mutex.Unlock()

		sps := m.streamInfo.VideoSPS

		if m.initContent == nil ||
			(m.streamInfo.VideoTrackExist &&
				(!bytes.Equal(m.videoLastSPS, sps) ||
					!bytes.Equal(m.videoLastPPS, m.streamInfo.VideoPPS))) {
			initContent, err := generateInit(m.streamInfo)
			if err != nil {
				m.logf(log.LevelError, "generate init.mp4: %w", err)
				return &MuxerFileResponse{Status: http.StatusInternalServerError}
			}
			m.videoLastSPS = m.streamInfo.VideoSPS
			m.videoLastPPS = m.streamInfo.VideoPPS
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

// StreamInfo return information about the stream.
func (m *Muxer) StreamInfo() *StreamInfo {
	return &m.streamInfo
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

	NextDTS int64
}

func (s VideoSample) duration() time.Duration {
	return time.Duration(s.NextDTS - s.DTS)
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
