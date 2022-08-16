package mp4

import "nvr/pkg/video/mp4/bitio"

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

	// Marshal box to writer.
	Marshal(w *bitio.Writer) error
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
func (b *Boxes) Marshal(w *bitio.Writer) error {
	size := b.Size()

	err := writeBoxInfo(w, uint32(size), b.Box.Type())
	if err != nil {
		return err
	}

	// The size of a empty box is 8 bytes.
	if size != 8 {
		err := b.Box.Marshal(w)
		if err != nil {
			return err
		}
	}

	for _, child := range b.Children {
		err := child.Marshal(w)
		if err != nil {
			return err
		}
	}
	return nil
}

func writeBoxInfo(w *bitio.Writer, size uint32, typ BoxType) error {
	w.TryWriteUint32(size)
	w.TryWrite(typ[:])
	return w.TryError
}
