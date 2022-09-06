// https://github.com/abema/go-mp4

// Copyright (C) 2020 AbemaTV
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package mp4

import (
	"bytes"
	"testing"

	"nvr/pkg/video/mp4/bitio"

	"github.com/stretchr/testify/require"
)

func TestBoxTypes(t *testing.T) {
	testCases := []struct {
		name string
		src  ImmutableBox
		bin  []byte
	}{
		{
			name: "btrt",
			src: &Btrt{
				BufferSizeDB: 0x12345678,
				MaxBitrate:   0x3456789a,
				AvgBitrate:   0x56789abc,
			},
			bin: []byte{
				0x12, 0x34, 0x56, 0x78, // bufferSizeDB
				0x34, 0x56, 0x78, 0x9a, // maxBitrate
				0x56, 0x78, 0x9a, 0xbc, // avgBitrate
			},
		},
		{
			name: "ctts: version 0",
			src: &Ctts{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				EntryCount: 2,
				Entries: []CttsEntry{
					{SampleCount: 0x01234567, SampleOffsetV0: 0x12345678},
					{SampleCount: 0x89abcdef, SampleOffsetV0: 0x789abcde},
				},
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x00, 0x00, 0x00, 0x02, // entry count
				0x01, 0x23, 0x45, 0x67, // sample count
				0x12, 0x34, 0x56, 0x78, // sample offset
				0x89, 0xab, 0xcd, 0xef, // sample count
				0x78, 0x9a, 0xbc, 0xde, // sample offset
			},
		},
		{
			name: "ctts: version 1",
			src: &Ctts{
				FullBox: FullBox{
					Version: 1,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				EntryCount: 2,
				Entries: []CttsEntry{
					{SampleCount: 0x01234567, SampleOffsetV1: 0x12345678},
					{SampleCount: 0x89abcdef, SampleOffsetV1: -0x789abcde},
				},
			},
			bin: []byte{
				1,                // version
				0x00, 0x00, 0x00, // flags
				0x00, 0x00, 0x00, 0x02, // entry count
				0x01, 0x23, 0x45, 0x67, // sample count
				0x12, 0x34, 0x56, 0x78, // sample offset
				0x89, 0xab, 0xcd, 0xef, // sample count
				0x87, 0x65, 0x43, 0x22, // sample offset
			},
		},
		{
			name: "dinf",
			src:  &Dinf{},
			bin:  []byte{},
		},
		{
			name: "dref",
			src: &Dref{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				EntryCount: 0x12345678,
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x12, 0x34, 0x56, 0x78, // entry count
			},
		},
		{
			name: "edts",
			src:  &Edts{},
			bin:  []byte{},
		},
		{
			name: "elst: version 0",
			src: &Elst{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				EntryCount: 2,
				Entries: []ElstEntry{
					{
						SegmentDurationV0: 0x0100000a,
						MediaTimeV0:       0x0100000b,
						MediaRateInteger:  0x010c,
						MediaRateFraction: 0x010d,
					}, {
						SegmentDurationV0: 0x0200000a,
						MediaTimeV0:       0x0200000b,
						MediaRateInteger:  0x020c,
						MediaRateFraction: 0x020d,
					},
				},
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x00, 0x00, 0x00, 0x02, // entry count
				0x01, 0x00, 0x00, 0x0a, // segment duration v0
				0x01, 0x00, 0x00, 0x0b, // media time v0
				0x01, 0x0c, // media rate integer
				0x01, 0x0d, // media rate fraction
				0x02, 0x00, 0x00, 0x0a, // segment duration v0
				0x02, 0x00, 0x00, 0x0b, // media time v0
				0x02, 0x0c, // media rate integer
				0x02, 0x0d, // media rate fraction
			},
		},
		{
			name: "elst: version 1",
			src: &Elst{
				FullBox: FullBox{
					Version: 1,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				EntryCount: 2,
				Entries: []ElstEntry{
					{
						SegmentDurationV1: 0x010000000000000a,
						MediaTimeV1:       0x010000000000000b,
						MediaRateInteger:  0x010c,
						MediaRateFraction: 0x010d,
					}, {
						SegmentDurationV1: 0x020000000000000a,
						MediaTimeV1:       0x020000000000000b,
						MediaRateInteger:  0x020c,
						MediaRateFraction: 0x020d,
					},
				},
			},
			bin: []byte{
				1,                // version
				0x00, 0x00, 0x00, // flags
				0x00, 0x00, 0x00, 0x02, // entry count
				0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0a, // segment duration v1
				0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0b, // media time v1
				0x01, 0x0c, // media rate integer
				0x01, 0x0d, // media rate fraction
				0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0a, // segment duration v1
				0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0b, // media time v1
				0x02, 0x0c, // media rate integer
				0x02, 0x0d, // media rate fraction
			},
		},
		{
			name: "url",
			src: &URL{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0, 0, 1},
				},
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x01, // flags
			},
		},
		{
			name: "ftyp",
			src: &Ftyp{
				MajorBrand:   [4]byte{'a', 'b', 'e', 'm'},
				MinorVersion: 0x12345678,
				CompatibleBrands: []CompatibleBrandElem{
					{CompatibleBrand: [4]byte{'a', 'b', 'c', 'd'}},
					{CompatibleBrand: [4]byte{'e', 'f', 'g', 'h'}},
				},
			},
			bin: []byte{
				'a', 'b', 'e', 'm', // major brand
				0x12, 0x34, 0x56, 0x78, // minor version
				'a', 'b', 'c', 'd', // compatible brand
				'e', 'f', 'g', 'h', // compatible brand
			},
		},
		{
			name: "hdlr",
			src: &Hdlr{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				PreDefined:  0x12345678,
				HandlerType: [4]byte{'a', 'b', 'e', 'm'},
				Reserved:    [3]uint32{0, 0, 0},
				Name:        "Abema",
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x12, 0x34, 0x56, 0x78, // pre-defined
				'a', 'b', 'e', 'm', // handler type
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, // reserved
				'A', 'b', 'e', 'm', 'a', 0x00, // name
			},
		},
		{
			name: "hdlr2",
			src: &Hdlr{
				HandlerType: [4]byte{'v', 'i', 'd', 'e'},
				Name:        "VideoHandler",
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x00, 0x00, 0x00, 0x00, // pre-defined
				'v', 'i', 'd', 'e', // handler type
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, // reserved
				'V', 'i', 'd', 'e', 'o', 'H', 'a', 'n', 'd', 'l', 'e', 'r', 0x00, // name
			},
		},
		{
			name: "mdat",
			src: &Mdat{
				Data: []byte{0x11, 0x22, 0x33},
			},
			bin: []byte{
				0x11, 0x22, 0x33,
			},
		},
		{
			name: "mdhd: version 0",
			src: &Mdhd{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				CreationTimeV0:     0x12345678,
				ModificationTimeV0: 0x23456789,
				Timescale:          0x01020304,
				DurationV0:         0x02030405,
				Pad:                true,
				Language:           [3]byte{'j' - 0x60, 'p' - 0x60, 'n' - 0x60}, // 0x0a, 0x10, 0x0e
				PreDefined:         0,
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x12, 0x34, 0x56, 0x78, // creation time
				0x23, 0x45, 0x67, 0x89, // modification time
				0x01, 0x02, 0x03, 0x04, // timescale
				0x02, 0x03, 0x04, 0x05, // duration
				0xaa, 0x0e, // pad, language (1 01010 10000 01110)
				0x00, 0x00, // pre defined
			},
		},
		{
			name: "mdhd: version 1",
			src: &Mdhd{
				FullBox: FullBox{
					Version: 1,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				CreationTimeV1:     0x123456789abcdef0,
				ModificationTimeV1: 0x23456789abcdef01,
				Timescale:          0x01020304,
				DurationV1:         0x0203040506070809,
				Pad:                true,
				Language:           [3]byte{'j' - 0x60, 'p' - 0x60, 'n' - 0x60}, // 0x0a, 0x10, 0x0e
				PreDefined:         0,
			},
			bin: []byte{
				1,                // version
				0x00, 0x00, 0x00, // flags
				0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, // creation time
				0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, // modification time
				0x01, 0x02, 0x03, 0x04, // timescale
				0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, // duration
				0xaa, 0x0e, // pad, language (1 01010 10000 01110)
				0x00, 0x00, // pre defined
			},
		},
		{
			name: "mdhd: language",
			src: &Mdhd{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				Timescale: 0x01020304,
				Language:  [3]byte{'u', 'n', 'd'},
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x0, 0x0, 0x0, 0x0, // creation time
				0x0, 0x0, 0x0, 0x0, // modification time
				0x01, 0x02, 0x03, 0x04, // timescale
				0x00, 0x00, 0x00, 0x00, // duration
				0x55, 0xc4, // pad, language
				0x00, 0x00, // pre defined
			},
		},

		{
			name: "mdia",
			src:  &Mdia{},
			bin:  []byte{},
		},
		{
			name: "meta",
			src: &Meta{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
			},
		},
		{
			name: "mfhd",
			src: &Mfhd{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				SequenceNumber: 0x12345678,
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x12, 0x34, 0x56, 0x78, // sequence number
			},
		},
		{
			name: "minf",
			src:  &Minf{},
			bin:  []byte{},
		},
		{
			name: "moof",
			src:  &Moof{},
			bin:  []byte{},
		},
		{
			name: "moov",
			src:  &Moov{},
			bin:  []byte{},
		},
		{
			name: "mvex",
			src:  &Mvex{},
			bin:  []byte{},
		},
		{
			name: "mvhd: version 0",
			src: &Mvhd{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				CreationTimeV0:     0x01234567,
				ModificationTimeV0: 0x23456789,
				Timescale:          0x456789ab,
				DurationV0:         0x6789abcd,
				Rate:               -0x01234567,
				Volume:             0x0123,
				Matrix:             [9]int32{},
				PreDefined:         [6]int32{},
				NextTrackID:        0xabcdef01,
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x01, 0x23, 0x45, 0x67, // creation time
				0x23, 0x45, 0x67, 0x89, // modification time
				0x45, 0x67, 0x89, 0xab, // timescale
				0x67, 0x89, 0xab, 0xcd, // duration
				0xfe, 0xdc, 0xba, 0x99, // rate
				0x01, 0x23, // volume
				0x00, 0x00, // reserved
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // reserved
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // matrix
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // pre-defined
				0xab, 0xcd, 0xef, 0x01, // next track ID
			},
		},
		{
			name: "mvhd: version 1",
			src: &Mvhd{
				FullBox: FullBox{
					Version: 1,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				CreationTimeV1:     0x0123456789abcdef,
				ModificationTimeV1: 0x23456789abcdef01,
				Timescale:          0x89abcdef,
				DurationV1:         0x456789abcdef0123,
				Rate:               -0x01234567,
				Volume:             0x0123,
				Matrix:             [9]int32{},
				PreDefined:         [6]int32{},
				NextTrackID:        0xabcdef01,
			},
			bin: []byte{
				1,                // version
				0x00, 0x00, 0x00, // flags
				0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, // creation time
				0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, // modification
				0x89, 0xab, 0xcd, 0xef, // timescale
				0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, // duration
				0xfe, 0xdc, 0xba, 0x99, // rate
				0x01, 0x23, // volume
				0x00, 0x00, // reserved
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // reserved
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // matrix
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // pre-defined
				0xab, 0xcd, 0xef, 0x01, // next track ID
			},
		},
		{
			name: "Avc1",
			src: &Avc1{
				SampleEntry: SampleEntry{
					DataReferenceIndex: 0x1234,
				},
				PreDefined:      0x0101,
				PreDefined2:     [3]uint32{0x01000001, 0x01000002, 0x01000003},
				Width:           0x0102,
				Height:          0x0103,
				Horizresolution: 0x01000004,
				Vertresolution:  0x01000005,
				Reserved2:       0x01000006,
				FrameCount:      0x0104,
				Compressorname:  [32]byte{8, 'a', 'b', 'e', 'm', 'a', 0x00, 't', 'v'},
				Depth:           0x0105,
				PreDefined3:     1001,
			},
			bin: []byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // reserved
				0x12, 0x34, // data reference index
				0x01, 0x01, // PreDefined
				0x00, 0x00, // Reserved
				0x01, 0x00, 0x00, 0x01,
				0x01, 0x00, 0x00, 0x02,
				0x01, 0x00, 0x00, 0x03, // PreDefined2
				0x01, 0x02, // Width
				0x01, 0x03, // Height
				0x01, 0x00, 0x00, 0x04, // Horizresolution
				0x01, 0x00, 0x00, 0x05, // Vertresolution
				0x01, 0x00, 0x00, 0x06, // Reserved2
				0x01, 0x04, // FrameCount
				8, 'a', 'b', 'e', 'm', 'a', 0x00, 't',
				'v', 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Compressorname
				0x01, 0x05, // Depth
				0x03, 0xe9, // PreDefined3
			},
		},
		{
			name: "Mp4a",
			src: &Mp4a{
				SampleEntry: SampleEntry{
					DataReferenceIndex: 0x1234,
				},
				EntryVersion: 0x0123,
				ChannelCount: 0x2345,
				SampleSize:   0x4567,
				PreDefined:   0x6789,
				SampleRate:   0x01234567,
			},
			bin: []byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // reserved
				0x12, 0x34, // data reference index
				0x01, 0x23, // entry version
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // reserved
				0x23, 0x45, // channel count
				0x45, 0x67, // sample size
				0x67, 0x89, // pre-defined
				0x00, 0x00, // reserved
				0x01, 0x23, 0x45, 0x67, // sample rate
			},
		},
		{
			name: "AvcC main profile",
			src: &AvcC{
				ConfigurationVersion:       0x12,
				Profile:                    AVCMainProfile,
				ProfileCompatibility:       0x40,
				Level:                      0x1f,
				Reserved:                   0x3f,
				LengthSizeMinusOne:         0x2,
				Reserved2:                  0x7,
				NumOfSequenceParameterSets: 2,
				SequenceParameterSets: []AVCParameterSet{
					{Length: 2, NALUnit: []byte{0x12, 0x34}},
					{Length: 3, NALUnit: []byte{0x12, 0x34, 0x56}},
				},
				NumOfPictureParameterSets: 2,
				PictureParameterSets: []AVCParameterSet{
					{Length: 2, NALUnit: []byte{0xab, 0xcd}},
					{Length: 3, NALUnit: []byte{0xab, 0xcd, 0xef}},
				},
			},
			bin: []byte{
				0x12,       // configuration version
				0x4d,       // profile
				0x40,       // profile compatibility
				0x1f,       // level
				0xfe,       // reserved,  lengthSizeMinusOne
				0xe2,       // reserved, numOfSequenceParameterSets
				0x00, 0x02, // length
				0x12, 0x34, // nalUnit
				0x00, 0x03, // length
				0x12, 0x34, 0x56, // nalUnit
				0x02,       // reserved, numOfSequenceParameterSets
				0x00, 0x02, // length
				0xab, 0xcd, // nalUnit
				0x00, 0x03, // length
				0xab, 0xcd, 0xef, // nalUnit
			},
		},
		{
			name: "AvcC high profile old spec",
			src: &AvcC{
				ConfigurationVersion:       0x12,
				Profile:                    AVCHighProfile,
				ProfileCompatibility:       0x00,
				Level:                      0x28,
				Reserved:                   0x3f,
				LengthSizeMinusOne:         0x2,
				Reserved2:                  0x7,
				NumOfSequenceParameterSets: 2,
				SequenceParameterSets: []AVCParameterSet{
					{Length: 2, NALUnit: []byte{0x12, 0x34}},
					{Length: 3, NALUnit: []byte{0x12, 0x34, 0x56}},
				},
				NumOfPictureParameterSets: 2,
				PictureParameterSets: []AVCParameterSet{
					{Length: 2, NALUnit: []byte{0xab, 0xcd}},
					{Length: 3, NALUnit: []byte{0xab, 0xcd, 0xef}},
				},
			},
			bin: []byte{
				0x12,       // configuration version
				0x64,       // profile
				0x00,       // profile compatibility
				0x28,       // level
				0xfe,       // reserved,  lengthSizeMinusOne
				0xe2,       // reserved, numOfSequenceParameterSets
				0x00, 0x02, // length
				0x12, 0x34, // nalUnit
				0x00, 0x03, // length
				0x12, 0x34, 0x56, // nalUnit
				0x02,       // reserved, numOfSequenceParameterSets
				0x00, 0x02, // length
				0xab, 0xcd, // nalUnit
				0x00, 0x03, // length
				0xab, 0xcd, 0xef, // nalUnit
			},
		},
		{
			name: "AvcC high profile new spec",
			src: &AvcC{
				ConfigurationVersion:       0x12,
				Profile:                    AVCHighProfile,
				ProfileCompatibility:       0x00,
				Level:                      0x28,
				Reserved:                   0x3f,
				LengthSizeMinusOne:         0x2,
				Reserved2:                  0x7,
				NumOfSequenceParameterSets: 2,
				SequenceParameterSets: []AVCParameterSet{
					{Length: 2, NALUnit: []byte{0x12, 0x34}},
					{Length: 3, NALUnit: []byte{0x12, 0x34, 0x56}},
				},
				NumOfPictureParameterSets: 2,
				PictureParameterSets: []AVCParameterSet{
					{Length: 2, NALUnit: []byte{0xab, 0xcd}},
					{Length: 3, NALUnit: []byte{0xab, 0xcd, 0xef}},
				},
				HighProfileFieldsEnabled:     true,
				Reserved3:                    0x3f,
				ChromaFormat:                 0x1,
				Reserved4:                    0x1f,
				BitDepthLumaMinus8:           0x2,
				Reserved5:                    0x1f,
				BitDepthChromaMinus8:         0x3,
				NumOfSequenceParameterSetExt: 2,
				SequenceParameterSetsExt: []AVCParameterSet{
					{Length: 2, NALUnit: []byte{0x12, 0x34}},
					{Length: 3, NALUnit: []byte{0x12, 0x34, 0x56}},
				},
			},
			bin: []byte{
				0x12,       // configuration version
				0x64,       // profile
				0x00,       // profile compatibility
				0x28,       // level
				0xfe,       // reserved,  lengthSizeMinusOne
				0xe2,       // reserved, numOfSequenceParameterSets
				0x00, 0x02, // length
				0x12, 0x34, // nalUnit
				0x00, 0x03, // length
				0x12, 0x34, 0x56, // nalUnit
				0x02,       // numOfSequenceParameterSets
				0x00, 0x02, // length
				0xab, 0xcd, // nalUnit
				0x00, 0x03, // length
				0xab, 0xcd, 0xef, // nalUnit
				0xfd,       // reserved, chromaFormat
				0xfa,       // reserved, bitDepthLumaMinus8
				0xfb,       // reserved, bitDepthChromaMinus8
				0x02,       // numOfSequenceParameterSets
				0x00, 0x02, // length
				0x12, 0x34, // nalUnit
				0x00, 0x03, // length
				0x12, 0x34, 0x56, // nalUnit
			},
		},

		{
			name: "smhd",
			src: &Smhd{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				Balance: 0x0123,
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x01, 0x23, // balance
				0x00, 0x00, // reserved
			},
		},
		{
			name: "stbl",
			src:  &Stbl{},
			bin:  []byte{},
		},
		{
			name: "stco",
			src: &Stco{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				EntryCount:  2,
				ChunkOffset: []uint32{0x01234567, 0x89abcdef},
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x00, 0x00, 0x00, 0x02, // entry count
				0x01, 0x23, 0x45, 0x67, // chunk offset
				0x89, 0xab, 0xcd, 0xef, // chunk offset
			},
		},
		{
			name: "stsc",
			src: &Stsc{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				EntryCount: 2,
				Entries: []StscEntry{
					{FirstChunk: 0x01234567, SamplesPerChunk: 0x23456789, SampleDescriptionIndex: 0x456789ab},
					{FirstChunk: 0x6789abcd, SamplesPerChunk: 0x89abcdef, SampleDescriptionIndex: 0xabcdef01},
				},
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x00, 0x00, 0x00, 0x02, // entry count
				0x01, 0x23, 0x45, 0x67, // first chunk
				0x23, 0x45, 0x67, 0x89, // sample per chunk
				0x45, 0x67, 0x89, 0xab, // sample description index
				0x67, 0x89, 0xab, 0xcd, // first chunk
				0x89, 0xab, 0xcd, 0xef, // sample per chunk
				0xab, 0xcd, 0xef, 0x01, // sample description index
			},
		},
		{
			name: "stsd",
			src: &Stsd{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				EntryCount: 0x01234567,
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x01, 0x23, 0x45, 0x67, // entry count
			},
		},
		{
			name: "stss",
			src: &Stss{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				EntryCount:   2,
				SampleNumber: []uint32{0x01234567, 0x89abcdef},
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x00, 0x00, 0x00, 0x02, // entry count
				0x01, 0x23, 0x45, 0x67, // sample number
				0x89, 0xab, 0xcd, 0xef, // sample number
			},
		},
		{
			name: "stsz: common sample size",
			src: &Stsz{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				SampleSize:  0x01234567,
				SampleCount: 2,
				EntrySize:   []uint32{},
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x01, 0x23, 0x45, 0x67, // sample size
				0x00, 0x00, 0x00, 0x02, // sample count
			},
		},
		{
			name: "stsz: sample size array",
			src: &Stsz{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				SampleCount: 2,
				EntrySize:   []uint32{0x01234567, 0x23456789},
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x00, 0x00, 0x00, 0x00, // sample size
				0x00, 0x00, 0x00, 0x02, // sample count
				0x01, 0x23, 0x45, 0x67, // entry size
				0x23, 0x45, 0x67, 0x89, // entry size
			},
		},
		{
			name: "stts",
			src: &Stts{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				EntryCount: 2,
				Entries: []SttsEntry{
					{SampleCount: 0x01234567, SampleDelta: 0x23456789},
					{SampleCount: 0x456789ab, SampleDelta: 0x6789abcd},
				},
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x00, 0x00, 0x00, 0x02, // entry count
				0x01, 0x23, 0x45, 0x67, // sample count
				0x23, 0x45, 0x67, 0x89, // sample delta
				0x45, 0x67, 0x89, 0xab, // sample count
				0x67, 0x89, 0xab, 0xcd, // sample delta
			},
		},
		{
			name: "tfdt: version 0",
			src: &Tfdt{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				BaseMediaDecodeTimeV0: 0x01234567,
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x01, 0x23, 0x45, 0x67, // base media decode time
			},
		},
		{
			name: "tfdt: version 1",
			src: &Tfdt{
				FullBox: FullBox{
					Version: 1,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				BaseMediaDecodeTimeV1: 0x0123456789abcdef,
			},
			bin: []byte{
				1,                // version
				0x00, 0x00, 0x00, // flags
				0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, // base media decode time
			},
		},
		{
			name: "tfhd: no flags",
			src: &Tfhd{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				TrackID: 0x08404649,
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x08, 0x40, 0x46, 0x49, // track ID
			},
		},
		{
			name: "tfhd: base data offset & default sample duration",
			src: &Tfhd{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, TfhdBaseDataOffsetPresent | TfhdDefaultSampleDurationPresent},
				},
				TrackID:               0x08404649,
				BaseDataOffset:        0x0123456789abcdef,
				DefaultSampleDuration: 0x23456789,
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x09, // flags (0000 0000 1001)
				0x08, 0x40, 0x46, 0x49, // track ID
				0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
				0x23, 0x45, 0x67, 0x89,
			},
		},
		{
			name: "tkhd version 0",
			src: &Tkhd{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				CreationTimeV0:     0x01234567,
				ModificationTimeV0: 0x12345678,
				TrackID:            0x23456789,
				Reserved0:          0x3456789a,
				DurationV0:         0x456789ab,
				Reserved1:          [2]uint32{0, 0},
				Layer:              23456,  // 0x5ba0
				AlternateGroup:     -23456, // 0xdba0
				Volume:             0x0100,
				Reserved2:          0,
				Matrix: [9]int32{
					0x00010000, 0, 0,
					0, 0x00010000, 0,
					0, 0, 0x40000000,
				},
				Width:  125829120,
				Height: 70778880,
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x01, 0x23, 0x45, 0x67, // creation time
				0x12, 0x34, 0x56, 0x78, // modification time
				0x23, 0x45, 0x67, 0x89, // track ID
				0x34, 0x56, 0x78, 0x9a, // reserved
				0x45, 0x67, 0x89, 0xab, // duration
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // reserved
				0x5b, 0xa0, // layer
				0xa4, 0x60, // alternate group
				0x01, 0x00, // volume
				0x00, 0x00, // reserved
				0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, // matrix
				0x07, 0x80, 0x00, 0x00, // width
				0x04, 0x38, 0x00, 0x00, // height
			},
		},
		{
			name: "tkhd version 1",
			src: &Tkhd{
				FullBox: FullBox{
					Version: 1,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				CreationTimeV1:     0x0123456789abcdef,
				ModificationTimeV1: 0x123456789abcdef0,
				TrackID:            0x23456789,
				Reserved0:          0x3456789a,
				DurationV1:         0x456789abcdef0123,
				Reserved1:          [2]uint32{0, 0},
				Layer:              23456,  // 0x5ba0
				AlternateGroup:     -23456, // 0xdba0
				Volume:             0x0100,
				Reserved2:          0,
				Matrix: [9]int32{
					0x00010000, 0, 0,
					0, 0x00010000, 0,
					0, 0, 0x40000000,
				},
				Width:  125829120,
				Height: 70778880,
			},
			bin: []byte{
				1,                // version
				0x00, 0x00, 0x00, // flags
				0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, // creation time
				0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, // modification time
				0x23, 0x45, 0x67, 0x89, // track ID
				0x34, 0x56, 0x78, 0x9a, // reserved
				0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, // duration
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // reserved
				0x5b, 0xa0, // layer
				0xa4, 0x60, // alternate group
				0x01, 0x00, // volume
				0x00, 0x00, // reserved
				0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, // matrix
				0x07, 0x80, 0x00, 0x00, // width
				0x04, 0x38, 0x00, 0x00, // height
			},
		},
		{
			name: "traf",
			src:  &Traf{},
			bin:  []byte{},
		},
		{
			name: "trak",
			src:  &Trak{},
			bin:  []byte{},
		},
		{
			name: "trex",
			src: &Trex{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				TrackID:                       0x01234567,
				DefaultSampleDescriptionIndex: 0x23456789,
				DefaultSampleDuration:         0x456789ab,
				DefaultSampleSize:             0x6789abcd,
				DefaultSampleFlags:            0x89abcdef,
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x01, 0x23, 0x45, 0x67, // track ID
				0x23, 0x45, 0x67, 0x89, // default sample description index
				0x45, 0x67, 0x89, 0xab, // default sample duration
				0x67, 0x89, 0xab, 0xcd, // default sample size
				0x89, 0xab, 0xcd, 0xef, // default sample flags
			},
		},
		{
			name: "trun: version=0 flag=0x101",
			src: &Trun{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x01, 0x01},
				},
				SampleCount: 3,
				DataOffset:  50,
				Entries: []TrunEntry{
					{SampleDuration: 100},
					{SampleDuration: 101},
					{SampleDuration: 102},
				},
			},
			bin: []byte{
				0,                // version
				0x00, 0x01, 0x01, // flags
				0x00, 0x00, 0x00, 0x03, // sample count
				0x00, 0x00, 0x00, 0x32, // data offset
				0x00, 0x00, 0x00, 0x64, // sample duration
				0x00, 0x00, 0x00, 0x65, // sample duration
				0x00, 0x00, 0x00, 0x66, // sample duration
			},
		},
		{
			name: "trun: version=0 flag=0x204",
			src: &Trun{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x02, 0x04},
				},
				SampleCount:      3,
				FirstSampleFlags: 0x02468ace,
				Entries: []TrunEntry{
					{SampleSize: 100},
					{SampleSize: 101},
					{SampleSize: 102},
				},
			},
			bin: []byte{
				0,                // version
				0x00, 0x02, 0x04, // flags
				0x00, 0x00, 0x00, 0x03, // sample count
				0x02, 0x46, 0x8a, 0xce, // first sample flags
				0x00, 0x00, 0x00, 0x64, // sample size
				0x00, 0x00, 0x00, 0x65, // sample size
				0x00, 0x00, 0x00, 0x66, // sample size
			},
		},
		{
			name: "trun: version=0 flag=0xc00",
			src: &Trun{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x0c, 0x00},
				},
				SampleCount: 3,
				Entries: []TrunEntry{
					{SampleFlags: 100, SampleCompositionTimeOffsetV0: 200},
					{SampleFlags: 101, SampleCompositionTimeOffsetV0: 201},
					{SampleFlags: 102, SampleCompositionTimeOffsetV0: 202},
				},
			},
			bin: []byte{
				0,                // version
				0x00, 0x0c, 0x00, // flags
				0x00, 0x00, 0x00, 0x03, // sample count
				0x00, 0x00, 0x00, 0x64, // sample flags
				0x00, 0x00, 0x00, 0xc8, // sample composition time offset
				0x00, 0x00, 0x00, 0x65, // sample flags
				0x00, 0x00, 0x00, 0xc9, // sample composition time offset
				0x00, 0x00, 0x00, 0x66, // sample flags
				0x00, 0x00, 0x00, 0xca, // sample composition time offset
			},
		},
		{
			name: "trun: version=1 flag=0x800",
			src: &Trun{
				FullBox: FullBox{
					Version: 1,
					Flags:   [3]byte{0x00, 0x08, 0x00},
				},
				SampleCount: 3,
				Entries: []TrunEntry{
					{SampleCompositionTimeOffsetV1: 200},
					{SampleCompositionTimeOffsetV1: 201},
					{SampleCompositionTimeOffsetV1: -202},
				},
			},
			bin: []byte{
				1,                // version
				0x00, 0x08, 0x00, // flags
				0x00, 0x00, 0x00, 0x03, // sample count
				0x00, 0x00, 0x00, 0xc8, // sample composition time offset
				0x00, 0x00, 0x00, 0xc9, // sample composition time offset
				0xff, 0xff, 0xff, 0x36, // sample composition time offset
			},
		},
		{
			name: "udta",
			src:  &Udta{},
			bin:  []byte{},
		},
		{
			name: "vmhd",
			src: &Vmhd{
				FullBox: FullBox{
					Version: 0,
					Flags:   [3]byte{0x00, 0x00, 0x00},
				},
				Graphicsmode: 0x0123,
				Opcolor:      [3]uint16{0x2345, 0x4567, 0x6789},
			},
			bin: []byte{
				0,                // version
				0x00, 0x00, 0x00, // flags
				0x01, 0x23, // graphics mode
				0x23, 0x45, 0x45, 0x67, 0x67, 0x89, // opcolor
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Marshal
			box := Boxes{Box: tc.src}
			buf := bytes.NewBuffer(make([]byte, 0, tc.src.Size()))

			w := bitio.NewWriter(buf)
			box.Box.Marshal(w)

			require.Equal(t, int(tc.src.Size()), buf.Len())
			require.Equal(t, tc.bin, buf.Bytes())
		})
	}
}
