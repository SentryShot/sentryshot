package hls

import (
	"bytes"
	"io"
	"math"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/aac"
	"nvr/pkg/video/hls/mp4"
	"strconv"
	"time"

	gomp4 "github.com/abema/go-mp4"
)

func durationGoToMp4(v time.Duration, timescale time.Duration) int64 {
	return int64(math.Round(float64(v*timescale) / float64(time.Second)))
}

func mp4PartGenerateVideoTraf( //nolint:funlen
	w *mp4.Writer,
	trackID int,
	videoSamples []*videoSample,
) (*gomp4.Trun, int, error) {
	/*
		traf
		- tfhd
		- tfdt
		- trun
	*/

	_, err := w.WriteBoxStart(&gomp4.Traf{}) // <traf>
	if err != nil {
		return nil, 0, err
	}

	flags := 0

	_, err = w.WriteBox(&gomp4.Tfhd{ // <tfhd/>
		FullBox: gomp4.FullBox{
			Flags: [3]byte{2, byte(flags >> 8), byte(flags)},
		},
		TrackID: uint32(trackID),
	})
	if err != nil {
		return nil, 0, err
	}

	_, err = w.WriteBox(&gomp4.Tfdt{ // <tfdt/>
		FullBox: gomp4.FullBox{
			Version: 1,
		},
		// sum of decode durations of all earlier samples
		BaseMediaDecodeTimeV1: uint64(durationGoToMp4(videoSamples[0].dts, videoTimescale)),
	})
	if err != nil {
		return nil, 0, err
	}

	flags = 0
	flags |= 0x01  // data offset present
	flags |= 0x100 // sample duration present
	flags |= 0x200 // sample size present
	flags |= 0x400 // sample flags present
	flags |= 0x800 // sample composition time offset present or v1

	trun := &gomp4.Trun{ // <trun/>
		FullBox: gomp4.FullBox{
			Version: 1,
			Flags:   [3]byte{0, byte(flags >> 8), byte(flags)},
		},
		SampleCount: uint32(len(videoSamples)),
	}

	for _, e := range videoSamples {
		off := e.pts - e.dts

		flags := uint32(0)
		if !e.idrPresent {
			flags |= 1 << 16 // sample_is_non_sync_sample
		}

		trun.Entries = append(trun.Entries, gomp4.TrunEntry{
			SampleDuration:                uint32(durationGoToMp4(e.duration(), videoTimescale)),
			SampleSize:                    uint32(len(e.avcc)),
			SampleFlags:                   flags,
			SampleCompositionTimeOffsetV1: int32(durationGoToMp4(off, videoTimescale)),
		})
	}

	trunOffset, err := w.WriteBox(trun)
	if err != nil {
		return nil, 0, err
	}

	err = w.WriteBoxEnd() // </traf>
	if err != nil {
		return nil, 0, err
	}

	return trun, trunOffset, nil
}

func mp4PartGenerateAudioTraf( //nolint:funlen
	w *mp4.Writer,
	trackID int,
	audioTrack *gortsplib.TrackAAC,
	audioSamples []*audioSample,
) (*gomp4.Trun, int, error) {
	/*
		traf
		- tfhd
		- tfdt
		- trun
	*/

	if len(audioSamples) == 0 {
		return nil, 0, nil
	}

	_, err := w.WriteBoxStart(&gomp4.Traf{}) // <traf>
	if err != nil {
		return nil, 0, err
	}

	flags := 0

	_, err = w.WriteBox(&gomp4.Tfhd{ // <tfhd/>
		FullBox: gomp4.FullBox{
			Flags: [3]byte{2, byte(flags >> 8), byte(flags)},
		},
		TrackID: uint32(trackID),
	})
	if err != nil {
		return nil, 0, err
	}

	_, err = w.WriteBox(&gomp4.Tfdt{ // <tfdt/>
		FullBox: gomp4.FullBox{
			Version: 1,
		},
		// sum of decode durations of all earlier samples
		BaseMediaDecodeTimeV1: uint64(durationGoToMp4(audioSamples[0].pts, time.Duration(audioTrack.ClockRate()))),
	})
	if err != nil {
		return nil, 0, err
	}

	flags = 0
	flags |= 0x01  // data offset present
	flags |= 0x100 // sample duration present
	flags |= 0x200 // sample size present

	trun := &gomp4.Trun{ // <trun/>
		FullBox: gomp4.FullBox{
			Version: 0,
			Flags:   [3]byte{0, byte(flags >> 8), byte(flags)},
		},
		SampleCount: uint32(len(audioSamples)),
	}

	for _, e := range audioSamples {
		trun.Entries = append(trun.Entries, gomp4.TrunEntry{
			SampleDuration: uint32(durationGoToMp4(e.duration(), time.Duration(audioTrack.ClockRate()))),
			SampleSize:     uint32(len(e.au)),
		})
	}

	trunOffset, err := w.WriteBox(trun)
	if err != nil {
		return nil, 0, err
	}

	err = w.WriteBoxEnd() // </traf>
	if err != nil {
		return nil, 0, err
	}

	return trun, trunOffset, nil
}

func mp4PartGenerate( //nolint:funlen,gocognit
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
	videoSamples []*videoSample,
	audioSamples []*audioSample,
) ([]byte, error) {
	/*
		moof
		- mfhd
		- traf (video)
		- traf (audio)
		mdat
	*/

	w := mp4.NewWriter()

	moofOffset, err := w.WriteBoxStart(&gomp4.Moof{}) // <moof>
	if err != nil {
		return nil, err
	}

	_, err = w.WriteBox(&gomp4.Mfhd{ // <mfhd/>
		SequenceNumber: 0,
	})
	if err != nil {
		return nil, err
	}

	trackID := 1

	var videoTrun *gomp4.Trun
	var videoTrunOffset int
	if videoTrack != nil {
		var err error
		videoTrun, videoTrunOffset, err = mp4PartGenerateVideoTraf(
			w, trackID, videoSamples)
		if err != nil {
			return nil, err
		}

		trackID++
	}

	var audioTrun *gomp4.Trun
	var audioTrunOffset int
	if audioTrack != nil {
		var err error
		audioTrun, audioTrunOffset, err = mp4PartGenerateAudioTraf(w, trackID, audioTrack, audioSamples)
		if err != nil {
			return nil, err
		}
	}

	err = w.WriteBoxEnd() // </moof>
	if err != nil {
		return nil, err
	}

	mdat := &gomp4.Mdat{} // <mdat/>

	dataSize := 0
	videoDataSize := 0

	if videoTrack != nil {
		for _, e := range videoSamples {
			dataSize += len(e.avcc)
		}
		videoDataSize = dataSize
	}

	if audioTrack != nil {
		for _, e := range audioSamples {
			dataSize += len(e.au)
		}
	}

	mdat.Data = make([]byte, dataSize)
	pos := 0

	if videoTrack != nil {
		for _, e := range videoSamples {
			pos += copy(mdat.Data[pos:], e.avcc)
		}
	}

	if audioTrack != nil {
		for _, e := range audioSamples {
			pos += copy(mdat.Data[pos:], e.au)
		}
	}

	mdatOffset, err := w.WriteBox(mdat)
	if err != nil {
		return nil, err
	}

	if videoTrack != nil {
		videoTrun.DataOffset = int32(mdatOffset - moofOffset + 8)
		err = w.RewriteBox(videoTrunOffset, videoTrun)
		if err != nil {
			return nil, err
		}
	}

	if audioTrack != nil && audioTrun != nil {
		audioTrun.DataOffset = int32(videoDataSize + mdatOffset - moofOffset + 8)
		err = w.RewriteBox(audioTrunOffset, audioTrun)
		if err != nil {
			return nil, err
		}
	}

	return w.Bytes(), nil
}

func partName(id uint64) string {
	return "part" + strconv.FormatUint(id, 10)
}

type muxerPart struct {
	videoTrack *gortsplib.TrackH264
	audioTrack *gortsplib.TrackAAC
	id         uint64

	isIndependent    bool
	videoSamples     []*videoSample
	audioSamples     []*audioSample
	renderedContent  []byte
	renderedDuration time.Duration
}

func newPart(
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
	id uint64,
) *muxerPart {
	p := &muxerPart{
		videoTrack: videoTrack,
		audioTrack: audioTrack,
		id:         id,
	}

	if videoTrack == nil {
		p.isIndependent = true
	}

	return p
}

func (p *muxerPart) name() string {
	return partName(p.id)
}

func (p *muxerPart) reader() io.Reader {
	return bytes.NewReader(p.renderedContent)
}

func (p *muxerPart) duration() time.Duration {
	if p.videoTrack != nil {
		ret := time.Duration(0)
		for _, e := range p.videoSamples {
			ret += e.duration()
		}
		return ret
	}

	// use the sum of the default duration of all samples,
	// not the real duration,
	// otherwise on iPhone iOS the stream freezes.
	return time.Duration(len(p.audioSamples)) * time.Second *
		time.Duration(aac.SamplesPerAccessUnit) / time.Duration(p.audioTrack.ClockRate())
}

func (p *muxerPart) finalize() error {
	if len(p.videoSamples) > 0 || len(p.audioSamples) > 0 {
		var err error
		p.renderedContent, err = mp4PartGenerate(
			p.videoTrack,
			p.audioTrack,
			p.videoSamples,
			p.audioSamples)
		if err != nil {
			return err
		}

		p.renderedDuration = p.duration()
	}

	p.videoSamples = nil
	p.audioSamples = nil

	return nil
}

func (p *muxerPart) writeH264(sample *videoSample) {
	if sample.idrPresent {
		p.isIndependent = true
	}
	p.videoSamples = append(p.videoSamples, sample)
}

func (p *muxerPart) writeAAC(sample *audioSample) {
	p.audioSamples = append(p.audioSamples, sample)
}
