package storage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"nvr/pkg/video/customformat"
	"nvr/pkg/video/mp4muxer"
	"os"
	"sync"
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
func NewVideoReader(recordingPath string, cache *VideoCache) (*VideoReader, error) {
	metaPath := recordingPath + ".meta"
	mdatPath := recordingPath + ".mdat"

	var meta *videoMetadata
	var err error
	if cache != nil {
		var exist bool
		meta, exist = cache.get(recordingPath)
		if !exist {
			meta, err = readVideoMetadata(metaPath)
			if err != nil {
				return nil, err
			}
			cache.add(recordingPath, meta)
		}
	} else {
		meta, err = readVideoMetadata(metaPath)
		if err != nil {
			return nil, err
		}
	}

	mdat, err := os.Open(mdatPath)
	if err != nil {
		return nil, fmt.Errorf("open mdat file: %w", err)
	}

	return &VideoReader{
		meta: bytes.NewReader(meta.buf),
		mdat: mdat,

		metaSize: int64(len(meta.buf)),
		mdatSize: meta.mdatSize,

		modTime: meta.modTime,
	}, nil
}

func readVideoMetadata(metaPath string) (*videoMetadata, error) {
	metaStat, err := os.Stat(metaPath)
	if err != nil {
		return nil, fmt.Errorf("stat meta file: %w", err)
	}
	metaSize := int(metaStat.Size())
	modTime := metaStat.ModTime()

	meta, err := os.Open(metaPath)
	if err != nil {
		return nil, fmt.Errorf("open meta file: %w", err)
	}
	defer meta.Close()

	reader, header, err := customformat.NewReader(meta, metaSize)
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

	metaBuf := &bytes.Buffer{}
	mdatSize, err := mp4muxer.GenerateMP4(
		metaBuf, header.StartTime, samples, videoTrack, audioTrack)
	if err != nil {
		return nil, fmt.Errorf("generate meta: %w", err)
	}

	return &videoMetadata{
		buf:      metaBuf.Bytes(),
		mdatSize: mdatSize,
		modTime:  modTime,
	}, nil
}

// Read implements io.Reader .
func (r *VideoReader) Read(p []byte) (int, error) {
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

// Size of video.
func (r *VideoReader) Size() int64 {
	return r.metaSize + r.mdatSize
}

// VideoCache Caches the n most recent video readers.
type VideoCache struct {
	items map[string]*videoMetadata
	age   int

	maxSize int

	mu sync.Mutex
}

type videoMetadata struct {
	buf      []byte
	mdatSize int64
	modTime  time.Time

	key string
	age int
}

const videoCacheSize = 10

// NewVideoCache creates a video cache.
func NewVideoCache() *VideoCache {
	return &VideoCache{
		items:   map[string]*videoMetadata{},
		maxSize: videoCacheSize,
	}
}

// add item to the cache.
func (c *VideoCache) add(key string, video *videoMetadata) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Ignore duplicate keys.
	if _, exist := c.items[key]; exist {
		return
	}

	c.age++
	if len(c.items) >= c.maxSize {
		// Delete the oldest item.
		oldestItem := &videoMetadata{age: -1}
		for _, item := range c.items {
			if oldestItem.age == -1 || item.age < oldestItem.age {
				oldestItem = item
			}
		}
		delete(c.items, oldestItem.key)
	}

	video.key = key
	video.age = c.age
	c.items[key] = video
}

// get item by key and update its age if it exists.
func (c *VideoCache) get(key string) (*videoMetadata, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, item := range c.items {
		if item.key == key {
			c.age++
			item.age = c.age
			return item, true
		}
	}
	return nil, false
}
