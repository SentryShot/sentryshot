package customformat

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReader(t *testing.T) {
	meta := bytes.NewReader([]byte{
		0,    // Version.
		0, 1, // Video sps size.
		2,    // Video sps.
		0, 1, // Video pps size.
		3,    // Video pps.
		0, 1, // Audio config size.
		4,                               // Audio Config.
		0, 0, 0, 0, 0x3b, 0x9a, 0xca, 0, // Start time.

		// Sample1.
		0x2,                                          // Flags.
		0x1, 0x63, 0x45, 0x78, 0x99, 0x24, 0xca, 0x0, // PTS.
		0x2, 0xc6, 0x8a, 0xf0, 0xf6, 0xae, 0xca, 0x0, // DTS.
		0x4, 0x29, 0xd0, 0x69, 0x54, 0x38, 0xca, 0x0, // Next dts.
		0, 0, 0, 0, // Offset.
		0, 0, 0, 0, // Size.

		// Sample2.
		0,                      // Flags.
		0, 0, 0, 0, 0, 0, 0, 2, // PTS.
		0, 0, 0, 0, 0, 0, 0, 0, // DTS.
		0, 0, 0, 0, 0, 0, 0, 0, // Next.
		0, 0, 0, 0, // Offset.
		0, 0, 0, 0, // Size.

		// Sample3.
		0,                      // Flags.
		0, 0, 0, 0, 0, 0, 0, 3, // PTS.
		0, 0, 0, 0, 0, 0, 0, 0, // DTS.
		0, 0, 0, 0, 0, 0, 0, 0, // Next.
		0, 0, 0, 0, // Offset.
		0, 0, 0, 0, // Size.
	})

	r, header, err := NewReader(meta, meta.Len())
	require.NoError(t, err)

	expectedHeader := Header{
		VideoSPS:    []byte{2},
		VideoPPS:    []byte{3},
		AudioConfig: []byte{4},
		StartTime:   1000000000,
	}
	require.Equal(t, expectedHeader, *header)

	meta.Seek(20, io.SeekStart)
	samples, err := r.ReadAllSamples()
	expectedSamples := []Sample{
		{
			PTS:          100000001000000000,
			DTS:          200000001000000000,
			Next:         300000001000000000,
			IsSyncSample: true,
		},
		{PTS: 2},
		{PTS: 3},
	}
	require.Equal(t, expectedSamples, samples)
}
