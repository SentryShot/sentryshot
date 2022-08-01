package hls

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	primaryPlaylist *primaryPlaylist
	playlist        *playlist
	segmenter       *segmenter
	logFunc         func(string)
	videoTrack      *gortsplib.TrackH264
	audioTrack      *gortsplib.TrackAAC

	mutex        sync.Mutex
	videoLastSPS []byte
	videoLastPPS []byte
	initContent  []byte
}

// ErrTrackInvalid invalid H264 track: SPS or PPS not provided into the SDP.
var ErrTrackInvalid = errors.New("invalid H264 track: SPS or PPS not provided into the SDP")

// NewMuxer allocates a Muxer.
func NewMuxer(
	segmentCount int,
	segmentDuration time.Duration,
	partDuration time.Duration,
	segmentMaxSize uint64,
	onNewSegment chan<- []SegmentOrGap,
	logFunc func(string),
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
) *Muxer {
	m := &Muxer{
		primaryPlaylist: newPrimaryPlaylist(videoTrack, audioTrack),
		playlist: newPlaylist(
			segmentCount,
			videoTrack,
			audioTrack,
			onNewSegment,
		),
		logFunc:    logFunc,
		videoTrack: videoTrack,
		audioTrack: audioTrack,
	}

	m.segmenter = newSegmenter(
		segmentCount,
		segmentDuration,
		partDuration,
		segmentMaxSize,
		videoTrack,
		audioTrack,
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

// File returns a file reader.
func (m *Muxer) File(name string, msn string, part string, skip string) *MuxerFileResponse {
	if name == "index.m3u8" {
		return m.primaryPlaylist.file()
	}

	if name == "init.mp4" {
		m.mutex.Lock()
		defer m.mutex.Unlock()

		var sps []byte
		var pps []byte
		if m.videoTrack != nil {
			sps = m.videoTrack.SafeSPS()
			pps = m.videoTrack.SafePPS()
		}

		if m.initContent == nil ||
			(m.videoTrack != nil && (!bytes.Equal(m.videoLastSPS, sps) || !bytes.Equal(m.videoLastPPS, pps))) {
			initContent, err := generateInit(m.videoTrack, m.audioTrack)
			if err != nil {
				m.logFunc(fmt.Sprintf("generate init: %v", err))
				return &MuxerFileResponse{Status: http.StatusInternalServerError}
			}

			m.videoLastSPS = sps
			m.videoLastPPS = pps
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
