package mp4muxer

import (
	"context"
	"fmt"
	"io"
	"nvr/pkg/video/hls"
	"nvr/pkg/video/mp4"
	"nvr/pkg/video/mp4/bitio"
	"time"
)

const (
	videoTrackID = 0
	audioTrackID = 1
)

type muxer struct {
	file    io.WriteSeeker
	w       *bitio.Writer
	hlsChan chan []*hls.Segment
	info    hls.StreamInfo

	startTime time.Time
	endTime   time.Time
	duration  time.Duration
	stopTime  time.Time

	pos        uint32
	mdatOffset uint32
	prevSeg    uint64

	videoStts []mp4.SttsEntry
	videoStss []uint32
	videoCtts []mp4.CttsEntry
	videoStsc []mp4.StscEntry
	videoStsz []uint32
	videoStco []uint32

	audioStts []mp4.SttsEntry
	audioStsc []mp4.StscEntry
	audioStsz []uint32
	audioStco []uint32

	prevChunkVideo bool
	prevChunkAudio bool

	firstSample bool
	dtsShift    int64
}

// WriteVideo writes a mp4 video.
func WriteVideo(
	ctx context.Context,
	file io.WriteSeeker,
	hlsChan chan []*hls.Segment,
	firstSegment *hls.Segment,
	info hls.StreamInfo,
	maxDuration time.Duration,
) (uint64, *time.Time, error) {
	bw := bitio.NewByteWriter(file)
	m := &muxer{
		file:    file,
		w:       bitio.NewWriter(bw),
		hlsChan: hlsChan,
		info:    info,

		startTime: firstSegment.StartTime,
		stopTime:  firstSegment.StartTime.Add(maxDuration),

		firstSample: true,
		prevSeg:     firstSegment.ID,
	}

	if err := m.writeFtypAndMdat(); err != nil {
		return 0, nil, err
	}

	err := m.parseSegment(firstSegment)
	if err != nil {
		return 0, nil, err
	}

	for {
		select {
		case <-ctx.Done():
			if err := m.writeMetadata(); err != nil {
				return 0, nil, fmt.Errorf("write metadata: %w", err)
			}
			return m.prevSeg, &m.endTime, nil

		case segs := <-m.hlsChan:
			for _, seg := range segs {
				if seg.ID <= m.prevSeg {
					continue
				}
				if err := m.parseSegment(seg); err != nil {
					return 0, nil, err
				}
				if seg.StartTime.After(m.stopTime) {
					if err := m.writeMetadata(); err != nil {
						return 0, nil, fmt.Errorf("write metadata: %w", err)
					}
					return m.prevSeg, &m.endTime, nil
				}
			}
		}
	}
}

func (m *muxer) writeFtypAndMdat() error {
	ftyp := &mp4.Ftyp{
		MajorBrand:   [4]byte{'i', 's', 'o', '4'},
		MinorVersion: 512,
		CompatibleBrands: []mp4.CompatibleBrandElem{
			{CompatibleBrand: [4]byte{'i', 's', 'o', '4'}},
		},
	}
	n, err := mp4.WriteSingleBox(m.w, ftyp)
	if err != nil {
		return fmt.Errorf("write ftyp: %w", err)
	}
	m.pos = uint32(n)
	m.mdatOffset = m.pos

	n, err = mp4.WriteSingleBox(m.w, &mp4.Mdat{})
	if err != nil {
		return fmt.Errorf("write mdat: %w", err)
	}
	m.pos += uint32(n)
	return nil
}

func (m *muxer) parseSegment(seg *hls.Segment) error {
	for _, part := range seg.Parts {
		for _, sample := range part.VideoSamples {
			err := m.writeVideoSample(sample)
			if err != nil {
				return err
			}
		}
	}
	for _, part := range seg.Parts {
		for _, sample := range part.AudioSamples {
			err := m.writeAudioSample(sample)
			if err != nil {
				return err
			}
		}
	}
	m.prevSeg = seg.ID
	m.endTime = seg.StartTime.Add(seg.RenderedDuration)
	return nil
}

func (m *muxer) writeVideoSample(sample *hls.VideoSample) error {
	pts := hls.DurationGoToMp4(sample.Pts, hls.VideoTimescale)
	dts := hls.DurationGoToMp4(sample.Dts, hls.VideoTimescale)
	nextDts := hls.DurationGoToMp4(sample.Next.Dts, hls.VideoTimescale)

	if m.firstSample {
		m.dtsShift = pts - dts
		m.firstSample = false
	}

	delta := nextDts - dts
	if len(m.videoStts) > 0 && m.videoStts[len(m.videoStts)-1].SampleDelta == uint32(delta) {
		m.videoStts[len(m.videoStts)-1].SampleCount++
	} else {
		m.videoStts = append(m.videoStts, mp4.SttsEntry{
			SampleCount: 1,
			SampleDelta: uint32(delta),
		})
	}

	cts := pts - (dts + m.dtsShift)
	if len(m.videoCtts) > 0 && m.videoCtts[len(m.videoCtts)-1].SampleOffsetV1 == int32(cts) {
		m.videoCtts[len(m.videoCtts)-1].SampleCount++
	} else {
		m.videoCtts = append(m.videoCtts, mp4.CttsEntry{
			SampleCount:    1,
			SampleOffsetV1: int32(cts),
		})
	}

	if m.prevChunkVideo {
		m.videoStsc[len(m.videoStsc)-1].SamplesPerChunk++
	} else {
		m.videoStco = append(m.videoStco, m.pos)
		m.videoStsc = append(m.videoStsc, mp4.StscEntry{
			FirstChunk:             uint32(len(m.videoStco)),
			SamplesPerChunk:        1,
			SampleDescriptionIndex: 1,
		})
		m.prevChunkVideo = true
		m.prevChunkAudio = false
	}

	n, err := m.w.Write(sample.Avcc)
	if err != nil {
		return fmt.Errorf("write video sample: %w", err)
	}
	m.pos += uint32(n)
	m.videoStsz = append(m.videoStsz, uint32(n))

	if sample.IdrPresent {
		m.videoStss = append(m.videoStss, uint32(len(m.videoStsz)))
	}

	return nil
}

func (m *muxer) writeAudioSample(sample *hls.AudioSample) error {
	delta := hls.DurationGoToMp4(sample.Duration(), hls.VideoTimescale)
	if len(m.audioStts) > 0 && m.audioStts[len(m.audioStts)-1].SampleDelta == uint32(delta) {
		m.audioStts[len(m.audioStts)-1].SampleCount++
	} else {
		m.audioStts = append(m.audioStts, mp4.SttsEntry{
			SampleCount: 1,
			SampleDelta: uint32(delta),
		})
	}

	if m.prevChunkAudio {
		m.audioStsc[len(m.audioStsc)-1].SamplesPerChunk++
	} else {
		m.audioStco = append(m.audioStco, m.pos)
		m.audioStsc = append(m.audioStsc, mp4.StscEntry{
			FirstChunk:             uint32(len(m.audioStco)),
			SamplesPerChunk:        1,
			SampleDescriptionIndex: 1,
		})
		m.prevChunkVideo = false
		m.prevChunkAudio = true
	}

	n, err := m.w.Write(sample.Au)
	if err != nil {
		return fmt.Errorf("write audio sample: %w", err)
	}
	m.pos += uint32(n)
	m.audioStsz = append(m.audioStsz, uint32(n))

	return nil
}

func (m *muxer) writeMetadata() error {
	mdatSize := m.pos - m.mdatOffset

	/*
	   moov
	   - mvhd
	   - trak (video)
	   - trak (audio)
	*/

	m.duration = m.endTime.Sub(m.startTime)

	moov := mp4.Boxes{
		Box: &mp4.Moov{},
		Children: []mp4.Boxes{
			{Box: &mp4.Mvhd{
				Timescale:   1000,
				DurationV0:  uint32(m.duration.Milliseconds()),
				Rate:        65536,
				Volume:      256,
				Matrix:      [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
				NextTrackID: videoTrackID + 1,
			}},
			m.generateVideoTrak(),
			m.generateAudioTrak(),
		},
	}
	if err := moov.Marshal(m.w); err != nil {
		return err
	}

	// Seek to mdat offset and update size.
	_, err := m.file.Seek(int64(m.mdatOffset), io.SeekStart)
	if err != nil {
		return err
	}
	return m.w.WriteUint32(mdatSize)
}

func (m *muxer) generateVideoTrak() mp4.Boxes {
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
				TrackID:    videoTrackID,
				DurationV0: uint32(m.duration.Milliseconds()),
				Width:      uint32(m.info.VideoWidth * 65536),
				Height:     uint32(m.info.VideoHeight * 65536),
				Matrix:     [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
			}},
			{
				Box: &mp4.Mdia{},
				Children: []mp4.Boxes{
					{Box: &mp4.Mdhd{
						Timescale:  hls.VideoTimescale, // the number of time units that pass per second
						Language:   [3]byte{'u', 'n', 'd'},
						DurationV0: uint32(hls.DurationGoToMp4(m.duration, hls.VideoTimescale)),
					}},
					{Box: &mp4.Hdlr{
						HandlerType: [4]byte{'v', 'i', 'd', 'e'},
						Name:        "VideoHandler",
					}},
					m.generateVideoMinf(),
				},
			},
		},
	}
	return trak
}

func (m *muxer) generateVideoMinf() mp4.Boxes { //nolint:funlen
	/*
	   minf
	   - vmhd
	   - dinf
	     - dref
	       - url
	     - stbl
	       - stsd
	         - avc1
	           - avcC
	       - stts
	       - stsc
	       - stco
	*/
	stbl := mp4.Boxes{
		Box: &mp4.Stbl{},
		Children: []mp4.Boxes{
			{
				Box: &mp4.Stsd{EntryCount: 1},
				Children: []mp4.Boxes{
					{
						Box: &mp4.Avc1{
							SampleEntry: mp4.SampleEntry{
								DataReferenceIndex: 1,
							},
							Width:           uint16(m.info.VideoWidth),
							Height:          uint16(m.info.VideoHeight),
							Horizresolution: 4718592,
							Vertresolution:  4718592,
							FrameCount:      1,
							Depth:           24,
							PreDefined3:     -1,
						},
						Children: []mp4.Boxes{
							{Box: &mp4.AvcC{
								ConfigurationVersion:       1,
								Profile:                    m.info.VideoSPSP.ProfileIdc,
								ProfileCompatibility:       m.info.VideoSPS[2],
								Level:                      m.info.VideoSPSP.LevelIdc,
								LengthSizeMinusOne:         3,
								NumOfSequenceParameterSets: 1,
								SequenceParameterSets: []mp4.AVCParameterSet{
									{
										Length:  uint16(len(m.info.VideoSPS)),
										NALUnit: m.info.VideoSPS,
									},
								},
								NumOfPictureParameterSets: 1,
								PictureParameterSets: []mp4.AVCParameterSet{
									{
										Length:  uint16(len(m.info.VideoPPS)),
										NALUnit: m.info.VideoPPS,
									},
								},
							}},
						},
					},
				},
			},
			{Box: &mp4.Stts{
				EntryCount: uint32(len(m.videoStts)),
				Entries:    m.videoStts,
			}},
			{Box: &mp4.Stss{
				EntryCount:   uint32(len(m.videoStss)),
				SampleNumber: m.videoStss,
			}},
			{Box: &mp4.Ctts{
				FullBox:    mp4.FullBox{Version: 1},
				EntryCount: uint32(len(m.videoCtts)),
				Entries:    m.videoCtts,
			}},
			{Box: &mp4.Stsc{
				EntryCount: uint32(len(m.videoStsc)),
				Entries:    m.videoStsc,
			}},
			{Box: &mp4.Stsz{
				SampleCount: uint32(len(m.videoStsz)),
				EntrySize:   m.videoStsz,
			}},
			{Box: &mp4.Stco{
				EntryCount:  uint32(len(m.videoStco)),
				ChunkOffset: m.videoStco,
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
							{Box: &mp4.Url{
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

func (m *muxer) generateAudioTrak() mp4.Boxes {
	if !m.info.AudioTrackExist {
		return mp4.Boxes{Box: &mp4.Free{}}
	}

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
				DurationV0:     uint32(m.duration.Milliseconds()),
				TrackID:        audioTrackID,
				AlternateGroup: 1,
				Volume:         256,
				Matrix:         [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
			}},
			{
				Box: &mp4.Mdia{},
				Children: []mp4.Boxes{
					{Box: &mp4.Mdhd{
						Timescale: uint32(m.info.AudioClockRate),
						Language:  [3]byte{'u', 'n', 'd'},
						DurationV0: uint32(
							hls.DurationGoToMp4(
								m.duration,
								time.Duration(m.info.AudioClockRate),
							)),
					}},
					{Box: &mp4.Hdlr{
						HandlerType: [4]byte{'s', 'o', 'u', 'n'},
						Name:        "SoundHandler",
					}},
					m.generateAudioMinf(),
				},
			},
		},
	}
	return trak
}

func (m *muxer) generateAudioMinf() mp4.Boxes { //nolint:funlen
	/*
	   minf
	   - vmhd
	   - dinf
	     - dref
	       - url
	   - stbl
	     - stsd
	       - mp4a
	         - esds
	     - stts
	     - stsc
	     - stsz
	     - stco
	*/
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
							{Box: &mp4.Url{
								FullBox: mp4.FullBox{Flags: [3]byte{0, 0, 1}},
							}},
						},
					},
				},
			},
			{
				Box: &mp4.Stbl{},
				Children: []mp4.Boxes{
					{
						Box: &mp4.Stsd{EntryCount: 1},
						Children: []mp4.Boxes{
							{
								Box: &mp4.Mp4a{
									SampleEntry: mp4.SampleEntry{
										DataReferenceIndex: 1,
									},
									ChannelCount: uint16(m.info.AudioChannelCount),
									SampleSize:   16,
									SampleRate:   uint32(m.info.AudioClockRate * 65536),
								},
								Children: []mp4.Boxes{
									{Box: &myEsds{
										ESID:   audioTrackID,
										config: m.info.AudioTrackConfig,
									}},
								},
							},
						},
					},
					{Box: &mp4.Stts{
						EntryCount: uint32(len(m.audioStts)),
						Entries:    m.audioStts,
					}},
					{Box: &mp4.Stsc{
						EntryCount: uint32(len(m.audioStsc)),
						Entries:    m.audioStsc,
					}},
					{Box: &mp4.Stsz{
						SampleCount: uint32(len(m.audioStsz)),
						EntrySize:   m.audioStsz,
					}},
					{Box: &mp4.Stco{
						EntryCount:  uint32(len(m.audioStco)),
						ChunkOffset: m.audioStco,
					}},
				},
			},
		},
	}
	return minf
}

//  ISO/IEC 14496-1
type myEsds struct {
	mp4.FullBox
	ESID   uint8
	config []byte
}

func (*myEsds) Type() mp4.BoxType {
	return [4]byte{'e', 's', 'd', 's'}
}

func (b *myEsds) Size() int {
	return 41 + len(b.config)
}

func (b *myEsds) Marshal(w *bitio.Writer) error {
	err := b.FullBox.MarshalField(w)
	if err != nil {
		return err
	}

	decSpecificInfoTagSize := uint8(len(b.config))

	w.TryWrite([]byte{
		mp4.ESDescrTag,
		0x80, 0x80, 0x80,
		32 + decSpecificInfoTagSize, // Size.
		0, b.ESID,                   // ES_ID.
		0, // Flags.
	})

	w.TryWrite([]byte{
		mp4.DecoderConfigDescrTag,
		0x80, 0x80, 0x80,
		18 + decSpecificInfoTagSize, // Size

		0x40,    // Object type indicator (MPEG-4 Audio)
		0x15,    // StreamType and upStream.
		0, 0, 0, // BufferSizeDB.
		0, 1, 0xf7, 0x39, // MaxBitrate.
		0, 1, 0xf7, 0x39, // AverageBitrate.
	})

	w.TryWrite([]byte{
		mp4.DecSpecificInfoTag,
		0x80, 0x80, 0x80,
		decSpecificInfoTagSize, // Size.
	})
	w.TryWrite(b.config)

	w.TryWrite([]byte{
		mp4.SLConfigDescrTag,
		0x80, 0x80, 0x80,
		1, // Size.
		2, // Flags.
	})

	return w.TryError
}
