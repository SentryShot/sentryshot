package hls

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib/pkg/aac"
	"nvr/pkg/video/gortsplib/pkg/h264"
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
	streamInfo StreamInfoFunc

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
	segmentCount int,
	segmentDuration time.Duration,
	partDuration time.Duration,
	segmentMaxSize uint64,
	onNewSegment chan<- []SegmentOrGap,
	logf logFunc,
	videoTrackExist func() bool,
	videoSps videoSpsFunc,
	audioTrackExist func() bool,
	audioClockRate audioClockRateFunc,
	getStreamInfo StreamInfoFunc,
) *Muxer {
	m := &Muxer{
		playlist:   newPlaylist(segmentCount, onNewSegment),
		logf:       logf,
		streamInfo: getStreamInfo,
	}

	m.segmenter = newSegmenter(
		segmentCount,
		segmentDuration,
		partDuration,
		segmentMaxSize,
		videoTrackExist,
		videoSps,
		audioTrackExist,
		audioClockRate,
		m.playlist.onSegmentFinalized,
		m.playlist.onPartFinalized,
	)
	return m
}

// Close closes a Muxer.
func (m *Muxer) Close() {
	m.playlist.close()
}

// WriteH264 writes H264 NALUs, grouped by timestamp.
func (m *Muxer) WriteH264(pts time.Duration, nalus [][]byte) error {
	return m.segmenter.writeH264(pts, nalus)
}

// WriteAAC writes AAC AUs, grouped by timestamp.
func (m *Muxer) WriteAAC(pts time.Duration, aus [][]byte) error {
	return m.segmenter.writeAAC(pts, aus)
}

// StreamInfoFunc returns the stream information.
type StreamInfoFunc func() (*StreamInfo, error)

// StreamInfo Stream information required for decoding.
type StreamInfo struct {
	VideoTrackExist bool
	VideoSPS        []byte
	VideoPPS        []byte
	VideoSPSP       h264.SPS
	VideoWidth      int
	VideoHeight     int

	AudioTrackExist   bool
	AudioTrackConfig  []byte
	AudioChannelCount int
	AudioClockRate    int
	AudioType         aac.MPEG4AudioType
}

// File returns a file reader.
func (m *Muxer) File(
	name string,
	msn string,
	part string,
	skip string,
) *MuxerFileResponse {
	info, err := m.streamInfo()
	if err != nil {
		m.logf(log.LevelDebug, "generate stream info: %v", err)
		return &MuxerFileResponse{Status: http.StatusInternalServerError}
	}

	if name == "index.m3u8" {
		return primaryPlaylist(*info)
	}

	if name == "init.mp4" {
		m.mutex.Lock()
		defer m.mutex.Unlock()

		if m.initContent == nil ||
			(info.VideoTrackExist &&
				(!bytes.Equal(m.videoLastSPS, info.VideoSPS) ||
					!bytes.Equal(m.videoLastPPS, info.VideoPPS))) {
			initContent, err := generateInit(*info)
			if err != nil {
				m.logf(log.LevelError, "generate init.mp4: %w", err)
				return &MuxerFileResponse{Status: http.StatusInternalServerError}
			}
			m.videoLastSPS = info.VideoSPS
			m.videoLastPPS = info.VideoPPS
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

const (
	videoTimescale = 90000
)

type videoSample struct {
	nalus      [][]byte
	pts        time.Duration
	dts        time.Duration
	avcc       []byte
	idrPresent bool
	next       *videoSample
}

func (s videoSample) duration() time.Duration {
	return s.next.dts - s.dts
}

type audioSample struct {
	au   []byte
	pts  time.Duration
	next *audioSample
}

func (s audioSample) duration() time.Duration {
	return s.next.pts - s.pts
}
