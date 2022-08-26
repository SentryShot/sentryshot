package bitio

import (
	"io"
)

// WriterAndByteWriter io.Writer and io.ByteWriter at the same time.
type WriterAndByteWriter interface {
	io.Writer
	io.ByteWriter
}

// Writer is the bit writer implementation.
type Writer struct {
	out WriterAndByteWriter

	// TryError holds the first error occurred in TryXXX() methods.
	TryError error
}

// NewWriter returns a new Writer using the specified io.Writer as the output.
func NewWriter(out WriterAndByteWriter) *Writer {
	return &Writer{out: out}
}

// Write implements io.Writer.
func (w *Writer) Write(p []byte) (int, error) {
	return w.out.Write(p)
}

// WriteByte implements io.ByteWriter.
func (w *Writer) WriteByte(b byte) error {
	return w.out.WriteByte(b)
}

// WriteUint16 writes 16 bits.
func (w *Writer) WriteUint16(r uint16) error {
	_, err := w.Write([]byte{
		byte(r >> 8),
		byte(r),
	})
	return err
}

// WriteUint32 writes 32 bits.
func (w *Writer) WriteUint32(r uint32) error {
	_, err := w.Write([]byte{
		byte(r >> 24),
		byte(r >> 16),
		byte(r >> 8),
		byte(r),
	})
	return err
}

// WriteUint64 writes 64 bits.
func (w *Writer) WriteUint64(r uint64) error {
	_, err := w.Write([]byte{
		byte(r >> 56),
		byte(r >> 48),
		byte(r >> 40),
		byte(r >> 32),
		byte(r >> 24),
		byte(r >> 16),
		byte(r >> 8),
		byte(r),
	})
	return err
}

// TryWrite tries to write len(p) bytes.
func (w *Writer) TryWrite(p []byte) {
	if w.TryError == nil {
		_, w.TryError = w.Write(p)
	}
}

// TryWriteByte tries to write 1 byte.
func (w *Writer) TryWriteByte(b byte) {
	if w.TryError == nil {
		w.TryError = w.WriteByte(b)
	}
}

// TryWriteUint16 tries to write 16 bits.
func (w *Writer) TryWriteUint16(r uint16) {
	if w.TryError == nil {
		w.TryError = w.WriteUint16(r)
	}
}

// TryWriteUint32 tries to write 32 bits.
func (w *Writer) TryWriteUint32(r uint32) {
	if w.TryError == nil {
		w.TryError = w.WriteUint32(r)
	}
}

// TryWriteUint64 tries to write 64 bits.
func (w *Writer) TryWriteUint64(r uint64) {
	if w.TryError == nil {
		w.TryError = w.WriteUint64(r)
	}
}

// ByteWriter is a helper for io.Writers without io.ByteWriter.
type ByteWriter struct {
	out io.Writer
}

// NewByteWriter returns a new ByteWriter using the specified io.Writer as the output.
func NewByteWriter(out io.Writer) *ByteWriter {
	return &ByteWriter{out: out}
}

// Write implements io.Writer.
func (w *ByteWriter) Write(p []byte) (int, error) {
	return w.out.Write(p)
}

// WriteByte implements io.ByteWriter.
func (w *ByteWriter) WriteByte(b byte) error {
	_, err := w.out.Write([]byte{b})
	return err
}
