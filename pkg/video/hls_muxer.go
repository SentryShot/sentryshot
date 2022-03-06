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
	"nvr/pkg/video/gortsplib/pkg/rtph264"
	"nvr/pkg/video/hls"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtp"
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

type hlsMuxerTrackIDPayloadPair struct {
	trackID int
	buf     []byte
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
	logger *log.Logger) *hlsMuxer {
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
	ErrNoVideoTrack  = errors.New("the stream doesn't contain a H264 track")
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
	var h264Decoder *rtph264.Decoder
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
			h264Decoder = rtph264.NewDecoder()

		case *gortsplib.TrackAAC:
			if audioTrack != nil {
				return fmt.Errorf("can't encode track %d with HLS: %w", i+1, ErrTooManyTracks)
			}

			audioTrack = tt
			audioTrackID = i
			aacDecoder = rtpaac.NewDecoder(track.ClockRate())
		}
	}

	if videoTrack == nil {
		return ErrNoVideoTrack
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
		for {
			data, ok := m.ringBuffer.Pull()
			if !ok {
				writerDone <- context.Canceled
				return
			}

			pair := data.(hlsMuxerTrackIDPayloadPair) //nolint:forcetypeassert

			err := m.decodePacket(
				pair, videoTrack, videoTrackID, h264Decoder,
				audioTrack, audioTrackID, aacDecoder)
			if err != nil {
				m.logf("unable to decode RTP packet: %v", err)
			}
		}
	}()

	select {
	case err := <-writerDone:
		return err

	case <-innerCtx.Done():
		m.ringBuffer.Close()
		<-writerDone
		return context.Canceled
	}
}

func (m *hlsMuxer) decodePacket(
	pair hlsMuxerTrackIDPayloadPair,
	videoTrack *gortsplib.TrackH264,
	videoTrackID int,
	h264Decoder *rtph264.Decoder,
	audioTrack *gortsplib.TrackAAC,
	audioTrackID int,
	aacDecoder *rtpaac.Decoder) error {
	if videoTrack != nil && pair.trackID == videoTrackID { //nolint:nestif
		var pkt rtp.Packet
		err := pkt.Unmarshal(pair.buf)
		if err != nil {
			return fmt.Errorf("unable to decode RTP packet: %w", err)
		}

		nalus, pts, err := h264Decoder.DecodeUntilMarker(&pkt)
		if err != nil {
			if !errors.Is(err, rtph264.ErrMorePacketsNeeded) &&
				!errors.Is(err, rtph264.ErrNonStartingPacketAndNoPrevious) {
				return fmt.Errorf("unable to decode video track: %w", err)
			}
			return nil
		}

		err = m.muxer.WriteH264(pts, nalus)
		if err != nil {
			return fmt.Errorf("unable to write segment: %w", err)
		}
	} else if audioTrack != nil && pair.trackID == audioTrackID {
		var pkt rtp.Packet
		err := pkt.Unmarshal(pair.buf)
		if err != nil {
			return fmt.Errorf("unable to decode RTP packet: %w", err)
		}

		aus, pts, err := aacDecoder.Decode(&pkt)
		if err != nil {
			if !errors.Is(err, rtpaac.ErrMorePacketsNeeded) {
				return fmt.Errorf("unable to decode audio track: %w", err)
			}
			return nil
		}

		err = m.muxer.WriteAAC(pts, aus)
		if err != nil {
			return fmt.Errorf("unable to write segment: %w", err)
		}
	}
	return nil
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

// onReaderPacketRTP implements reader.
func (m *hlsMuxer) onReaderPacketRTP(trackID int, payload []byte) {
	m.ringBuffer.Push(hlsMuxerTrackIDPayloadPair{trackID, payload})
}
