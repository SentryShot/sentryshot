package writerseeker

import (
	"io"
	"strings"
	"testing"
)

func TestWrite(t *testing.T) {
	ws := &WriterSeeker{}
	checkWrite(t, ws, "hello", "hello")
	checkWrite(t, ws, " world", "hello world")
}

func TestSeek(t *testing.T) {
	ws := &WriterSeeker{}
	checkWrite(t, ws, "hello", "hello")
	checkWrite(t, ws, " world", "hello world")

	checkSeek(t, ws, -2, io.SeekEnd, len("hello world")-2)
	checkWrite(t, ws, "k!", "hello work!")

	checkSeek(t, ws, 6, io.SeekStart, 6)
	checkWrite(t, ws, "gopher", "hello gopher")

	// Seek back a bit and check that we overwrite the existing buffer before growing it.
	checkSeek(t, ws, -4, io.SeekCurrent, len("hello gopher")-4)
	checkWrite(t, ws, "lang fans", "hello golang fans")

	// If we seek past the end of the buffer, the empty space should be filled with null bytes.
	checkSeek(t, ws, 4, io.SeekCurrent, len("hello golang fans")+4)
	checkWrite(t, ws, "!", "hello golang fans\x00\x00\x00\x00!")
}

func TestSeek_LargeGap(t *testing.T) {
	ws := &WriterSeeker{}
	checkSeek(t, ws, 1024, io.SeekStart, 1024)
	checkWrite(t, ws, "hello", strings.Repeat("\x00", 1024)+"hello")
}

// checkWrite passes data to ws.Write and compares the resulting buffer against exp.
func checkWrite(t *testing.T, ws *WriterSeeker, data, exp string) {
	if n, err := ws.Write([]byte(data)); err != nil {
		t.Fatalf("Write(%q) failed: %v", data, err)
	} else if ws.buf.String() != exp {
		t.Fatalf("Write(%q) produced %q; want %q", data, ws.buf.String(), exp)
	} else if n != len(data) {
		t.Fatalf("Write(%q) = %v; want %q", data, n, len(data))
	}
}

// checkSeek calls ws.Seek with the supplied parameters and compares the returned offset against exp.
func checkSeek(t *testing.T, ws *WriterSeeker, offset int64, whence, exp int) {
	if newOffset, err := ws.Seek(offset, whence); err != nil {
		t.Fatalf("Seek(%v, %v) failed: %v", offset, whence, err)
	} else if newOffset != int64(exp) {
		t.Fatalf("Seek(%v, %v) = %v; want %v", offset, whence, newOffset, exp)
	}
}
