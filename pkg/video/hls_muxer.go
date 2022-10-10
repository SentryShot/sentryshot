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
	"nvr/pkg/video/gortsplib/pkg/rtpmpeg4audio"
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
	muxerClose      muxerCloseFunc
	logger          *log.Logger

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
	logger *log.Logger,
) *HLSMuxer {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	return &HLSMuxer{
		readBufferCount: readBufferCount,
		wg:              wg,
		path:            path,
		muxerClose:      muxerClose,
		logger:          logger,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
		chRequest:       make(chan *hlsMuxerRequest),
	}
}

func (m *HLSMuxer) close() {
	m.ctxCancel()
}

func (m *HLSMuxer) logf(format string, a ...interface{}) {
	if m.path == nil {
		return
	}
	sendLogf(m.logger, *m.path.conf, log.LevelError, "HLS:", format, a...)
}

func (m *HLSMuxer) start(tracks gortsplib.Tracks) error {
	if err := m.run(tracks); err != nil {
		m.ctxCancel()
		return err
	}
	return nil
}

func (m *HLSMuxer) run(tracks gortsplib.Tracks) error {
	videoTrack, videoTrackID, audioTrack,
		audioTrackID, aacDecoder, err := parseTracks(tracks)
	if err != nil {
		return err
	}

	m.muxer = m.createMuxer(videoTrack, audioTrack)

	m.ringBuffer, err = ringbuffer.New(uint64(m.readBufferCount))
	if err != nil {
		return err
	}

	innerErr := make(chan error)
	go func() {
		innerErr <- m.runInner(
			videoTrack,
			videoTrackID,
			audioTrack,
			audioTrackID,
			aacDecoder,
		)
	}()

	m.wg.Add(1)
	go func() {
		defer func() {
			m.muxerClose(m)

			// This will disconnect FFmpeg and restart the input process.
			m.path.close()

			m.wg.Done()
		}()

		for {
			select {
			case <-m.ctx.Done():
				m.ringBuffer.Close()
				<-innerErr
				return

			case req := <-m.chRequest:
				req.res <- m.handleRequest(req)

			case err := <-innerErr:
				m.ctxCancel()
				m.ringBuffer.Close()
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
	*gortsplib.TrackMPEG4Audio, int,
	*rtpmpeg4audio.Decoder, error,
) {
	var videoTrack *gortsplib.TrackH264
	videoTrackID := -1
	var audioTrack *gortsplib.TrackMPEG4Audio
	audioTrackID := -1
	var aacDecoder *rtpmpeg4audio.Decoder

	for i, track := range tracks {
		switch tt := track.(type) {
		case *gortsplib.TrackH264:
			if videoTrack != nil {
				return nil, 0, nil, 0, nil,
					fmt.Errorf("can't encode track %d with HLS: %w", i+1, ErrTooManyTracks)
			}

			videoTrack = tt
			videoTrackID = i

		case *gortsplib.TrackMPEG4Audio:
			if audioTrack != nil {
				return nil, 0, nil, 0, nil,
					fmt.Errorf("can't encode track %d with HLS: %w", i+1, ErrTooManyTracks)
			}

			audioTrack = tt
			audioTrackID = i
			aacDecoder = &rtpmpeg4audio.Decoder{
				SampleRate:       tt.Config.SampleRate,
				SizeLength:       tt.SizeLength,
				IndexLength:      tt.IndexLength,
				IndexDeltaLength: tt.IndexDeltaLength,
			}
			aacDecoder.Init()
		}
	}

	if videoTrack == nil && audioTrack == nil {
		return nil, 0, nil, 0, nil, ErrNoTracks
	}

	return videoTrack, videoTrackID, audioTrack, audioTrackID, aacDecoder, nil
}

func (m *HLSMuxer) createMuxer(
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackMPEG4Audio,
) *hls.Muxer {
	muxerLogFunc := func(level log.Level, format string, a ...interface{}) {
		sendLogf(m.logger, *m.path.conf, level, "HLS:", format, a...)
	}
	videoTrackExist := videoTrack != nil
	audioTrackExist := audioTrack != nil

	streamInfo := func() (*hls.StreamInfo, error) {
		info := hls.StreamInfo{
			VideoTrackExist: videoTrackExist,
			AudioTrackExist: audioTrackExist,
		}
		if info.VideoTrackExist {
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

	return hls.NewMuxer(
		m.ctx,
		m.path.hlsSegmentCount(),
		m.path.hlsSegmentDuration(),
		m.path.hlsPartDuration(),
		m.path.hlsSegmentMaxSize(),
		muxerLogFunc,
		videoTrackExist,
		videoTrack.SafeSPS,
		audioTrackExist,
		audioTrack.ClockRate,
		streamInfo,
	)
}

// Errors.
var (
	ErrTooManyTracks = errors.New("too many tracks")
	ErrNoTracks      = errors.New("the stream doesn't contain an H264 track or an AAC track")
)

func (m *HLSMuxer) runInner( //nolint:gocognit
	videoTrack *gortsplib.TrackH264,
	videoTrackID int,
	audioTrack *gortsplib.TrackMPEG4Audio,
	audioTrackID int,
	aacDecoder *rtpmpeg4audio.Decoder,
) error {
	var videoInitialPTS *time.Duration
	for {
		item, ok := m.ringBuffer.Pull()
		if !ok {
			return context.Canceled
		}
		data := item.(*data) //nolint:forcetypeassert

		if videoTrack != nil && data.trackID == videoTrackID {
			if data.h264NALUs == nil {
				continue
			}

			// video is decoded in another routine,
			// while audio is decoded in this routine:
			// we have to sync their PTS.
			if videoInitialPTS == nil {
				v := data.pts
				videoInitialPTS = &v
			}
			pts := data.pts - *videoInitialPTS

			err := m.muxer.WriteH264(time.Now(), pts, data.h264NALUs)
			if err != nil {
				return fmt.Errorf("unable to write segment: %w", err)
			}
		} else if audioTrack != nil && data.trackID == audioTrackID {
			aus, pts, err := aacDecoder.Decode(data.rtpPacket)
			if err != nil {
				if !errors.Is(err, rtpmpeg4audio.ErrMorePacketsNeeded) {
					return fmt.Errorf("unable to decode audio track: %w", err)
				}
				continue
			}

			for i, au := range aus {
				err = m.muxer.WriteAAC(
					time.Now(),
					pts+time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*
						time.Second/time.Duration(audioTrack.ClockRate()),
					au)
				if err != nil {
					return fmt.Errorf("write aac: %w", err)
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

// readerData implements reader.
func (m *HLSMuxer) readerData(data *data) {
	m.ringBuffer.Push(data)
}
