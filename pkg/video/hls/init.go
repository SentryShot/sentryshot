package hls

import (
	"bytes"
	"nvr/pkg/video/mp4"
	"nvr/pkg/video/mp4/bitio"
)

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

func initGenerateVideoTrack(trackID int, info StreamInfo) mp4.Boxes { // nolint:funlen
	/*
		trak
		- tkhd
		- mdia
		  - mdhd
		  - hdlr
		  - minf
		    - vmhd
			- dinf
			  - dref
			    - url
			- stbl
			  - stsd
			    - avc1
				  - avcC
				  - btrt
			  - stts
			  - stsc
			  - stsz
			  - stco
	*/

	width := info.VideoSPSP.Width()
	height := info.VideoSPSP.Height()

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
							Width:           uint16(width),
							Height:          uint16(height),
							Horizresolution: 4718592,
							Vertresolution:  4718592,
							FrameCount:      1,
							Depth:           24,
							PreDefined3:     -1,
						},
						Children: []mp4.Boxes{
							{Box: &mp4.AvcC{
								ConfigurationVersion:       1,
								Profile:                    info.VideoSPSP.ProfileIdc,
								ProfileCompatibility:       info.VideoSPS[2],
								Level:                      info.VideoSPSP.LevelIdc,
								LengthSizeMinusOne:         3,
								NumOfSequenceParameterSets: 1,
								SequenceParameterSets: []mp4.AVCParameterSet{
									{
										Length:  uint16(len(info.VideoSPS)),
										NALUnit: info.VideoSPS,
									},
								},
								NumOfPictureParameterSets: 1,
								PictureParameterSets: []mp4.AVCParameterSet{
									{
										Length:  uint16(len(info.VideoPPS)),
										NALUnit: info.VideoPPS,
									},
								},
							}},
							{Box: &mp4.Btrt{
								MaxBitrate: 1000000,
								AvgBitrate: 1000000,
							}},
						},
					},
				},
			},
			{Box: &mp4.Stts{}},
			{Box: &mp4.Stsc{}},
			{Box: &mp4.Stsz{}},
			{Box: &mp4.Stco{}},
		},
	}

	minf := mp4.Boxes{
		Box: &mp4.Minf{},
		Children: []mp4.Boxes{
			{
				Box: &mp4.Vmhd{
					FullBox: mp4.FullBox{
						Flags: [3]byte{0, 0, 1},
					},
				},
			},
			{
				Box: &mp4.Dinf{},
				Children: []mp4.Boxes{
					{
						Box: &mp4.Dref{
							EntryCount: 1,
						},
						Children: []mp4.Boxes{
							{Box: &mp4.Url{
								FullBox: mp4.FullBox{
									Flags: [3]byte{0, 0, 1},
								},
							}},
						},
					},
				},
			},
			stbl,
		},
	}

	trak := mp4.Boxes{
		Box: &mp4.Trak{},
		Children: []mp4.Boxes{
			{
				Box: &mp4.Tkhd{
					FullBox: mp4.FullBox{
						Flags: [3]byte{0, 0, 3},
					},
					TrackID: uint32(trackID),
					Width:   uint32(width * 65536),
					Height:  uint32(height * 65536),
					Matrix:  [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
				},
			},
			{
				Box: &mp4.Mdia{},
				Children: []mp4.Boxes{
					{Box: &mp4.Mdhd{
						Timescale: videoTimescale, // the number of time units that pass per second
						Language:  [3]byte{'u', 'n', 'd'},
					}},
					{Box: &mp4.Hdlr{
						HandlerType: [4]byte{'v', 'i', 'd', 'e'},
						Name:        "VideoHandler",
					}},
					minf,
				},
			},
		},
	}
	return trak
}

func initGenerateAudioTrack(trackID int, info StreamInfo) mp4.Boxes { // nolint:funlen
	/*
		trak
		- tkhd
		- mdia
		  - mdhd
		  - hdlr
		  - minf
		    - smhd
		    - dinf
			  - dref
			    - url
		    - stbl
			  - stsd
			    - mp4a
				  - esds
				  - btrt
			  - stts
			  - stsc
			  - stsz
			  - stco
	*/

	minf := mp4.Boxes{
		Box: &mp4.Minf{},
		Children: []mp4.Boxes{
			{Box: &mp4.Smhd{}},
			{
				Box: &mp4.Dinf{},

				Children: []mp4.Boxes{
					{
						Box: &mp4.Dref{EntryCount: 1},
						Children: []mp4.Boxes{
							{Box: &mp4.Url{
								FullBox: mp4.FullBox{
									Flags: [3]byte{0, 0, 1},
								},
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
									ChannelCount: uint16(info.AudioChannelCount),
									SampleSize:   16,
									SampleRate:   uint32(info.AudioClockRate * 65536),
								},
								Children: []mp4.Boxes{
									{Box: &myEsds{
										ESID:   uint8(trackID),
										config: info.AudioTrackConfig,
									}},
									{Box: &mp4.Btrt{
										MaxBitrate: 128825,
										AvgBitrate: 128825,
									}},
								},
							},
						},
					},
					{Box: &mp4.Stts{}},
					{Box: &mp4.Stsc{}},
					{Box: &mp4.Stsz{}},
					{Box: &mp4.Stco{}},
				},
			},
		},
	}

	trak := mp4.Boxes{
		Box: &mp4.Trak{},
		Children: []mp4.Boxes{
			{Box: &mp4.Tkhd{
				FullBox: mp4.FullBox{
					Flags: [3]byte{0, 0, 3},
				},
				TrackID:        uint32(trackID),
				AlternateGroup: 1,
				Volume:         256,
				Matrix:         [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
			}},
			{
				Box: &mp4.Mdia{},
				Children: []mp4.Boxes{
					{Box: &mp4.Mdhd{
						Timescale: uint32(info.AudioClockRate),
						Language:  [3]byte{'u', 'n', 'd'},
					}},
					{Box: &mp4.Hdlr{
						HandlerType: [4]byte{'s', 'o', 'u', 'n'},
						Name:        "SoundHandler",
					}},
					minf,
				},
			},
		},
	}

	return trak
}

func initGenerateMvex(info StreamInfo) mp4.Boxes {
	mvex := mp4.Boxes{
		Box: &mp4.Mvex{},
	}
	trackID := 1
	if info.VideoTrackExist {
		trex := mp4.Boxes{
			Box: &mp4.Trex{
				TrackID:                       uint32(trackID),
				DefaultSampleDescriptionIndex: 1,
			},
		}
		mvex.Children = append(mvex.Children, trex)
		trackID++
	}
	if info.AudioTrackExist {
		trex := mp4.Boxes{
			Box: &mp4.Trex{
				TrackID:                       uint32(trackID),
				DefaultSampleDescriptionIndex: 1,
			},
		}
		mvex.Children = append(mvex.Children, trex)
	}
	return mvex
}

func generateInit(info StreamInfo) ([]byte, error) { // nolint:funlen
	/*
		- ftyp
		- moov
		  - mvhd
		  - trak (video)
		  - trak (audio)
		  - mvex
		    - trex (video)
		    - trex (audio)
	*/

	ftyp := mp4.Boxes{
		Box: &mp4.Ftyp{
			MajorBrand:   [4]byte{'m', 'p', '4', '2'},
			MinorVersion: 1,
			CompatibleBrands: []mp4.CompatibleBrandElem{
				{CompatibleBrand: [4]byte{'m', 'p', '4', '1'}},
				{CompatibleBrand: [4]byte{'m', 'p', '4', '2'}},
				{CompatibleBrand: [4]byte{'i', 's', 'o', 'm'}},
				{CompatibleBrand: [4]byte{'h', 'l', 's', 'f'}},
			},
		},
	}

	moov := mp4.Boxes{
		Box: &mp4.Moov{},
		Children: []mp4.Boxes{
			{Box: &mp4.Mvhd{
				Timescale:   1000,
				Rate:        65536,
				Volume:      256,
				Matrix:      [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
				NextTrackID: 2,
			}},
		},
	}

	trackID := 1
	if info.VideoTrackExist {
		videoTrak := initGenerateVideoTrack(trackID, info)
		moov.Children = append(moov.Children, videoTrak)
		trackID++
	}
	if info.AudioTrackExist {
		audioTrak := initGenerateAudioTrack(trackID, info)
		moov.Children = append(moov.Children, audioTrak)
	}

	mvex := initGenerateMvex(info)
	moov.Children = append(moov.Children, mvex)

	size := ftyp.Size() + moov.Size()
	buf := bytes.NewBuffer(make([]byte, 0, size))

	w := bitio.NewWriter(buf)

	if err := ftyp.Marshal(w); err != nil {
		return nil, err
	}
	if err := moov.Marshal(w); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
