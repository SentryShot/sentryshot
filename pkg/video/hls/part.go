package hls

import (
	"bytes"
	"io"
	"log"
	"math"
	"nvr/pkg/video/gortsplib/pkg/aac"
	"nvr/pkg/video/mp4"
	"nvr/pkg/video/mp4/bitio"
	"strconv"
	"time"
)

type myMdat struct {
	videoSamples []*VideoSample
	audioSamples []*AudioSample
}

func (*myMdat) Type() mp4.BoxType {
	return [4]byte{'m', 'd', 'a', 't'}
}

func (b *myMdat) Size() int {
	var total int
	for _, e := range b.videoSamples {
		total += len(e.Avcc)
	}
	for _, e := range b.audioSamples {
		total += len(e.Au)
	}
	return total
}

func (b *myMdat) Marshal(w *bitio.Writer) error {
	for _, e := range b.videoSamples {
		if _, err := w.Write(e.Avcc); err != nil {
			return err
		}
	}
	for _, e := range b.audioSamples {
		if _, err := w.Write(e.Au); err != nil {
			return err
		}
	}
	return nil
}

const _3000Days = 3000 * (time.Hour * 24)

// DurationGoToMp4 this function will overflow if the input is greater than 3000 days.
func DurationGoToMp4(v time.Duration, timescale time.Duration) int64 {
	if v > _3000Days {
		log.Fatal("You win!")
	}
	v /= 1000
	return int64(math.Round(float64(v*timescale) / float64(time.Millisecond)))
}

func generateVideoTraf( //nolint:funlen
	trackID int,
	videoSamples []*VideoSample,
	dataOffset int32,
) mp4.Boxes {
	/*
	   traf
	   - tfhd
	   - tfdt
	   - trun
	*/

	tfhd := &mp4.Tfhd{
		FullBox: mp4.FullBox{
			Flags: [3]byte{2, 0, 0},
		},
		TrackID: uint32(trackID),
	}

	tfdt := &mp4.Tfdt{
		FullBox: mp4.FullBox{
			Version: 1,
		},
		// sum of decode durations of all earlier samples
		BaseMediaDecodeTimeV1: uint64(
			DurationGoToMp4(videoSamples[0].Dts, VideoTimescale)),
	}

	flags := 0
	flags |= 0x01      // data offset present
	flags |= 0x100     // sample duration present
	flags |= 0x200     // sample size present
	flags |= 0x400     // sample flags present
	flags |= 0x800     // sample composition time offset present or v1
	trun := &mp4.Trun{ // <trun/>
		FullBox: mp4.FullBox{
			Version: 1,
			Flags:   [3]byte{0, byte(flags >> 8), byte(flags)},
		},
		SampleCount: uint32(len(videoSamples)),
		DataOffset:  dataOffset,
	}

	trun.Entries = make([]mp4.TrunEntry, len(videoSamples))
	for i, e := range videoSamples {
		off := e.Pts - e.Dts

		flags := uint32(0)
		if !e.IdrPresent {
			flags |= 1 << 16 // sample_is_non_sync_sample
		}
		trun.Entries[i] = mp4.TrunEntry{
			SampleDuration:                uint32(DurationGoToMp4(e.duration(), VideoTimescale)),
			SampleSize:                    uint32(len(e.Avcc)),
			SampleFlags:                   flags,
			SampleCompositionTimeOffsetV1: int32(DurationGoToMp4(off, VideoTimescale)),
		}
	}

	return mp4.Boxes{
		Box: &mp4.Traf{},
		Children: []mp4.Boxes{
			{Box: tfhd},
			{Box: tfdt},
			{Box: trun},
		},
	}
}

func generateAudioTraf(
	trackID int,
	audioClockRate int,
	audioSamples []*AudioSample,
	dataOffset int32,
) mp4.Boxes {
	/*
	   traf
	   - tfhd
	   - tfdt
	   - trun
	*/

	tfhd := &mp4.Tfhd{
		FullBox: mp4.FullBox{
			Flags: [3]byte{2, 0, 0},
		},
		TrackID: uint32(trackID),
	}

	tfdt := &mp4.Tfdt{ // <tfdt/>
		FullBox: mp4.FullBox{
			Version: 1,
		},
		BaseMediaDecodeTimeV1: uint64(
			DurationGoToMp4(audioSamples[0].Pts,
				time.Duration(audioClockRate))),
	}

	flags := 0
	flags |= 0x01  // data offset present
	flags |= 0x100 // sample duration present
	flags |= 0x200 // sample size present

	trun := &mp4.Trun{ // <trun/>
		FullBox: mp4.FullBox{
			Version: 0,
			Flags:   [3]byte{0, byte(flags >> 8), byte(flags)},
		},
		SampleCount: uint32(len(audioSamples)),
		DataOffset:  dataOffset,
		Entries:     nil,
	}

	trun.Entries = make([]mp4.TrunEntry, len(audioSamples))
	for i, e := range audioSamples {
		trun.Entries[i] = mp4.TrunEntry{
			SampleDuration: uint32(DurationGoToMp4(e.Duration(), time.Duration(audioClockRate))),
			SampleSize:     uint32(len(e.Au)),
		}
	}

	return mp4.Boxes{
		Box: &mp4.Traf{},
		Children: []mp4.Boxes{
			{Box: tfhd},
			{Box: tfdt},
			{Box: trun},
		},
	}
}

func generatePart( //nolint:funlen
	audioTrackExist bool,
	audioClockRate func() int,
	videoSamples []*VideoSample,
	audioSamples []*AudioSample,
) ([]byte, error) {
	/*
	   moof
	   - mfhd
	   - traf (video)
	     - tfhd
	     - tfdt
	     - trun
	   - traf (audio)
	     - tfhd
	     - tfdt
	     - trun
	   mdat
	*/

	moof := mp4.Boxes{
		Box: &mp4.Moof{},
		Children: []mp4.Boxes{
			{Box: &mp4.Mfhd{
				SequenceNumber: 0,
			}},
		},
	}

	mfhdOffset := 24
	videoTrunSize := len(videoSamples)*16 + 20
	audioOffset := mfhdOffset + videoTrunSize + 44

	mdatOffset := audioOffset
	if audioTrackExist && len(audioSamples) != 0 {
		audioTrunOffset := audioOffset + 44
		audioTrunSize := len(audioSamples)*8 + 20
		mdatOffset = audioTrunOffset + audioTrunSize
	}

	trackID := 1
	videoDataOffset := int32(mdatOffset + 8)
	traf := generateVideoTraf(trackID, videoSamples, videoDataOffset)
	moof.Children = append(moof.Children, traf)
	trackID++

	dataSize := 0
	for _, e := range videoSamples {
		dataSize += len(e.Avcc)
	}
	videoDataSize := dataSize
	if audioTrackExist {
		for _, e := range audioSamples {
			dataSize += len(e.Au)
		}
	}

	if audioTrackExist && len(audioSamples) != 0 {
		audioDataOffset := int32(mdatOffset + 8 + videoDataSize)
		traf := generateAudioTraf(
			trackID,
			audioClockRate(),
			audioSamples,
			audioDataOffset)
		moof.Children = append(moof.Children, traf)
	}

	mdat := &mp4.Boxes{
		Box: &myMdat{
			videoSamples: videoSamples,
			audioSamples: audioSamples,
		},
	}

	size := moof.Size() + mdat.Size()
	buf := bytes.NewBuffer(make([]byte, 0, size))

	w := bitio.NewWriter(buf)

	if err := moof.Marshal(w); err != nil {
		return nil, err
	}

	if err := mdat.Marshal(w); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func partName(id uint64) string {
	return "part" + strconv.FormatUint(id, 10)
}

// MuxerPart fmp4 part.
type MuxerPart struct {
	videoTrackExist func() bool
	audioTrackExist func() bool
	audioClockRate  audioClockRateFunc
	id              uint64

	isIndependent    bool
	VideoSamples     []*VideoSample
	AudioSamples     []*AudioSample
	renderedContent  []byte
	renderedDuration time.Duration
}

type audioClockRateFunc func() int

func newPart(
	videoTrackExist func() bool,
	audioTrackExist func() bool,
	audioClockRate audioClockRateFunc,
	id uint64,
) *MuxerPart {
	p := &MuxerPart{
		videoTrackExist: videoTrackExist,
		audioTrackExist: audioTrackExist,
		audioClockRate:  audioClockRate,
		id:              id,
	}

	if !videoTrackExist() {
		p.isIndependent = true
	}

	return p
}

func (p *MuxerPart) name() string {
	return partName(p.id)
}

func (p *MuxerPart) reader() io.Reader {
	return bytes.NewReader(p.renderedContent)
}

func (p *MuxerPart) duration() time.Duration {
	if p.videoTrackExist() {
		ret := time.Duration(0)
		for _, e := range p.VideoSamples {
			ret += e.duration()
		}
		return ret
	}

	// use the sum of the default duration of all samples,
	// not the real duration,
	// otherwise on iPhone iOS the stream freezes.
	return time.Duration(len(p.AudioSamples)) * time.Second *
		time.Duration(aac.SamplesPerAccessUnit) / time.Duration(p.audioClockRate())
}

func (p *MuxerPart) finalize() error {
	if len(p.VideoSamples) > 0 || len(p.AudioSamples) > 0 {
		var err error
		p.renderedContent, err = generatePart(
			p.audioTrackExist(),
			p.audioClockRate,
			p.VideoSamples,
			p.AudioSamples)
		if err != nil {
			return err
		}
		p.renderedDuration = p.duration()
	}

	return nil
}

func (p *MuxerPart) writeH264(sample *VideoSample) {
	if sample.IdrPresent {
		p.isIndependent = true
	}
	p.VideoSamples = append(p.VideoSamples, sample)
}

func (p *MuxerPart) writeAAC(sample *AudioSample) {
	p.AudioSamples = append(p.AudioSamples, sample)
}
