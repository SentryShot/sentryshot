package mp4

// BoxType is mpeg box type.
type BoxType [4]byte

// ImmutableBox is common interface of box.
type ImmutableBox interface {
	// Type returns the BoxType.
	Type() BoxType

	// Size returns the marshaled size in bytes.
	// The size must be known before marshaling
	// since the box header contains the size.
	Size() int

	// Marshal box to buffer.
	Marshal(buf []byte, pos *int)
}

// Boxes is a structure of boxes that can be marshaled together.
type Boxes struct {
	Box      ImmutableBox
	Children []Boxes
}

// Size returns the total size of the box including children.
func (b *Boxes) Size() int {
	total := b.Box.Size() + 8
	for _, child := range b.Children {
		size := child.Size()
		total += size
	}
	return total
}

// Marshal box including children.
func (b *Boxes) Marshal(buf []byte, pos *int) {
	size := b.Size()
	writeBoxInfo(buf, pos, uint32(size), b.Box.Type())

	// The size of a empty box is 8 bytes.
	if size != 8 {
		b.Box.Marshal(buf, pos)
	}

	for _, child := range b.Children {
		child.Marshal(buf, pos)
	}
}

func writeBoxInfo(buf []byte, pos *int, size uint32, typ BoxType) {
	WriteUint32(buf, pos, size)
	Write(buf, pos, typ[:])
}
