package mp4

import (
	"log"
	"nvr/pkg/video/mp4/bitio"
)

/************************* FullBox **************************/

// FullBox is ISOBMFF FullBox.
type FullBox struct {
	Version uint8
	Flags   [3]byte
}

// GetFlags returns the flags.
func (b *FullBox) GetFlags() uint32 {
	flag := uint32(b.Flags[0]) << 16
	flag ^= uint32(b.Flags[1]) << 8
	flag ^= uint32(b.Flags[2])
	return flag
}

// CheckFlag checks the flag status.
func (b *FullBox) CheckFlag(flag uint32) bool {
	return b.GetFlags()&flag != 0
}

// FieldSize returns the marshaled size in bytes.
func (*FullBox) FieldSize() int {
	return 4
}

// MarshalField box to writer.
func (b *FullBox) MarshalField(w *bitio.Writer) error {
	w.TryWriteByte(b.Version)
	w.TryWriteByte(b.Flags[0])
	w.TryWriteByte(b.Flags[1])
	w.TryWriteByte(b.Flags[2])
	return w.TryError
}

/*************************** btrt ****************************/

// TypeBtrt BoxType.
func TypeBtrt() BoxType { return [4]byte{'b', 't', 'r', 't'} }

// Btrt ?
type Btrt struct {
	BufferSizeDB uint32
	MaxBitrate   uint32
	AvgBitrate   uint32
}

// Type returns the BoxType.
func (*Btrt) Type() BoxType { return TypeBtrt() }

// Size returns the marshaled size in bytes.
func (*Btrt) Size() int {
	return 12
}

// Marshal box to writer.
func (b *Btrt) Marshal(w *bitio.Writer) error {
	w.TryWriteUint32(b.BufferSizeDB)
	w.TryWriteUint32(b.MaxBitrate)
	w.TryWriteUint32(b.AvgBitrate)
	return w.TryError
}

/*************************** ctts ****************************/

// TypeCtts BoxType.
func TypeCtts() BoxType { return [4]byte{'c', 't', 't', 's'} }

// Ctts is ISOBMFF ctts box type.
type Ctts struct {
	FullBox
	EntryCount uint32
	Entries    []CttsEntry
}

// CttsEntry .
type CttsEntry struct {
	SampleCount    uint32
	SampleOffsetV0 uint32
	SampleOffsetV1 int32
}

// Type returns the BoxType.
func (*Ctts) Type() BoxType { return TypeCtts() }

// Size returns the marshaled size in bytes.
func (b *Ctts) Size() int {
	return 8 + len(b.Entries)*8
}

// Marshal is never called.
func (b *Ctts) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	w.TryWriteUint32(b.EntryCount)
	for _, entry := range b.Entries {
		w.TryWriteUint32(entry.SampleCount)
		if b.FullBox.Version == 0 {
			w.TryWriteUint32(entry.SampleOffsetV0)
		} else {
			w.TryWriteUint32(uint32(entry.SampleOffsetV1))
		}
	}
	return nil
}

/*************************** dinf ****************************/

// TypeDinf BoxType.
func TypeDinf() BoxType { return [4]byte{'d', 'i', 'n', 'f'} }

// Dinf is ISOBMFF dinf box type.
type Dinf struct{}

// Type returns the BoxType.
func (*Dinf) Type() BoxType { return TypeDinf() }

// Size returns the marshaled size in bytes.
func (*Dinf) Size() int { return 0 }

// Marshal is never called.
func (b *Dinf) Marshal(w *bitio.Writer) error { return nil }

/*************************** dref ****************************/

// TypeDref BoxType.
func TypeDref() BoxType { return [4]byte{'d', 'r', 'e', 'f'} }

// Dref is ISOBMFF dref box type.
type Dref struct {
	FullBox
	EntryCount uint32
}

// Type returns the BoxType.
func (*Dref) Type() BoxType { return TypeDref() }

// Size returns the marshaled size in bytes.
func (*Dref) Size() int {
	return 8
}

// Marshal box to writer.
func (b *Dref) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	return w.WriteUint32(b.EntryCount)
}

/*************************** url ****************************/

// TypeURL BoxType.
func TypeURL() BoxType { return [4]byte{'u', 'r', 'l', ' '} }

// URL is ISOBMFF url box type.
type URL struct {
	FullBox
	Location string
}

// Type returns the BoxType.
func (*URL) Type() BoxType { return TypeURL() }

// Size returns the marshaled size in bytes.
func (b *URL) Size() int {
	if !b.FullBox.CheckFlag(urlNopt) {
		return len(b.Location) + 5
	}
	return 4
}

const urlNopt = 0x000001

// Marshal box to writer.
func (b *URL) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	if !b.FullBox.CheckFlag(urlNopt) {
		_, err := w.Write([]byte(b.Location + "\000"))
		return err
	}
	return nil
}

/*************************** edts ****************************/

// TypeEdts BoxType.
func TypeEdts() BoxType { return [4]byte{'e', 'd', 't', 's'} }

// Edts is ISOBMFF edts box type.
type Edts struct{}

// Type returns the BoxType.
func (*Edts) Type() BoxType { return TypeEdts() }

// Size returns the marshaled size in bytes.
func (*Edts) Size() int { return 0 }

// Marshal is never called.
func (b *Edts) Marshal(w *bitio.Writer) error { return nil }

/*************************** elst ****************************/

// TypeElts BoxType.
func TypeElts() BoxType { return [4]byte{'e', 'l', 't', 's'} }

// Elst is ISOBMFF elst box type.
type Elst struct {
	FullBox
	EntryCount uint32
	Entries    []ElstEntry
}

// ElstEntry .
type ElstEntry struct {
	SegmentDurationV0 uint32
	MediaTimeV0       int32
	SegmentDurationV1 uint64
	MediaTimeV1       int64
	MediaRateInteger  int16
	MediaRateFraction int16
}

// Type returns the BoxType.
func (*Elst) Type() BoxType { return TypeElts() }

// Size returns the marshaled size in bytes.
func (b *Elst) Size() int {
	if b.FullBox.Version == 0 {
		return 8 + len(b.Entries)*12
	}
	return 8 + len(b.Entries)*20
}

// Marshal box to writer.
func (b *Elst) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	w.TryWriteUint32(b.EntryCount)
	for _, entry := range b.Entries {
		if b.FullBox.Version == 0 {
			w.TryWriteUint32(entry.SegmentDurationV0)
			w.TryWriteUint32(uint32(entry.MediaTimeV0))
		} else {
			w.TryWriteUint64(entry.SegmentDurationV1)
			w.TryWriteUint64(uint64(entry.MediaTimeV1))
		}
		w.TryWriteUint16(uint16(entry.MediaRateInteger))
		w.TryWriteUint16(uint16(entry.MediaRateFraction))
	}
	return w.TryError
}

/*************************** esds ****************************/

// TypeEsds BoxType.
func TypeEsds() BoxType { return [4]byte{'e', 's', 'd', 's'} }

// https://developer.apple.com/library/content/documentation/QuickTime/QTFF/QTFFChap3/qtff3.html
const (
	ESDescrTag            = 0x03
	DecoderConfigDescrTag = 0x04
	DecSpecificInfoTag    = 0x05
	SLConfigDescrTag      = 0x06
)

/*************************** free ****************************/

// TypeFree BoxType.
func TypeFree() BoxType { return [4]byte{'f', 'r', 'e', 'e'} }

// Free is ISOBMFF free box type.
type Free struct{}

// Type returns the BoxType.
func (*Free) Type() BoxType { return TypeFree() }

// Size returns the marshaled size in bytes.
func (*Free) Size() int { return 0 }

// Marshal is never called.
func (*Free) Marshal(w *bitio.Writer) error { return nil }

/*************************** ftyp ****************************/

// TypeFtyp BoxType.
func TypeFtyp() BoxType { return [4]byte{'f', 't', 'y', 'p'} }

// Ftyp is ISOBMFF ftyp box type.
type Ftyp struct {
	MajorBrand       [4]byte
	MinorVersion     uint32
	CompatibleBrands []CompatibleBrandElem
}

// CompatibleBrandElem .
type CompatibleBrandElem struct {
	CompatibleBrand [4]byte
}

// Type returns the BoxType.
func (*Ftyp) Type() BoxType { return TypeFtyp() }

// Size returns the marshaled size in bytes.
func (b *Ftyp) Size() int {
	total := len(b.MajorBrand) + 4
	total += len(b.CompatibleBrands) * 4
	return total
}

// Marshal box to writer.
func (b *Ftyp) Marshal(w *bitio.Writer) error {
	w.TryWrite(b.MajorBrand[:])
	w.TryWriteUint32(b.MinorVersion)
	for _, brands := range b.CompatibleBrands {
		w.TryWrite(brands.CompatibleBrand[:])
	}
	return w.TryError
}

/*************************** hdlr ****************************/

// TypeHdlr BoxType.
func TypeHdlr() BoxType { return [4]byte{'h', 'd', 'l', 'r'} }

// Hdlr is ISOBMFF hdlr box type.
type Hdlr struct {
	FullBox
	// Predefined corresponds to component_type of QuickTime.
	// pre_defined of ISO-14496 has albufays zero,
	// hobufever component_type has "mhlr" or "dhlr".
	PreDefined  uint32
	HandlerType [4]byte
	Reserved    [3]uint32
	Name        string
}

// Type returns the BoxType.
func (*Hdlr) Type() BoxType { return TypeHdlr() }

// Size returns the marshaled size in bytes.
func (b *Hdlr) Size() int {
	total := len(b.HandlerType) + 9
	total += len(b.Reserved) * 4
	total += len(b.Name)
	return total
}

// Marshal box to writer.
func (b *Hdlr) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	w.TryWriteUint32(b.PreDefined)
	w.TryWrite(b.HandlerType[:])
	for _, reserved := range b.Reserved {
		w.TryWriteUint32(reserved)
	}
	w.TryWrite([]byte(b.Name + "\000"))
	return w.TryError
}

/*************************** mdat ****************************/

// TypeMdat BoxType.
func TypeMdat() BoxType { return [4]byte{'m', 'd', 'a', 't'} }

// Mdat is ISOBMFF mdat box type.
type Mdat struct {
	Data []byte
}

// Type returns the BoxType.
func (*Mdat) Type() BoxType { return TypeMdat() }

// Size returns the marshaled size in bytes.
func (b *Mdat) Size() int {
	return len(b.Data)
}

// Marshal box to writer.
func (b *Mdat) Marshal(w *bitio.Writer) error {
	_, err := w.Write(b.Data)
	return err
}

/*************************** mdhd ****************************/

// TypeMdhd BoxType.
func TypeMdhd() BoxType { return [4]byte{'m', 'd', 'h', 'd'} }

// Mdhd is ISOBMFF mdhd box type.
type Mdhd struct {
	FullBox
	CreationTimeV0     uint32
	ModificationTimeV0 uint32
	CreationTimeV1     uint64
	ModificationTimeV1 uint64
	Timescale          uint32
	DurationV0         uint32
	DurationV1         uint64
	//
	Pad        bool    // 1 bit.
	Language   [3]byte // 5 bits. ISO-639-2/T language code
	PreDefined uint16
}

// Type returns the BoxType.
func (*Mdhd) Type() BoxType { return TypeMdhd() }

// Size returns the marshaled size in bytes.
func (b *Mdhd) Size() int {
	if b.FullBox.Version == 0 {
		return 24
	}
	return 36
}

// Marshal box to writer.
func (b *Mdhd) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	if b.FullBox.Version == 0 {
		w.TryWriteUint32(b.CreationTimeV0)
		w.TryWriteUint32(b.ModificationTimeV0)
	} else {
		w.TryWriteUint64(b.CreationTimeV1)
		w.TryWriteUint64(b.ModificationTimeV1)
	}
	w.TryWriteUint32(b.Timescale)
	if b.FullBox.Version == 0 {
		w.TryWriteUint32(b.DurationV0)
	} else {
		w.TryWriteUint64(b.DurationV1)
	}
	if b.Pad {
		w.TryWriteByte(byte(0x1)<<7 | b.Language[0]&0x1f<<2 | b.Language[1]&0x1f>>3)
	} else {
		w.TryWriteByte(b.Language[0]&0x1f<<2 | b.Language[1]&0x1f>>3)
	}
	w.TryWriteByte(b.Language[1]<<5 | b.Language[2]&0x1f)
	w.TryWriteUint16(b.PreDefined)
	return w.TryError
}

/*************************** mdia ****************************/

// TypeMdia BoxType.
func TypeMdia() BoxType { return [4]byte{'m', 'd', 'i', 'a'} }

// Mdia is ISOBMFF mdia box type.
type Mdia struct{}

// Type returns the BoxType.
func (*Mdia) Type() BoxType { return TypeMdia() }

// Size returns the marshaled size in bytes.
func (*Mdia) Size() int { return 0 }

// Marshal is never called.
func (*Mdia) Marshal(w *bitio.Writer) error { return nil }

/*************************** meta ****************************/

// TypeMeta BoxType.
func TypeMeta() BoxType { return [4]byte{'m', 'e', 't', 'a'} }

// Meta is ISOBMFF meta box type.
type Meta struct {
	FullBox
}

// Type returns the BoxType.
func (*Meta) Type() BoxType { return TypeMeta() }

// Size returns the marshaled size in bytes.
func (*Meta) Size() int {
	return 4
}

// Marshal is never called.
func (b *Meta) Marshal(w *bitio.Writer) error {
	return b.FullBox.MarshalField(w)
}

/*************************** mfhd ****************************/

// TypeMfhd BoxType.
func TypeMfhd() BoxType { return [4]byte{'m', 'f', 'h', 'd'} }

// Mfhd is ISOBMFF mfhd box type.
type Mfhd struct {
	FullBox
	SequenceNumber uint32
}

// Type returns the BoxType.
func (*Mfhd) Type() BoxType { return TypeMfhd() }

// Size returns the marshaled size in bytes.
func (*Mfhd) Size() int {
	return 8
}

// Marshal box to writer.
func (b *Mfhd) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	return w.WriteUint32(b.SequenceNumber)
}

/*************************** minf ****************************/

// TypeMinf BoxType.
func TypeMinf() BoxType { return [4]byte{'m', 'i', 'n', 'f'} }

// Minf is ISOBMFF minf box type.
type Minf struct{}

// Type returns the BoxType.
func (*Minf) Type() BoxType { return TypeMinf() }

// Size returns the marshaled size in bytes.
func (*Minf) Size() int { return 0 }

// Marshal is never called.
func (b *Minf) Marshal(w *bitio.Writer) error { return nil }

/*************************** moof ****************************/

// TypeMoof BoxType.
func TypeMoof() BoxType { return [4]byte{'m', 'o', 'o', 'f'} }

// Moof is ISOBMFF moof box type.
type Moof struct{}

// Type returns the BoxType.
func (*Moof) Type() BoxType { return TypeMoof() }

// Size returns the marshaled size in bytes.
func (*Moof) Size() int { return 0 }

// Marshal is never called.
func (b *Moof) Marshal(w *bitio.Writer) error { return nil }

/*************************** moov ****************************/

// TypeMoov BoxType.
func TypeMoov() BoxType { return [4]byte{'m', 'o', 'o', 'v'} }

// Moov is ISOBMFF moov box type.
type Moov struct{}

// Type returns the BoxType.
func (*Moov) Type() BoxType { return TypeMoov() }

// Size returns the marshaled size in bytes.
func (*Moov) Size() int { return 0 }

// Marshal is never called.
func (b *Moov) Marshal(w *bitio.Writer) error { return nil }

/*************************** mvex ****************************/

// TypeMvex BoxType.
func TypeMvex() BoxType { return [4]byte{'m', 'v', 'e', 'x'} }

// Mvex is ISOBMFF mvex box type.
type Mvex struct{}

// Type returns the BoxType.
func (*Mvex) Type() BoxType { return TypeMvex() }

// Size returns the marshaled size in bytes.
func (*Mvex) Size() int { return 0 }

// Marshal is never called.
func (b *Mvex) Marshal(w *bitio.Writer) error { return nil }

/*************************** mvhd ****************************/

// TypeMvhd BoxType.
func TypeMvhd() BoxType { return [4]byte{'m', 'v', 'h', 'd'} }

// Mvhd is ISOBMFF mvhd box type.
type Mvhd struct {
	FullBox
	CreationTimeV0     uint32
	ModificationTimeV0 uint32
	CreationTimeV1     uint64
	ModificationTimeV1 uint64
	Timescale          uint32
	DurationV0         uint32
	DurationV1         uint64
	Rate               int32 // fixed-point 16.16 - template=0x00010000
	Volume             int16 // template=0x0100
	Reserved           int16
	Reserved2          [2]uint32
	Matrix             [9]int32 // template={ 0x00010000,0,0,0,0x00010000,0,0,0,0x40000000 }
	PreDefined         [6]int32
	NextTrackID        uint32
}

// Type returns the BoxType.
func (*Mvhd) Type() BoxType { return TypeMvhd() }

// Size returns the marshaled size in bytes.
func (b *Mvhd) Size() int {
	if b.FullBox.Version == 0 {
		return 100
	}
	return 112
}

// Marshal box to writer.
func (b *Mvhd) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	if b.FullBox.Version == 0 {
		w.TryWriteUint32(b.CreationTimeV0)
		w.TryWriteUint32(b.ModificationTimeV0)
	} else {
		w.TryWriteUint64(b.CreationTimeV1)
		w.TryWriteUint64(b.ModificationTimeV1)
	}
	w.TryWriteUint32(b.Timescale)
	if b.FullBox.Version == 0 {
		w.TryWriteUint32(b.DurationV0)
	} else {
		w.TryWriteUint64(b.DurationV1)
	}
	w.TryWriteUint32(uint32(b.Rate))
	w.TryWriteUint16(uint16(b.Volume))
	w.TryWriteUint16(uint16(b.Reserved))
	for _, reserved := range b.Reserved2 {
		w.TryWriteUint32(reserved)
	}
	for _, matrix := range b.Matrix {
		w.TryWriteUint32(uint32(matrix))
	}
	for _, preDefined := range b.PreDefined {
		w.TryWriteUint32(uint32(preDefined))
	}
	w.TryWriteUint32(b.NextTrackID)
	return w.TryError
}

/*********************** SampleEntry *************************/

// SampleEntry .
type SampleEntry struct {
	Reserved           [6]uint8
	DataReferenceIndex uint16
}

// Marshal entry to buffer.
func (b *SampleEntry) Marshal(w *bitio.Writer) error {
	for _, reserved := range b.Reserved {
		w.TryWriteByte(reserved)
	}
	w.TryWriteUint16(b.DataReferenceIndex)
	return w.TryError
}

/*********************** avc1 *************************/

// TypeAvc1 BoxType.
func TypeAvc1() BoxType { return [4]byte{'a', 'v', 'c', '1'} }

// Avc1 is ISOBMFF AVC box type.
type Avc1 struct {
	SampleEntry
	PreDefined      uint16
	Reserved        uint16
	PreDefined2     [3]uint32
	Width           uint16
	Height          uint16
	Horizresolution uint32
	Vertresolution  uint32
	Reserved2       uint32
	FrameCount      uint16
	Compressorname  [32]byte
	Depth           uint16
	PreDefined3     int16
}

// Type returns the BoxType.
func (*Avc1) Type() BoxType { return TypeAvc1() }

// Size returns the marshaled size in bytes.
func (*Avc1) Size() int {
	return 78
}

// Marshal box to writer.
func (b *Avc1) Marshal(w *bitio.Writer) error {
	err := b.SampleEntry.Marshal(w)
	if err != nil {
		return err
	}
	w.TryWriteUint16(b.PreDefined)
	w.TryWriteUint16(b.Reserved)
	for _, preDefined := range b.PreDefined2 {
		w.TryWriteUint32(preDefined)
	}
	w.TryWriteUint16(b.Width)
	w.TryWriteUint16(b.Height)
	w.TryWriteUint32(b.Horizresolution)
	w.TryWriteUint32(b.Vertresolution)
	w.TryWriteUint32(b.Reserved2)
	w.TryWriteUint16(b.FrameCount)
	w.TryWrite(b.Compressorname[:])
	w.TryWriteUint16(b.Depth)
	w.TryWriteUint16(uint16(b.PreDefined3))
	return w.TryError
}

/*********************** mp4a *************************/

// TypeMp4a BoxType.
func TypeMp4a() BoxType { return [4]byte{'m', 'p', '4', 'a'} }

// Mp4a ?
type Mp4a struct {
	SampleEntry
	EntryVersion uint16
	Reserved     [3]uint16
	ChannelCount uint16
	SampleSize   uint16
	PreDefined   uint16
	Reserved2    uint16
	SampleRate   uint32
}

// Type returns the BoxType.
func (*Mp4a) Type() BoxType { return TypeMp4a() }

// Size returns the marshaled size in bytes.
func (*Mp4a) Size() int {
	return 28
}

// Marshal box to writer.
func (b *Mp4a) Marshal(w *bitio.Writer) error {
	err := b.SampleEntry.Marshal(w)
	if err != nil {
		return err
	}
	w.TryWriteUint16(b.EntryVersion)
	for _, reserved := range b.Reserved {
		w.TryWriteUint16(reserved)
	}
	w.TryWriteUint16(b.ChannelCount)
	w.TryWriteUint16(b.SampleSize)
	w.TryWriteUint16(b.PreDefined)
	w.TryWriteUint16(b.Reserved2)
	w.TryWriteUint32(b.SampleRate)
	return w.TryError
}

/**************** AVCDecoderConfiguration ****************.*/
const (
	AVCBaselineProfile uint8 = 66  // 0x42
	AVCMainProfile     uint8 = 77  // 0x4d
	AVCExtendedProfile uint8 = 88  // 0x58
	AVCHighProfile     uint8 = 100 // 0x64
	AVCHigh10Profile   uint8 = 110 // 0x6e
	AVCHigh422Profile  uint8 = 122 // 0x7a
)

// AVCParameterSet .
type AVCParameterSet struct {
	Length  uint16
	NALUnit []byte
}

// FieldSize returns the marshaled size in bytes.
func (b *AVCParameterSet) FieldSize() int {
	return len(b.NALUnit) + 2
}

// MarshalField box to writer.
func (b *AVCParameterSet) MarshalField(w *bitio.Writer) error {
	w.TryWriteUint16(b.Length)
	w.TryWrite(b.NALUnit)
	return w.TryError
}

/*************************** avcC ****************************/

// TypeAvcC BoxType.
func TypeAvcC() BoxType { return [4]byte{'a', 'v', 'c', 'C'} }

// AvcC is ISOBMFF AVC configuration box type.
type AvcC struct {
	ConfigurationVersion         uint8
	Profile                      uint8
	ProfileCompatibility         uint8
	Level                        uint8
	Reserved                     uint8 // 6 bits.
	LengthSizeMinusOne           uint8 // 2 bits.
	Reserved2                    uint8 // 3 bits.
	NumOfSequenceParameterSets   uint8 // 5 bits.
	SequenceParameterSets        []AVCParameterSet
	NumOfPictureParameterSets    uint8
	PictureParameterSets         []AVCParameterSet
	HighProfileFieldsEnabled     bool
	Reserved3                    uint8 // 6 bits.
	ChromaFormat                 uint8 // 2 bits.
	Reserved4                    uint8 // 5 bits.
	BitDepthLumaMinus8           uint8 // 3 bits.
	Reserved5                    uint8 // 5 bits.
	BitDepthChromaMinus8         uint8 // 3 bits.
	NumOfSequenceParameterSetExt uint8
	SequenceParameterSetsExt     []AVCParameterSet
}

// Type returns the BoxType.
func (*AvcC) Type() BoxType { return TypeAvcC() }

// Size returns the marshaled size in bytes.
func (b *AvcC) Size() int {
	total := 7
	for _, sets := range b.SequenceParameterSets {
		total += sets.FieldSize()
	}
	for _, sets := range b.PictureParameterSets {
		total += sets.FieldSize()
	}
	if b.Reserved3 != 0 {
		total += 4
		for _, sets := range b.SequenceParameterSetsExt {
			total += sets.FieldSize()
		}
	}
	return total
}

// Marshal box to writer.
func (b *AvcC) Marshal(w *bitio.Writer) error {
	w.TryWriteByte(b.ConfigurationVersion)
	w.TryWriteByte(b.Profile)
	w.TryWriteByte(b.ProfileCompatibility)
	w.TryWriteByte(b.Level)
	w.TryWriteByte(b.Reserved<<2 | b.LengthSizeMinusOne&0x3)
	w.TryWriteByte(b.Reserved2<<5 | b.NumOfSequenceParameterSets&0x1f)
	for _, sets := range b.SequenceParameterSets {
		err := sets.MarshalField(w)
		if err != nil {
			return err
		}
	}
	w.TryWriteByte(b.NumOfPictureParameterSets)
	for _, sets := range b.PictureParameterSets {
		err := sets.MarshalField(w)
		if err != nil {
			return err
		}
	}
	if b.HighProfileFieldsEnabled &&
		b.Profile != AVCHighProfile &&
		b.Profile != AVCHigh10Profile &&
		b.Profile != AVCHigh422Profile &&
		b.Profile != 144 {
		log.Fatal("fmp4 each values of Profile and" +
			" HighProfileFieldsEnabled are inconsistent")
	}
	if b.Reserved3 != 0 {
		w.TryWriteByte(b.Reserved3<<2 | b.ChromaFormat&0x3)
		w.TryWriteByte(b.Reserved4<<3 | b.BitDepthLumaMinus8&0x7)
		w.TryWriteByte(b.Reserved5<<3 | b.BitDepthChromaMinus8&0x7)
		w.TryWriteByte(b.NumOfSequenceParameterSetExt)
		for _, sets := range b.SequenceParameterSetsExt {
			err := sets.MarshalField(w)
			if err != nil {
				return err
			}
		}
	}
	return w.TryError
}

/*************************** smhd ****************************/

// TypeSmhd BoxType.
func TypeSmhd() BoxType { return [4]byte{'s', 'm', 'h', 'd'} }

// Smhd is ISOBMFF smhd box type.
type Smhd struct {
	FullBox
	Balance  int16 // fixed-point 8.8 template=0
	Reserved uint16
}

// Type returns the BoxType.
func (*Smhd) Type() BoxType { return TypeSmhd() }

// Size returns the marshaled size in bytes.
func (*Smhd) Size() int {
	return 8
}

// Marshal box to writer.
func (b *Smhd) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	w.TryWriteUint16(uint16(b.Balance))
	w.TryWriteUint16(b.Reserved)
	return w.TryError
}

/*************************** stbl ****************************/

// TypeStbl BoxType.
func TypeStbl() BoxType { return [4]byte{'s', 't', 'b', 'l'} }

// Stbl is ISOBMFF stbl box type.
type Stbl struct{}

// Type returns the BoxType.
func (*Stbl) Type() BoxType { return TypeStbl() }

// Size returns the marshaled size in bytes.
func (*Stbl) Size() int { return 0 }

// Marshal is never called.
func (*Stbl) Marshal(w *bitio.Writer) error { return nil }

/*************************** stco ****************************/

// TypeStco BoxType.
func TypeStco() BoxType { return [4]byte{'s', 't', 'c', 'o'} }

// Stco is ISOBMFF stco box type.
type Stco struct {
	FullBox
	EntryCount  uint32
	ChunkOffset []uint32
}

// Type returns the BoxType.
func (*Stco) Type() BoxType { return TypeStco() }

// Size returns the marshaled size in bytes.
func (b *Stco) Size() int {
	return 8 + len(b.ChunkOffset)*4
}

// Marshal box to writer.
func (b *Stco) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	w.TryWriteUint32(b.EntryCount)
	for _, offset := range b.ChunkOffset {
		w.TryWriteUint32(offset)
	}
	return w.TryError
}

/*************************** stsc ****************************/

// TypeStsc BoxType.
func TypeStsc() BoxType { return [4]byte{'s', 't', 's', 'c'} }

// StscEntry .
type StscEntry struct {
	FirstChunk             uint32
	SamplesPerChunk        uint32
	SampleDescriptionIndex uint32
}

// MarshalField entry to buffer.
func (b *StscEntry) MarshalField(w *bitio.Writer) error {
	w.TryWriteUint32(b.FirstChunk)
	w.TryWriteUint32(b.SamplesPerChunk)
	w.TryWriteUint32(b.SampleDescriptionIndex)
	return w.TryError
}

// Stsc is ISOBMFF stsc box type.
type Stsc struct {
	FullBox
	EntryCount uint32
	Entries    []StscEntry
}

// Type returns the BoxType.
func (*Stsc) Type() BoxType { return TypeStsc() }

// Size returns the marshaled size in bytes.
func (b *Stsc) Size() int {
	return 8 + len(b.Entries)*12
}

// Marshal box to writer.
func (b *Stsc) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	err = w.WriteUint32(b.EntryCount)
	if err != nil {
		return err
	}
	for _, entry := range b.Entries {
		err := entry.MarshalField(w)
		if err != nil {
			return err
		}
	}
	return nil
}

/*************************** stsd ****************************/

// TypeStsd BoxType.
func TypeStsd() BoxType { return [4]byte{'s', 't', 's', 'd'} }

// Stsd is ISOBMFF stsd box type.
type Stsd struct {
	FullBox
	EntryCount uint32
}

// Type returns the BoxType.
func (*Stsd) Type() BoxType { return TypeStsd() }

// Size returns the marshaled size in bytes.
func (*Stsd) Size() int {
	return 8
}

// Marshal box to writer.
func (b *Stsd) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return nil
	}
	return w.WriteUint32(b.EntryCount)
}

/*************************** stss ****************************/

// TypeStss BoxType.
func TypeStss() BoxType { return [4]byte{'s', 't', 's', 's'} }

// Stss is ISOBMFF stss box type.
type Stss struct {
	FullBox
	EntryCount   uint32
	SampleNumber []uint32
}

// Type returns the BoxType.
func (*Stss) Type() BoxType { return TypeStss() }

// Size returns the marshaled size in bytes.
func (b *Stss) Size() int {
	return 8 + len(b.SampleNumber)*4
}

// Marshal is never called.
func (b *Stss) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	err = w.WriteUint32(b.EntryCount)
	if err != nil {
		return err
	}
	for _, number := range b.SampleNumber {
		err := w.WriteUint32(number)
		if err != nil {
			return err
		}
	}
	return nil
}

/*************************** stsz ****************************/

// TypeStsz BoxType.
func TypeStsz() BoxType { return [4]byte{'s', 't', 's', 'z'} }

// Stsz is ISOBMFF stsz box type.
type Stsz struct {
	FullBox
	SampleSize  uint32
	SampleCount uint32
	EntrySize   []uint32
}

// Type returns the BoxType.
func (*Stsz) Type() BoxType { return TypeStsz() }

// Size returns the marshaled size in bytes.
func (b *Stsz) Size() int {
	return 12 + len(b.EntrySize)*4
}

// Marshal box to writer.
func (b *Stsz) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	w.TryWriteUint32(b.SampleSize)
	w.TryWriteUint32(b.SampleCount)
	for _, entry := range b.EntrySize {
		w.TryWriteUint32(entry)
	}
	return w.TryError
}

/*************************** stts ****************************/

// TypeStts BoxType.
func TypeStts() BoxType { return [4]byte{'s', 't', 't', 's'} }

// Stts is ISOBMFF stts box type.
type Stts struct {
	FullBox
	EntryCount uint32
	Entries    []SttsEntry
}

// SttsEntry .
type SttsEntry struct {
	SampleCount uint32
	SampleDelta uint32
}

// Marshal entry to buffer.
func (b *SttsEntry) Marshal(w *bitio.Writer) error {
	w.TryWriteUint32(b.SampleCount)
	w.TryWriteUint32(b.SampleDelta)
	return w.TryError
}

// Type returns the BoxType.
func (*Stts) Type() BoxType { return TypeStts() }

// Size returns the marshaled size in bytes.
func (b *Stts) Size() int {
	return 8 + len(b.Entries)*8
}

// Marshal box to writer.
func (b *Stts) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	err = w.WriteUint32(b.EntryCount)
	if err != nil {
		return err
	}
	for _, entry := range b.Entries {
		err := entry.Marshal(w)
		if err != nil {
			return err
		}
	}
	return nil
}

/*************************** tfdt ****************************/

// TypeTfdt BoxType.
func TypeTfdt() BoxType { return [4]byte{'t', 'f', 'd', 't'} }

// Tfdt is ISOBMFF tfdt box type.
type Tfdt struct {
	FullBox
	BaseMediaDecodeTimeV0 uint32
	BaseMediaDecodeTimeV1 uint64
}

// Type returns the BoxType.
func (*Tfdt) Type() BoxType { return TypeTfdt() }

// Size returns the marshaled size in bytes.
func (b *Tfdt) Size() int {
	total := b.FullBox.FieldSize()
	if b.FullBox.Version == 0 {
		total += 4
	} else {
		total += 8
	}
	return total
}

// Marshal box to writer.
func (b *Tfdt) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	if b.FullBox.Version == 0 {
		err = w.WriteUint32(b.BaseMediaDecodeTimeV0)
	} else {
		err = w.WriteUint64(b.BaseMediaDecodeTimeV1)
	}
	return err
}

/*************************** tfhd ****************************/

// TypeTfhd BoxType.
func TypeTfhd() BoxType { return [4]byte{'t', 'f', 'h', 'd'} }

// Tfhd is ISOBMFF tfhd box type.
type Tfhd struct {
	FullBox
	TrackID uint32

	// optional
	BaseDataOffset         uint64
	SampleDescriptionIndex uint32
	DefaultSampleDuration  uint32
	DefaultSampleSize      uint32
	DefaultSampleFlags     uint32
}

// tfhd flags.
const (
	TfhdBaseDataOffsetPresent         = 0x000001
	TfhdSampleDescriptionIndexPresent = 0x000002
	TfhdDefaultSampleDurationPresent  = 0x000008
	TfhdDefaultSampleSizePresent      = 0x000010
	TfhdDefaultSampleFlagsPresent     = 0x000020
)

// Type returns the BoxType.
func (*Tfhd) Type() BoxType { return TypeTfhd() }

// Size returns the marshaled size in bytes.
func (b *Tfhd) Size() int {
	total := b.FullBox.FieldSize() + 4
	if b.FullBox.CheckFlag(TfhdBaseDataOffsetPresent) {
		total += 8
	}
	if b.FullBox.CheckFlag(TfhdSampleDescriptionIndexPresent) {
		total += 4
	}
	if b.FullBox.CheckFlag(TfhdDefaultSampleDurationPresent) {
		total += 4
	}
	if b.FullBox.CheckFlag(TfhdDefaultSampleSizePresent) {
		total += 4
	}
	if b.FullBox.CheckFlag(TfhdDefaultSampleFlagsPresent) {
		total += 4
	}
	return total
}

// Marshal box to writer.
func (b *Tfhd) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	w.TryWriteUint32(b.TrackID)
	if b.FullBox.CheckFlag(TfhdBaseDataOffsetPresent) {
		w.TryWriteUint64(b.BaseDataOffset)
	}
	if b.FullBox.CheckFlag(TfhdSampleDescriptionIndexPresent) {
		w.TryWriteUint32(b.SampleDescriptionIndex)
	}
	if b.FullBox.CheckFlag(TfhdDefaultSampleDurationPresent) {
		w.TryWriteUint32(b.DefaultSampleDuration)
	}
	if b.FullBox.CheckFlag(TfhdDefaultSampleSizePresent) {
		w.TryWriteUint32(b.DefaultSampleSize)
	}
	if b.FullBox.CheckFlag(TfhdDefaultSampleFlagsPresent) {
		w.TryWriteUint32(b.DefaultSampleFlags)
	}
	return w.TryError
}

/*************************** tkhd ****************************/

// TypeTkhd BoxType.
func TypeTkhd() BoxType { return [4]byte{'t', 'k', 'h', 'd'} }

// Tkhd is ISOBMFF tkhd box type.
type Tkhd struct {
	FullBox
	CreationTimeV0     uint32
	ModificationTimeV0 uint32
	CreationTimeV1     uint64
	ModificationTimeV1 uint64
	TrackID            uint32
	Reserved0          uint32
	DurationV0         uint32
	DurationV1         uint64

	Reserved1      [2]uint32
	Layer          int16 // template=0
	AlternateGroup int16 // template=0
	Volume         int16 // template={if track_is_audio 0x0100 else 0}
	Reserved2      uint16
	Matrix         [9]int32 // template={ 0x00010000,0,0,0,0x00010000,0,0,0,0x40000000 };
	Width          uint32   // fixed-point 16.16
	Height         uint32   // fixed-point 16.16
}

// Type returns the BoxType.
func (*Tkhd) Type() BoxType { return TypeTkhd() }

// Size returns the marshaled size in bytes.
func (b *Tkhd) Size() int {
	if b.FullBox.Version == 0 {
		return 84
	}
	return 96
}

// Marshal box to writer.
func (b *Tkhd) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	if b.FullBox.Version == 0 {
		w.TryWriteUint32(b.CreationTimeV0)
		w.TryWriteUint32(b.ModificationTimeV0)
	} else {
		w.TryWriteUint64(b.CreationTimeV1)
		w.TryWriteUint64(b.ModificationTimeV1)
	}
	w.TryWriteUint32(b.TrackID)
	w.TryWriteUint32(b.Reserved0)
	if b.FullBox.Version == 0 {
		w.TryWriteUint32(b.DurationV0)
	} else {
		w.TryWriteUint64(b.DurationV1)
	}
	for _, reserved := range b.Reserved1 {
		w.TryWriteUint32(reserved)
	}
	w.TryWriteUint16(uint16(b.Layer))
	w.TryWriteUint16(uint16(b.AlternateGroup))
	w.TryWriteUint16(uint16(b.Volume))
	w.TryWriteUint16(b.Reserved2)
	for _, matrix := range b.Matrix {
		w.TryWriteUint32(uint32(matrix))
	}
	w.TryWriteUint32(b.Width)
	w.TryWriteUint32(b.Height)
	return w.TryError
}

/*************************** traf ****************************/

// TypeTraf BoxType.
func TypeTraf() BoxType { return [4]byte{'t', 'r', 'a', 'f'} }

// Traf is ISOBMFF traf box type.
type Traf struct{}

// Type returns the BoxType.
func (*Traf) Type() BoxType { return TypeTraf() }

// Size returns the marshaled size in bytes.
func (*Traf) Size() int { return 0 }

// Marshal is never called.
func (*Traf) Marshal(w *bitio.Writer) error { return nil }

/*************************** trak ****************************/

// TypeTrak BoxType.
func TypeTrak() BoxType { return [4]byte{'t', 'r', 'a', 'k'} }

// Trak is ISOBMFF trak box type.
type Trak struct{}

// Type returns the BoxType.
func (*Trak) Type() BoxType { return TypeTrak() }

// Size returns the marshaled size in bytes.
func (*Trak) Size() int { return 0 }

// Marshal is never called.
func (*Trak) Marshal(w *bitio.Writer) error { return nil }

/*************************** trex ****************************/

// TypeTrex BoxType.
func TypeTrex() BoxType { return [4]byte{'t', 'r', 'e', 'x'} }

// Trex is ISOBMFF trex box type.
type Trex struct {
	FullBox
	TrackID                       uint32
	DefaultSampleDescriptionIndex uint32
	DefaultSampleDuration         uint32
	DefaultSampleSize             uint32
	DefaultSampleFlags            uint32
}

// Type returns the BoxType.
func (*Trex) Type() BoxType { return TypeTrex() }

// Size returns the marshaled size in bytes.
func (*Trex) Size() int {
	return 24
}

// Marshal box to writer.
func (b *Trex) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	w.TryWriteUint32(b.TrackID)
	w.TryWriteUint32(b.DefaultSampleDescriptionIndex)
	w.TryWriteUint32(b.DefaultSampleDuration)
	w.TryWriteUint32(b.DefaultSampleSize)
	w.TryWriteUint32(b.DefaultSampleFlags)
	return nil
}

/*************************** trun ****************************/

// TrunEntry .
type TrunEntry struct {
	SampleDuration                uint32
	SampleSize                    uint32
	SampleFlags                   uint32
	SampleCompositionTimeOffsetV0 uint32
	SampleCompositionTimeOffsetV1 int32
}

// trun flags.
const (
	TrunDataOffsetPresent                  = 0x000001
	TrunFirstSampleFlagsPresent            = 0x000004
	TrunSampleDurationPresent              = 0x000100
	TrunSampleSizePresent                  = 0x000200
	TrunSampleFlagsPresent                 = 0x000400
	TrunSampleCompositionTimeOffsetPresent = 0x000800
)

// FieldSize returns the marshaled size in bytes.
func (b *TrunEntry) FieldSize(fullBox FullBox) int {
	total := 0
	if fullBox.CheckFlag(TrunSampleDurationPresent) {
		total += 4
	}
	if fullBox.CheckFlag(TrunSampleSizePresent) {
		total += 4
	}
	if fullBox.CheckFlag(TrunSampleFlagsPresent) {
		total += 4
	}
	if fullBox.CheckFlag(TrunSampleCompositionTimeOffsetPresent) {
		total += 4
	}
	return total
}

// MarshalField entry to buffer.
func (b *TrunEntry) MarshalField(w *bitio.Writer, fullBox FullBox) error {
	if fullBox.CheckFlag(TrunSampleDurationPresent) {
		w.TryWriteUint32(b.SampleDuration)
	}
	if fullBox.CheckFlag(TrunSampleSizePresent) {
		w.TryWriteUint32(b.SampleSize)
	}
	if fullBox.CheckFlag(TrunSampleFlagsPresent) {
		w.TryWriteUint32(b.SampleFlags)
	}
	if fullBox.CheckFlag(TrunSampleCompositionTimeOffsetPresent) {
		if fullBox.Version == 0 {
			w.TryWriteUint32(b.SampleCompositionTimeOffsetV0)
		} else {
			w.TryWriteUint32(uint32(b.SampleCompositionTimeOffsetV1))
		}
	}
	return w.TryError
}

// TypeTrun BoxType.
func TypeTrun() BoxType { return [4]byte{'t', 'r', 'u', 'n'} }

// Trun is ISOBMFF trun box type.
type Trun struct {
	FullBox
	SampleCount uint32

	// optional fields
	DataOffset       int32
	FirstSampleFlags uint32
	Entries          []TrunEntry
}

// Type returns the BoxType.
func (*Trun) Type() BoxType { return TypeTrun() }

// Size returns the marshaled size in bytes.
func (b *Trun) Size() int {
	total := 8
	if b.FullBox.CheckFlag(TrunDataOffsetPresent) {
		total += 4
	}
	if b.FullBox.CheckFlag(TrunFirstSampleFlagsPresent) {
		total += 4
	}
	for _, entry := range b.Entries {
		total += entry.FieldSize(b.FullBox)
	}
	return total
}

// Marshal box to writer.
func (b *Trun) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	w.TryWriteUint32(b.SampleCount)
	if b.FullBox.CheckFlag(TrunDataOffsetPresent) {
		w.TryWriteUint32(uint32(b.DataOffset))
	}
	if b.FullBox.CheckFlag(TrunFirstSampleFlagsPresent) {
		w.TryWriteUint32(b.FirstSampleFlags)
	}
	if w.TryError != nil {
		return nil
	}
	for _, entry := range b.Entries {
		err := entry.MarshalField(w, b.FullBox)
		if err != nil {
			return err
		}
	}
	return nil
}

/*************************** udta ****************************/

// TypeUdta BoxType.
func TypeUdta() BoxType { return [4]byte{'u', 'd', 't', 'a'} }

// Udta is ISOBMFF udta box type.
type Udta struct{}

// Type returns the BoxType.
func (*Udta) Type() BoxType { return TypeUdta() }

// Size returns the marshaled size in bytes.
func (*Udta) Size() int { return 0 }

// Marshal is never called.
func (*Udta) Marshal(w *bitio.Writer) error { return nil }

/*************************** vmhd ****************************/

// TypeVmhd BoxType.
func TypeVmhd() BoxType { return [4]byte{'v', 'm', 'h', 'd'} }

// Vmhd is ISOBMFF vmhd box type.
type Vmhd struct {
	FullBox
	Graphicsmode uint16    // template=0
	Opcolor      [3]uint16 // template={0, 0, 0}
}

// Type returns the BoxType.
func (*Vmhd) Type() BoxType { return TypeVmhd() }

// Size returns the marshaled size in bytes.
func (*Vmhd) Size() int {
	return 12
}

// Marshal box to writer.
func (b *Vmhd) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}
	w.TryWriteUint16(b.Graphicsmode)
	for _, color := range b.Opcolor {
		w.TryWriteUint16(color)
	}
	return w.TryError
}
