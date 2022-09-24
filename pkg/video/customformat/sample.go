package customformat

import "encoding/binary"

// Sample flags.
const (
	FlagIsAudioSample = uint8(0x1)
	FlagIsSyncSample  = uint8(0x2)
)

const sampleSize = 33

// Sample .
type Sample struct {
	IsAudioSample bool
	IsSyncSample  bool

	PTS    int64
	DTS    int64
	Next   int64
	Size   uint32
	Offset uint32
}

// Marshal sample.
func (s Sample) Marshal() []byte {
	out := make([]byte, sampleSize)

	var flags uint8
	if s.IsAudioSample {
		flags |= FlagIsAudioSample
	}
	if s.IsSyncSample {
		flags |= FlagIsSyncSample
	}

	out[0] = flags
	binary.BigEndian.PutUint64(out[1:9], uint64(s.PTS))
	binary.BigEndian.PutUint64(out[9:17], uint64(s.DTS))
	binary.BigEndian.PutUint64(out[17:25], uint64(s.Next))
	binary.BigEndian.PutUint32(out[25:29], s.Offset)
	binary.BigEndian.PutUint32(out[29:33], s.Size)
	return out
}

// Unmarshal sample.
func (s *Sample) Unmarshal(buf []byte) {
	flags := buf[0]
	s.IsAudioSample = flags&FlagIsAudioSample != 0
	s.IsSyncSample = flags&FlagIsSyncSample != 0

	s.PTS = int64(binary.BigEndian.Uint64(buf[1:9]))
	s.DTS = int64(binary.BigEndian.Uint64(buf[9:17]))
	s.Next = int64(binary.BigEndian.Uint64(buf[17:25]))
	s.Offset = binary.BigEndian.Uint32(buf[25:29])
	s.Size = binary.BigEndian.Uint32(buf[29:33])
}
