package hls

import (
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/h264"
	"nvr/pkg/video/mp4"
)

type myEsds struct {
	mp4.FullBox
	Data []byte
}

func (*myEsds) Type() mp4.BoxType {
	return [4]byte{'e', 's', 'd', 's'}
}

func (b *myEsds) Size() int {
	return 4 + len(b.Data)
}

func (b *myEsds) Marshal(buf []byte, pos *int) {
	b.FullBox.Marshal(buf, pos)
	mp4.Write(buf, pos, b.Data)
}

func mp4InitGenerateVideoTrack( //nolint:funlen
	trackID int,
	videoTrack *gortsplib.TrackH264,
	spsp h264.SPS,
) mp4.Boxes {
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

	sps := videoTrack.SafeSPS()
	pps := videoTrack.SafePPS()

	width := spsp.Width()
	height := spsp.Height()

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
								Profile:                    spsp.ProfileIdc,
								ProfileCompatibility:       sps[2],
								Level:                      spsp.LevelIdc,
								LengthSizeMinusOne:         3,
								NumOfSequenceParameterSets: 1,
								SequenceParameterSets: []mp4.AVCParameterSet{
									{
										Length:  uint16(len(sps)),
										NALUnit: sps,
									},
								},
								NumOfPictureParameterSets: 1,
								PictureParameterSets: []mp4.AVCParameterSet{
									{
										Length:  uint16(len(pps)),
										NALUnit: pps,
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

func generateAudioEsdsData(trackID int, audioTrack *gortsplib.TrackAAC) []byte {
	enc, _ := audioTrack.Config.Marshal()

	decSpecificInfoTagSize := uint8(len(enc))
	decSpecificInfoTag := append(
		[]byte{
			mp4.DecSpecificInfoTag,
			0x80, 0x80, 0x80, decSpecificInfoTagSize, // size
		},
		enc...,
	)

	esDescrTag := []byte{
		mp4.ESDescrTag,
		0x80, 0x80, 0x80, 32 + decSpecificInfoTagSize, // size
		0x00,
		byte(trackID), // ES_ID
		0x00,
	}

	decoderConfigDescrTag := []byte{
		mp4.DecoderConfigDescrTag,
		0x80, 0x80, 0x80, 18 + decSpecificInfoTagSize, // size
		0x40, // object type indicator (MPEG-4 Audio)
		0x15, 0x00,
		0x00, 0x00, 0x00, 0x01,
		0xf7, 0x39, 0x00, 0x01,
		0xf7, 0x39,
	}

	slConfigDescrTag := []byte{
		mp4.SLConfigDescrTag,
		0x80, 0x80, 0x80, 0x01, // size (1)
		0x02,
	}

	data := make([]byte, len(esDescrTag)+len(decoderConfigDescrTag)+len(decSpecificInfoTag)+len(slConfigDescrTag))
	pos := 0

	pos += copy(data[pos:], esDescrTag)
	pos += copy(data[pos:], decoderConfigDescrTag)
	pos += copy(data[pos:], decSpecificInfoTag)
	copy(data[pos:], slConfigDescrTag)

	return data
}

func mp4InitGenerateAudioTrack( //nolint:funlen
	trackID int,
	audioTrack *gortsplib.TrackAAC,
) mp4.Boxes {
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
									ChannelCount: uint16(audioTrack.Config.ChannelCount),
									SampleSize:   16,
									SampleRate:   uint32(audioTrack.ClockRate() * 65536),
								},
								Children: []mp4.Boxes{
									{Box: &myEsds{Data: generateAudioEsdsData(trackID, audioTrack)}},
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
						Timescale: uint32(audioTrack.ClockRate()),
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

func generateInit( //nolint:funlen
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
) ([]byte, error) {
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
	if videoTrack != nil {
		var spsp h264.SPS
		err := spsp.Unmarshal(videoTrack.SafeSPS())
		if err != nil {
			return nil, err
		}
		videoTrak := mp4InitGenerateVideoTrack(trackID, videoTrack, spsp)
		moov.Children = append(moov.Children, videoTrak)
		trackID++
	}
	if audioTrack != nil {
		audioTrak := mp4InitGenerateAudioTrack(trackID, audioTrack)
		moov.Children = append(moov.Children, audioTrak)
	}

	mvex := mp4.Boxes{
		Box: &mp4.Mvex{},
	}
	trackID = 1
	if videoTrack != nil {
		trex := mp4.Boxes{
			Box: &mp4.Trex{
				TrackID:                       uint32(trackID),
				DefaultSampleDescriptionIndex: 1,
			},
		}
		mvex.Children = append(mvex.Children, trex)
		trackID++
	}
	if audioTrack != nil {
		trex := mp4.Boxes{
			Box: &mp4.Trex{
				TrackID:                       uint32(trackID),
				DefaultSampleDescriptionIndex: 1,
			},
		}
		mvex.Children = append(mvex.Children, trex)
	}
	moov.Children = append(moov.Children, mvex)

	size := ftyp.Size() + moov.Size()
	buf := make([]byte, size)

	var pos int
	ftyp.Marshal(buf, &pos)
	moov.Marshal(buf, &pos)

	return buf, nil
}
