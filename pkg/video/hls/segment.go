package hls

import (
	"errors"
	"io"
	"nvr/pkg/video/gortsplib"
	"strconv"
	"time"
)

type partsReader struct {
	parts   []*muxerPart
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
	id              uint64
	startTime       time.Time
	startDTS        time.Duration
	segmentMaxSize  uint64
	videoTrack      *gortsplib.TrackH264
	audioTrack      *gortsplib.TrackAAC
	genPartID       func() uint64
	onPartFinalized func(*muxerPart)

	size             uint64
	parts            []*muxerPart
	currentPart      *muxerPart
	renderedDuration time.Duration
}

func newSegment(
	id uint64,
	startTime time.Time,
	startDTS time.Duration,
	segmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
	genPartID func() uint64,
	onPartFinalized func(*muxerPart),
) *Segment {
	s := &Segment{
		id:              id,
		startTime:       startTime,
		startDTS:        startDTS,
		segmentMaxSize:  segmentMaxSize,
		videoTrack:      videoTrack,
		audioTrack:      audioTrack,
		genPartID:       genPartID,
		onPartFinalized: onPartFinalized,
	}

	s.currentPart = newPart(
		s.videoTrack,
		s.audioTrack,
		s.genPartID(),
	)

	return s
}

// Duration .
func (s *Segment) Duration() time.Duration {
	return s.renderedDuration
}

func (s *Segment) name() string {
	return "seg" + strconv.FormatUint(s.id, 10)
}

func (s *Segment) reader() io.Reader {
	return &partsReader{parts: s.parts}
}

func (s *Segment) getRenderedDuration() time.Duration {
	return s.renderedDuration
}

func (s *Segment) finalize(nextVideoSample *videoSample) error {
	err := s.currentPart.finalize()
	if err != nil {
		return err
	}

	if s.currentPart.renderedContent != nil {
		s.onPartFinalized(s.currentPart)
		s.parts = append(s.parts, s.currentPart)
	}

	s.currentPart = nil

	if s.videoTrack != nil {
		s.renderedDuration = nextVideoSample.dts - s.startDTS
	} else {
		s.renderedDuration = 0
		for _, pa := range s.parts {
			s.renderedDuration += pa.renderedDuration
		}
	}

	return nil
}

// ErrMaximumSegmentSize reached maximum segment size.
var ErrMaximumSegmentSize = errors.New("reached maximum segment size")

func (s *Segment) writeH264(sample *videoSample, adjustedPartDuration time.Duration) error {
	size := uint64(len(sample.avcc))

	if (s.size + size) > s.segmentMaxSize {
		return ErrMaximumSegmentSize
	}

	s.currentPart.writeH264(sample)

	s.size += size

	// switch part
	if s.currentPart.duration() >= adjustedPartDuration {
		err := s.currentPart.finalize()
		if err != nil {
			return err
		}

		s.parts = append(s.parts, s.currentPart)
		s.onPartFinalized(s.currentPart)

		s.currentPart = newPart(
			s.videoTrack,
			s.audioTrack,
			s.genPartID(),
		)
	}

	return nil
}

func (s *Segment) writeAAC(sample *audioSample, adjustedPartDuration time.Duration) error {
	size := uint64(len(sample.au))

	if (s.size + size) > s.segmentMaxSize {
		return ErrMaximumSegmentSize
	}

	s.currentPart.writeAAC(sample)

	s.size += size

	// switch part
	if s.videoTrack == nil &&
		s.currentPart.duration() >= adjustedPartDuration {
		err := s.currentPart.finalize()
		if err != nil {
			return err
		}

		s.parts = append(s.parts, s.currentPart)
		s.onPartFinalized(s.currentPart)

		s.currentPart = newPart(
			s.videoTrack,
			s.audioTrack,
			s.genPartID(),
		)
	}

	return nil
}
