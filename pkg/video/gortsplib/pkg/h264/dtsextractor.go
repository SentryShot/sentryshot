package h264

import (
	"errors"
	"fmt"
	"nvr/pkg/video/gortsplib/pkg/bits"
	"time"
)

// Errors.
var (
	ErrNotEnoughOrderCountBits    = errors.New("not enough bits")
	ErrFrameMbsOnlyUnsupported    = errors.New("unsupported")
	ErrPicOrderCntTypeUnsupported = errors.New("pic_order_cnt_type = 1 is unsupported")
	ErrPocMissing                 = errors.New("POC not found")
	ErrInvalidFramePoc            = errors.New("invalid frame POC")
	ErrSpsInvalid                 = errors.New("invalid SPS")
	ErrSpsNotReceivedYet          = errors.New("SPS not received yet")
)

func getPictureOrderCount(buf []byte, sps *SPS) (uint32, error) {
	if len(buf) < 6 {
		return 0, ErrNotEnoughOrderCountBits
	}

	buf = AntiCompetitionRemove(buf[:6])

	buf = buf[1:]
	pos := 0

	_, err := bits.ReadGolombUnsigned(buf, &pos) // first_mb_in_slice
	if err != nil {
		return 0, err
	}

	_, err = bits.ReadGolombUnsigned(buf, &pos) // slice_type
	if err != nil {
		return 0, err
	}

	_, err = bits.ReadGolombUnsigned(buf, &pos) // pic_parameter_set_id

	if err != nil {
		return 0, err
	}

	_, err = bits.ReadBits(buf, &pos, int(sps.Log2MaxFrameNumMinus4+4)) // frame_num
	if err != nil {
		return 0, err
	}

	if !sps.FrameMbsOnlyFlag {
		return 0, ErrFrameMbsOnlyUnsupported
	}

	picOrderCntLsb, err := bits.ReadBits(buf, &pos, int(sps.Log2MaxPicOrderCntLsbMinus4+4))
	if err != nil {
		return 0, err
	}

	return uint32(picOrderCntLsb), nil
}

func findPictureOrderCount(au [][]byte, sps *SPS) (uint32, error) {
	for _, nalu := range au {
		typ := NALUType(nalu[0] & 0x1F)
		if typ == NALUTypeIDR || typ == NALUTypeNonIDR {
			poc, err := getPictureOrderCount(nalu, sps)
			if err != nil {
				return 0, err
			}
			return poc, nil
		}
	}
	return 0, ErrPocMissing
}

func getPictureOrderCountDiff(poc1 uint32, poc2 uint32, sps *SPS) int32 {
	diff := int32(poc1) - int32(poc2)
	switch {
	case diff < -((1 << (sps.Log2MaxPicOrderCntLsbMinus4 + 3)) - 1):
		diff += (1 << (sps.Log2MaxPicOrderCntLsbMinus4 + 4))

	case diff > ((1 << (sps.Log2MaxPicOrderCntLsbMinus4 + 3)) - 1):
		diff -= (1 << (sps.Log2MaxPicOrderCntLsbMinus4 + 4))
	}
	return diff
}

// DTSExtractor allows to extract DTS from PTS.
type DTSExtractor struct {
	spsp            *SPS
	prevDTSFilled   bool
	prevDTS         time.Duration
	expectedPOC     uint32
	reorderedFrames int
	pauseDTS        int
	pocIncrement    int
}

// NewDTSExtractor allocates a DTSExtractor.
func NewDTSExtractor() *DTSExtractor {
	return &DTSExtractor{
		pocIncrement: 2,
	}
}

func (d *DTSExtractor) extractInner(au [][]byte, pts time.Duration) (time.Duration, error) { //nolint:funlen
	idrPresent := false

	for _, nalu := range au {
		typ := NALUType(nalu[0] & 0x1F)
		switch typ {
		case NALUTypeSPS:
			var spsp SPS
			err := spsp.Unmarshal(nalu)
			if err != nil {
				return 0, fmt.Errorf("invalid SPS: %w", err)
			}
			d.spsp = &spsp

		case NALUTypeIDR:
			idrPresent = true
		}
	}

	if d.spsp == nil {
		return 0, ErrSpsNotReceivedYet
	}

	if d.spsp.PicOrderCntType == 2 {
		return pts, nil
	}

	if d.spsp.PicOrderCntType == 1 {
		return 0, ErrPicOrderCntTypeUnsupported
	}

	if idrPresent {
		d.expectedPOC = 0
		d.reorderedFrames = 0
		d.pauseDTS = 0
		d.pocIncrement = 2
		return pts, nil
	}

	d.expectedPOC += uint32(d.pocIncrement)
	d.expectedPOC &= ((1 << (d.spsp.Log2MaxPicOrderCntLsbMinus4 + 4)) - 1)

	if d.pauseDTS > 0 {
		d.pauseDTS--
		return d.prevDTS + 1*time.Millisecond, nil
	}

	poc, err := findPictureOrderCount(au, d.spsp)
	if err != nil {
		return 0, err
	}

	if d.pocIncrement == 2 && (poc%2) != 0 {
		d.pocIncrement = 1
		d.expectedPOC /= 2
	}

	pocDiff := int(getPictureOrderCountDiff(poc, d.expectedPOC, d.spsp)) + d.reorderedFrames*d.pocIncrement

	if pocDiff < 0 {
		return 0, ErrInvalidFramePoc
	}

	if pocDiff == 0 {
		return pts, nil
	}

	reorderedFrames := (pocDiff - d.reorderedFrames*d.pocIncrement) / d.pocIncrement
	if reorderedFrames > d.reorderedFrames {
		d.pauseDTS = (reorderedFrames - d.reorderedFrames - 1)
		d.reorderedFrames = reorderedFrames
		return d.prevDTS + 1*time.Millisecond, nil
	}

	return d.prevDTS + ((pts - d.prevDTS) * time.Duration(d.pocIncrement) / time.Duration(pocDiff+d.pocIncrement)), nil
}

// DtsIncreasingError .
type DtsIncreasingError struct {
	Was time.Duration
	Is  time.Duration
}

func (e DtsIncreasingError) Error() string {
	return fmt.Sprintf("DTS is not monotonically increasing was %v, now is %v", e.Was, e.Is)
}

// Extract extracts the DTS of a group of NALUs.
func (d *DTSExtractor) Extract(
	nalus [][]byte,
	pts time.Duration,
) (time.Duration, error) {
	dts, err := d.extractInner(nalus, pts)
	if err != nil {
		return 0, err
	}

	if dts > pts {
		return 0, DtsIncreasingError{Was: d.prevDTS, Is: dts}
	}

	d.prevDTS = dts
	d.prevDTSFilled = true

	return dts, err
}
