package customformat

import (
	"fmt"
	"io"
	"log"
	"nvr/pkg/video/hls"
	"sort"
)

// Writer writes videos in our custom format.
type Writer struct {
	meta io.Writer // Output file.
	mdat io.Writer // Output file.

	mdatPos int
}

// NewWriter creates a new Writer and writes the header.
func NewWriter(meta io.Writer, mdat io.Writer, header Header) (*Writer, error) {
	w := &Writer{
		meta: meta,
		mdat: mdat,
	}

	_, err := meta.Write(header.Marshal())
	if err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}

	return w, nil
}

// WriteSegment Writes a HLS segment in the custom format to the output files.
func (w *Writer) WriteSegment(segment *hls.Segment) error {
	samples := sortSamples(*segment)
	for _, s := range samples {
		switch v := s.(type) {
		case *hls.VideoSample:
			if err := w.writeVideoSample(v); err != nil {
				return fmt.Errorf("write video sample: %w", err)
			}
		case *hls.AudioSample:
			if err := w.writeAudioSample(v); err != nil {
				return fmt.Errorf("write audio sample: %w", err)
			}
		default:
			log.Fatalf("unexpected type: %v", v)
		}
	}
	return nil
}

type pair struct {
	pts    int64
	sample hls.Sample
}

func sortSamples(segment hls.Segment) []hls.Sample {
	pairs := []pair{}
	for _, part := range segment.Parts {
		for _, videoSample := range part.VideoSamples {
			pairs = append(pairs, pair{
				pts:    videoSample.DTS,
				sample: videoSample,
			})
		}
		for _, audioSample := range part.AudioSamples {
			pairs = append(pairs, pair{
				pts:    audioSample.PTS,
				sample: audioSample,
			})
		}
	}

	lessFunc := func(i, j int) bool {
		return pairs[i].pts < pairs[j].pts
	}
	sort.Slice(pairs, lessFunc)

	sorted := make([]hls.Sample, len(pairs))
	for i := 0; i < len(pairs); i++ {
		sorted[i] = pairs[i].sample
	}
	return sorted
}

func (w *Writer) writeVideoSample(sample *hls.VideoSample) error {
	s := Sample{
		IsSyncSample: sample.IdrPresent,
		PTS:          sample.PTS,
		DTS:          sample.DTS,
		Next:         sample.NextDTS,
		Offset:       uint32(w.mdatPos),
		Size:         uint32(len(sample.AVCC)),
	}
	marshaled := s.Marshal()

	n, err := w.mdat.Write(sample.AVCC)
	if err != nil {
		return err
	}
	w.mdatPos += n

	_, err = w.meta.Write(marshaled)
	if err != nil {
		return err
	}

	return nil
}

func (w *Writer) writeAudioSample(sample *hls.AudioSample) error {
	s := Sample{
		IsAudioSample: true,
		PTS:           sample.PTS,
		Next:          sample.NextPTS,
		Offset:        uint32(w.mdatPos),
		Size:          uint32(len(sample.AU)),
	}
	marshaled := s.Marshal()

	n, err := w.mdat.Write(sample.AU)
	if err != nil {
		return err
	}
	w.mdatPos += n

	_, err = w.meta.Write(marshaled)
	if err != nil {
		return err
	}

	return nil
}
