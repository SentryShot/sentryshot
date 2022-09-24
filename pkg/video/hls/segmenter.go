package hls

import (
	"bytes"
	"nvr/pkg/video/gortsplib/pkg/h264"
	"time"
)

func partDurationIsCompatible(partDuration time.Duration, sampleDuration time.Duration) bool {
	if sampleDuration > partDuration {
		return false
	}

	f := (partDuration / sampleDuration)
	if (partDuration % sampleDuration) != 0 {
		f++
	}
	f *= sampleDuration

	return partDuration > ((f * 85) / 100)
}

func findCompatiblePartDuration(
	minPartDuration time.Duration,
	sampleDurations map[time.Duration]struct{},
) time.Duration {
	i := minPartDuration
	for ; i < 5*time.Second; i += 5 * time.Millisecond {
		isCompatible := func() bool {
			for sd := range sampleDurations {
				if !partDurationIsCompatible(i, sd) {
					return false
				}
			}
			return true
		}()
		if isCompatible {
			break
		}
	}
	return i
}

type segmenter struct {
	segmentDuration    time.Duration
	partDuration       time.Duration
	segmentMaxSize     uint64
	videoTrackExist    func() bool
	videoSps           videoSPSFunc
	audioTrackExist    func() bool
	audioClockRate     audioClockRateFunc
	onSegmentFinalized func(*Segment)
	onPartFinalized    func(*MuxerPart)

	startDTS              time.Duration
	muxerStartTime        int64
	videoFirstIDRReceived bool
	videoDTSExtractor     *h264.DTSExtractor
	nextSegmentID         uint64
	videoSPS              []byte
	currentSegment        *Segment
	nextPartID            uint64
	nextVideoSample       *VideoSample
	nextAudioSample       *AudioSample
	firstSegmentFinalized bool
	sampleDurations       map[time.Duration]struct{}
	adjustedPartDuration  time.Duration
}

type videoSPSFunc func() []byte

func newSegmenter(
	muxerStartTime int64,
	segmentDuration time.Duration,
	partDuration time.Duration,
	segmentMaxSize uint64,
	videoTrackExist func() bool,
	videoSps videoSPSFunc,
	audioTrackExist func() bool,
	audioClockRate audioClockRateFunc,
	onSegmentFinalized func(*Segment),
	onPartFinalized func(*MuxerPart),
) *segmenter {
	return &segmenter{
		segmentDuration:    segmentDuration,
		partDuration:       partDuration,
		segmentMaxSize:     segmentMaxSize,
		videoTrackExist:    videoTrackExist,
		videoSps:           videoSps,
		audioTrackExist:    audioTrackExist,
		audioClockRate:     audioClockRate,
		onSegmentFinalized: onSegmentFinalized,
		onPartFinalized:    onPartFinalized,
		muxerStartTime:     muxerStartTime,
		nextSegmentID:      7, // Required by iOS.
		sampleDurations:    make(map[time.Duration]struct{}),
	}
}

func (m *segmenter) genSegmentID() uint64 {
	id := m.nextSegmentID
	m.nextSegmentID++
	return id
}

func (m *segmenter) genPartID() uint64 {
	id := m.nextPartID
	m.nextPartID++
	return id
}

// iPhone iOS fails if part durations are less than 85% of maximum part duration.
// find a part duration that is compatible with all received sample durations.
func (m *segmenter) adjustPartDuration(du time.Duration) {
	if m.firstSegmentFinalized {
		return
	}

	if _, ok := m.sampleDurations[du]; !ok {
		m.sampleDurations[du] = struct{}{}
		m.adjustedPartDuration = findCompatiblePartDuration(
			m.partDuration,
			m.sampleDurations,
		)
	}
}

func (m *segmenter) writeH264(now time.Time, pts time.Duration, nalus [][]byte) error {
	idrPresent := false
	nonIDRPresent := false

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeIDR:
			idrPresent = true

		case h264.NALUTypeNonIDR:
			nonIDRPresent = true
		}
	}

	if !idrPresent && !nonIDRPresent {
		return nil
	}

	avcc := h264.AVCCMarshal(nalus)

	var dts time.Duration
	if !m.videoFirstIDRReceived {
		// skip sample silently until we find one with an IDR
		if !idrPresent {
			return nil
		}

		m.videoFirstIDRReceived = true
		m.videoDTSExtractor = h264.NewDTSExtractor()
		m.videoSPS = m.videoSps()

		var err error
		dts, err = m.videoDTSExtractor.Extract(nalus, dts)
		if err != nil {
			return err
		}

		m.startDTS = dts
		dts = 0
		pts -= m.startDTS
	} else {
		var err error
		dts, err = m.videoDTSExtractor.Extract(nalus, pts)
		if err != nil {
			return err
		}

		pts -= m.startDTS
		dts -= m.startDTS
	}

	return m.writeH264Entry(now, &VideoSample{
		PTS:        m.muxerStartTime + int64(pts),
		DTS:        m.muxerStartTime + int64(dts),
		AVCC:       avcc,
		IdrPresent: idrPresent,
	})
}

func (m *segmenter) writeH264Entry(now time.Time, sample *VideoSample) error { //nolint:funlen
	sample, m.nextVideoSample = m.nextVideoSample, sample
	if sample == nil {
		return nil
	}

	next := m.nextVideoSample
	sample.NextDTS = next.DTS

	if m.currentSegment == nil {
		// create first segment
		m.currentSegment = newSegment(
			m.genSegmentID(),
			now,
			time.Duration(sample.DTS-m.muxerStartTime),
			m.muxerStartTime,
			m.segmentMaxSize,
			m.videoTrackExist,
			m.audioTrackExist,
			m.audioClockRate,
			m.genPartID,
			m.onPartFinalized,
		)
	}

	m.adjustPartDuration(sample.duration())

	err := m.currentSegment.writeH264(sample, m.adjustedPartDuration)
	if err != nil {
		return err
	}

	if !next.IdrPresent {
		return nil
	}
	// switch segment
	sps := m.videoSps()
	spsChanged := !bytes.Equal(m.videoSPS, sps)

	if (time.Duration(sample.NextDTS-m.muxerStartTime)-
		m.currentSegment.startDTS) >= m.segmentDuration || spsChanged {
		err := m.currentSegment.finalize(next)
		if err != nil {
			return err
		}
		m.onSegmentFinalized(m.currentSegment)

		m.firstSegmentFinalized = true

		m.currentSegment = newSegment(
			m.genSegmentID(),
			now,
			time.Duration(next.PTS-m.muxerStartTime),
			m.muxerStartTime,
			m.segmentMaxSize,
			m.videoTrackExist,
			m.audioTrackExist,
			m.audioClockRate,
			m.genPartID,
			m.onPartFinalized,
		)

		// if SPS changed, reset adjusted part duration
		if spsChanged {
			m.videoSPS = sps
			m.firstSegmentFinalized = false
			m.sampleDurations = make(map[time.Duration]struct{})
		}
	}

	return nil
}

func (m *segmenter) writeAAC(now time.Time, pts time.Duration, au []byte) error {
	return m.writeAACEntry(now, &AudioSample{
		PTS: int64(pts),
		AU:  au,
	})
}

func (m *segmenter) writeAACEntry(now time.Time, sample *AudioSample) error { //nolint:funlen
	if m.videoTrackExist() {
		// wait for the video track
		if !m.videoFirstIDRReceived {
			return nil
		}

		sample.PTS -= int64(m.startDTS)
	}

	sample.PTS += m.muxerStartTime

	// put samples into a queue in order to
	// allow to compute the sample duration
	sample, m.nextAudioSample = m.nextAudioSample, sample
	if sample == nil {
		return nil
	}

	sample.NextPTS = m.nextAudioSample.PTS

	if !m.videoTrackExist() {
		if m.currentSegment == nil {
			// create first segment
			m.currentSegment = newSegment(
				m.genSegmentID(),
				now,
				time.Duration(sample.PTS),
				m.muxerStartTime,
				m.segmentMaxSize,
				m.videoTrackExist,
				m.audioTrackExist,
				m.audioClockRate,
				m.genPartID,
				m.onPartFinalized,
			)
		}
	} else {
		// wait for the video track
		if m.currentSegment == nil {
			return nil
		}
	}

	err := m.currentSegment.writeAAC(sample, m.partDuration)
	if err != nil {
		return err
	}

	// switch segment
	if !m.videoTrackExist() &&
		(time.Duration(sample.NextPTS-m.muxerStartTime)-
			m.currentSegment.startDTS) >= m.segmentDuration {
		err := m.currentSegment.finalize(nil)
		if err != nil {
			return nil
		}
		m.onSegmentFinalized(m.currentSegment)

		m.firstSegmentFinalized = true

		m.currentSegment = newSegment(
			m.genSegmentID(),
			now,
			time.Duration(sample.NextPTS),
			m.muxerStartTime,
			m.segmentMaxSize,
			m.videoTrackExist,
			m.audioTrackExist,
			m.audioClockRate,
			m.genPartID,
			m.onPartFinalized,
		)
	}

	return nil
}
