package mp4muxer

import (
	"bytes"
	"testing"
	"time"

	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/h264"
	"nvr/pkg/video/hls"

	"github.com/stretchr/testify/require"
)

func TestGenerateThumbnailVideo(t *testing.T) {
	videoSample1 := &hls.VideoSample{
		PTS:        30000,
		DTS:        40000,
		AVCC:       []byte{0x0, 0x1},
		IdrPresent: true,
		NextDTS:    60000,
	}
	audioSample1 := &hls.AudioSample{
		AU:  []byte{0x6, 0x7},
		PTS: 10000,
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
	/*info := hls.StreamInfo{
		VideoSPS:        sps,
		VideoSPSP:       spsp,
		VideoWidth:      640,
		VideoHeight:     480,
		AudioTrackExist: true,
	}*/
	videoTrack := &gortsplib.TrackH264{SPS: sps}

	firstSegment := &hls.Segment{
		StartTime:        time.Unix(0, int64(1*time.Hour)),
		RenderedDuration: 1 * time.Hour,

		ID: 1,
		Parts: []*hls.MuxerPart{
			{
				VideoSamples: []*hls.VideoSample{videoSample1},
				AudioSamples: []*hls.AudioSample{audioSample1},
			},
		},
	}

	err = GenerateThumbnailVideo(buf, firstSegment, videoTrack)
	require.NoError(t, err)

	expected := []byte{
		0, 0, 0, 0x14, 'f', 't', 'y', 'p',
		'i', 's', 'o', '4',
		0, 0, 2, 0, // Minor version.
		'i', 's', 'o', '4',

		0, 0, 2, 0x69, 'm', 'o', 'o', 'v',
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
		0, 0, 0, 2, // Next track ID.

		/* Video trak */
		0, 0, 1, 0xf5, 't', 'r', 'a', 'k',
		0, 0, 0, 0x5c, 't', 'k', 'h', 'd',
		0, 0, 0, 3, // Fullbox.
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
		0, 1, 0, 0, 0, 0, 0, 0, 0, // Matrix.
		0, 0, 0, 0, 0, 0, 0, 0, 1,
		0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0x40, 0, 0, 0,
		2, 0x8a, 0, 0, // Width.
		1, 0xc2, 0, 0, // Height.
		0, 0, 1, 0x91, 'm', 'd', 'i', 'a',
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
		0, 0, 1, 0x3c, 'm', 'i', 'n', 'f',
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
		0, 0, 0, 0xfc, 's', 't', 'b', 'l',
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
		0, 0, 0, 0x18, 's', 't', 't', 's',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 0, 1, // Entry1 sample count.
		0, 0, 0, 0, // Entry1 sample delta.
		0, 0, 0, 0x1c, 's', 't', 's', 'c',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 0, 1, // Entry1 first chunk.
		0, 0, 0, 1, // Entry1 samples per chunk.
		0, 0, 0, 1, // Entry1 sample description index.
		0, 0, 0, 0x18, 's', 't', 's', 'z',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Sample size.
		0, 0, 0, 1, // Sample count.
		0, 0, 0, 2, // Entry1 size.
		0, 0, 0, 0x14, 's', 't', 'c', 'o',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 1, // Entry count.
		0, 0, 2, 0x85, // Chunk offset1.

		0, 0, 0, 0x0a, 'm', 'd', 'a', 't',
		0x0, 0x1, // Video sample 1.
	}
	require.Equal(t, expected, buf.Bytes())
}

func TestGenerateThumbnailVideoErrors(t *testing.T) {
	t.Run("sampleMissing", func(t *testing.T) {
		segment := &hls.Segment{
			Parts: []*hls.MuxerPart{{
				VideoSamples: []*hls.VideoSample{},
			}},
		}
		err := GenerateThumbnailVideo(nil, segment, &gortsplib.TrackH264{})
		require.ErrorIs(t, err, ErrSampleMissing)
	})
	t.Run("sampleInvalid", func(t *testing.T) {
		segment := &hls.Segment{
			Parts: []*hls.MuxerPart{{
				VideoSamples: []*hls.VideoSample{{
					IdrPresent: false,
				}},
			}},
		}
		err := GenerateThumbnailVideo(nil, segment, &gortsplib.TrackH264{})
		require.ErrorIs(t, err, ErrSampleInvalid)
	})
}
