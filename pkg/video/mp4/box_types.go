package mp4

import "log"

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

// Size returns the marshaled size in bytes.
func (b *FullBox) Size() int {
	return 4
}

// Marshal box to buffer.
func (b *FullBox) Marshal(buf []byte, pos *int) {
	WriteByte(buf, pos, b.Version)
	WriteByte(buf, pos, b.Flags[0])
	WriteByte(buf, pos, b.Flags[1])
	WriteByte(buf, pos, b.Flags[2])
}

/*************************** btrt ****************************/

// Btrt ?
type Btrt struct {
	BufferSizeDB uint32
	MaxBitrate   uint32
	AvgBitrate   uint32
}

// Type returns the BoxType.
func (*Btrt) Type() BoxType {
	return [4]byte{'b', 't', 'r', 't'}
}

// Size returns the marshaled size in bytes.
func (*Btrt) Size() int {
	return 12
}

// Marshal box to buffer.
func (b *Btrt) Marshal(buf []byte, pos *int) {
	WriteUint32(buf, pos, b.BufferSizeDB)
	WriteUint32(buf, pos, b.MaxBitrate)
	WriteUint32(buf, pos, b.AvgBitrate)
}

/*************************** dinf ****************************/

// Dinf is ISOBMFF dinf box type.
type Dinf struct{}

// Type returns the BoxType.
func (*Dinf) Type() BoxType {
	return [4]byte{'d', 'i', 'n', 'f'}
}

// Size returns the marshaled size in bytes.
func (*Dinf) Size() int {
	return 0
}

// Marshal is never called.
func (b *Dinf) Marshal(buf []byte, pos *int) {}

/*************************** dref ****************************/

// Dref is ISOBMFF dref box type.
type Dref struct {
	FullBox
	EntryCount uint32
}

// Type returns the BoxType.
func (*Dref) Type() BoxType {
	return [4]byte{'d', 'r', 'e', 'f'}
}

// Size returns the marshaled size in bytes.
func (b *Dref) Size() int {
	return 8
}

// Marshal box to buffer.
func (b *Dref) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	WriteUint32(buf, pos, b.EntryCount)
}

/*************************** url ****************************/

// Url is ISOBMFF url box type.
type Url struct { // nolint:revive,stylecheck
	FullBox
	Location string
}

// Type returns the BoxType.
func (*Url) Type() BoxType {
	return [4]byte{'u', 'r', 'l', ' '}
}

// Size returns the marshaled size in bytes.
func (b *Url) Size() int {
	if !b.FullBox.CheckFlag(urlNopt) {
		return len(b.Location) + 5
	}
	return 4
}

const urlNopt = 0x000001

// Marshal box to buffer.
func (b *Url) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	if !b.FullBox.CheckFlag(urlNopt) {
		WriteString(buf, pos, b.Location)
	}
}

/*************************** esds ****************************/

// https://developer.apple.com/library/content/documentation/QuickTime/QTFF/QTFFChap3/qtff3.html
const (
	ESDescrTag            = 0x03
	DecoderConfigDescrTag = 0x04
	DecSpecificInfoTag    = 0x05
	SLConfigDescrTag      = 0x06
)

/*************************** ftyp ****************************/

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
func (*Ftyp) Type() BoxType {
	return [4]byte{'f', 't', 'y', 'p'}
}

// Size returns the marshaled size in bytes.
func (b *Ftyp) Size() int {
	total := len(b.MajorBrand) + 4
	total += len(b.CompatibleBrands) * 4
	return total
}

// Marshal box to buffer.
func (b *Ftyp) Marshal(buf []byte, pos *int) {
	Write(buf, pos, b.MajorBrand[:])
	WriteUint32(buf, pos, b.MinorVersion)
	for _, brands := range b.CompatibleBrands {
		Write(buf, pos, brands.CompatibleBrand[:])
	}
}

/*************************** hdlr ****************************/

// Hdlr is ISOBMFF hdlr box type.
type Hdlr struct {
	FullBox `mp4:"0,extend"`
	// Predefined corresponds to component_type of QuickTime.
	// pre_defined of ISO-14496 has albufays zero,
	// hobufever component_type has "mhlr" or "dhlr".
	PreDefined  uint32
	HandlerType [4]byte
	Reserved    [3]uint32
	Name        string
}

// Type returns the BoxType.
func (*Hdlr) Type() BoxType {
	return [4]byte{'h', 'd', 'l', 'r'}
}

// Size returns the marshaled size in bytes.
func (b *Hdlr) Size() int {
	total := len(b.HandlerType) + 9
	total += len(b.Reserved) * 4
	total += len(b.Name)
	return total
}

// Marshal box to buffer.
func (b *Hdlr) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	WriteUint32(buf, pos, b.PreDefined)
	Write(buf, pos, b.HandlerType[:])
	for _, reserved := range b.Reserved {
		WriteUint32(buf, pos, reserved)
	}
	WriteString(buf, pos, b.Name)
}

/*************************** mdat ****************************/

// Mdat is ISOBMFF mdat box type.
type Mdat struct {
	Data []byte
}

// Type returns the BoxType.
func (*Mdat) Type() BoxType {
	return [4]byte{'m', 'd', 'a', 't'}
}

// Size returns the marshaled size in bytes.
func (b *Mdat) Size() int {
	return len(b.Data)
}

// Marshal box to buffer.
func (b *Mdat) Marshal(buf []byte, pos *int) {
	Write(buf, pos, b.Data)
}

/*************************** mdhd ****************************/

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
func (*Mdhd) Type() BoxType {
	return [4]byte{'m', 'd', 'h', 'd'}
}

// Size returns the marshaled size in bytes.
func (b *Mdhd) Size() int {
	if b.FullBox.Version == 0 {
		return 24
	}
	return 36
}

// Marshal box to buffer.
func (b *Mdhd) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	if b.FullBox.Version == 0 {
		WriteUint32(buf, pos, b.CreationTimeV0)
		WriteUint32(buf, pos, b.ModificationTimeV0)
	} else {
		WriteUint64(buf, pos, b.CreationTimeV1)
		WriteUint64(buf, pos, b.ModificationTimeV1)
	}
	WriteUint32(buf, pos, b.Timescale)
	if b.FullBox.Version == 0 {
		WriteUint32(buf, pos, b.DurationV0)
	} else {
		WriteUint64(buf, pos, b.DurationV1)
	}
	if b.Pad {
		WriteByte(buf, pos, byte(0x1)<<7|(b.Language[0]&0x1f)<<2|(b.Language[1]&0x1f)>>3)
	} else {
		WriteByte(buf, pos, (b.Language[0]&0x1f)<<2|(b.Language[1]&0x1f)>>3)
	}
	WriteByte(buf, pos, (b.Language[1]&0x7)<<5|(b.Language[2]&0x1f))
	WriteUint16(buf, pos, b.PreDefined)
}

/*************************** mdia ****************************/

// Mdia is ISOBMFF mdia box type.
type Mdia struct{}

// Type returns the BoxType.
func (*Mdia) Type() BoxType {
	return [4]byte{'m', 'd', 'i', 'a'}
}

// Size returns the marshaled size in bytes.
func (b *Mdia) Size() int {
	return 0
}

// Marshal is never called.
func (b *Mdia) Marshal(buf []byte, pos *int) {
}

/*************************** mfhd ****************************/

// Mfhd is ISOBMFF mfhd box type.
type Mfhd struct {
	FullBox
	SequenceNumber uint32
}

// Type returns the BoxType.
func (*Mfhd) Type() BoxType {
	return [4]byte{'m', 'f', 'h', 'd'}
}

// Size returns the marshaled size in bytes.
func (b *Mfhd) Size() int {
	return 8
}

// Marshal box to buffer.
func (b *Mfhd) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	WriteUint32(buf, pos, b.SequenceNumber)
}

/*************************** minf ****************************/

// Minf is ISOBMFF minf box type.
type Minf struct{}

// Type returns the BoxType.
func (*Minf) Type() BoxType {
	return [4]byte{'m', 'i', 'n', 'f'}
}

// Size returns the marshaled size in bytes.
func (b *Minf) Size() int {
	return 0
}

// Marshal is never called.
func (b *Minf) Marshal(buf []byte, pos *int) {
}

/*************************** moof ****************************/

// Moof is ISOBMFF moof box type.
type Moof struct{}

// Type returns the BoxType.
func (*Moof) Type() BoxType {
	return [4]byte{'m', 'o', 'o', 'f'}
}

// Size returns the marshaled size in bytes.
func (b *Moof) Size() int {
	return 0
}

// Marshal is never called.
func (b *Moof) Marshal(buf []byte, pos *int) {
}

/*************************** moov ****************************/

// Moov is ISOBMFF moov box type.
type Moov struct{}

// Type returns the BoxType.
func (*Moov) Type() BoxType {
	return [4]byte{'m', 'o', 'o', 'v'}
}

// Size returns the marshaled size in bytes.
func (b *Moov) Size() int {
	return 0
}

// Marshal is never called.
func (b *Moov) Marshal(buf []byte, pos *int) {
}

/*************************** mvex ****************************/

// Mvex is ISOBMFF mvex box type.
type Mvex struct{}

// Type returns the BoxType.
func (*Mvex) Type() BoxType {
	return [4]byte{'m', 'v', 'e', 'x'}
}

// Size returns the marshaled size in bytes.
func (b *Mvex) Size() int {
	return 0
}

// Marshal is never called.
func (b *Mvex) Marshal(buf []byte, pos *int) {
}

/*************************** mvhd ****************************/

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
func (*Mvhd) Type() BoxType {
	return [4]byte{'m', 'v', 'h', 'd'}
}

// Size returns the marshaled size in bytes.
func (b *Mvhd) Size() int {
	if b.FullBox.Version == 0 {
		return 100
	}
	return 112
}

// Marshal box to buffer.
func (b *Mvhd) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	if b.FullBox.Version == 0 {
		WriteUint32(buf, pos, b.CreationTimeV0)
		WriteUint32(buf, pos, b.ModificationTimeV0)
	} else {
		WriteUint64(buf, pos, b.CreationTimeV1)
		WriteUint64(buf, pos, b.ModificationTimeV1)
	}
	WriteUint32(buf, pos, b.Timescale)
	if b.FullBox.Version == 0 {
		WriteUint32(buf, pos, b.DurationV0)
	} else {
		WriteUint64(buf, pos, b.DurationV1)
	}
	WriteUint32(buf, pos, uint32(b.Rate))
	WriteUint16(buf, pos, uint16(b.Volume))
	WriteUint16(buf, pos, uint16(b.Reserved))
	for _, reserved := range b.Reserved2 {
		WriteUint32(buf, pos, reserved)
	}
	for _, matrix := range b.Matrix {
		WriteUint32(buf, pos, uint32(matrix))
	}
	for _, preDefined := range b.PreDefined {
		WriteUint32(buf, pos, uint32(preDefined))
	}
	WriteUint32(buf, pos, b.NextTrackID)
}

/*********************** SampleEntry *************************/

// SampleEntry .
type SampleEntry struct {
	Reserved           [6]uint8
	DataReferenceIndex uint16
}

// Marshal entry to buffer.
func (b *SampleEntry) Marshal(buf []byte, pos *int) {
	for _, reserved := range b.Reserved {
		WriteByte(buf, pos, reserved)
	}
	WriteUint16(buf, pos, b.DataReferenceIndex)
}

/*********************** avc1 *************************/

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
func (*Avc1) Type() BoxType {
	return [4]byte{'a', 'v', 'c', '1'}
}

// Size returns the marshaled size in bytes.
func (b *Avc1) Size() int {
	return 78
}

// Marshal box to buffer.
func (b *Avc1) Marshal(buf []byte, pos *int) {
	b.SampleEntry.Marshal(buf, pos)
	WriteUint16(buf, pos, b.PreDefined)
	WriteUint16(buf, pos, b.Reserved)
	for _, preDefined := range b.PreDefined2 {
		WriteUint32(buf, pos, preDefined)
	}
	WriteUint16(buf, pos, b.Width)
	WriteUint16(buf, pos, b.Height)
	WriteUint32(buf, pos, b.Horizresolution)
	WriteUint32(buf, pos, b.Vertresolution)
	WriteUint32(buf, pos, b.Reserved2)
	WriteUint16(buf, pos, b.FrameCount)
	Write(buf, pos, b.Compressorname[:])
	WriteUint16(buf, pos, b.Depth)
	WriteUint16(buf, pos, uint16(b.PreDefined3))
}

/*********************** mp4a *************************/

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
func (*Mp4a) Type() BoxType {
	return [4]byte{'m', 'p', '4', 'a'}
}

// Size returns the marshaled size in bytes.
func (b *Mp4a) Size() int {
	return 28
}

// Marshal box to buffer.
func (b *Mp4a) Marshal(buf []byte, pos *int) {
	b.SampleEntry.Marshal(buf, pos)
	WriteUint16(buf, pos, b.EntryVersion)
	for _, reserved := range b.Reserved {
		WriteUint16(buf, pos, reserved)
	}
	WriteUint16(buf, pos, b.ChannelCount)
	WriteUint16(buf, pos, b.SampleSize)
	WriteUint16(buf, pos, b.PreDefined)
	WriteUint16(buf, pos, b.Reserved2)
	WriteUint32(buf, pos, b.SampleRate)
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

// Size returns the marshaled size in bytes.
func (b *AVCParameterSet) Size() int {
	return len(b.NALUnit) + 2
}

// Marshal box to buffer.
func (b *AVCParameterSet) Marshal(buf []byte, pos *int) {
	WriteUint16(buf, pos, b.Length)
	Write(buf, pos, b.NALUnit)
}

/*************************** avcC ****************************/

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
func (*AvcC) Type() BoxType {
	return [4]byte{'a', 'v', 'c', 'C'}
}

// Size returns the marshaled size in bytes.
func (b *AvcC) Size() int {
	total := 7
	for _, sets := range b.SequenceParameterSets {
		total += sets.Size()
	}
	for _, sets := range b.PictureParameterSets {
		total += sets.Size()
	}
	if b.Reserved3 != 0 {
		total += 4
		for _, sets := range b.SequenceParameterSetsExt {
			total += sets.Size()
		}
	}
	return total
}

// Marshal box to buffer.
func (b *AvcC) Marshal(buf []byte, pos *int) {
	WriteByte(buf, pos, b.ConfigurationVersion)
	WriteByte(buf, pos, b.Profile)
	WriteByte(buf, pos, b.ProfileCompatibility)
	WriteByte(buf, pos, b.Level)
	WriteByte(buf, pos, b.Reserved&0x3f<<2|b.LengthSizeMinusOne&0x3)
	WriteByte(buf, pos, b.Reserved2&0x7<<5|b.NumOfSequenceParameterSets&0x1f)
	for _, sets := range b.SequenceParameterSets {
		sets.Marshal(buf, pos)
	}
	WriteByte(buf, pos, b.NumOfPictureParameterSets)
	for _, sets := range b.PictureParameterSets {
		sets.Marshal(buf, pos)
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
		WriteByte(buf, pos, b.Reserved3&0x3f<<2|b.ChromaFormat&0x3)
		WriteByte(buf, pos, b.Reserved4&0x1f<<3|b.BitDepthLumaMinus8&0x7)
		WriteByte(buf, pos, b.Reserved4&0x1f<<3|b.BitDepthChromaMinus8&0x7)
		WriteByte(buf, pos, b.NumOfSequenceParameterSetExt)
		for _, sets := range b.SequenceParameterSetsExt {
			sets.Marshal(buf, pos)
		}
	}
}

/*************************** smhd ****************************/

// Smhd is ISOBMFF smhd box type.
type Smhd struct {
	FullBox
	Balance  int16 // fixed-point 8.8 template=0
	Reserved uint16
}

// Type returns the BoxType.
func (*Smhd) Type() BoxType {
	return [4]byte{'s', 'm', 'h', 'd'}
}

// Size returns the marshaled size in bytes.
func (b *Smhd) Size() int {
	return 8
}

// Marshal box to buffer.
func (b *Smhd) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	WriteUint16(buf, pos, uint16(b.Balance))
	WriteUint16(buf, pos, b.Reserved)
}

/*************************** stbl ****************************/

// Stbl is ISOBMFF stbl box type.
type Stbl struct{}

// Type returns the BoxType.
func (*Stbl) Type() BoxType {
	return [4]byte{'s', 't', 'b', 'l'}
}

// Size returns the marshaled size in bytes.
func (b *Stbl) Size() int {
	return 0
}

// Marshal is never called.
func (b *Stbl) Marshal(buf []byte, pos *int) {}

/*************************** stco ****************************/

// Stco is ISOBMFF stco box type.
type Stco struct {
	FullBox
	EntryCount  uint32
	ChunkOffset []uint32
}

// Type returns the BoxType.
func (*Stco) Type() BoxType {
	return [4]byte{'s', 't', 'c', 'o'}
}

// Size returns the marshaled size in bytes.
func (b *Stco) Size() int {
	return 8 + len(b.ChunkOffset)*4
}

// Marshal box to buffer.
func (b *Stco) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	WriteUint32(buf, pos, b.EntryCount)
	for _, offset := range b.ChunkOffset {
		WriteUint32(buf, pos, offset)
	}
}

/*************************** stsc ****************************/

// StscEntry .
type StscEntry struct {
	FirstChunk             uint32
	SamplesPerChunk        uint32
	SampleDescriptionIndex uint32
}

// Marshal entry to buffer.
func (b *StscEntry) Marshal(buf []byte, pos *int) {
	WriteUint32(buf, pos, b.FirstChunk)
	WriteUint32(buf, pos, b.SamplesPerChunk)
	WriteUint32(buf, pos, b.SampleDescriptionIndex)
}

// Stsc is ISOBMFF stsc box type.
type Stsc struct {
	FullBox
	EntryCount uint32
	Entries    []StscEntry
}

// Type returns the BoxType.
func (*Stsc) Type() BoxType {
	return [4]byte{'s', 't', 's', 'c'}
}

// Size returns the marshaled size in bytes.
func (b *Stsc) Size() int {
	return 8 + len(b.Entries)*12
}

// Marshal box to buffer.
func (b *Stsc) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	WriteUint32(buf, pos, b.EntryCount)
	for _, entry := range b.Entries {
		entry.Marshal(buf, pos)
	}
}

/*************************** stsd ****************************/

// Stsd is ISOBMFF stsd box type.
type Stsd struct {
	FullBox
	EntryCount uint32
}

// Type returns the BoxType.
func (*Stsd) Type() BoxType {
	return [4]byte{'s', 't', 's', 'd'}
}

// Size returns the marshaled size in bytes.
func (b *Stsd) Size() int {
	return 8
}

// Marshal box to buffer.
func (b *Stsd) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	WriteUint32(buf, pos, b.EntryCount)
}

/*************************** stsz ****************************/

// Stsz is ISOBMFF stsz box type.
type Stsz struct {
	FullBox
	SampleSize  uint32
	SampleCount uint32
	EntrySize   []uint32
}

// Type returns the BoxType.
func (*Stsz) Type() BoxType {
	return [4]byte{'s', 't', 's', 'z'}
}

// Size returns the marshaled size in bytes.
func (b *Stsz) Size() int {
	return 12 + len(b.EntrySize)*4
}

// Marshal box to buffer.
func (b *Stsz) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	WriteUint32(buf, pos, b.SampleSize)
	WriteUint32(buf, pos, b.SampleCount)
	for _, entry := range b.EntrySize {
		WriteUint32(buf, pos, entry)
	}
}

/*************************** stts ****************************/

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
func (b *SttsEntry) Marshal(buf []byte, pos *int) {
	WriteUint32(buf, pos, b.SampleCount)
	WriteUint32(buf, pos, b.SampleDelta)
}

// Type returns the BoxType.
func (*Stts) Type() BoxType {
	return [4]byte{'s', 't', 't', 's'}
}

// Size returns the marshaled size in bytes.
func (b *Stts) Size() int {
	return 8 + len(b.Entries)*8
}

// Marshal box to buffer.
func (b *Stts) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	WriteUint32(buf, pos, b.EntryCount)
	for _, entry := range b.Entries {
		entry.Marshal(buf, pos)
	}
}

/*************************** tfdt ****************************/

// Tfdt is ISOBMFF tfdt box type.
type Tfdt struct {
	FullBox
	BaseMediaDecodeTimeV0 uint32
	BaseMediaDecodeTimeV1 uint64
}

// Type returns the BoxType.
func (*Tfdt) Type() BoxType {
	return [4]byte{'t', 'f', 'd', 't'}
}

// Size returns the marshaled size in bytes.
func (b *Tfdt) Size() int {
	total := b.FullBox.Size()
	if b.FullBox.Version == 0 {
		total += 4
	} else {
		total += 8
	}
	return total
}

// Marshal box to buffer.
func (b *Tfdt) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	if b.FullBox.Version == 0 {
		WriteUint32(buf, pos, b.BaseMediaDecodeTimeV0)
	} else {
		WriteUint64(buf, pos, b.BaseMediaDecodeTimeV1)
	}
}

/*************************** tfhd ****************************/

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
func (*Tfhd) Type() BoxType {
	return [4]byte{'t', 'f', 'h', 'd'}
}

// Size returns the marshaled size in bytes.
func (b *Tfhd) Size() int {
	total := b.FullBox.Size() + 4
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

// Marshal box to buffer.
func (b *Tfhd) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	WriteUint32(buf, pos, b.TrackID)
	if b.FullBox.CheckFlag(TfhdBaseDataOffsetPresent) {
		WriteUint64(buf, pos, b.BaseDataOffset)
	}
	if b.FullBox.CheckFlag(TfhdSampleDescriptionIndexPresent) {
		WriteUint32(buf, pos, b.SampleDescriptionIndex)
	}
	if b.FullBox.CheckFlag(TfhdDefaultSampleDurationPresent) {
		WriteUint32(buf, pos, b.DefaultSampleDuration)
	}
	if b.FullBox.CheckFlag(TfhdDefaultSampleSizePresent) {
		WriteUint32(buf, pos, b.DefaultSampleSize)
	}
	if b.FullBox.CheckFlag(TfhdDefaultSampleFlagsPresent) {
		WriteUint32(buf, pos, b.DefaultSampleFlags)
	}
}

/*************************** tkhd ****************************/

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
func (*Tkhd) Type() BoxType {
	return [4]byte{'t', 'k', 'h', 'd'}
}

// Size returns the marshaled size in bytes.
func (b *Tkhd) Size() int {
	if b.FullBox.Version == 0 {
		return 84
	}
	return 96
}

// Marshal box to buffer.
func (b *Tkhd) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	if b.FullBox.Version == 0 {
		WriteUint32(buf, pos, b.CreationTimeV0)
		WriteUint32(buf, pos, b.ModificationTimeV0)
	} else {
		WriteUint64(buf, pos, b.CreationTimeV1)
		WriteUint64(buf, pos, b.ModificationTimeV1)
	}
	WriteUint32(buf, pos, b.TrackID)
	WriteUint32(buf, pos, b.Reserved0)
	if b.FullBox.Version == 0 {
		WriteUint32(buf, pos, b.DurationV0)
	} else {
		WriteUint64(buf, pos, b.DurationV1)
	}
	for _, reserved := range b.Reserved1 {
		WriteUint32(buf, pos, reserved)
	}
	WriteUint16(buf, pos, uint16(b.Layer))
	WriteUint16(buf, pos, uint16(b.AlternateGroup))
	WriteUint16(buf, pos, uint16(b.Volume))
	WriteUint16(buf, pos, b.Reserved2)
	for _, matrix := range b.Matrix {
		WriteUint32(buf, pos, uint32(matrix))
	}
	WriteUint32(buf, pos, b.Width)
	WriteUint32(buf, pos, b.Height)
}

/*************************** traf ****************************/

// Traf is ISOBMFF traf box type.
type Traf struct{}

// Type returns the BoxType.
func (*Traf) Type() BoxType {
	return [4]byte{'t', 'r', 'a', 'f'}
}

// Size returns the marshaled size in bytes.
func (b *Traf) Size() int {
	return 0
}

// Marshal is never called.
func (b *Traf) Marshal(buf []byte, pos *int) {}

/*************************** trak ****************************/

// Trak is ISOBMFF trak box type.
type Trak struct{}

// Type returns the BoxType.
func (*Trak) Type() BoxType {
	return [4]byte{'t', 'r', 'a', 'k'}
}

// Size returns the marshaled size in bytes.
func (b *Trak) Size() int {
	return 0
}

// Marshal is never called.
func (b *Trak) Marshal(buf []byte, pos *int) {}

/*************************** trex ****************************/

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
func (*Trex) Type() BoxType {
	return [4]byte{'t', 'r', 'e', 'x'}
}

// Size returns the marshaled size in bytes.
func (b *Trex) Size() int {
	return 24
}

// Marshal box to buffer.
func (b *Trex) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	WriteUint32(buf, pos, b.TrackID)
	WriteUint32(buf, pos, b.DefaultSampleDescriptionIndex)
	WriteUint32(buf, pos, b.DefaultSampleDuration)
	WriteUint32(buf, pos, b.DefaultSampleSize)
	WriteUint32(buf, pos, b.DefaultSampleFlags)
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

// Size returns the marshaled size in bytes.
func (b *TrunEntry) Size(fullBox FullBox) int {
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

// Marshal entry to buffer.
func (b *TrunEntry) Marshal(buf []byte, pos *int, fullBox FullBox) {
	if fullBox.CheckFlag(TrunSampleDurationPresent) {
		WriteUint32(buf, pos, b.SampleDuration)
	}
	if fullBox.CheckFlag(TrunSampleSizePresent) {
		WriteUint32(buf, pos, b.SampleSize)
	}
	if fullBox.CheckFlag(TrunSampleFlagsPresent) {
		WriteUint32(buf, pos, b.SampleFlags)
	}
	if fullBox.CheckFlag(TrunSampleCompositionTimeOffsetPresent) {
		if fullBox.Version == 0 {
			WriteUint32(buf, pos, b.SampleCompositionTimeOffsetV0)
		} else {
			WriteUint32(buf, pos, uint32(b.SampleCompositionTimeOffsetV1))
		}
	}
}

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
func (*Trun) Type() BoxType {
	return [4]byte{'t', 'r', 'u', 'n'}
}

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
		total += entry.Size(b.FullBox)
	}
	return total
}

// Marshal box to buffer.
func (b *Trun) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	WriteUint32(buf, pos, b.SampleCount)
	if b.FullBox.CheckFlag(TrunDataOffsetPresent) {
		WriteUint32(buf, pos, uint32(b.DataOffset))
	}
	if b.FullBox.CheckFlag(TrunFirstSampleFlagsPresent) {
		WriteUint32(buf, pos, b.FirstSampleFlags)
	}
	for _, entry := range b.Entries {
		entry.Marshal(buf, pos, b.FullBox)
	}
}

/*************************** vmhd ****************************/

// Vmhd is ISOBMFF vmhd box type.
type Vmhd struct {
	FullBox
	Graphicsmode uint16    // template=0
	Opcolor      [3]uint16 // template={0, 0, 0}
}

// Type returns the BoxType.
func (*Vmhd) Type() BoxType {
	return [4]byte{'v', 'm', 'h', 'd'}
}

// Size returns the marshaled size in bytes.
func (b *Vmhd) Size() int {
	return 12
}

// Marshal box to buffer.
func (b *Vmhd) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	WriteUint16(buf, pos, b.Graphicsmode)
	for _, color := range b.Opcolor {
		WriteUint16(buf, pos, color)
	}
}
