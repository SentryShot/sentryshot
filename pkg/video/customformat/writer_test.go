package customformat

import (
	"bytes"
	"testing"

	"nvr/pkg/video/hls"

	"github.com/stretchr/testify/require"
)

func TestWriter(t *testing.T) {
	meta := &bytes.Buffer{}
	mdat := &bytes.Buffer{}

	testHeader := Header{
		VideoSPS:    []byte{0, 1},
		VideoPPS:    []byte{2, 3, 4},
		AudioConfig: []byte{5, 6, 7, 8},
		StartTime:   1000000000,
	}

	w, err := NewWriter(meta, mdat, testHeader)
	require.NoError(t, err)

	segment := &hls.Segment{
		Parts: []*hls.MuxerPart{{
			VideoSamples: []*hls.VideoSample{{
				PTS:        100000000000000000,
				DTS:        200000000000000000,
				NextDTS:    300000000000000000,
				IdrPresent: true,
				AVCC:       []byte{9},
			}},
			AudioSamples: []*hls.AudioSample{{
				PTS:     1,
				NextPTS: 2,
				AU:      []byte{7, 8},
			}},
		}},
	}
	err = w.WriteSegment(segment)
	require.NoError(t, err)

	metaExpected := []byte{
		0,    // Version.
		0, 2, // Video sps size.
		0, 1, // Video sps.
		0, 3, // Video pps size.
		2, 3, 4, // Video pps.
		0, 4, // Audio config size.
		5, 6, 7, 8, // Audio Config.
		0, 0, 0, 0, 0x3b, 0x9a, 0xca, 0, // Start time.

		// Audio sample.
		0x1,                    // Flags.
		0, 0, 0, 0, 0, 0, 0, 1, // PTS.
		0, 0, 0, 0, 0, 0, 0, 0, // Wasted space.
		0, 0, 0, 0, 0, 0, 0, 2, // Next pts.
		0, 0, 0, 0, // Offset.
		0, 0, 0, 2, // Size.

		// Video sample.
		0x2,                                     // Flags.
		0x1, 0x63, 0x45, 0x78, 0x5d, 0x8a, 0, 0, // PTS.
		0x2, 0xc6, 0x8a, 0xf0, 0xbb, 0x14, 0, 0, // DTS.
		0x4, 0x29, 0xd0, 0x69, 0x18, 0x9e, 0, 0, // Next dts.
		0, 0, 0, 2, // Offset.
		0, 0, 0, 1, // Size.
	}
	mdatExpected := []byte{7, 8, 9}

	require.Equal(t, metaExpected, meta.Bytes())
	require.Equal(t, mdatExpected, mdat.Bytes())

	r, header, err := NewReader(bytes.NewReader(metaExpected), len(metaExpected))
	require.NoError(t, err)
	require.Equal(t, testHeader, *header)

	expectedSamples := []Sample{
		{
			IsAudioSample: true,
			PTS:           1,
			Next:          2,
			Size:          2,
			Offset:        0,
		},
		{
			IsSyncSample: true,
			PTS:          100000000000000000,
			DTS:          200000000000000000,
			Next:         300000000000000000,
			Size:         1,
			Offset:       2,
		},
	}

	samples, err := r.ReadAllSamples()
	require.NoError(t, err)
	require.Equal(t, expectedSamples, samples)
}
