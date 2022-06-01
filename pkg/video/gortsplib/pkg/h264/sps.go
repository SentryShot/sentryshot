package h264

import (
	"bytes"
	"errors"

	"github.com/icza/bitio"
)

func readGolombUnsigned(br *bitio.Reader) (uint32, error) {
	leadingZeroBits := uint32(0)

	for {
		b, err := br.ReadBits(1)
		if err != nil {
			return 0, err
		}

		if b != 0 {
			break
		}

		leadingZeroBits++
	}

	codeNum := uint32(0)

	for n := leadingZeroBits; n > 0; n-- {
		b, err := br.ReadBits(1)
		if err != nil {
			return 0, err
		}

		codeNum |= uint32(b) << (n - 1)
	}

	codeNum = (1 << leadingZeroBits) - 1 + codeNum

	return codeNum, nil
}

func readGolombSigned(br *bitio.Reader) (int32, error) {
	v, err := readGolombUnsigned(br)
	if err != nil {
		return 0, err
	}
	vi := int32(v)

	if (vi & 0x01) != 0 {
		return (vi + 1) / 2, nil
	}

	return -vi / 2, nil
}

func readFlag(br *bitio.Reader) (bool, error) {
	tmp, err := br.ReadBits(1)
	if err != nil {
		return false, err
	}

	return (tmp == 1), nil
}

func readScalingList(br *bitio.Reader, size int) ([]int32, bool, error) {
	lastScale := int32(8)
	nextScale := int32(8)
	scalingList := make([]int32, size)
	var useDefaultScalingMatrixFlag bool

	for j := 0; j < size; j++ {
		if nextScale != 0 {
			deltaScale, err := readGolombSigned(br)
			if err != nil {
				return nil, false, err
			}

			nextScale = (lastScale + deltaScale + 256) % 256
			useDefaultScalingMatrixFlag = (j == 0 && nextScale == 0)
		}

		if nextScale == 0 {
			scalingList[j] = lastScale
		} else {
			scalingList[j] = nextScale
		}

		lastScale = scalingList[j]
	}

	return scalingList, useDefaultScalingMatrixFlag, nil
}

// SpsHrd is a hypotetical reference decoder.
type SpsHrd struct {
	CpbCntMinus1                       uint32
	BitRateScale                       uint8
	CpbSizeScale                       uint8
	BitRateValueMinus1                 []uint32
	CpbSizeValueMinus1                 []uint32
	CbrFlag                            []bool
	InitialCpbRemovalDelayLengthMinus1 uint8
	CpbRemovalDelayLengthMinus1        uint8
	DpbOutputDelayLengthMinus1         uint8
	TimeOffsetLength                   uint8
}

func (h *SpsHrd) unmarshal(br *bitio.Reader) error { //nolint:funlen
	var err error
	h.CpbCntMinus1, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}

	tmp, err := br.ReadBits(4)
	if err != nil {
		return err
	}
	h.BitRateScale = uint8(tmp)

	tmp, err = br.ReadBits(4)
	if err != nil {
		return err
	}
	h.CpbSizeScale = uint8(tmp)

	for i := uint32(0); i <= h.CpbCntMinus1; i++ {
		v, err := readGolombUnsigned(br)
		if err != nil {
			return err
		}
		h.BitRateValueMinus1 = append(h.BitRateValueMinus1, v)

		v, err = readGolombUnsigned(br)
		if err != nil {
			return err
		}
		h.CpbSizeValueMinus1 = append(h.CpbSizeValueMinus1, v)

		vb, err := readFlag(br)
		if err != nil {
			return err
		}
		h.CbrFlag = append(h.CbrFlag, vb)
	}

	tmp, err = br.ReadBits(5)
	if err != nil {
		return err
	}
	h.InitialCpbRemovalDelayLengthMinus1 = uint8(tmp)

	tmp, err = br.ReadBits(5)
	if err != nil {
		return err
	}
	h.CpbRemovalDelayLengthMinus1 = uint8(tmp)

	tmp, err = br.ReadBits(5)
	if err != nil {
		return err
	}
	h.DpbOutputDelayLengthMinus1 = uint8(tmp)

	tmp, err = br.ReadBits(5)
	if err != nil {
		return err
	}
	h.TimeOffsetLength = uint8(tmp)

	return nil
}

// SpsVui is a video usability information.
type SpsVui struct {
	AspectRatioInfoPresentFlag bool
	AspectRatioIdc             uint8
	SarWidth                   uint16
	SarHeight                  uint16
	OverscanInfoPresentFlag    bool
	OverscanAppropriateFlag    bool
	VideoSignalTypePresentFlag bool

	// VideoSignalTypePresentFlag == true
	VideoFormat                  uint8
	VideoFullRangeFlag           bool
	ColourDescriptionPresentFlag bool

	// ColourDescriptionPresentFlag == true
	ColourPrimaries         uint8
	TransferCharacteristics uint8
	MatrixCoefficients      uint8

	ChromaLocInfoPresentFlag bool

	// ChromaLocInfoPresentFlag == true
	ChromaSampleLocTypeTopField    uint32
	ChromaSampleLocTypeBottomField uint32

	TimingInfoPresentFlag bool

	// TimingInfoPresentFlag == true
	NumUnitsInTick     uint32
	TimeScale          uint32
	FixedFrameRateFlag bool

	// nalHrdParametersPresentFlag == true
	NalHRD *SpsHrd

	// vclHrdParametersPresentFlag == true
	VclHRD *SpsHrd

	LowDelayHrdFlag          bool
	PicStructPresentFlag     bool
	BitstreamRestrictionFlag bool

	// BitstreamRestrictionFlag == true
	MotionVectorsOverPicBoundariesFlag bool
	MaxBytesPerPicDenom                uint32
	MaxBitsPerMbDenom                  uint32
	Log2MaxMvLengthHorizontal          uint32
	Log2MaxMvLengthVertical            uint32
	MaxNumReorderFrames                uint32
	MaxDecFrameBuffering               uint32
}

func (v *SpsVui) unmarshal(br *bitio.Reader) error { //nolint:funlen,gocognit
	var err error
	v.AspectRatioInfoPresentFlag, err = readFlag(br)
	if err != nil {
		return err
	}

	if v.AspectRatioInfoPresentFlag {
		err = v.unmarshalAspectRatioInfo(br)
		if err != nil {
			return err
		}
	}

	v.OverscanInfoPresentFlag, err = readFlag(br)
	if err != nil {
		return err
	}

	if v.OverscanInfoPresentFlag {
		v.OverscanAppropriateFlag, err = readFlag(br)
		if err != nil {
			return err
		}
	}

	v.VideoSignalTypePresentFlag, err = readFlag(br)
	if err != nil {
		return err
	}

	if v.VideoSignalTypePresentFlag {
		err = v.unmarshalVideoSignalType(br)
		if err != nil {
			return err
		}
	}

	v.ChromaLocInfoPresentFlag, err = readFlag(br)
	if err != nil {
		return err
	}

	if v.ChromaLocInfoPresentFlag {
		v.ChromaSampleLocTypeTopField, err = readGolombUnsigned(br)
		if err != nil {
			return err
		}

		v.ChromaSampleLocTypeBottomField, err = readGolombUnsigned(br)
		if err != nil {
			return err
		}
	}

	v.TimingInfoPresentFlag, err = readFlag(br)
	if err != nil {
		return err
	}

	if v.TimingInfoPresentFlag {
		err = v.unmarshalTimingInfo(br)
		if err != nil {
			return err
		}
	}

	nalHrdParametersPresentFlag, err := readFlag(br)
	if err != nil {
		return err
	}

	if nalHrdParametersPresentFlag {
		v.NalHRD = &SpsHrd{}
		err := v.NalHRD.unmarshal(br)
		if err != nil {
			return err
		}
	}

	vclHrdParametersPresentFlag, err := readFlag(br)
	if err != nil {
		return err
	}

	if vclHrdParametersPresentFlag {
		v.VclHRD = &SpsHrd{}
		err := v.VclHRD.unmarshal(br)
		if err != nil {
			return err
		}
	}

	if nalHrdParametersPresentFlag || vclHrdParametersPresentFlag {
		v.LowDelayHrdFlag, err = readFlag(br)
		if err != nil {
			return err
		}
	}

	v.PicStructPresentFlag, err = readFlag(br)
	if err != nil {
		return err
	}

	v.BitstreamRestrictionFlag, err = readFlag(br)
	if err != nil {
		return err
	}

	if v.BitstreamRestrictionFlag {
		err = v.unmarshalBitstreamRestriction(br)
		if err != nil {
			return err
		}
	}

	return nil
}

func (v *SpsVui) unmarshalAspectRatioInfo(br *bitio.Reader) error {
	tmp, err := br.ReadBits(8)
	if err != nil {
		return err
	}
	v.AspectRatioIdc = uint8(tmp)

	if v.AspectRatioIdc == 255 { // Extended_SAR
		tmp, err := br.ReadBits(16)
		if err != nil {
			return err
		}
		v.SarWidth = uint16(tmp)

		tmp, err = br.ReadBits(16)
		if err != nil {
			return err
		}
		v.SarHeight = uint16(tmp)
	}
	return nil
}

func (v *SpsVui) unmarshalVideoSignalType(br *bitio.Reader) error {
	tmp, err := br.ReadBits(3)
	if err != nil {
		return err
	}
	v.VideoFormat = uint8(tmp)

	v.VideoFullRangeFlag, err = readFlag(br)
	if err != nil {
		return err
	}

	v.ColourDescriptionPresentFlag, err = readFlag(br)
	if err != nil {
		return err
	}

	if v.ColourDescriptionPresentFlag {
		tmp, err := br.ReadBits(8)
		if err != nil {
			return err
		}
		v.ColourPrimaries = uint8(tmp)

		tmp, err = br.ReadBits(8)
		if err != nil {
			return err
		}
		v.TransferCharacteristics = uint8(tmp)

		tmp, err = br.ReadBits(8)
		if err != nil {
			return err
		}
		v.MatrixCoefficients = uint8(tmp)
	}

	return nil
}

func (v *SpsVui) unmarshalTimingInfo(br *bitio.Reader) error {
	tmp, err := br.ReadBits(32)
	if err != nil {
		return err
	}
	v.NumUnitsInTick = uint32(tmp)

	tmp, err = br.ReadBits(32)
	if err != nil {
		return err
	}
	v.TimeScale = uint32(tmp)

	v.FixedFrameRateFlag, err = readFlag(br)
	if err != nil {
		return err
	}
	return nil
}

func (v *SpsVui) unmarshalBitstreamRestriction(br *bitio.Reader) error {
	var err error
	v.MotionVectorsOverPicBoundariesFlag, err = readFlag(br)
	if err != nil {
		return err
	}

	v.MaxBytesPerPicDenom, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}

	v.MaxBitsPerMbDenom, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}

	v.Log2MaxMvLengthHorizontal, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}

	v.Log2MaxMvLengthVertical, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}

	v.MaxNumReorderFrames, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}

	v.MaxDecFrameBuffering, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}
	return nil
}

// SpsFramecropping is the frame cropping part of a SPS.
type SpsFramecropping struct {
	LeftOffset   uint32
	RightOffset  uint32
	TopOffset    uint32
	BottomOffset uint32
}

func (c *SpsFramecropping) unmarshal(br *bitio.Reader) error {
	var err error
	c.LeftOffset, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}

	c.RightOffset, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}

	c.TopOffset, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}

	c.BottomOffset, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}

	return nil
}

// SPS is a H264 sequence parameter set.
type SPS struct {
	ProfileIdc         uint8
	ConstraintSet0Flag bool
	ConstraintSet1Flag bool
	ConstraintSet2Flag bool
	ConstraintSet3Flag bool
	ConstraintSet4Flag bool
	ConstraintSet5Flag bool
	LevelIdc           uint8
	ID                 uint32

	// only for selected ProfileIdcs
	ChromeFormatIdc                 uint32
	SeparateColourPlaneFlag         bool
	BitDepthLumaMinus8              uint32
	BitDepthChromaMinus8            uint32
	QpprimeYZeroTransformBypassFlag bool

	// seqScalingListPresentFlag == true
	ScalingList4x4                 [][]int32
	UseDefaultScalingMatrix4x4Flag []bool
	ScalingList8x8                 [][]int32
	UseDefaultScalingMatrix8x8Flag []bool

	Log2MaxFrameNumMinus4 uint32
	PicOrderCntType       uint32

	// PicOrderCntType == 0
	Log2MaxPicOrderCntLsbMinus4 uint32

	// PicOrderCntType == 1
	DeltaPicOrderAlwaysZeroFlag bool
	OffsetForNonRefPic          int32
	OffsetForTopToBottomField   int32
	OffsetForRefFrames          []int32

	MaxNumRefFrames                uint32
	GapsInFrameNumValueAllowedFlag bool
	PicWidthInMbsMinus1            uint32
	PicHeightInMbsMinus1           uint32
	FrameMbsOnlyFlag               bool

	// FrameMbsOnlyFlag == false
	MbAdaptiveFrameFieldFlag bool

	Direct8x8InferenceFlag bool

	// frameCroppingFlag == true
	FrameCropping *SpsFramecropping

	// vuiParameterPresentFlag == true
	VUI *SpsVui
}

// SPS errors.
var (
	ErrSPSBufferTooShort    = errors.New("buffer too short")
	ErrSPSWrongForbiddenBit = errors.New("wrong forbidden bit")
	ErrSPSWrongNalRefIdc    = errors.New("wrong nal_ref_idc")
	ErrSPSWrongType         = errors.New("not a SPS")
)

// Unmarshal decodes a SPS from bytes.
func (s *SPS) Unmarshal(buf []byte) error { //nolint:funlen
	// ref: ISO/IEC 14496-10:2020

	buf = AntiCompetitionRemove(buf)

	if len(buf) < 4 {
		return ErrSPSBufferTooShort
	}

	forbidden := buf[0] >> 7
	nalRefIdc := (buf[0] >> 5) & 0x03
	typ := NALUType(buf[0] & 0x1F)

	if forbidden != 0 {
		return ErrSPSWrongForbiddenBit
	}

	if nalRefIdc != 3 {
		return ErrSPSWrongForbiddenBit
	}

	if typ != NALUTypeSPS {
		return ErrSPSWrongType
	}

	s.ProfileIdc = buf[1]
	s.ConstraintSet0Flag = (buf[2] >> 7) == 1
	s.ConstraintSet1Flag = (buf[2] >> 6 & 0x01) == 1
	s.ConstraintSet2Flag = (buf[2] >> 5 & 0x01) == 1
	s.ConstraintSet3Flag = (buf[2] >> 4 & 0x01) == 1
	s.ConstraintSet4Flag = (buf[2] >> 3 & 0x01) == 1
	s.ConstraintSet5Flag = (buf[2] >> 2 & 0x01) == 1
	s.LevelIdc = buf[3]

	r := bytes.NewReader(buf[4:])
	br := bitio.NewReader(r)

	var err error
	s.ID, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}

	err = s.unmarshalProfileIdc(br)
	if err != nil {
		return err
	}

	s.Log2MaxFrameNumMinus4, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}

	s.PicOrderCntType, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}

	err = s.unmarshalPicOrderCnt(br)
	if err != nil {
		return err
	}

	s.MaxNumRefFrames, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}

	s.GapsInFrameNumValueAllowedFlag, err = readFlag(br)
	if err != nil {
		return err
	}

	s.PicWidthInMbsMinus1, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}

	s.PicHeightInMbsMinus1, err = readGolombUnsigned(br)
	if err != nil {
		return err
	}

	s.FrameMbsOnlyFlag, err = readFlag(br)
	if err != nil {
		return err
	}

	if !s.FrameMbsOnlyFlag {
		s.MbAdaptiveFrameFieldFlag, err = readFlag(br)
		if err != nil {
			return err
		}
	} else {
		s.MbAdaptiveFrameFieldFlag = false
	}

	s.Direct8x8InferenceFlag, err = readFlag(br)
	if err != nil {
		return err
	}

	frameCroppingFlag, err := readFlag(br)
	if err != nil {
		return err
	}

	if frameCroppingFlag {
		s.FrameCropping = &SpsFramecropping{}
		err := s.FrameCropping.unmarshal(br)
		if err != nil {
			return err
		}
	} else {
		s.FrameCropping = nil
	}

	vuiParameterPresentFlag, err := readFlag(br)
	if err != nil {
		return err
	}

	if vuiParameterPresentFlag {
		s.VUI = &SpsVui{}
		err := s.VUI.unmarshal(br)
		if err != nil {
			return err
		}
	} else {
		s.VUI = nil
	}

	return nil
}

func (s *SPS) unmarshalProfileIdc(br *bitio.Reader) error {
	var err error
	switch s.ProfileIdc {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
		s.ChromeFormatIdc, err = readGolombUnsigned(br)
		if err != nil {
			return err
		}

		if s.ChromeFormatIdc == 3 {
			s.SeparateColourPlaneFlag, err = readFlag(br)
			if err != nil {
				return err
			}
		} else {
			s.SeparateColourPlaneFlag = false
		}

		s.BitDepthLumaMinus8, err = readGolombUnsigned(br)
		if err != nil {
			return err
		}

		s.BitDepthChromaMinus8, err = readGolombUnsigned(br)
		if err != nil {
			return err
		}

		s.QpprimeYZeroTransformBypassFlag, err = readFlag(br)
		if err != nil {
			return err
		}

		seqScalingMatrixPresentFlag, err := readFlag(br)
		if err != nil {
			return err
		}

		if seqScalingMatrixPresentFlag {
			err = s.unmarshalSeqScalingMatrix(br)
			if err != nil {
				return err
			}
		}

	default:
		s.ChromeFormatIdc = 0
		s.SeparateColourPlaneFlag = false
		s.BitDepthLumaMinus8 = 0
		s.BitDepthChromaMinus8 = 0
		s.QpprimeYZeroTransformBypassFlag = false
	}
	return nil
}

func (s *SPS) unmarshalSeqScalingMatrix(br *bitio.Reader) error {
	var lim int
	if s.ChromeFormatIdc != 3 {
		lim = 8
	} else {
		lim = 12
	}

	for i := 0; i < lim; i++ {
		seqScalingListPresentFlag, err := readFlag(br)
		if err != nil {
			return err
		}

		if seqScalingListPresentFlag { //nolint:nestif
			if i < 6 {
				scalingList, useDefaultScalingMatrixFlag, err := readScalingList(br, 16)
				if err != nil {
					return err
				}
				s.ScalingList4x4 = append(s.ScalingList4x4, scalingList)
				s.UseDefaultScalingMatrix4x4Flag = append(s.UseDefaultScalingMatrix4x4Flag,
					useDefaultScalingMatrixFlag)
			} else {
				scalingList, useDefaultScalingMatrixFlag, err := readScalingList(br, 64)
				if err != nil {
					return err
				}
				s.ScalingList8x8 = append(s.ScalingList8x8, scalingList)
				s.UseDefaultScalingMatrix8x8Flag = append(s.UseDefaultScalingMatrix8x8Flag,
					useDefaultScalingMatrixFlag)
			}
		}
	}
	return nil
}

func (s *SPS) unmarshalPicOrderCnt(br *bitio.Reader) error {
	var err error
	switch s.PicOrderCntType {
	case 0:
		s.Log2MaxPicOrderCntLsbMinus4, err = readGolombUnsigned(br)
		if err != nil {
			return err
		}

	case 1:
		s.Log2MaxPicOrderCntLsbMinus4 = 0

		s.DeltaPicOrderAlwaysZeroFlag, err = readFlag(br)
		if err != nil {
			return err
		}

		s.OffsetForNonRefPic, err = readGolombSigned(br)
		if err != nil {
			return err
		}

		s.OffsetForTopToBottomField, err = readGolombSigned(br)
		if err != nil {
			return err
		}

		numRefFramesInPicOrderCntCycle, err := readGolombUnsigned(br)
		if err != nil {
			return err
		}

		s.OffsetForRefFrames = nil
		for i := uint32(0); i < numRefFramesInPicOrderCntCycle; i++ {
			v, err := readGolombSigned(br)
			if err != nil {
				return err
			}

			s.OffsetForRefFrames = append(s.OffsetForRefFrames, v)
		}

	default:
		s.Log2MaxPicOrderCntLsbMinus4 = 0
		s.DeltaPicOrderAlwaysZeroFlag = false
		s.OffsetForNonRefPic = 0
		s.OffsetForTopToBottomField = 0
		s.OffsetForRefFrames = nil
	}
	return nil
}

// Width returns the video width.
func (s SPS) Width() int {
	if s.FrameCropping != nil {
		return int(((s.PicWidthInMbsMinus1 + 1) * 16) - (s.FrameCropping.LeftOffset+s.FrameCropping.RightOffset)*2)
	}

	return int((s.PicWidthInMbsMinus1 + 1) * 16)
}

// Height returns the video height.
func (s SPS) Height() int {
	f := uint32(0)
	if s.FrameMbsOnlyFlag {
		f = 1
	}

	if s.FrameCropping != nil {
		return int(((2 - f) * (s.PicHeightInMbsMinus1 + 1) * 16) - (s.FrameCropping.TopOffset+s.FrameCropping.BottomOffset)*2)
	}

	return int((2 - f) * (s.PicHeightInMbsMinus1 + 1) * 16)
}

// FPS returns the frame per second of the video.
func (s SPS) FPS() float64 {
	if s.VUI == nil {
		return 0
	}

	if !s.VUI.TimingInfoPresentFlag {
		return 0
	}

	return float64(s.VUI.TimeScale) / (2 * float64(s.VUI.NumUnitsInTick))
}
