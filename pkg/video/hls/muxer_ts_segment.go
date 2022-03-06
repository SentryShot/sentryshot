package hls

import (
	"bytes"
	"errors"
	"io"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/mpegts"
	"strconv"
	"time"
)

// MuxerTSSegment hls segment.
type MuxerTSSegment struct {
	hlsSegmentMaxSize uint64
	videoTrack        *gortsplib.TrackH264
	writer            *muxerTSWriter

	name           string
	buf            bytes.Buffer
	startPTS       *time.Duration
	endPTS         time.Duration
	pcrSendCounter int
	audioAUCount   int
}

func newMuxerTSSegment(
	hlsSegmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	writer *muxerTSWriter,
) *MuxerTSSegment {
	t := &MuxerTSSegment{
		hlsSegmentMaxSize: hlsSegmentMaxSize,
		videoTrack:        videoTrack,
		writer:            writer,
		name:              strconv.FormatInt(time.Now().Unix(), 10),
	}

	// WriteTable() is called automatically when WriteData() is called with
	// - PID == PCRPID
	// - AdaptationField != nil
	// - RandomAccessIndicator = true

	writer.currentSegment = t

	return t
}

// Duration segment duration.
func (t *MuxerTSSegment) Duration() time.Duration {
	return t.endPTS - *t.startPTS
}

// ErrMaxSegmentSize reached maximum segment size.
var ErrMaxSegmentSize = errors.New("reached maximum segment size")

func (t *MuxerTSSegment) write(p []byte) (int, error) {
	if uint64(len(p)+t.buf.Len()) > t.hlsSegmentMaxSize {
		return 0, ErrMaxSegmentSize
	}

	return t.buf.Write(p)
}

func (t *MuxerTSSegment) reader() io.Reader {
	return bytes.NewReader(t.buf.Bytes())
}

func (t *MuxerTSSegment) writeH264(
	startPCR time.Time,
	dts time.Duration,
	pts time.Duration,
	idrPresent bool,
	enc []byte) error {
	var af *mpegts.PacketAdaptationField

	if idrPresent {
		if af == nil {
			af = &mpegts.PacketAdaptationField{}
		}
		af.RandomAccessIndicator = true
	}

	// send PCR once in a while
	if t.pcrSendCounter == 0 {
		if af == nil {
			af = &mpegts.PacketAdaptationField{}
		}
		af.HasPCR = true
		af.PCR = &mpegts.ClockReference{Base: int64(time.Since(startPCR).Seconds() * 90000)}
		t.pcrSendCounter = 3
	}
	t.pcrSendCounter--

	oh := &mpegts.PESOptionalHeader{
		MarkerBits: 2,
	}

	if dts == pts {
		oh.PTSDTSIndicator = mpegts.PTSDTSIndicatorOnlyPTS
		oh.PTS = &mpegts.ClockReference{Base: int64(pts.Seconds() * 90000)}
	} else {
		oh.PTSDTSIndicator = mpegts.PTSDTSIndicatorBothPresent
		oh.DTS = &mpegts.ClockReference{Base: int64(dts.Seconds() * 90000)}
		oh.PTS = &mpegts.ClockReference{Base: int64(pts.Seconds() * 90000)}
	}

	_, err := t.writer.WriteData(&mpegts.MuxerData{
		PID:             256,
		AdaptationField: af,
		PES: &mpegts.PESData{
			Header: &mpegts.PESHeader{
				OptionalHeader: oh,
				StreamID:       224, // video
			},
			Data: enc,
		},
	})
	if err != nil {
		return err
	}

	if t.startPTS == nil {
		t.startPTS = &pts
	}
	t.endPTS = pts // save endPTS in case next write fails
	return nil
}

func (t *MuxerTSSegment) writeAAC(
	startPCR time.Time,
	pts time.Duration,
	enc []byte,
	ausLen int) error {
	af := &mpegts.PacketAdaptationField{
		RandomAccessIndicator: true,
	}

	if t.videoTrack == nil {
		// send PCR once in a while
		if t.pcrSendCounter == 0 {
			af.HasPCR = true
			af.PCR = &mpegts.ClockReference{Base: int64(time.Since(startPCR).Seconds() * 90000)}
			t.pcrSendCounter = 3
		}
	}

	_, err := t.writer.WriteData(&mpegts.MuxerData{
		PID:             257,
		AdaptationField: af,
		PES: &mpegts.PESData{
			Header: &mpegts.PESHeader{
				OptionalHeader: &mpegts.PESOptionalHeader{
					MarkerBits:      2,
					PTSDTSIndicator: mpegts.PTSDTSIndicatorOnlyPTS,
					PTS:             &mpegts.ClockReference{Base: int64(pts.Seconds() * 90000)},
				},
				PacketLength: uint16(len(enc) + 8),
				StreamID:     192, // audio
			},
			Data: enc,
		},
	})
	if err != nil {
		return err
	}

	if t.videoTrack == nil {
		t.audioAUCount += ausLen
	}

	if t.startPTS == nil {
		t.startPTS = &pts
	}
	t.endPTS = pts // save endPTS in case next write fails
	return nil
}
