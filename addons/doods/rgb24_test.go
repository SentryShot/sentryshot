// Tests modified from stdlib.

package doods

import (
	"image"
	"image/color"
	"testing"
)

func cmp(cm color.Model, c0, c1 color.Color) bool {
	r0, g0, b0, a0 := cm.Convert(c0).RGBA()
	r1, g1, b1, a1 := cm.Convert(c1).RGBA()
	return r0 == r1 && g0 == g1 && b0 == b1 && a0 == a1
}

func TestImage(t *testing.T) {
	m := NewRGB24(image.Rect(0, 0, 10, 10))
	if !image.Rect(0, 0, 10, 10).Eq(m.Bounds()) {
		t.Errorf("%T: want bounds %v, got %v", m, image.Rect(0, 0, 10, 10), m.Bounds())
	}
	if !cmp(m.ColorModel(), image.Transparent, m.At(6, 3)) {
		t.Errorf("%T: at (6, 3), want a zero color, got %v", m, m.At(6, 3))
	}
	if m.At(-1, -1) != (RGB{}) {
		t.Errorf("%T: at (-1, -1), want RGB{}, got %v", m, m.At(-1, -1))
	}
}

func TestNewBadRectangle(t *testing.T) {
	// call calls f(r) and reports whether it ran without panicking.
	call := func(f func(image.Rectangle), r image.Rectangle) (ok bool) {
		defer func() {
			if recover() != nil {
				ok = false
			}
		}()
		f(r)
		return true
	}

	f := func(r image.Rectangle) {
		NewRGB24(r)
	}

	// Calling NewRGB24(r) should fail (panic, since NewRGB24 doesn't return an
	// error) unless r's width and height are both non-negative.
	for _, negDx := range []bool{false, true} {
		for _, negDy := range []bool{false, true} {
			r := image.Rectangle{
				Min: image.Point{15, 28},
				Max: image.Point{16, 29},
			}
			if negDx {
				r.Max.X = 14
			}
			if negDy {
				r.Max.Y = 27
			}
			got := call(f, r)
			want := !negDx && !negDy
			if got != want {
				t.Errorf("negDx=%t, negDy=%t: got %t, want %t",
					negDx, negDy, got, want)
			}
		}
	}

	// Passing a Rectangle whose width and height is MaxInt should also fail
	// (panic), due to overflow.
	{
		zeroAsUint := uint(0)
		maxUint := zeroAsUint - 1
		maxInt := int(maxUint / 2)
		got := call(f, image.Rectangle{
			Min: image.Point{0, 0},
			Max: image.Point{maxInt, maxInt},
		})
		if got {
			t.Error("overflow: got ok, want !ok")
		}
	}
}
