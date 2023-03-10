package mp4muxer

import (
	"fmt"
	"io"
	"nvr/pkg/video/customformat"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/h264"
	"nvr/pkg/video/hls"
	"nvr/pkg/video/mp4"
	"nvr/pkg/video/mp4/bitio"
	"time"
)

type muxer struct {
	out         *bitio.Writer
	videoTrack  *gortsplib.TrackH264
	videoSPSP   h264.SPS
	audioTrack  *gortsplib.TrackMPEG4Audio
	audioConfig []byte

	startTime int64
	endTime   int64

	firstSample bool
	dtsShift    int64
	mdatPos     uint32

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
}

// GenerateMP4 generates mp4 from samples.
func GenerateMP4(
	out io.Writer,
	startTime int64,
	samples []customformat.Sample,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackMPEG4Audio,
) (int64, error) {
	bw := bitio.NewByteWriter(out)
	m := &muxer{
		out:        bitio.NewWriter(bw),
		videoTrack: videoTrack,
		audioTrack: audioTrack,

		startTime:   startTime,
		firstSample: true,
	}

	err := m.videoSPSP.Unmarshal(videoTrack.SPS)
	if err != nil {
		return 0, fmt.Errorf("unmarshal video spsp: %w", err)
	}

	if audioTrack != nil {
		m.audioConfig, err = audioTrack.Config.Marshal()
		if err != nil {
			return 0, fmt.Errorf("marshal audio config: %w", err)
		}
	}

	ftyp := &mp4.Ftyp{
		MajorBrand:   [4]byte{'i', 's', 'o', '4'},
		MinorVersion: 512,
		CompatibleBrands: []mp4.CompatibleBrandElem{
			{CompatibleBrand: [4]byte{'i', 's', 'o', '4'}},
		},
	}
	_, err = mp4.WriteSingleBox(m.out, ftyp)
	if err != nil {
		return 0, fmt.Errorf("write ftyp: %w", err)
	}

	for _, sample := range samples {
		if sample.IsAudioSample {
			m.writeAudioSample(sample)
		} else {
			m.writeVideoSample(sample)
		}
	}

	if err := m.writeMetadata(); err != nil {
		return 0, fmt.Errorf("write metadata: %w", err)
	}
	return int64(m.mdatPos), nil
}

func (m *muxer) writeVideoSample(sample customformat.Sample) {
	delta := hls.NanoToTimescale(sample.Next-sample.DTS, hls.VideoTimescale)
	if len(m.videoStts) > 0 && m.videoStts[len(m.videoStts)-1].SampleDelta == uint32(delta) {
		m.videoStts[len(m.videoStts)-1].SampleCount++
	} else {
		m.videoStts = append(m.videoStts, mp4.SttsEntry{
			SampleCount: 1,
			SampleDelta: uint32(delta),
		})
	}

	pts := hls.NanoToTimescale(sample.PTS-m.startTime, hls.VideoTimescale)
	dts := hls.NanoToTimescale(sample.DTS-m.startTime, hls.VideoTimescale)

	if m.firstSample {
		m.dtsShift = pts - dts
		m.firstSample = false
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
		m.videoStco = append(m.videoStco, m.mdatPos)
		m.videoStsc = append(m.videoStsc, mp4.StscEntry{
			FirstChunk:             uint32(len(m.videoStco)),
			SamplesPerChunk:        1,
			SampleDescriptionIndex: 1,
		})
		m.prevChunkVideo = true
		m.prevChunkAudio = false
	}

	m.mdatPos += sample.Size
	m.videoStsz = append(m.videoStsz, sample.Size)

	if sample.IsSyncSample {
		m.videoStss = append(m.videoStss, uint32(len(m.videoStsz)))
	}

	m.endTime = sample.Next
}

func (m *muxer) writeAudioSample(sample customformat.Sample) {
	delta := hls.NanoToTimescale(sample.Next-sample.PTS, int64(m.audioTrack.ClockRate()))
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
		m.audioStco = append(m.audioStco, m.mdatPos)
		m.audioStsc = append(m.audioStsc, mp4.StscEntry{
			FirstChunk:             uint32(len(m.audioStco)),
			SamplesPerChunk:        1,
			SampleDescriptionIndex: 1,
		})
		m.prevChunkVideo = false
		m.prevChunkAudio = true
	}

	m.mdatPos += sample.Size
	m.audioStsz = append(m.audioStsz, sample.Size)
}

func (m *muxer) writeMetadata() error {
	/*
	   moov
	   - mvhd
	   - trak (video)
	   - trak (audio)
	*/

	duration := time.Duration(m.endTime - m.startTime)

	moov := mp4.Boxes{
		Box: &mp4.Moov{},
		Children: []mp4.Boxes{
			{Box: &mp4.Mvhd{
				Timescale:   1000,
				DurationV0:  uint32(duration.Milliseconds()),
				Rate:        65536,
				Volume:      256,
				Matrix:      [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
				NextTrackID: hls.VideoTrackID + 1,
			}},
			m.generateVideoTrak(duration),
			m.generateAudioTrak(duration),
		},
	}

	const ftypSize = 20
	const mdatHeaderSize = 8
	mdatOffset := uint32(ftypSize + moov.Size() + mdatHeaderSize)
	for i := 0; i < len(m.videoStco); i++ {
		m.videoStco[i] += mdatOffset
	}
	for i := 0; i < len(m.audioStco); i++ {
		m.audioStco[i] += mdatOffset
	}

	if err := moov.Marshal(m.out); err != nil {
		return fmt.Errorf("marshal moov: %w", err)
	}

	m.out.TryWriteUint32(8 + m.mdatPos)
	m.out.TryWrite([]byte{'m', 'd', 'a', 't'})
	return m.out.TryError
}

func (m *muxer) generateVideoTrak(duration time.Duration) mp4.Boxes {
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
				TrackID:    hls.VideoTrackID,
				DurationV0: uint32(duration.Milliseconds()),
				Width:      uint32(m.videoSPSP.Width() * 65536),
				Height:     uint32(m.videoSPSP.Height() * 65536),
				Matrix:     [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
			}},
			{
				Box: &mp4.Mdia{},
				Children: []mp4.Boxes{
					{Box: &mp4.Mdhd{
						Timescale: hls.VideoTimescale, // the number of time units that pass per second
						Language:  [3]byte{'u', 'n', 'd'},
						DurationV0: uint32(
							hls.NanoToTimescale(
								int64(duration),
								hls.VideoTimescale,
							)),
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

func (m *muxer) generateVideoMinf() mp4.Boxes {
	/*
	   minf
	   - vmhd
	   - dinf
	     - dref
	       - url
	     - stbl
	       - stsd
	       - stts
	       - stss
	       - ctts
	       - stsc
	       - stsz
	       - stco
	*/

	stbl := mp4.Boxes{
		Box: &mp4.Stbl{},
		Children: []mp4.Boxes{
			generateVideoStsd(m.videoTrack, m.videoSPSP),
			{Box: &mp4.Stts{
				Entries: m.videoStts,
			}},
			{Box: &mp4.Stss{
				SampleNumbers: m.videoStss,
			}},
			{Box: &mp4.Ctts{
				FullBox: mp4.FullBox{Version: 1},
				Entries: m.videoCtts,
			}},
			{Box: &mp4.Stsc{
				Entries: m.videoStsc,
			}},
			{Box: &mp4.Stsz{
				SampleCount: uint32(len(m.videoStsz)),
				EntrySizes:  m.videoStsz,
			}},
			{Box: &mp4.Stco{
				ChunkOffsets: m.videoStco,
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

func generateVideoStsd(
	videoTrack *gortsplib.TrackH264,
	videoSPSP h264.SPS,
) mp4.Boxes {
	/*
	   - stsd
	     - avc1
	       - avcC
	*/

	stsd := mp4.Boxes{
		Box: &mp4.Stsd{EntryCount: 1},
		Children: []mp4.Boxes{
			{
				Box: &mp4.Avc1{
					SampleEntry: mp4.SampleEntry{
						DataReferenceIndex: 1,
					},
					Width:           uint16(videoSPSP.Width()),
					Height:          uint16(videoSPSP.Height()),
					Horizresolution: 4718592,
					Vertresolution:  4718592,
					FrameCount:      1,
					Depth:           24,
					PreDefined3:     -1,
				},
				Children: []mp4.Boxes{
					{Box: &mp4.AvcC{
						ConfigurationVersion:       1,
						Profile:                    videoSPSP.ProfileIdc,
						ProfileCompatibility:       videoTrack.SPS[2],
						Level:                      videoSPSP.LevelIdc,
						LengthSizeMinusOne:         3,
						NumOfSequenceParameterSets: 1,
						SequenceParameterSets: []mp4.AVCParameterSet{
							{NALUnit: videoTrack.SPS},
						},
						NumOfPictureParameterSets: 1,
						PictureParameterSets: []mp4.AVCParameterSet{
							{NALUnit: videoTrack.PPS},
						},
					}},
				},
			},
		},
	}

	return stsd
}

func (m *muxer) generateAudioTrak(duration time.Duration) mp4.Boxes {
	if m.audioTrack == nil {
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
				DurationV0:     uint32(duration.Milliseconds()),
				TrackID:        hls.AudioTrackID,
				AlternateGroup: 1,
				Volume:         256,
				Matrix:         [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
			}},
			{
				Box: &mp4.Mdia{},
				Children: []mp4.Boxes{
					{Box: &mp4.Mdhd{
						Timescale: uint32(m.audioTrack.ClockRate()),
						Language:  [3]byte{'u', 'n', 'd'},
						DurationV0: uint32(
							hls.NanoToTimescale(int64(duration), int64(m.audioTrack.ClockRate()))),
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
							{Box: &mp4.URL{
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
									ChannelCount: uint16(m.audioTrack.Config.ChannelCount),
									SampleSize:   16,
									SampleRate:   uint32(m.audioTrack.ClockRate() * 65536),
								},
								Children: []mp4.Boxes{
									{Box: &myEsds{
										ESID:   hls.AudioTrackID,
										config: m.audioConfig,
									}},
								},
							},
						},
					},
					{Box: &mp4.Stts{
						Entries: m.audioStts,
					}},
					{Box: &mp4.Stsc{
						Entries: m.audioStsc,
					}},
					{Box: &mp4.Stsz{
						SampleCount: uint32(len(m.audioStsz)),
						EntrySizes:  m.audioStsz,
					}},
					{Box: &mp4.Stco{
						ChunkOffsets: m.audioStco,
					}},
				},
			},
		},
	}
	return minf
}

// ISO/IEC 14496-1.
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
