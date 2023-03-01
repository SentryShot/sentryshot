// Package rec2sample is a script for debugging the mp4 muxer.
package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"nvr/pkg/video/customformat"
	"nvr/pkg/video/mp4muxer"
	"os"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	recording := "./temp/video"

	video, err := newVideoReader(recording, 1)
	if err != nil {
		return fmt.Errorf("create video reader: %w", err)
	}
	defer video.Close()

	mp4File := recording + ".mp4"
	os.Remove(mp4File)

	file, err := os.OpenFile(mp4File, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	_, err = io.CopyN(file, video, 137861)
	if err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return nil
}

type videoReader struct {
	meta io.ReadSeeker
	mdat io.ReadSeekCloser

	metaSize int64
	mdatSize int64

	i int64 // current reading index
}

// newVideoReader creates a video reader.
// Caller must call Close() when done.
func newVideoReader(recordingPath string, nSamples int) (*videoReader, error) { //nolint:funlen
	metaPath := recordingPath + ".meta"
	mdatPath := recordingPath + ".mdat"
	naluPath := recordingPath + ".h264"

	metaStat, err := os.Stat(metaPath)
	if err != nil {
		return nil, fmt.Errorf("stat meta file: %w", err)
	}
	metaSize := int(metaStat.Size())

	metaFile, err := os.Open(metaPath)
	if err != nil {
		return nil, fmt.Errorf("open meta file: %w", err)
	}
	defer metaFile.Close()

	reader, header, err := customformat.NewReader(metaFile, metaSize)
	if err != nil {
		return nil, fmt.Errorf("new reader: %w", err)
	}

	videoTrack, audioTrack, err := header.GetTracks()
	if err != nil {
		return nil, fmt.Errorf("get tracks: %w", err)
	}

	samples, err := reader.ReadAllSamples()
	if err != nil {
		return nil, fmt.Errorf("read all samples: %w", err)
	}
	samples = samples[:nSamples]

	metaBuf := &bytes.Buffer{}
	mdatSize, err := mp4muxer.GenerateMP4(metaBuf, header.StartTime, samples, videoTrack, audioTrack)
	if err != nil {
		return nil, fmt.Errorf("generate meta: %w", err)
	}

	meta := &videoMetadata{
		buf:      metaBuf.Bytes(),
		mdatSize: mdatSize,
	}

	mdat, err := os.Open(mdatPath)
	if err != nil {
		return nil, fmt.Errorf("open mdat file: %w", err)
	}

	naluBuf := make([]byte, mdatSize)
	_, err = io.ReadFull(mdat, naluBuf)
	if err != nil {
		return nil, fmt.Errorf("fill nalu buffer: %w", err)
	}

	_, err = mdat.Seek(0, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("mdat seek 0: %w", err)
	}

	err = os.WriteFile(naluPath, naluBuf, 0o700)
	if err != nil {
		return nil, fmt.Errorf("write nalu file: %w", err)
	}

	return &videoReader{
		meta: bytes.NewReader(meta.buf),
		mdat: mdat,

		metaSize: int64(len(meta.buf)),
		mdatSize: meta.mdatSize,
	}, nil
}

// Read implements io.Reader .
func (r *videoReader) Read(p []byte) (int, error) {
	if r.i >= r.metaSize+r.mdatSize {
		return 0, io.EOF
	}

	pLen := int64(len(p))

	// Read starts within meta.
	if r.i <= r.metaSize {
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

// Close implements io.Closer .
func (r *videoReader) Close() error {
	return r.mdat.Close()
}

type videoMetadata struct {
	buf      []byte
	mdatSize int64
}
