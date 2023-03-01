package hls

import (
	"testing"

	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/mpeg4audio"

	"github.com/stretchr/testify/require"
)

func TestGenerateInit(t *testing.T) {
	sps := []byte{
		103, 100, 0, 22, 172, 217, 64, 164,
		59, 228, 136, 192, 68, 0, 0, 3,
		0, 4, 0, 0, 3, 0, 96, 60,
		88, 182, 88,
	}

	videoTrack := &gortsplib.TrackH264{SPS: sps}
	audioTrack := &gortsplib.TrackMPEG4Audio{Config: &mpeg4audio.Config{ChannelCount: 1}}

	actual, err := generateInit(
		videoTrack,
		audioTrack,
	)
	require.NoError(t, err)
	expected := []byte{
		0, 0, 0, 0x20, 'f', 't', 'y', 'p',
		'm', 'p', '4', '2', // Major brand.
		0, 0, 0, 1, // Minor version.
		'm', 'p', '4', '1', // Compatible brand.
		'm', 'p', '4', '2', // Compatible brand.
		'i', 's', 'o', 'm', // Compatible brand.
		'h', 'l', 's', 'f', // Compatible brand.
		0, 0, 4, 0x68, 'm', 'o', 'o', 'v',
		0, 0, 0, 0x6c, 'm', 'v', 'h', 'd',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Creation time.
		0, 0, 0, 0, // Modification time.
		0, 0, 3, 0xe8, // Time scale.
		0, 0, 0, 0, // Duration.
		0, 1, 0, 0, // Rate.
		1, 0, // Volume.
		0, 0, // Reserved.
		0, 0, 0, 0, 0, 0, 0, 0, // Reserved2.
		0, 1, 0, 0, // 1 Matrix.
		0, 0, 0, 0, // 2.
		0, 0, 0, 0, // 3.
		0, 0, 0, 0, // 4.
		0, 1, 0, 0, // 5.
		0, 0, 0, 0, // 6.
		0, 0, 0, 0, // 7.
		0, 0, 0, 0, // 8.
		0x40, 0, 0, 0, // 9.
		0, 0, 0, 0, // 1 Predefined.
		0, 0, 0, 0, // 2.
		0, 0, 0, 0, // 3.
		0, 0, 0, 0, // 4.
		0, 0, 0, 0, // 5.
		0, 0, 0, 0, // 6.
		0, 0, 0, 2, // Next track ID.
		0, 0, 1, 0xed, 't', 'r', 'a', 'k', // Video.
		0, 0, 0, 0x5c, 't', 'k', 'h', 'd',
		0, 0, 0, 3, // FullBox.
		0, 0, 0, 0, // Creation time.
		0, 0, 0, 0, // Modification time.
		0, 0, 0, 1, // Track ID.
		0, 0, 0, 0, // Reserved0.
		0, 0, 0, 0, // Duration.
		0, 0, 0, 0, 0, 0, 0, 0, // Reserved1.
		0, 0, // Layer.
		0, 0, // Alternate group.
		0, 0, // Volume.
		0, 0, // Reserved2.
		0, 1, 0, 0, // 1 Matrix.
		0, 0, 0, 0, // 2.
		0, 0, 0, 0, // 3.
		0, 0, 0, 0, // 4.
		0, 1, 0, 0, // 5.
		0, 0, 0, 0, // 6.
		0, 0, 0, 0, // 7.
		0, 0, 0, 0, // 8.
		0x40, 0, 0, 0, // 9.
		2, 0x8a, 0, 0, // Width
		1, 0xc2, 0, 0, // Height
		0, 0, 1, 0x89, 'm', 'd', 'i', 'a',
		0, 0, 0, 0x20, 'm', 'd', 'h', 'd',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Creation time.
		0, 0, 0, 0, // Modification time.
		0, 1, 0x5f, 0x90, // Time scale.
		0, 0, 0, 0, // Duration.
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
		0, 0, 1, 0x34, 'm', 'i', 'n', 'f',
		0, 0, 0, 0x14, 'v', 'm', 'h', 'd',
		0, 0, 0, 1, // FullBox.
		0, 0, // Graphics mode.
		0, 0, 0, 0, 0, 0, // OpColor.
		0, 0, 0, 0x24, 'd', 'i', 'n', 'f',
		0, 0, 0, 0x1c, 'd', 'r', 'e', 'f',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 0, 0xc, 'u', 'r', 'l', ' ',
		0, 0, 0, 1, // FullBox.
		0, 0, 0, 0xf4, 's', 't', 'b', 'l',
		0, 0, 0, 0xa8, 's', 't', 's', 'd',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 0, 0x98, 'a', 'v', 'c', '1',
		0, 0, 0, 0, 0, 0, // Reserved.
		0, 1, // Data reference index.
		0, 0, // Predefined.
		0, 0, // Reserved.
		0, 0, 0, 0, // Predefined2.
		0, 0, 0, 0,
		0, 0, 0, 0,
		2, 0x8a, // Width.
		1, 0xc2, // Height.
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
		0, 0, 0, 0x14, 'b', 't', 'r', 't',
		0, 0, 0, 0, // Buffer size.
		0, 0xf, 0x42, 0x40, // Max bitrate.
		0, 0xf, 0x42, 0x40, // Average bitrate.
		0, 0, 0, 0x10, 's', 't', 't', 's',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Entry count.
		0, 0, 0, 0x10, 's', 't', 's', 'c',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Entry count.
		0, 0, 0, 0x14, 's', 't', 's', 'z',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Sample size.
		0, 0, 0, 0, // Sample count.
		0, 0, 0, 0x10, 's', 't', 'c', 'o',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Entry count.
		0, 0, 1, 0xbf, 't', 'r', 'a', 'k', // Audio.
		0, 0, 0, 0x5c, 't', 'k', 'h', 'd',
		0, 0, 0, 3, // FullBox.
		0, 0, 0, 0, // Creation time.
		0, 0, 0, 0, // Modification time.
		0, 0, 0, 2, // Track ID.
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
		0, 0, 1, 0x5b, 'm', 'd', 'i', 'a',
		0, 0, 0, 0x20, 'm', 'd', 'h', 'd',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Creation time.
		0, 0, 0, 0, // Modification time.
		0, 0, 0, 0, // Timescale.
		0, 0, 0, 0, // Duration.
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
		0, 0, 1, 6, 'm', 'i', 'n', 'f',
		0, 0, 0, 0x10, 's', 'm', 'h', 'd',
		0, 0, 0, 0, // FullBox.
		0, 0, // Balance.
		0, 0, // Reserved.
		0, 0, 0, 0x24, 'd', 'i', 'n', 'f',
		0, 0, 0, 0x1c, 'd', 'r', 'e', 'f',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 0, 0xc, 'u', 'r', 'l', ' ',
		0, 0, 0, 1, // FullBox.
		0, 0, 0, 0xca, 's', 't', 'b', 'l',
		0, 0, 0, 0x7e, 's', 't', 's', 'd',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 0, 0x6e, 'm', 'p', '4', 'a',
		0, 0, 0, 0, 0, 0, // Reserved.
		0, 1, // Data reference index.
		0, 0, // Entry version.
		0, 0, 0, 0, 0, 0,
		0, 1, //  Channel count.
		0, 0x10, // Sample size 16.
		0, 0, // Predefined.
		0, 0, // Reserved2.
		0, 0, 0, 0, // Sample rate.
		0, 0, 0, 0x36, 'e', 's', 'd', 's',
		0, 0, 0, 0, // FullBox.
		3, // Tag ES_Descriptor.
		0x80, 0x80, 0x80,
		0x25, // Size.
		0, 2, // ES_ID.
		0, // Flags.
		4, // Tag DecoderConfigDescriptor.
		0x80, 0x80, 0x80,
		0x17,    // Size.
		0x40,    // ObjectTypeIndicator.
		0x15,    // StreamType and upStream.
		0, 0, 0, // BufferSizeDB.
		0, 1, 0xf7, 0x39, // MaxBitrate.
		0, 1, 0xf7, 0x39, // AverageBitrate.
		5, // Tag DecoderSpecificInfo.
		0x80, 0x80, 0x80,
		5,                // Size
		7, 0x80, 0, 0, 8, // Config.
		6, // Tag SLConfigDescriptor.
		0x80, 0x80, 0x80,
		1, // Size.
		2, // Flags.
		0, 0, 0, 0x14, 'b', 't', 'r', 't',
		0, 0, 0, 0, // Buffer size.
		0, 1, 0xf7, 0x39, // Max bitrate.
		0, 1, 0xf7, 0x39, // Average bitrate.
		0, 0, 0, 0x10, 's', 't', 't', 's',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Entry count.
		0, 0, 0, 0x10, 's', 't', 's', 'c',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Entry count.
		0, 0, 0, 0x14, 's', 't', 's', 'z',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Sample size.
		0, 0, 0, 0, // Sample count.
		0, 0, 0, 0x10, 's', 't', 'c', 'o',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Entry count.
		0, 0, 0, 0x48, 'm', 'v', 'e', 'x',
		0, 0, 0, 0x20, 't', 'r', 'e', 'x',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Track ID.
		0, 0, 0, 1, // Default sample description index.
		0, 0, 0, 0, // Default sample duration.
		0, 0, 0, 0, // Default sample size.
		0, 0, 0, 0, // Default sample flags.
		0, 0, 0, 0x20, 't', 'r', 'e', 'x',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 2, // Track ID.
		0, 0, 0, 1, // Default sample description index.
		0, 0, 0, 0, // Default sample duration.
		0, 0, 0, 0, // Default sample size.
		0, 0, 0, 0, // Default sample flags.
	}
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}
