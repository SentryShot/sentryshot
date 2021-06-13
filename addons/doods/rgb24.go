// RGB24 implementation using stdlib image.Image interface.

package doods

import (
	"image"
	"image/color"
	"math/bits"
)

// RGB Color.
type RGB struct {
	R, G, B uint8
}

// RGBA .
func (c RGB) RGBA() (r, g, b, a uint32) {
	r = uint32(c.R)
	r |= r << 8

	g = uint32(c.G)
	g |= g << 8

	b = uint32(c.B)
	b |= b << 8

	a = 0xffff
	return
}

// NewRGB24 .
func NewRGB24(r image.Rectangle) *RGB24 {
	return &RGB24{
		Pix:    make([]uint8, pixelBufferLength(3, r)),
		Stride: 3 * r.Dx(),
		Rect:   r,
	}
}

// RGB24 is an in-memory image whose At method returns RGB values.
type RGB24 struct {

	// Pix holds the image's pixels, in R, G, B order. The pixel at
	// (x, y) starts at Pix[(y-Rect.Min.Y)*Stride + (x-Rect.Min.X)*4].
	Pix []uint8

	// Stride is the Pix stride (in bytes) between vertically adjacent pixels.
	Stride int

	// Rect is the image's bounds.
	Rect image.Rectangle
}

// ColorModel .
func (p *RGB24) ColorModel() color.Model { return RGB24Model }

// Bounds .
func (p *RGB24) Bounds() image.Rectangle { return p.Rect }

// At .
func (p *RGB24) At(x, y int) color.Color {
	return p.RGB24At(x, y)
}

// RGB24At .
func (p *RGB24) RGB24At(x, y int) RGB {
	if !(image.Point{x, y}.In(p.Rect)) {
		return RGB{}
	}

	i := p.PixOffset(x, y)

	return RGB{p.Pix[i], p.Pix[i+1], p.Pix[i+2]}
}

// PixOffset returns the index of the first element of Pix that corresponds to
// the pixel at (x, y).
func (p *RGB24) PixOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*3
}

// RGB24Model .
var RGB24Model color.Model = color.ModelFunc(rgbaModel)

func rgbaModel(c color.Color) color.Color {
	if _, ok := c.(RGB); ok {
		return c
	}
	r, g, b, _ := c.RGBA()

	return RGB{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
}

// pixelBufferLength returns the length of the []uint8 typed Pix slice field
// for the NewXxx functions. Conceptually, this is just (bpp * width * height),
// but this function panics if at least one of those is negative or if the
// computation would overflow the int type.
//
// This panics instead of returning an error because of backwards
// compatibility. The NewXxx functions do not return an error.
func pixelBufferLength(bytesPerPixel int, r image.Rectangle) int {
	totalLength := mul3NonNeg(bytesPerPixel, r.Dx(), r.Dy())
	if totalLength < 0 {
		panic("image: NewRGB24 Rectangle has huge or negative dimensions")
	}
	return totalLength
}

// mul3NonNeg returns (x * y * z), unless at least one argument is negative or
// if the computation overflows the int type, in which case it returns -1.
func mul3NonNeg(x int, y int, z int) int {
	if (x < 0) || (y < 0) || (z < 0) {
		return -1
	}
	hi, lo := bits.Mul64(uint64(x), uint64(y))
	if hi != 0 {
		return -1
	}
	hi, lo = bits.Mul64(lo, uint64(z))
	if hi != 0 {
		return -1
	}
	a := int(lo)
	if (a < 0) || (uint64(a) != lo) {
		return -1
	}
	return a
}
