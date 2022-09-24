package customformat

import (
	"fmt"
	"io"
)

// Reader reads a single meta file.
type Reader struct {
	in io.ReadSeeker

	headerSize  int
	fileSize    int
	sampleCount int
}

// NewReader creates a new reader.
func NewReader(in io.ReadSeeker, fileSize int) (*Reader, *Header, error) {
	var header Header
	headerSize, err := header.Unmarshal(in)
	if err != nil {
		return nil, nil, fmt.Errorf("unmarshal header: %w", err)
	}

	r := Reader{
		in:          in,
		headerSize:  headerSize,
		fileSize:    fileSize,
		sampleCount: (fileSize - headerSize) / sampleSize,
	}

	return &r, &header, nil
}

// ReadAllSamples reads and returns all samples in the file.
func (r *Reader) ReadAllSamples() ([]Sample, error) {
	// Seek to end of the header.
	_, err := r.in.Seek(int64(r.headerSize), io.SeekStart)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, sampleSize)
	samples := make([]Sample, r.sampleCount)
	for i := 0; i < r.sampleCount; i++ {
		if _, err := io.ReadFull(r.in, buf); err != nil {
			return nil, err
		}
		samples[i].Unmarshal(buf)
	}

	return samples, nil
}
