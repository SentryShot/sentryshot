package mp4muxer

import (
	"bytes"
	"testing"

	"nvr/pkg/video/customformat"
	"nvr/pkg/video/gortsplib/pkg/h264"
	"nvr/pkg/video/hls"

	"github.com/stretchr/testify/require"
)

func TestGenerateMP4(t *testing.T) {
	samples := []customformat.Sample{
		{ // VideoSample3. B-Frame.
			PTS:  200000,
			DTS:  300001,
			Next: 400001,
			Size: 2,
		},
		{ // VideoSample2. P-Frame.
			PTS:  300000,
			DTS:  200001,
			Next: 300001,
			Size: 2,
		},
		{ // VideoSample1. I-Frame.
			IsSyncSample: true,
			PTS:          150000,
			DTS:          100001,
			Next:         200001,
			Size:         2,
		},

		{ // AudioSample2.
			IsAudioSample: true,
			PTS:           300000,
			Next:          400000,
			Size:          2,
		},
		{ // AudioSample1.
			IsAudioSample: true,
			PTS:           200000,
			Next:          300000,
			Size:          2,
		},
	}

	sps := []byte{
		103, 100, 0, 22, 172, 217, 64, 164,
		59, 228, 136, 192, 68, 0, 0, 3,
		0, 4, 0, 0, 3, 0, 96, 60,
		88, 182, 88,
	}

	var spsp h264.SPS
	err := spsp.Unmarshal(sps)
	require.NoError(t, err)

	buf := &bytes.Buffer{}
	info := hls.StreamInfo{
		VideoTrackExist: true,
		VideoSPS:        sps,
		VideoSPSP:       spsp,
		VideoWidth:      640,
		VideoHeight:     480,
		AudioTrackExist: true,
		AudioClockRate:  48000,
	}

	startTime := int64(10000)
	mdatSize, err := GenerateMP4(buf, startTime, samples, info)
	require.NoError(t, err)
	require.Equal(t, int64(10), mdatSize)

	expected := []byte{
		0, 0, 0, 0x14, 'f', 't', 'y', 'p',
		'i', 's', 'o', '4',
		0, 0, 2, 0, // Minor version.
		'i', 's', 'o', '4',

		0, 0, 4, 0x77, 'm', 'o', 'o', 'v',
		0, 0, 0, 0x6c, 'm', 'v', 'h', 'd',
		0, 0, 0, 0, // Fullbox.
		0, 0, 0, 0, // Creation time.
		0, 0, 0, 0, // Modification time.
		0, 0, 3, 0xe8, // Timescale.
		0, 0, 0, 0, // Duration.
		0, 1, 0, 0, // Rate.
		1, 0, // Volume.
		0, 0, // Reserved.
		0, 0, 0, 0, 0, 0, 0, 0, // Reserved2.
		0, 1, 0, 0, 0, 0, 0, 0, 0, // Matrix.
		0, 0, 0, 0, 0, 0, 0, 0, 1,
		0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0x40, 0, 0, 0,
		0, 0, 0, 0, 0, 0, // Pre-defined.
		0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0,
		0, 0, 0, 1, // Next track ID.

		/* Video trak */
		0, 0, 2, 0x39, 't', 'r', 'a', 'k',
		0, 0, 0, 0x5c, 't', 'k', 'h', 'd',
		0, 0, 0, 3, // Fullbox.
		0, 0, 0, 0, // Creation time.
		0, 0, 0, 0, // Modification time.
		0, 0, 0, 0, // Track ID.
		0, 0, 0, 0, // Reserved0.
		0, 0, 0, 0, // Duration.
		0, 0, 0, 0, 0, 0, 0, 0, // Reserved1.
		0, 0, // Layer.
		0, 0, // Alternate group.
		0, 0, // Volume.
		0, 0, // Reserved2.
		0, 1, 0, 0, 0, 0, 0, 0, 0, // Matrix.
		0, 0, 0, 0, 0, 0, 0, 0, 1,
		0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0x40, 0, 0, 0,
		2, 0x80, 0, 0, // Width.
		1, 0xe0, 0, 0, // Height.
		0, 0, 1, 0xd5, 'm', 'd', 'i', 'a',
		0, 0, 0, 0x20, 'm', 'd', 'h', 'd',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Creation time.
		0, 0, 0, 0, // Modification time.
		0, 1, 0x5f, 0x90, // Time scale.
		0, 0, 0, 0x11, // Duration.
		0x55, 0xc4, // Language.
		0, 0, // Predefined.
		0, 0, 0, 0x2d, 'h', 'd', 'l', 'r',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Predefined.
		'v', 'i', 'd', 'e', // Handler type.
		0, 0, 0, 0, // Reserved.
		0, 0, 0, 0,
		0, 0, 0, 0,
		'V', 'i', 'd', 'e', 'o', 'H', 'a', 'n', 'd', 'l', 'e', 'r', 0,
		0, 0, 1, 0x80, 'm', 'i', 'n', 'f',
		0, 0, 0, 0x14, 'v', 'm', 'h', 'd',
		0, 0, 0, 0, // FullBox.
		0, 0, // Graphics mode.
		0, 0, 0, 0, 0, 0, // OpColor.
		0, 0, 0, 0x24, 'd', 'i', 'n', 'f',
		0, 0, 0, 0x1c, 'd', 'r', 'e', 'f',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 0, 0xc, 'u', 'r', 'l', ' ',
		0, 0, 0, 1, // FullBox.
		0, 0, 1, 0x40, 's', 't', 'b', 'l',
		0, 0, 0, 0x94, 's', 't', 's', 'd',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 0, 0x84, 'a', 'v', 'c', '1',
		0, 0, 0, 0, 0, 0, // Reserved.
		0, 1, // Data reference index.
		0, 0, // Predefined.
		0, 0, // Reserved.
		0, 0, 0, 0, // Predefined2.
		0, 0, 0, 0,
		0, 0, 0, 0,
		2, 0x80, // Width.
		1, 0xe0, // Height.
		0, 0x48, 0, 0, // Horizresolution
		0, 0x48, 0, 0, // Vertresolution
		0, 0, 0, 0, // Reserved2.
		0, 1, // Frame count.
		0, 0, 0, 0, 0, 0, 0, 0, // Compressor name.
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0x18, // Depth.
		0xff, 0xff, // Predefined3.
		0, 0, 0, 0x2e, 'a', 'v', 'c', 'C',
		1,       // Configuration version.
		0x64,    // Profile.
		0,       // Profile compatibility.
		0x16,    // Level.
		3,       // Reserved, Length size minus one.
		1,       // Reserved, N sequence parameters.
		0, 0x1b, // Length 27.
		0x67, 0x64, 0, 0x16, 0xac, // Parameter set.
		0xd9, 0x40, 0xa4, 0x3b, 0xe4,
		0x88, 0xc0, 0x44, 0, 0,
		3, 0, 4, 0, 0,
		3, 0, 0x60, 0x3c, 0x58,
		0xb6, 0x58,
		1,    // Reserved N sequence parameters.
		0, 0, // Length.
		0, 0, 0, 0x18, 's', 't', 't', 's',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 0, 3, // Entry1 sample count.
		0, 0, 0, 9, // Entry1 sample delta.
		0, 0, 0, 0x14, 's', 't', 's', 's',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 0, 3, // Entry1.
		0, 0, 0, 0x28, 'c', 't', 't', 's',
		1, 0, 0, 0, // FullBox.
		0, 0, 0, 3, // Entry count.
		0, 0, 0, 1, // Entry1 sample count.
		0, 0, 0, 0, // Entry1 sample offset
		0, 0, 0, 1, // Entry2 sample count.
		0, 0, 0, 0x12, // Entry2 sample offset
		0, 0, 0, 1, // Entry3 sample count.
		0, 0, 0, 0xe, // Entry3 sample offset
		0, 0, 0, 0x1c, 's', 't', 's', 'c',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 0, 1, // Entry1 first chunk.
		0, 0, 0, 3, // Entry1 samples per chunk.
		0, 0, 0, 1, // Entry1 sample description index.
		0, 0, 0, 0x20, 's', 't', 's', 'z',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Sample size.
		0, 0, 0, 3, // Sample count.
		0, 0, 0, 2, // Entry1 size.
		0, 0, 0, 2, // Entry2 size.
		0, 0, 0, 2, // Entry3 size.
		0, 0, 0, 0x14, 's', 't', 'c', 'o',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 4, 0x93, // Chunk offset1.

		/* Audio trak */
		0, 0, 1, 0xca, 't', 'r', 'a', 'k',
		0, 0, 0, 0x5c, 't', 'k', 'h', 'd',
		0, 0, 0, 3, // FullBox.
		0, 0, 0, 0, // Creation time.
		0, 0, 0, 0, // Modification time.
		0, 0, 0, 1, // Track ID.
		0, 0, 0, 0, // Reserved.
		0, 0, 0, 0, // Duration.
		0, 0, 0, 0, 0, 0, 0, 0, // Reserved.
		0, 0, // Layer.
		0, 1, // Alternate group.
		1, 0, // Volume.
		0, 0, // Reserved.
		0, 1, 0, 0, // 1 Matrix.
		0, 0, 0, 0, // 2.
		0, 0, 0, 0, // 3.
		0, 0, 0, 0, // 4.
		0, 1, 0, 0, // 5.
		0, 0, 0, 0, // 6.
		0, 0, 0, 0, // 7.
		0, 0, 0, 0, // 8.
		0x40, 0, 0, 0, // 9.
		0, 0, 0, 0, // Width.
		0, 0, 0, 0, // Height
		0, 0, 1, 0x66, 'm', 'd', 'i', 'a',
		0, 0, 0, 0x20, 'm', 'd', 'h', 'd',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Creation time.
		0, 0, 0, 0, // Modification time.
		0, 0, 0xbb, 0x80, // Timescale.
		0, 0, 0, 0x9, // Duration.
		0x55, 0xc4, // Language.
		0, 0, // Predefined.
		0, 0, 0, 0x2d, 'h', 'd', 'l', 'r',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Predefined.
		's', 'o', 'u', 'n', // Handler type.
		0, 0, 0, 0, // Reserved.
		0, 0, 0, 0,
		0, 0, 0, 0,
		'S', 'o', 'u', 'n', 'd', 'H', 'a', 'n', 'd', 'l', 'e', 'r', 0,
		0, 0, 1, 0x11, 'm', 'i', 'n', 'f',
		0, 0, 0, 0x14, 'v', 'm', 'h', 'd',
		0, 0, 0, 0, // FullBox.
		0, 0, // Graphics mode.
		0, 0, 0, 0, 0, 0, // OpColor.
		0, 0, 0, 0x24, 'd', 'i', 'n', 'f',
		0, 0, 0, 0x1c, 'd', 'r', 'e', 'f',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 0, 0xc, 'u', 'r', 'l', ' ',
		0, 0, 0, 1, // FullBox.
		0, 0, 0, 0xd1, 's', 't', 'b', 'l',
		0, 0, 0, 0x65, 's', 't', 's', 'd',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 0, 0x55, 'm', 'p', '4', 'a',
		0, 0, 0, 0, 0, 0, // Reserved.
		0, 1, // Data reference index.
		0, 0, // Entry version.
		0, 0, 0, 0, 0, 0,
		0, 0, //  Channel count.
		0, 0x10, // Sample size 16.
		0, 0, // Predefined.
		0, 0, // Reserved2.
		0xbb, 0x80, 0, 0, // Sample rate.
		0, 0, 0, 0x31, 'e', 's', 'd', 's',
		0, 0, 0, 0, // FullBox.
		3, 0x80, 0x80, 0x80, 0x20, 0, 1, 0, // Data.
		4, 0x80, 0x80, 0x80, 0x12, 0x40, 0x15, 0,
		0, 0, 0, 1,
		0xf7, 0x39, 0, 1,
		0xf7, 0x39, 5, 0x80,
		0x80, 0x80, 0, 6, 0x80, 0x80, 0x80, 1, 2,
		0, 0, 0, 0x18, 's', 't', 't', 's',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 0, 2, // Entry1 sample count.
		0, 0, 0, 5, // Entry1 sample delta.
		0, 0, 0, 0x1c, 's', 't', 's', 'c',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 0, 1, // Entry1 first chunk.
		0, 0, 0, 2, // Entry1 samples per chunk.
		0, 0, 0, 1, // Entry1 sample description index.
		0, 0, 0, 0x1c, 's', 't', 's', 'z',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Sample size.
		0, 0, 0, 2, // Sample count.
		0, 0, 0, 2, // Entry1 size.
		0, 0, 0, 2, // Entry2 size.
		0, 0, 0, 0x14, 's', 't', 'c', 'o',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 4, 0x99, // Chunk offset1.

		0, 0, 0, 0x12, 'm', 'd', 'a', 't',
	}
	require.Equal(t, expected, buf.Bytes())
}
