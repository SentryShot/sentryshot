package storage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"nvr/pkg/video/customformat"
	"nvr/pkg/video/mp4muxer"
	"os"
	"time"
)

// VideoReader implements io.ReadSeekCloser .
type VideoReader struct {
	meta io.ReadSeeker // This could be cached.
	mdat io.ReadSeekCloser

	metaSize int64
	mdatSize int64

	i int64 // current reading index

	modTime time.Time
}

// NewVideoReader creates a video reader.
// Caller must call Close() when done.
func NewVideoReader(recordingPath string) (*VideoReader, error) {
	metaPath := recordingPath + ".meta"
	mdatPath := recordingPath + ".mdat"

	metaStat, err := os.Stat(metaPath)
	if err != nil {
		return nil, fmt.Errorf("stat meta file: %w", err)
	}
	metaSize := int(metaStat.Size())

	meta, err := os.Open(metaPath)
	if err != nil {
		return nil, fmt.Errorf("open meta file: %w", err)
	}
	defer meta.Close()

	mdat, err := os.Open(mdatPath)
	if err != nil {
		return nil, fmt.Errorf("open mdat file: %w", err)
	}

	reader, header, err := customformat.NewReader(meta, metaSize)
	if err != nil {
		mdat.Close()
		return nil, fmt.Errorf("new reader: %w", err)
	}

	info, err := header.ToStreamInfo()
	if err != nil {
		mdat.Close()
		return nil, fmt.Errorf("stream info: %w", err)
	}

	samples, err := reader.ReadAllSamples()
	if err != nil {
		mdat.Close()
		return nil, fmt.Errorf("read all samples: %w", err)
	}

	metaBuf := &bytes.Buffer{}
	mdatSize, err := mp4muxer.GenerateMP4(metaBuf, header.StartTime, samples, *info)
	if err != nil {
		mdat.Close()
		return nil, fmt.Errorf("generate meta: %w", err)
	}

	return &VideoReader{
		meta: bytes.NewReader(metaBuf.Bytes()),
		mdat: mdat,

		metaSize: int64(metaBuf.Len()),
		mdatSize: mdatSize,

		modTime: metaStat.ModTime(),
	}, nil
}

// Read implements io.Reader .
func (r *VideoReader) Read(p []byte) (int, error) {
	if r.i >= r.metaSize+r.mdatSize {
		return 0, io.EOF
	}

	pLen := int64(len(p))

	// Read starts within meta.
	if r.i <= r.metaSize { //nolint:nestif
		_, err := r.meta.Seek(r.i, io.SeekStart)
		if err != nil {
			return 0, err
		}

		// Read within meta.
		if pLen <= r.metaSize-r.i {
			n, err := r.meta.Read(p)
			if err != nil {
				return 0, err
			}
			r.i += int64(n)
			return n, nil
		}

		// Read across border.
		n, err := r.meta.Read(p)
		if err != nil {
			return 0, err
		}
		n2, err := r.mdat.Read(p[n:])
		if err != nil {
			return 0, err
		}
		r.i += int64(n + n2)
		return n + n2, nil
	}

	// Read within mdat.
	_, err := r.mdat.Seek(r.i-r.metaSize, io.SeekStart)
	if err != nil {
		return 0, err
	}
	n, err := r.mdat.Read(p)
	if err != nil {
		return 0, err
	}
	r.i += int64(n)
	return n, nil
}

// Testing.
var errInvalidWhence = errors.New("bytes.Reader.Seek: invalid whence")
var errNegativePosition = errors.New("bytes.Reader.Seek: negative position")

// Seek implements io.Seeker .
func (r *VideoReader) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.i + offset
	case io.SeekEnd:
		abs = (r.metaSize + r.mdatSize) + offset
	default:
		return 0, errInvalidWhence
	}
	if abs < 0 {
		return 0, errNegativePosition
	}
	r.i = abs
	return abs, nil
}

// Close implements io.Closer .
func (r *VideoReader) Close() error {
	return r.mdat.Close()
}

// ModTime video modification time.
func (r *VideoReader) ModTime() time.Time {
	return r.modTime
}
