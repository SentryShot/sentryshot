package mp4muxer

import (
	"errors"
	"fmt"
	"io"
	"nvr/pkg/video/hls"
	"nvr/pkg/video/mp4"
	"nvr/pkg/video/mp4/bitio"
)

// Errors.
var (
	ErrSampleMissing = errors.New("missing sample")
	ErrSampleInvalid = errors.New("sample invalid")
)

// GenerateThumbnailVideo generates a mp4 video with a single
// frame that will be converted to jpeg by FFmpeg.
func GenerateThumbnailVideo(
	out io.Writer,
	segment *hls.Segment,
	info hls.StreamInfo,
) error {
	if segment == nil || len(segment.Parts) == 0 ||
		len(segment.Parts[0].VideoSamples) == 0 {
		return ErrSampleMissing
	}

	sample := segment.Parts[0].VideoSamples[0]
	if !sample.IdrPresent {
		return fmt.Errorf("%w: first sample isn't a sync sample", ErrSampleInvalid)
	}

	bw := bitio.NewByteWriter(out)
	w := bitio.NewWriter(bw)

	ftyp := &mp4.Ftyp{
		MajorBrand:   [4]byte{'i', 's', 'o', '4'},
		MinorVersion: 512,
		CompatibleBrands: []mp4.CompatibleBrandElem{
			{CompatibleBrand: [4]byte{'i', 's', 'o', '4'}},
		},
	}
	_, err := mp4.WriteSingleBox(w, ftyp)
	if err != nil {
		return fmt.Errorf("write ftyp: %w", err)
	}

	/*
	   moov
	   - mvhd
	   - trak
	   mdat
	*/

	mdatOffset := 610 + uint32(len(info.VideoPPS)+len(info.VideoSPS))
	stco := []uint32{mdatOffset + 8}
	stsz := []uint32{uint32(len(sample.AVCC))}
	moov := mp4.Boxes{
		Box: &mp4.Moov{},
		Children: []mp4.Boxes{
			{Box: &mp4.Mvhd{
				Timescale:   1000,
				Rate:        65536,
				Volume:      256,
				Matrix:      [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
				NextTrackID: videoTrackID + 1,
			}},
			generateThumbnailVideoTrak(info, stsz, stco),
		},
	}
	if err := moov.Marshal(w); err != nil {
		return fmt.Errorf("marshal moov: %w", err)
	}

	_, err = mp4.WriteSingleBox(w, &mp4.Mdat{Data: sample.AVCC})
	if err != nil {
		return fmt.Errorf("write mdat: %w", err)
	}

	return nil
}

func generateThumbnailVideoTrak(
	info hls.StreamInfo,
	stsz []uint32,
	stco []uint32,
) mp4.Boxes {
	/*
	   trak
	   - tkhd
	   - mdia
	     - mdhd
	     - hdlr
	     - minf
	*/

	trak := mp4.Boxes{
		Box: &mp4.Trak{},
		Children: []mp4.Boxes{
			{Box: &mp4.Tkhd{
				FullBox: mp4.FullBox{
					Flags: [3]byte{0, 0, 3},
				},
				TrackID: videoTrackID,
				Width:   uint32(info.VideoWidth * 65536),
				Height:  uint32(info.VideoHeight * 65536),
				Matrix:  [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
			}},
			{
				Box: &mp4.Mdia{},
				Children: []mp4.Boxes{
					{Box: &mp4.Mdhd{
						Timescale: hls.VideoTimescale, // the number of time units that pass per second
						Language:  [3]byte{'u', 'n', 'd'},
					}},
					{Box: &mp4.Hdlr{
						HandlerType: [4]byte{'v', 'i', 'd', 'e'},
						Name:        "VideoHandler",
					}},
					generateThumbnailVideoMinf(info, stsz, stco),
				},
			},
		},
	}
	return trak
}

func generateThumbnailVideoMinf(
	info hls.StreamInfo,
	stsz []uint32,
	stco []uint32,
) mp4.Boxes {
	/*
	   minf
	   - vmhd
	   - dinf
	     - dref
	       - url
	     - stbl
	       - stsd
	       - stss
	       - stsc
	       - stsz
	       - stco
	*/
	stbl := mp4.Boxes{
		Box: &mp4.Stbl{},
		Children: []mp4.Boxes{
			generateVideoStsd(info),
			{Box: &mp4.Stts{
				Entries: []mp4.SttsEntry{
					{SampleCount: 1},
				},
			}},
			{Box: &mp4.Stsc{
				Entries: []mp4.StscEntry{{
					FirstChunk:             1,
					SamplesPerChunk:        1,
					SampleDescriptionIndex: 1,
				}},
			}},
			{Box: &mp4.Stsz{
				SampleCount: 1,
				EntrySizes:  stsz,
			}},
			{Box: &mp4.Stco{
				ChunkOffsets: stco,
			}},
		},
	}

	minf := mp4.Boxes{
		Box: &mp4.Minf{},
		Children: []mp4.Boxes{
			{Box: &mp4.Vmhd{}},
			{
				Box: &mp4.Dinf{},
				Children: []mp4.Boxes{
					{
						Box: &mp4.Dref{EntryCount: 1},
						Children: []mp4.Boxes{
							{Box: &mp4.URL{
								FullBox: mp4.FullBox{Flags: [3]byte{0, 0, 1}},
							}},
						},
					},
				},
			},
			stbl,
		},
	}

	return minf
}
