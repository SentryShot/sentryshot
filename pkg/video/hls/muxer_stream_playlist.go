package hls

import (
	"bytes"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
)

type asyncReader struct {
	generator func() []byte
	reader    *bytes.Reader
}

func (r *asyncReader) Read(buf []byte) (int, error) {
	if r.reader == nil {
		r.reader = bytes.NewReader(r.generator())
	}
	return r.reader.Read(buf)
}

// Segments TS segments.
type Segments []*MuxerTSSegment

type muxerStreamPlaylist struct {
	hlsSegmentCount int

	mutex              sync.Mutex
	cond               *sync.Cond
	closed             bool
	segments           Segments
	segmentByName      map[string]*MuxerTSSegment
	segmentDeleteCount int

	onNewSegment chan<- Segments
}

func newMuxerStreamPlaylist(
	hlsSegmentCount int, onNewSegment chan<- Segments,
) *muxerStreamPlaylist {
	p := &muxerStreamPlaylist{
		hlsSegmentCount: hlsSegmentCount,
		segmentByName:   make(map[string]*MuxerTSSegment),
		onNewSegment:    onNewSegment,
	}
	p.cond = sync.NewCond(&p.mutex)
	return p
}

func (p *muxerStreamPlaylist) close() {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		p.closed = true
	}()

	p.cond.Broadcast()
}

func (p *muxerStreamPlaylist) reader() io.Reader {
	return &asyncReader{generator: func() []byte {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		if !p.closed && len(p.segments) == 0 {
			p.cond.Wait()
		}

		if p.closed {
			return nil
		}

		cnt := "#EXTM3U\n"
		cnt += "#EXT-X-VERSION:3\n"
		cnt += "#EXT-X-ALLOW-CACHE:NO\n"

		targetDuration := func() uint {
			ret := uint(0)

			// EXTINF, when rounded to the nearest integer, must be <= EXT-X-TARGETDURATION
			for _, s := range p.segments {
				v2 := uint(math.Round(s.Duration().Seconds()))
				if v2 > ret {
					ret = v2
				}
			}

			return ret
		}()
		cnt += "#EXT-X-TARGETDURATION:" + strconv.FormatUint(uint64(targetDuration), 10) + "\n"

		cnt += "#EXT-X-MEDIA-SEQUENCE:" + strconv.FormatInt(int64(p.segmentDeleteCount), 10) + "\n"
		cnt += "\n"

		for _, s := range p.segments {
			cnt += "#EXT-X-PROGRAM-DATE-TIME:" + s.startTime.Format("2006-01-02T15:04:05.999Z07:00") + "\n" +
				"#EXTINF:" + strconv.FormatFloat(s.Duration().Seconds(), 'f', -1, 64) + ",\n" +
				s.name + ".ts\n"
		}

		return []byte(cnt)
	}}
}

func (p *muxerStreamPlaylist) segment(fname string) io.Reader {
	base := strings.TrimSuffix(fname, ".ts")

	p.mutex.Lock()
	f, ok := p.segmentByName[base]
	p.mutex.Unlock()

	if !ok {
		return nil
	}

	return f.reader()
}

func (p *muxerStreamPlaylist) pushSegment(t *MuxerTSSegment) {
	p.mutex.Lock()

	p.segmentByName[t.name] = t
	p.segments = append(p.segments, t)

	if len(p.segments) > p.hlsSegmentCount {
		delete(p.segmentByName, p.segments[0].name)
		p.segments = p.segments[1:]
		p.segmentDeleteCount++
	}
	p.mutex.Unlock()

	p.cond.Broadcast()

	for {
		select {
		case p.onNewSegment <- p.segments:
		default:
			return
		}
	}
}
