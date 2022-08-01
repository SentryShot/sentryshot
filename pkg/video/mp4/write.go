package mp4

import (
	"encoding/binary"
)

// Write writes len(p) bytes.
func Write(buf []byte, pos *int, p []byte) {
	*pos += copy(buf[*pos:], p)
}

// WriteByte writes 1 byte.
func WriteByte(buf []byte, pos *int, byt byte) {
	buf[*pos] = byt
	*pos++
}

// WriteUint16 writes 16 bits.
func WriteUint16(buf []byte, pos *int, r uint16) {
	binary.BigEndian.PutUint16(buf[*pos:], r)
	*pos += 2
}

// WriteUint32 writes 32 bits.
func WriteUint32(buf []byte, pos *int, r uint32) {
	binary.BigEndian.PutUint32(buf[*pos:], r)
	*pos += 4
}

// WriteUint64 writes 64 bits.
func WriteUint64(buf []byte, pos *int, r uint64) {
	binary.BigEndian.PutUint64(buf[*pos:], r)
	*pos += 8
}

// WriteString writes string and null character.
func WriteString(buf []byte, pos *int, str string) {
	Write(buf, pos, []byte(str))
	WriteByte(buf, pos, 0x00) // null character
}
