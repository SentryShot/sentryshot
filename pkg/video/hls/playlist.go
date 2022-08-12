package hls

import (
	"bytes"
	"encoding/hex"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SegmentOrGap .
type SegmentOrGap interface {
	getRenderedDuration() time.Duration
}

// Gap .
type Gap struct {
	renderedDuration time.Duration
}

func (g Gap) getRenderedDuration() time.Duration {
	return g.renderedDuration
}

func targetDuration(segments []SegmentOrGap) uint {
	ret := uint(0)

	// EXTINF, when rounded to the nearest integer, must be <= EXT-X-TARGETDURATION
	for _, sog := range segments {
		v := uint(math.Round(sog.getRenderedDuration().Seconds()))
		if v > ret {
			ret = v
		}
	}

	return ret
}

func partTargetDuration(
	segments []SegmentOrGap,
	nextSegmentParts []*muxerPart,
) time.Duration {
	var ret time.Duration

	for _, sog := range segments {
		seg, ok := sog.(*Segment)
		if !ok {
			continue
		}

		for _, part := range seg.parts {
			if part.renderedDuration > ret {
				ret = part.renderedDuration
			}
		}
	}

	for _, part := range nextSegmentParts {
		if part.renderedDuration > ret {
			ret = part.renderedDuration
		}
	}

	return ret
}

type playlist struct {
	segmentCount int

	mutex              sync.Mutex
	cond               *sync.Cond
	closed             bool
	segments           []SegmentOrGap
	segmentsByName     map[string]*Segment
	segmentDeleteCount int
	parts              []*muxerPart
	partsByName        map[string]*muxerPart
	nextSegmentID      uint64
	nextSegmentParts   []*muxerPart
	nextPartID         uint64

	onNewSegment chan<- []SegmentOrGap
}

func newPlaylist(segmentCount int, onNewSegment chan<- []SegmentOrGap) *playlist {
	p := &playlist{
		segmentCount:   segmentCount,
		segmentsByName: make(map[string]*Segment),
		partsByName:    make(map[string]*muxerPart),
		onNewSegment:   onNewSegment,
	}
	p.cond = sync.NewCond(&p.mutex)

	return p
}

func (p *playlist) close() {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		p.closed = true
	}()

	p.cond.Broadcast()
}

func (p *playlist) hasContent() bool {
	return len(p.segments) >= 1
}

func (p *playlist) hasPart(segmentID uint64, partID uint64) bool {
	if !p.hasContent() {
		return false
	}

	for _, sop := range p.segments {
		seg, ok := sop.(*Segment)
		if !ok {
			continue
		}

		if segmentID != seg.id {
			continue
		}

		// If the Client requests a Part Index greater than that of the final
		// Partial Segment of the Parent Segment, the Server MUST treat the
		// request as one for Part Index 0 of the following Parent Segment.
		if partID >= uint64(len(seg.parts)) {
			segmentID++
			partID = 0
			continue
		}

		return true
	}

	if segmentID != p.nextSegmentID {
		return false
	}

	if partID >= uint64(len(p.nextSegmentParts)) {
		return false
	}

	return true
}

func (p *playlist) file(name, msn, part, skip string) *MuxerFileResponse {
	switch {
	case name == "stream.m3u8":
		return p.playlistReader(msn, part, skip)

	case strings.HasSuffix(name, ".mp4"):
		return p.segmentReader(name)

	default:
		return &MuxerFileResponse{Status: http.StatusNotFound}
	}
}

func (p *playlist) playlistReader(msn, part, skip string) *MuxerFileResponse { //nolint:funlen
	isDeltaUpdate := skip == "YES" || skip == "v2"

	var msnint uint64
	if msn != "" {
		var err error
		msnint, err = strconv.ParseUint(msn, 10, 64)
		if err != nil {
			return &MuxerFileResponse{Status: http.StatusBadRequest}
		}
	}

	var partint uint64
	if part != "" {
		var err error
		partint, err = strconv.ParseUint(part, 10, 64)
		if err != nil {
			return &MuxerFileResponse{Status: http.StatusBadRequest}
		}
	}

	if msn != "" {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		// If the _HLS_msn is greater than the Media Sequence Number of the last
		// Media Segment in the current Playlist plus two, or if the _HLS_part
		// exceeds the last Partial Segment in the current Playlist by the
		// Advance Part Limit, then the server SHOULD immediately return Bad
		// Request, such as HTTP 400.
		if msnint > (p.nextSegmentID + 1) {
			return &MuxerFileResponse{Status: http.StatusBadRequest}
		}

		for !p.closed && !p.hasPart(msnint, partint) {
			p.cond.Wait()
		}

		if p.closed {
			return &MuxerFileResponse{Status: http.StatusInternalServerError}
		}

		return &MuxerFileResponse{
			Status: http.StatusOK,
			Header: map[string]string{
				"Content-Type": `audio/mpegURL`,
			},
			Body: p.fullPlaylist(isDeltaUpdate),
		}
	}

	// part without msn is not supported.
	if part != "" {
		return &MuxerFileResponse{Status: http.StatusBadRequest}
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	for !p.closed && !p.hasContent() {
		p.cond.Wait()
	}

	if p.closed {
		return &MuxerFileResponse{Status: http.StatusInternalServerError}
	}

	return &MuxerFileResponse{
		Status: http.StatusOK,
		Header: map[string]string{
			"Content-Type": `audio/mpegURL`,
		},
		Body: p.fullPlaylist(isDeltaUpdate),
	}
}

func primaryPlaylist(info StreamInfo) *MuxerFileResponse {
	return &MuxerFileResponse{
		Status: http.StatusOK,
		Header: map[string]string{
			"Content-Type": `audio/mpegURL`,
		},
		Body: func() io.Reader {
			var codecs []string

			if info.VideoTrackExist {
				sps := info.VideoSPS
				if len(sps) >= 4 {
					codecs = append(codecs, "avc1."+hex.EncodeToString(sps[1:4]))
				}
			}

			// https://developer.mozilla.org/en-US/docs/Web/Media/Formats/codecs_parameter
			if info.AudioTrackExist {
				codecs = append(
					codecs,
					"mp4a.40."+strconv.FormatInt(int64(info.AudioType), 10),
				)
			}

			return bytes.NewReader([]byte("#EXTM3U\n" +
				"#EXT-X-VERSION:9\n" +
				"#EXT-X-INDEPENDENT-SEGMENTS\n" +
				"\n" +
				"#EXT-X-STREAM-INF:BANDWIDTH=200000,CODECS=\"" + strings.Join(codecs, ",") + "\"\n" +
				"stream.m3u8\n" +
				"\n"))
		}(),
	}
}

func (p *playlist) fullPlaylist(isDeltaUpdate bool) io.Reader { //nolint:funlen
	cnt := "#EXTM3U\n"
	cnt += "#EXT-X-VERSION:9\n"

	targetDuration := targetDuration(p.segments)
	cnt += "#EXT-X-TARGETDURATION:" + strconv.FormatUint(uint64(targetDuration), 10) + "\n"

	skipBoundary := float64(targetDuration * 6)

	partTargetDuration := partTargetDuration(p.segments, p.nextSegmentParts)

	// The value is an enumerated-string whose value is YES if the server
	// supports Blocking Playlist Reload
	cnt += "#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES"

	// The value is a decimal-floating-point number of seconds that
	// indicates the server-recommended minimum distance from the end of
	// the Playlist at which clients should begin to play or to which
	// they should seek when playing in Low-Latency Mode.  Its value MUST
	// be at least twice the Part Target Duration.  Its value SHOULD be
	// at least three times the Part Target Duration.
	cnt += ",PART-HOLD-BACK=" + strconv.FormatFloat((partTargetDuration).Seconds()*2.5, 'f', 5, 64)

	// Indicates that the Server can produce Playlist Delta Updates in
	// response to the _HLS_skip Delivery Directive.  Its value is the
	// Skip Boundary, a decimal-floating-point number of seconds.  The
	// Skip Boundary MUST be at least six times the Target Duration.
	cnt += ",CAN-SKIP-UNTIL=" + strconv.FormatFloat(skipBoundary, 'f', -1, 64)

	cnt += "\n"

	cnt += "#EXT-X-PART-INF:PART-TARGET=" + strconv.FormatFloat(partTargetDuration.Seconds(), 'f', -1, 64) + "\n"

	cnt += "#EXT-X-MEDIA-SEQUENCE:" + strconv.FormatInt(int64(p.segmentDeleteCount), 10) + "\n"

	skipped := 0
	if !isDeltaUpdate {
		cnt += "#EXT-X-MAP:URI=\"init.mp4\"\n"
	} else {
		var curDuration time.Duration
		shown := 0
		for _, segment := range p.segments {
			curDuration += segment.getRenderedDuration()
			if curDuration.Seconds() >= skipBoundary {
				break
			}
			shown++
		}
		skipped = len(p.segments) - shown
		cnt += "#EXT-X-SKIP:SKIPPED-SEGMENTS=" + strconv.FormatInt(int64(skipped), 10) + "\n"
	}

	cnt += "\n"

	for i, sog := range p.segments {
		if i < skipped {
			continue
		}

		switch seg := sog.(type) {
		case *Segment:
			if (len(p.segments) - i) <= 2 {
				cnt += "#EXT-X-PROGRAM-DATE-TIME:" + seg.startTime.Format("2006-01-02T15:04:05.999Z07:00") + "\n"
			}

			if (len(p.segments) - i) <= 2 {
				for _, part := range seg.parts {
					cnt += "#EXT-X-PART:DURATION=" + strconv.FormatFloat(part.renderedDuration.Seconds(), 'f', 5, 64) +
						",URI=\"" + part.name() + ".mp4\""
					if part.isIndependent {
						cnt += ",INDEPENDENT=YES"
					}
					cnt += "\n"
				}
			}

			cnt += "#EXTINF:" + strconv.FormatFloat(seg.renderedDuration.Seconds(), 'f', 5, 64) + ",\n" +
				seg.name() + ".mp4\n"

		case *Gap:
			cnt += "#EXT-X-GAP\n" +
				"#EXTINF:" + strconv.FormatFloat(seg.renderedDuration.Seconds(), 'f', 5, 64) + ",\n" +
				"gap.mp4\n"
		}
	}

	for _, part := range p.nextSegmentParts {
		cnt += "#EXT-X-PART:DURATION=" + strconv.FormatFloat(part.renderedDuration.Seconds(), 'f', 5, 64) +
			",URI=\"" + part.name() + ".mp4\""
		if part.isIndependent {
			cnt += ",INDEPENDENT=YES"
		}
		cnt += "\n"
	}

	// preload hint must always be present
	// otherwise hls.js goes into a loop
	cnt += "#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"" + partName(p.nextPartID) + ".mp4\"\n"

	return bytes.NewReader([]byte(cnt))
}

func (p *playlist) segmentReader(fname string) *MuxerFileResponse { //nolint:funlen
	switch {
	case strings.HasPrefix(fname, "seg"):
		base := strings.TrimSuffix(fname, ".mp4")

		p.mutex.Lock()
		segment, ok := p.segmentsByName[base]
		p.mutex.Unlock()

		if !ok {
			return &MuxerFileResponse{Status: http.StatusNotFound}
		}

		return &MuxerFileResponse{
			Status: http.StatusOK,
			Header: map[string]string{
				"Content-Type": "video/mp4",
			},
			Body: segment.reader(),
		}

	case strings.HasPrefix(fname, "part"):
		base := strings.TrimSuffix(fname, ".mp4")

		p.mutex.Lock()
		part, ok := p.partsByName[base]
		nextPartID := p.nextPartID
		p.mutex.Unlock()

		if ok {
			return &MuxerFileResponse{
				Status: http.StatusOK,
				Header: map[string]string{
					"Content-Type": "video/mp4",
				},
				Body: part.reader(),
			}
		}

		if fname == partName(p.nextPartID) {
			p.mutex.Lock()
			defer p.mutex.Unlock()

			for {
				if p.closed {
					break
				}

				if p.nextPartID > nextPartID {
					break
				}

				p.cond.Wait()
			}

			if p.closed {
				return &MuxerFileResponse{Status: http.StatusInternalServerError}
			}

			return &MuxerFileResponse{
				Status: http.StatusOK,
				Header: map[string]string{
					"Content-Type": "video/mp4",
				},
				Body: p.partsByName[partName(nextPartID)].reader(),
			}
		}

		return &MuxerFileResponse{Status: http.StatusNotFound}

	default:
		return &MuxerFileResponse{Status: http.StatusNotFound}
	}
}

func (p *playlist) onSegmentFinalized(segment *Segment) {
	p.mutex.Lock()

	// Create initial gap.
	if len(p.segments) == 0 {
		for i := 0; i < p.segmentCount; i++ {
			p.segments = append(p.segments, &Gap{
				renderedDuration: segment.renderedDuration,
			})
		}
	}

	p.segmentsByName[segment.name()] = segment
	p.segments = append(p.segments, segment)
	p.nextSegmentID = segment.id + 1
	p.nextSegmentParts = p.nextSegmentParts[:0]

	if len(p.segments) > p.segmentCount {
		toDelete := p.segments[0]

		if toDeleteSeg, ok := toDelete.(*Segment); ok {
			for _, part := range toDeleteSeg.parts {
				delete(p.partsByName, part.name())
			}
			p.parts = p.parts[len(toDeleteSeg.parts):]

			delete(p.segmentsByName, toDeleteSeg.name())
		}

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

func (p *playlist) onPartFinalized(part *muxerPart) {
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		p.partsByName[part.name()] = part
		p.parts = append(p.parts, part)
		p.nextSegmentParts = append(p.nextSegmentParts, part)
		p.nextPartID = part.id + 1
	}()

	p.cond.Broadcast()
}
