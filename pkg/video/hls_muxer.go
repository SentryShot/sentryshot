package video

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/mpeg4audio"
	"nvr/pkg/video/gortsplib/pkg/ringbuffer"
	"nvr/pkg/video/hls"
	"sync"
	"time"
)

type muxerCloseFunc func(*HLSMuxer)

// HLSMuxer .
type HLSMuxer struct {
	wg              *sync.WaitGroup
	readBufferCount int
	path            *path
	pathConf        PathConf
	muxerClose      muxerCloseFunc

	ctx        context.Context
	ctxCancel  func()
	ringBuffer *ringbuffer.RingBuffer
	muxer      *hls.Muxer

	// in
	chRequest chan *hlsMuxerRequest
}

func newHLSMuxer(
	parentCtx context.Context,
	readBufferCount int,
	wg *sync.WaitGroup,
	path *path,
	muxerClose muxerCloseFunc,
) *HLSMuxer {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	return &HLSMuxer{
		readBufferCount: readBufferCount,
		wg:              wg,
		path:            path,
		pathConf:        *path.conf,
		muxerClose:      muxerClose,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
		chRequest:       make(chan *hlsMuxerRequest),
	}
}

func (m *HLSMuxer) close() {
	m.ctxCancel()
}

func (m *HLSMuxer) logf(format string, a ...interface{}) {
	m.path.logf(log.LevelError, "HLS: "+format, a...)
}

func (m *HLSMuxer) start(tracks gortsplib.Tracks) error {
	if err := m.run(tracks); err != nil {
		m.ctxCancel()
		return err
	}
	return nil
}

func (m *HLSMuxer) run(tracks gortsplib.Tracks) error {
	videoTrack, videoTrackID, audioTrack, audioTrackID, err := parseTracks(tracks)
	if err != nil {
		return fmt.Errorf("parse tracks: %w", err)
	}

	m.muxer = m.createMuxer(videoTrack, audioTrack)

	m.ringBuffer, err = ringbuffer.New(uint64(m.readBufferCount))
	if err != nil {
		return err
	}

	innerErr := make(chan error)
	go func() {
		innerErr <- m.runWriter(
			videoTrack,
			videoTrackID,
			audioTrack,
			audioTrackID,
		)
	}()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		cleanup := func() {
			m.ctxCancel()
			m.muxerClose(m)

			// This will disconnect FFmpeg and restart the input process.
			m.path.close()

			m.ringBuffer.Close()
		}

		for {
			select {
			case <-m.ctx.Done():
				cleanup()
				<-innerErr
				return

			case req := <-m.chRequest:
				req.res <- m.handleRequest(req)

			case err := <-innerErr:
				cleanup()
				if !errors.Is(err, context.Canceled) {
					m.logf("closed: %v", err)
				}
				return
			}
		}
	}()

	return nil
}

func parseTracks(tracks gortsplib.Tracks) (
	*gortsplib.TrackH264, int,
	*gortsplib.TrackMPEG4Audio,
	int,
	error,
) {
	var videoTrack *gortsplib.TrackH264
	videoTrackID := -1
	var audioTrack *gortsplib.TrackMPEG4Audio
	audioTrackID := -1

	for i, track := range tracks {
		switch tt := track.(type) {
		case *gortsplib.TrackH264:
			if videoTrack != nil {
				return nil, 0, nil, 0,
					fmt.Errorf("can't encode track %d with HLS: %w", i+1, ErrTooManyTracks)
			}

			videoTrack = tt
			videoTrackID = i

		case *gortsplib.TrackMPEG4Audio:
			if audioTrack != nil {
				return nil, 0, nil, 0,
					fmt.Errorf("can't encode track %d with HLS: %w", i+1, ErrTooManyTracks)
			}

			audioTrack = tt
			audioTrackID = i
		}
	}

	if videoTrack == nil && audioTrack == nil {
		return nil, 0, nil, 0, ErrNoTracks
	}

	return videoTrack, videoTrackID, audioTrack, audioTrackID, nil
}

const (
	hlsSegmentCount    = 3
	hlsSegmentDuration = 900 * time.Millisecond
	hlsPartDuration    = 300 * time.Millisecond
)

var mb = uint64(1000000)

var hlsSegmentMaxSize = 50 * mb

func (m *HLSMuxer) createMuxer(
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackMPEG4Audio,
) *hls.Muxer {
	muxerLogFunc := func(level log.Level, format string, a ...interface{}) {
		m.path.logf(level, "HLS: "+format, a...)
	}

	return hls.NewMuxer(
		m.ctx,
		hlsSegmentCount,
		hlsSegmentDuration,
		hlsPartDuration,
		hlsSegmentMaxSize,
		muxerLogFunc,
		videoTrack,
		audioTrack,
	)
}

// Errors.
var (
	ErrTooManyTracks = errors.New("too many tracks")
	ErrNoTracks      = errors.New("the stream doesn't contain an H264 track or an AAC track")
)

func (m *HLSMuxer) runWriter(
	videoTrack *gortsplib.TrackH264,
	videoTrackID int,
	audioTrack *gortsplib.TrackMPEG4Audio,
	audioTrackID int,
) error {
	videoStartPTSFilled := false
	var videoStartPTS time.Duration
	audioStartPTSFilled := false
	var audioStartPTS time.Duration

	for {
		item, ok := m.ringBuffer.Pull()
		if !ok {
			return context.Canceled
		}
		data := item.(data) //nolint:forcetypeassert

		if videoTrack != nil && data.getTrackID() == videoTrackID {
			tdata := data.(*dataH264) //nolint:forcetypeassert

			if tdata.nalus == nil {
				continue
			}

			if !videoStartPTSFilled {
				videoStartPTSFilled = true
				videoStartPTS = tdata.pts
			}
			pts := tdata.pts - videoStartPTS

			err := m.muxer.WriteH264(tdata.ntp, pts, tdata.nalus)
			if err != nil {
				return fmt.Errorf("muxer error: %w", err)
			}
		} else if audioTrack != nil && data.getTrackID() == audioTrackID {
			tdata := data.(*dataMPEG4Audio) //nolint:forcetypeassert

			if tdata.aus == nil {
				continue
			}

			if !audioStartPTSFilled {
				audioStartPTSFilled = true
				audioStartPTS = tdata.pts
			}
			pts := tdata.pts - audioStartPTS

			for i, au := range tdata.aus {
				err := m.muxer.WriteAAC(
					pts+time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*
						time.Second/time.Duration(audioTrack.ClockRate()),
					au)
				if err != nil {
					return fmt.Errorf("muxer error: %w", err)
				}
			}
		}
	}
}

type hlsMuxerRequest struct {
	path string
	file string
	req  *http.Request
	res  chan *hls.MuxerFileResponse
}

func (m *HLSMuxer) handleRequest(req *hlsMuxerRequest) *hls.MuxerFileResponse {
	p := req.req.URL.Query()
	msn := func() string {
		if len(p["_HLS_msn"]) > 0 {
			return p["_HLS_msn"][0]
		}
		return ""
	}()
	part := func() string {
		if len(p["_HLS_part"]) > 0 {
			return p["_HLS_part"][0]
		}
		return ""
	}()
	skip := func() string {
		if len(p["_HLS_skip"]) > 0 {
			return p["_HLS_skip"][0]
		}
		return ""
	}()

	return m.muxer.File(req.file, msn, part, skip)
}

// onRequest is called by hlsserver.Server (forwarded from ServeHTTP).
func (m *HLSMuxer) onRequest(req *hlsMuxerRequest) {
	select {
	case m.chRequest <- req:
	case <-m.ctx.Done():
		req.res <- &hls.MuxerFileResponse{
			Status: http.StatusInternalServerError,
		}
	}
}

// readerData is called by stream.
func (m *HLSMuxer) readerData(data data) {
	m.ringBuffer.Push(data)
}
