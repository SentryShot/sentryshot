package video

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/ringbuffer"
	"nvr/pkg/video/gortsplib/pkg/rtpaac"
	"nvr/pkg/video/hls"
	"sync"
	"sync/atomic"
	"time"
)

type hlsMuxerRequest struct {
	path string
	file string
	req  *http.Request
	res  chan func() *hls.MuxerFileResponse
}

type (
	onReaderSetupPlayFunc func(req pathReaderSetupPlayReq) pathReaderSetupPlayRes
	onMuxerCloseFunc      func(*hlsMuxer)
)

type hlsMuxer struct {
	wg                *sync.WaitGroup
	readBufferCount   int
	pathName          string
	onReaderSetupPlay onReaderSetupPlayFunc
	onMuxerClose      onMuxerCloseFunc
	logger            *log.Logger

	ctx             context.Context
	ctxCancel       func()
	path            *path
	ringBuffer      *ringbuffer.RingBuffer
	lastRequestTime *int64
	muxer           *hls.Muxer
	requests        []*hlsMuxerRequest

	// in
	request    chan *hlsMuxerRequest
	streamInfo hls.StreamInfoFunc
}

func newHLSMuxer(
	parentCtx context.Context,
	readBufferCount int,
	req *hlsMuxerRequest,
	wg *sync.WaitGroup,
	pathName string,
	onReaderSetupPlay onReaderSetupPlayFunc,
	onMuxerClose onMuxerCloseFunc,
	logger *log.Logger,
) *hlsMuxer {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	now := time.Now().Unix()

	m := &hlsMuxer{
		readBufferCount:   readBufferCount,
		wg:                wg,
		pathName:          pathName,
		onReaderSetupPlay: onReaderSetupPlay,
		onMuxerClose:      onMuxerClose,
		logger:            logger,
		ctx:               ctx,
		ctxCancel:         ctxCancel,
		lastRequestTime:   &now,
		request:           make(chan *hlsMuxerRequest),
	}

	if req != nil {
		m.requests = append(m.requests, req)
	}

	m.wg.Add(1)
	go m.run()

	return m
}

func (m *hlsMuxer) close() {
	m.ctxCancel()
}

func (m *hlsMuxer) logf(format string, a ...interface{}) {
	if m.path == nil {
		return
	}
	sendLogf(m.logger, *m.path.conf, log.LevelError, "HLS:", format, a...)
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
		req.res <- func() *hls.MuxerFileResponse {
			return &hls.MuxerFileResponse{Status: http.StatusNotFound}
		}
	}

	m.onMuxerClose(m)

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
	res := m.onReaderSetupPlay(pathReaderSetupPlayReq{
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
				SampleRate:       tt.Config.SampleRate,
				SizeLength:       tt.SizeLength,
				IndexLength:      tt.IndexLength,
				IndexDeltaLength: tt.IndexDeltaLength,
			}
			aacDecoder.Init()
		}
	}

	if videoTrack == nil && audioTrack == nil {
		return ErrNoTracks
	}

	muxerLogFunc := func(level log.Level, format string, a ...interface{}) {
		sendLogf(m.logger, *m.path.conf, level, "HLS:", format, a...)
	}
	videoTrackExist := func() bool { return videoTrack != nil }
	audioTrackExist := func() bool { return audioTrack != nil }
	m.streamInfo = getStreamInfo(videoTrack, videoTrackID, audioTrack, audioTrackID)

	m.muxer = hls.NewMuxer(
		m.path.hlsSegmentCount(),
		m.path.hlsSegmentDuration(),
		m.path.hlsPartDuration(),
		m.path.hlsSegmentMaxSize(),
		m.path.conf.onHlsSegmentFinalized,
		muxerLogFunc,
		videoTrackExist,
		videoTrack.SafeSPS,
		audioTrackExist,
		audioTrack.ClockRate,
		m.streamInfo,
	)
	defer m.muxer.Close()

	innerReady <- struct{}{}

	var err error
	m.ringBuffer, err = ringbuffer.New(uint64(m.readBufferCount))
	if err != nil {
		return err
	}

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

// Creates a functions that returns the stream info.
func getStreamInfo(
	videoTrack *gortsplib.TrackH264,
	videoTrackID int,
	audioTrack *gortsplib.TrackAAC,
	audioTrackID int,
) hls.StreamInfoFunc {
	return func() (*hls.StreamInfo, error) {
		info := hls.StreamInfo{
			VideoTrackExist: videoTrack != nil,
			AudioTrackExist: audioTrack != nil,
		}

		if info.VideoTrackExist {
			info.VideoTrackID = videoTrackID
			info.VideoSPS = videoTrack.SafeSPS()
			info.VideoPPS = videoTrack.SafePPS()
			err := info.VideoSPSP.Unmarshal(info.VideoSPS)
			if err != nil {
				return nil, err
			}
			info.VideoHeight = info.VideoSPSP.Height()
			info.VideoWidth = info.VideoSPSP.Width()
		}

		if info.AudioTrackExist {
			info.AudioTrackID = audioTrackID
			var err error
			info.AudioTrackConfig, err = audioTrack.Config.Marshal()
			if err != nil {
				return nil, err
			}
			info.AudioChannelCount = audioTrack.Config.ChannelCount
			info.AudioClockRate = audioTrack.ClockRate()
			info.AudioType = audioTrack.Config.Type
		}

		return &info, nil
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

func (m *hlsMuxer) handleRequest(req *hlsMuxerRequest) func() *hls.MuxerFileResponse {
	atomic.StoreInt64(m.lastRequestTime, time.Now().Unix())

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

	return func() *hls.MuxerFileResponse {
		return m.muxer.File(req.file, msn, part, skip)
	}
}

// onRequest is called by hlsserver.Server (forwarded from ServeHTTP).
func (m *hlsMuxer) onRequest(req *hlsMuxerRequest) {
	select {
	case m.request <- req:
	case <-m.ctx.Done():
		req.res <- func() *hls.MuxerFileResponse {
			return &hls.MuxerFileResponse{Status: http.StatusInternalServerError}
		}
	}
}

// onReaderAccepted implements reader.
func (m *hlsMuxer) onReaderAccepted() {
}

// onReaderData implements reader.
func (m *hlsMuxer) onReaderData(data *data) {
	m.ringBuffer.Push(data)
}
