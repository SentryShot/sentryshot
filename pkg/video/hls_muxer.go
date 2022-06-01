package video

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/ringbuffer"
	"nvr/pkg/video/gortsplib/pkg/rtpaac"
	"nvr/pkg/video/hls"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type hlsMuxerResponse struct {
	status int
	header map[string]string
	body   io.Reader
}

type hlsMuxerRequest struct {
	path string
	file string
	req  *http.Request
	res  chan hlsMuxerResponse
}

type hlsMuxerPathManager interface {
	onReaderSetupPlay(req pathReaderSetupPlayReq) pathReaderSetupPlayRes
}

type hlsMuxerParent interface {
	onMuxerClose(*hlsMuxer)
}

// StringSize is a size that is unmarshaled from a string.
type StringSize uint64

type hlsMuxer struct {
	name            string
	readBufferCount int
	wg              *sync.WaitGroup
	pathName        string
	pathManager     hlsMuxerPathManager
	parent          hlsMuxerParent
	logger          *log.Logger

	ctx             context.Context
	ctxCancel       func()
	path            *path
	ringBuffer      *ringbuffer.RingBuffer
	lastRequestTime *int64
	muxer           *hls.Muxer
	requests        []hlsMuxerRequest

	// in
	request chan hlsMuxerRequest
}

func newHLSMuxer(
	parentCtx context.Context,
	name string,
	readBufferCount int,
	wg *sync.WaitGroup,
	pathName string,
	pathManager hlsMuxerPathManager,
	parent hlsMuxerParent,
	logger *log.Logger,
) *hlsMuxer {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	now := time.Now().Unix()

	m := &hlsMuxer{
		name:            name,
		readBufferCount: readBufferCount,
		wg:              wg,
		pathName:        pathName,
		pathManager:     pathManager,
		parent:          parent,
		logger:          logger,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
		lastRequestTime: &now,
		request:         make(chan hlsMuxerRequest),
	}

	m.wg.Add(1)
	go m.run()

	return m
}

func (m *hlsMuxer) close() {
	m.ctxCancel()
}

func (m *hlsMuxer) logf(format string, args ...interface{}) {
	if m.path == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	sendLog(m.logger, *m.path.conf, log.LevelError, "HLS:", msg)
}

func (m *hlsMuxer) run() {
	defer m.wg.Done()

	innerCtx, innerCtxCancel := context.WithCancel(context.Background())
	innerReady := make(chan struct{})
	innerErr := make(chan error)
	go func() {
		innerErr <- m.runInner(innerCtx, innerReady)
	}()

	isReady := false

	err := func() error {
		for {
			select {
			case <-m.ctx.Done():
				innerCtxCancel()
				<-innerErr
				return context.Canceled

			case req := <-m.request:
				if isReady {
					req.res <- m.handleRequest(req)
				} else {
					m.requests = append(m.requests, req)
				}

			case <-innerReady:
				isReady = true
				for _, req := range m.requests {
					req.res <- m.handleRequest(req)
				}
				m.requests = nil

			case err := <-innerErr:
				innerCtxCancel()
				return err
			}
		}
	}()

	m.ctxCancel()

	for _, req := range m.requests {
		req.res <- hlsMuxerResponse{status: http.StatusNotFound}
	}

	m.parent.onMuxerClose(m)

	if err != nil && !errors.Is(err, context.Canceled) {
		m.logf("closed (%v)", err)
	}
}

// Errors.
var (
	ErrTooManyTracks = errors.New("too many tracks")
	ErrNoTracks      = errors.New("the stream doesn't contain an H264 track or an AAC track")
)

func (m *hlsMuxer) runInner(innerCtx context.Context, innerReady chan struct{}) error { //nolint:funlen
	res := m.pathManager.onReaderSetupPlay(pathReaderSetupPlayReq{
		author:   m,
		pathName: m.pathName,
	})
	if res.err != nil {
		return res.err
	}

	m.path = res.path

	defer func() {
		m.path.onReaderRemove(pathReaderRemoveReq{author: m})
	}()

	var videoTrack *gortsplib.TrackH264
	videoTrackID := -1
	var audioTrack *gortsplib.TrackAAC
	audioTrackID := -1
	var aacDecoder *rtpaac.Decoder

	for i, track := range res.stream.tracks() {
		switch tt := track.(type) {
		case *gortsplib.TrackH264:
			if videoTrack != nil {
				return fmt.Errorf("can't encode track %d with HLS: %w", i+1, ErrTooManyTracks)
			}

			videoTrack = tt
			videoTrackID = i

		case *gortsplib.TrackAAC:
			if audioTrack != nil {
				return fmt.Errorf("can't encode track %d with HLS: %w", i+1, ErrTooManyTracks)
			}

			audioTrack = tt
			audioTrackID = i
			aacDecoder = &rtpaac.Decoder{
				SampleRate:       tt.ClockRate(),
				SizeLength:       tt.SizeLength(),
				IndexLength:      tt.IndexLength(),
				IndexDeltaLength: tt.IndexDeltaLength(),
			}
			aacDecoder.Init()
		}
	}

	if videoTrack == nil && audioTrack == nil {
		return ErrNoTracks
	}

	var err error
	m.muxer, err = hls.NewMuxer(
		m.path.hlsSegmentCount(),
		m.path.hlsSegmentDuration(),
		m.path.hlsSegmentMaxSize(),
		m.path.conf.onNewHLSsegment,
		videoTrack,
		audioTrack,
	)
	if err != nil {
		return err
	}
	defer m.muxer.Close()

	innerReady <- struct{}{}

	m.ringBuffer = ringbuffer.New(uint64(m.readBufferCount))

	m.path.onReaderPlay(pathReaderPlayReq{author: m})

	writerDone := make(chan error)
	go func() {
		writerDone <- m.runInnerst(
			videoTrack,
			videoTrackID,
			audioTrack,
			audioTrackID,
			aacDecoder,
		)
	}()

	for {
		select {
		case err := <-writerDone:
			return err

		case <-innerCtx.Done():
			m.ringBuffer.Close()
			<-writerDone
			return context.Canceled
		}
	}
}

func (m *hlsMuxer) runInnerst(
	videoTrack *gortsplib.TrackH264,
	videoTrackID int,
	audioTrack *gortsplib.TrackAAC,
	audioTrackID int,
	aacDecoder *rtpaac.Decoder,
) error {
	var videoInitialPTS *time.Duration
	for {
		item, ok := m.ringBuffer.Pull()
		if !ok {
			return context.Canceled
		}
		data := item.(*data) //nolint:forcetypeassert

		if videoTrack != nil && data.trackID == videoTrackID { //nolint:nestif
			if data.h264NALUs == nil {
				continue
			}

			// video is decoded in another routine,
			// while audio is decoded in this routine:
			// we have to sync their PTS.
			if videoInitialPTS == nil {
				v := data.h264PTS
				videoInitialPTS = &v
			}
			pts := data.h264PTS - *videoInitialPTS

			err := m.muxer.WriteH264(pts, data.h264NALUs)
			if err != nil {
				return fmt.Errorf("unable to write segment: %w", err)
			}
		} else if audioTrack != nil && data.trackID == audioTrackID {
			aus, pts, err := aacDecoder.Decode(data.rtp)
			if err != nil {
				if !errors.Is(err, rtpaac.ErrMorePacketsNeeded) {
					return fmt.Errorf("unable to decode audio track: %w", err)
				}
				continue
			}

			err = m.muxer.WriteAAC(pts, aus)
			if err != nil {
				return fmt.Errorf("unable to write segment: %w", err)
			}
		}
	}
}

func (m *hlsMuxer) handleRequest(req hlsMuxerRequest) hlsMuxerResponse {
	atomic.StoreInt64(m.lastRequestTime, time.Now().Unix())

	switch {
	case req.file == "index.m3u8":
		return hlsMuxerResponse{
			status: http.StatusOK,
			header: map[string]string{
				"Content-Type": `application/x-mpegURL`,
			},
			body: m.muxer.PrimaryPlaylist(),
		}
	case req.file == "stream.m3u8":
		return hlsMuxerResponse{
			status: http.StatusOK,
			header: map[string]string{
				"Content-Type": `application/x-mpegURL`,
			},
			body: m.muxer.StreamPlaylist(),
		}

	case strings.HasSuffix(req.file, ".ts"):
		r := m.muxer.Segment(req.file)
		if r == nil {
			return hlsMuxerResponse{status: http.StatusNotFound}
		}

		return hlsMuxerResponse{
			status: http.StatusOK,
			header: map[string]string{
				"Content-Type": `video/MP2T`,
			},
			body: r,
		}

	default:
		return hlsMuxerResponse{status: http.StatusNotFound}
	}
}

// onRequest is called by hlsserver.Server (forwarded from ServeHTTP).
func (m *hlsMuxer) onRequest(req hlsMuxerRequest) {
	select {
	case m.request <- req:
	case <-m.ctx.Done():
		req.res <- hlsMuxerResponse{status: http.StatusNotFound}
	}
}

// onReaderAccepted implements reader.
func (m *hlsMuxer) onReaderAccepted() {
	// m.logf("is converting into HLS")
}

// onReaderData implements reader.
func (m *hlsMuxer) onReaderData(data *data) {
	m.ringBuffer.Push(data)
}
