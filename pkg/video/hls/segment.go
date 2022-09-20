package hls

import (
	"errors"
	"io"
	"strconv"
	"time"
)

type partsReader struct {
	parts   []*MuxerPart
	curPart int
	curPos  int
}

func (mbr *partsReader) Read(p []byte) (int, error) {
	n := 0
	lenp := len(p)

	for {
		if mbr.curPart >= len(mbr.parts) {
			return n, io.EOF
		}

		copied := copy(p[n:], mbr.parts[mbr.curPart].renderedContent[mbr.curPos:])
		mbr.curPos += copied
		n += copied

		if mbr.curPos == len(mbr.parts[mbr.curPart].renderedContent) {
			mbr.curPart++
			mbr.curPos = 0
		}

		if n == lenp {
			return n, nil
		}
	}
}

// Segment .
type Segment struct {
	ID              uint64
	StartTime       time.Time // Segment start time.
	startDTS        time.Duration
	muxerStartTime  int64
	segmentMaxSize  uint64
	videoTrackExist func() bool
	audioTrackExist func() bool
	audioClockRate  audioClockRateFunc
	genPartID       func() uint64
	onPartFinalized func(*MuxerPart)

	name             string
	size             uint64
	Parts            []*MuxerPart
	currentPart      *MuxerPart
	RenderedDuration time.Duration
}

func newSegment(
	id uint64,
	startTime time.Time,
	startDTS time.Duration,
	muxerStartTime int64,
	segmentMaxSize uint64,
	videoTrackExist func() bool,
	audioTrackExist func() bool,
	audioClockRate audioClockRateFunc,
	genPartID func() uint64,
	onPartFinalized func(*MuxerPart),
) *Segment {
	s := &Segment{
		ID:              id,
		StartTime:       startTime,
		startDTS:        startDTS,
		muxerStartTime:  muxerStartTime,
		segmentMaxSize:  segmentMaxSize,
		videoTrackExist: videoTrackExist,
		audioTrackExist: audioTrackExist,
		audioClockRate:  audioClockRate,
		genPartID:       genPartID,
		onPartFinalized: onPartFinalized,
		name:            "seg" + strconv.FormatUint(id, 10),
	}

	s.currentPart = newPart(
		s.videoTrackExist,
		s.audioTrackExist,
		s.audioClockRate,
		s.muxerStartTime,
		s.genPartID(),
	)

	return s
}

func (s *Segment) reader() io.Reader {
	return &partsReader{parts: s.Parts}
}

func (s *Segment) getRenderedDuration() time.Duration {
	return s.RenderedDuration
}

func (s *Segment) finalize(nextVideoSample *VideoSample) error {
	if err := s.currentPart.finalize(); err != nil {
		return err
	}

	if s.currentPart.renderedContent != nil {
		s.onPartFinalized(s.currentPart)
		s.Parts = append(s.Parts, s.currentPart)
	}

	s.currentPart = nil

	if s.videoTrackExist() {
		s.RenderedDuration = time.Duration(
			nextVideoSample.DTS-s.muxerStartTime) - s.startDTS
	} else {
		s.RenderedDuration = 0
		for _, pa := range s.Parts {
			s.RenderedDuration += pa.renderedDuration
		}
	}
	return nil
}

// ErrMaximumSegmentSize reached maximum segment size.
var ErrMaximumSegmentSize = errors.New("reached maximum segment size")

func (s *Segment) writeH264(sample *VideoSample, adjustedPartDuration time.Duration) error {
	size := uint64(len(sample.AVCC))

	if (s.size + size) > s.segmentMaxSize {
		return ErrMaximumSegmentSize
	}

	s.currentPart.writeH264(sample)

	s.size += size

	// switch part
	if s.currentPart.duration() >= adjustedPartDuration {
		if err := s.currentPart.finalize(); err != nil {
			return err
		}

		s.Parts = append(s.Parts, s.currentPart)
		s.onPartFinalized(s.currentPart)

		s.currentPart = newPart(
			s.videoTrackExist,
			s.audioTrackExist,
			s.audioClockRate,
			s.muxerStartTime,
			s.genPartID(),
		)
	}

	return nil
}

func (s *Segment) writeAAC(sample *AudioSample, adjustedPartDuration time.Duration) error {
	size := uint64(len(sample.AU))

	if (s.size + size) > s.segmentMaxSize {
		return ErrMaximumSegmentSize
	}

	s.currentPart.writeAAC(sample)

	s.size += size

	// switch part
	if s.videoTrackExist() &&
		s.currentPart.duration() >= adjustedPartDuration {
		if err := s.currentPart.finalize(); err != nil {
			return err
		}

		s.Parts = append(s.Parts, s.currentPart)
		s.onPartFinalized(s.currentPart)

		s.currentPart = newPart(
			s.videoTrackExist,
			s.audioTrackExist,
			s.audioClockRate,
			s.muxerStartTime,
			s.genPartID(),
		)
	}

	return nil
}
