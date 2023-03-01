package hls

import (
	"bytes"
	"io"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/mp4"
	"nvr/pkg/video/mp4/bitio"
	"strconv"
	"time"
)

type myMdat struct {
	videoSamples []*VideoSample
	audioSamples []*AudioSample
}

func (*myMdat) Type() mp4.BoxType { return mp4.TypeMdat() }

func (b *myMdat) Size() int {
	var total int
	for _, e := range b.videoSamples {
		total += len(e.AVCC)
	}
	for _, e := range b.audioSamples {
		total += len(e.AU)
	}
	return total
}

func (b *myMdat) Marshal(w *bitio.Writer) error {
	for _, e := range b.videoSamples {
		if _, err := w.Write(e.AVCC); err != nil {
			return err
		}
	}
	for _, e := range b.audioSamples {
		if _, err := w.Write(e.AU); err != nil {
			return err
		}
	}
	return nil
}

// NanoToTimescale converts value in nanoseconds into a different timescale.
func NanoToTimescale(value int64, timescale int64) int64 {
	secs := value / int64(time.Second)
	dec := value % int64(time.Second)
	return secs*timescale + dec*timescale/int64(time.Second)
}

func generateVideoTraf( //nolint:funlen
	muxerStartTime int64,
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
			NanoToTimescale(
				videoSamples[0].DTS-muxerStartTime, VideoTimescale)),
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
		DataOffset: dataOffset,
	}

	trun.Entries = make([]mp4.TrunEntry, len(videoSamples))
	for i, e := range videoSamples {
		off := e.PTS - e.DTS

		flags := uint32(0)
		if !e.IdrPresent {
			flags |= 1 << 16 // sample_is_non_sync_sample
		}
		trun.Entries[i] = mp4.TrunEntry{
			SampleDuration: uint32(
				NanoToTimescale(int64(e.duration()), VideoTimescale)),
			SampleSize:                    uint32(len(e.AVCC)),
			SampleFlags:                   flags,
			SampleCompositionTimeOffsetV1: int32(NanoToTimescale(off, VideoTimescale)),
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
	muxerStartTime int64,
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
			NanoToTimescale(
				audioSamples[0].PTS-muxerStartTime, int64(audioClockRate))),
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
		DataOffset: dataOffset,
		Entries:    nil,
	}

	trun.Entries = make([]mp4.TrunEntry, len(audioSamples))
	for i, e := range audioSamples {
		trun.Entries[i] = mp4.TrunEntry{
			SampleDuration: uint32(
				NanoToTimescale(int64(e.Duration()), int64(audioClockRate))),
			SampleSize: uint32(len(e.AU)),
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
	muxerStartTime int64,
	audioTrack *gortsplib.TrackMPEG4Audio,
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
	audioTrackExist := audioTrack != nil
	if audioTrackExist && len(audioSamples) != 0 {
		audioTrunOffset := audioOffset + 44
		audioTrunSize := len(audioSamples)*8 + 20
		mdatOffset = audioTrunOffset + audioTrunSize
	}

	trackID := 1
	videoDataOffset := int32(mdatOffset + 8)
	traf := generateVideoTraf(
		muxerStartTime,
		trackID,
		videoSamples,
		videoDataOffset)
	moof.Children = append(moof.Children, traf)
	trackID++

	dataSize := 0
	for _, e := range videoSamples {
		dataSize += len(e.AVCC)
	}
	videoDataSize := dataSize
	if audioTrackExist {
		for _, e := range audioSamples {
			dataSize += len(e.AU)
		}
	}

	if audioTrackExist && len(audioSamples) != 0 {
		audioDataOffset := int32(mdatOffset + 8 + videoDataSize)
		traf := generateAudioTraf(
			muxerStartTime,
			trackID,
			audioTrack.ClockRate(),
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
	audioTrack     *gortsplib.TrackMPEG4Audio
	muxerStartTime int64
	id             uint64

	isIndependent    bool
	VideoSamples     []*VideoSample
	AudioSamples     []*AudioSample
	renderedContent  []byte
	renderedDuration time.Duration
}

func newPart(
	audioTrack *gortsplib.TrackMPEG4Audio,
	muxerStartTime int64,
	id uint64,
) *MuxerPart {
	return &MuxerPart{
		audioTrack:     audioTrack,
		muxerStartTime: muxerStartTime,
		id:             id,
	}
}

func (p *MuxerPart) name() string {
	return partName(p.id)
}

func (p *MuxerPart) reader() io.Reader {
	return bytes.NewReader(p.renderedContent)
}

func (p *MuxerPart) duration() time.Duration {
	total := time.Duration(0)
	for _, e := range p.VideoSamples {
		total += e.duration()
	}
	return total
}

func (p *MuxerPart) finalize() error {
	if len(p.VideoSamples) > 0 || len(p.AudioSamples) > 0 {
		var err error
		p.renderedContent, err = generatePart(
			p.muxerStartTime,
			p.audioTrack,
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
