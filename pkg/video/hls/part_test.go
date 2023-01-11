package hls

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGeneratePart(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		actual, err := generatePart(
			0,
			false,
			func() int { return 0 },
			[]*VideoSample{{
				PTS:     0,
				DTS:     0,
				AVCC:    []byte{},
				NextDTS: 0,
			}},
			[]*AudioSample{},
		)
		require.NoError(t, err)
		expected := []byte{
			0, 0, 0, 0x68, 'm', 'o', 'o', 'f',
			0, 0, 0, 0x10, 'm', 'f', 'h', 'd',
			0, 0, 0, 0, // FullBox.
			0, 0, 0, 0, // Sequence number.
			0, 0, 0, 0x50, 't', 'r', 'a', 'f',
			0, 0, 0, 0x10, 't', 'f', 'h', 'd',
			0, 2, 0, 0, // Track id.
			0, 0, 0, 1, // Sample size.
			0, 0, 0, 0x14, 't', 'f', 'd', 't',
			1, 0, 0, 0, // Track id.
			0, 0, 0, 0, 0, 0, 0, 0, // BaseMediaDecodeTime.
			0, 0, 0, 0x24, 't', 'r', 'u', 'n', // Video trun.
			1, 0, 0xf, 1, // FullBox.
			0, 0, 0, 1, // Sample count.
			0, 0, 0, 0x70, // Data offset.
			0, 0, 0, 0, // Entry sample duration.
			0, 0, 0, 0, // Entry sample size.
			0, 1, 0, 0, // Entry sample flags.
			0, 0, 0, 0, // Entry SampleCompositionTimeOffset
			0, 0, 0, 8, 'm', 'd', 'a', 't',
		}
		require.Equal(t, expected, actual)
	})
	t.Run("videoSample", func(t *testing.T) {
		actual, err := generatePart(
			0,
			false,
			func() int { return 0 },
			[]*VideoSample{{
				PTS:     0,
				DTS:     0,
				AVCC:    []byte{'a', 'b', 'c', 'd'},
				NextDTS: 0,
			}},
			[]*AudioSample{},
		)
		require.NoError(t, err)
		expected := []byte{
			0, 0, 0, 0x68, 'm', 'o', 'o', 'f',
			0, 0, 0, 0x10, 'm', 'f', 'h', 'd',
			0, 0, 0, 0, // FullBox.
			0, 0, 0, 0, // Sequence number.
			0, 0, 0, 0x50, 't', 'r', 'a', 'f', // Video traf.
			0, 0, 0, 0x10, 't', 'f', 'h', 'd', // Video tfhd.
			0, 2, 0, 0, // Track id.
			0, 0, 0, 1, // Sample size.
			0, 0, 0, 0x14, 't', 'f', 'd', 't', // Video tfdt.
			1, 0, 0, 0, // Track id.
			0, 0, 0, 0, 0, 0, 0, 0, // BaseMediaDecodeTime.
			0, 0, 0, 0x24, 't', 'r', 'u', 'n', // Video trun.
			1, 0, 0xf, 1, // FullBox.
			0, 0, 0, 1, // Sample count.
			0, 0, 0, 0x70, // Data offset.
			0, 0, 0, 0, // Entry sample duration.
			0, 0, 0, 4, // Entry sample size.
			0, 1, 0, 0, // Entry sample flags.
			0, 0, 0, 0, // Entry SampleCompositionTimeOffset
			0, 0, 0, 0xc, 'm', 'd', 'a', 't',
			'a', 'b', 'c', 'd', // Video Sample
		}
		require.Equal(t, expected, actual)
	})
	t.Run("audioSample", func(t *testing.T) {
		actual, err := generatePart(
			0,
			true,
			func() int { return 0 },
			[]*VideoSample{{
				PTS:     0,
				DTS:     0,
				AVCC:    []byte{},
				NextDTS: 0,
			}},
			[]*AudioSample{{
				PTS:     0,
				NextPTS: 0,
				AU:      []byte{'a', 'b', 'c', 'd'},
			}},
		)
		require.NoError(t, err)
		expected := []byte{
			0, 0, 0, 0xb0, 'm', 'o', 'o', 'f',
			0, 0, 0, 0x10, 'm', 'f', 'h', 'd',
			0, 0, 0, 0, // FullBox.
			0, 0, 0, 0, // Sequence number.
			0, 0, 0, 0x50, 't', 'r', 'a', 'f', // Video traf.
			0, 0, 0, 0x10, 't', 'f', 'h', 'd', // Video tfhd.
			0, 2, 0, 0, // Track id.
			0, 0, 0, 1, // Sample size.
			0, 0, 0, 0x14, 't', 'f', 'd', 't', // Video tfdt.
			1, 0, 0, 0, // Track id.
			0, 0, 0, 0, 0, 0, 0, 0, // BaseMediaDecodeTime.
			0, 0, 0, 0x24, 't', 'r', 'u', 'n', // Video trun.
			1, 0, 0xf, 1, // Sample count.
			0, 0, 0, 1, // Data offset.
			0, 0, 0, 0xb8, // Entry sample duration.
			0, 0, 0, 0, // Entry sample size.
			0, 0, 0, 0, // Entry sample flags.
			0, 1, 0, 0, 0, 0, 0, 0, // Entry SampleCompositionTimeOffset
			0, 0, 0, 0x48, 't', 'r', 'a', 'f', // Audio traf.
			0, 0, 0, 0x10, 't', 'f', 'h', 'd', // Audio tfhd.
			0, 2, 0, 0, // Track id.
			0, 0, 0, 2, // Sample size.
			0, 0, 0, 0x14, 't', 'f', 'd', 't', // Audio tfdt.
			1, 0, 0, 0, // Track id.
			0, 0, 0, 0, 0, 0, 0, 0, // BaseMediaDecodeTime.
			0, 0, 0, 0x1c, 't', 'r', 'u', 'n', // Audio trun.
			0, 0, 3, 1, // FullBox.
			0, 0, 0, 1, // Sample count.
			0, 0, 0, 0xb8, // Data offset.
			0, 0, 0, 0, // Entry sample duration.
			0, 0, 0, 4, // Entry sample size.
			0, 0, 0, 0x0c, 'm', 'd', 'a', 't',
			'a', 'b', 'c', 'd', // Audio Sample
		}
		require.Equal(t, expected, actual)
	})
	t.Run("videoAndAudioSample", func(t *testing.T) {
		actual, err := generatePart(
			0,
			true,
			func() int { return 0 },
			[]*VideoSample{{
				PTS:     0,
				DTS:     0,
				AVCC:    []byte{'a', 'b', 'c', 'd'},
				NextDTS: 0,
			}},
			[]*AudioSample{{
				PTS:     0,
				NextPTS: 0,
				AU:      []byte{'e', 'f', 'g', 'h'},
			}},
		)
		require.NoError(t, err)
		expected := []byte{
			0, 0, 0, 0xb0, 'm', 'o', 'o', 'f',
			0, 0, 0, 0x10, 'm', 'f', 'h', 'd',
			0, 0, 0, 0, // FullBox.
			0, 0, 0, 0, // Sequence number.
			0, 0, 0, 0x50, 't', 'r', 'a', 'f', // Video traf.
			0, 0, 0, 0x10, 't', 'f', 'h', 'd', // Video tfhd.
			0, 2, 0, 0, // Track id.
			0, 0, 0, 1, // Sample size.
			0, 0, 0, 0x14, 't', 'f', 'd', 't', // Video tfdt.
			1, 0, 0, 0, // Track id.
			0, 0, 0, 0, 0, 0, 0, 0, // BaseMediaDecodeTime.
			0, 0, 0, 0x24, 't', 'r', 'u', 'n', // Video trun.
			1, 0, 0xf, 1, // FullBox.
			0, 0, 0, 1, // Sample count.
			0, 0, 0, 0xb8, // Data offset.
			0, 0, 0, 0, // Entry sample duration.
			0, 0, 0, 4, // Entry sample size.
			0, 1, 0, 0, // Entry sample flags.
			0, 0, 0, 0, // Entry SampleCompositionTimeOffset
			0, 0, 0, 0x48, 't', 'r', 'a', 'f', // Audio traf.
			0, 0, 0, 0x10, 't', 'f', 'h', 'd', // Audio tfhd.
			0, 2, 0, 0, // Track id.
			0, 0, 0, 2, // Sample size.
			0, 0, 0, 0x14, 't', 'f', 'd', 't', // Audio tfdt.
			1, 0, 0, 0, // Track id.
			0, 0, 0, 0, 0, 0, 0, 0, // BaseMediaDecodeTime.
			0, 0, 0, 0x1c, 't', 'r', 'u', 'n', // Audio trun.
			0, 0, 3, 1, // FullBox.
			0, 0, 0, 1, // Sample count.
			0, 0, 0, 0xbc, // Data offset.
			0, 0, 0, 0, // Entry sample duration.
			0, 0, 0, 4, // Entry sample size.
			0, 0, 0, 0x10, 'm', 'd', 'a', 't',
			'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', // Samples
		}
		require.Equal(t, expected, actual)
	})
	t.Run("multipleVideoSample", func(t *testing.T) {
		actual, err := generatePart(
			0,
			true,
			func() int { return 0 },
			[]*VideoSample{
				{
					PTS:        0,
					DTS:        0,
					AVCC:       []byte{'a', 'b', 'c', 'd'},
					IdrPresent: true,
					NextDTS:    0,
				},
				{
					PTS:     0,
					DTS:     0,
					AVCC:    []byte{'e', 'f', 'g', 'h'},
					NextDTS: 0,
				},
				{
					PTS:     0,
					DTS:     0,
					AVCC:    []byte{'i', 'j', 'k', 'l'},
					NextDTS: 0,
				},
			},
			[]*AudioSample{},
		)
		require.NoError(t, err)
		expected := []byte{
			0, 0, 0, 0x88, 'm', 'o', 'o', 'f',
			0, 0, 0, 0x10, 'm', 'f', 'h', 'd',
			0, 0, 0, 0, // FullBox.
			0, 0, 0, 0, // Sequence number.
			0, 0, 0, 0x70, 't', 'r', 'a', 'f', // Video traf.
			0, 0, 0, 0x10, 't', 'f', 'h', 'd', // Video tfhd.
			0, 2, 0, 0, // Track id.
			0, 0, 0, 1, // Sample size.
			0, 0, 0, 0x14, 't', 'f', 'd', 't', // Video tfdt.
			1, 0, 0, 0, // Track id.
			0, 0, 0, 0, 0, 0, 0, 0, // BaseMediaDecodeTime.
			0, 0, 0, 0x44, 't', 'r', 'u', 'n', // Video trun.
			1, 0, 0xf, 1, // FullBox.
			0, 0, 0, 3, // Sample count.
			0, 0, 0, 0x90, // Data offset.
			0, 0, 0, 0, // Entry1 sample duration.
			0, 0, 0, 4, // Entry1 sample size.
			0, 0, 0, 0, // Entry1 sample flags.
			0, 0, 0, 0, // Entry1 SampleCompositionTimeOffset
			0, 0, 0, 0, // Entry2 sample duration.
			0, 0, 0, 4, // Entry2 sample size.
			0, 1, 0, 0, // Entry2 sample flags.
			0, 0, 0, 0, // Entry2 SampleCompositionTimeOffset
			0, 0, 0, 0, // Entry3 sample duration.
			0, 0, 0, 4, // Entry3 sample size.
			0, 1, 0, 0, // Entry3 sample flags.
			0, 0, 0, 0, // Entry3 SampleCompositionTimeOffset
			0, 0, 0, 0x14, 'm', 'd', 'a', 't',
			'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', // Video Samples
		}
		require.Equal(t, expected, actual)
	})
	t.Run("real", func(t *testing.T) {
		muxerStartTime := int64(time.Hour)
		videoSample2 := &VideoSample{
			PTS:     muxerStartTime + 700000000,
			DTS:     muxerStartTime + 800000000,
			AVCC:    []byte{'e', 'f', 'g', 'h'},
			NextDTS: muxerStartTime + 900000000,
		}
		videoSample1 := &VideoSample{
			PTS:        muxerStartTime + 600000000,
			DTS:        muxerStartTime + 666666667,
			AVCC:       []byte{'a', 'b', 'c', 'd'},
			IdrPresent: true,
			NextDTS:    muxerStartTime + 800000000,
		}

		actual, err := generatePart(
			muxerStartTime,
			true,
			func() int { return 44100 },
			[]*VideoSample{
				videoSample1,
				videoSample2,
			},
			[]*AudioSample{{
				AU:      []byte{'i', 'j', 'k', 'l'},
				PTS:     muxerStartTime + 700000000,
				NextPTS: muxerStartTime + 800000000,
			}},
		)
		require.NoError(t, err)
		expected := []byte{
			0, 0, 0, 0xc0, 'm', 'o', 'o', 'f',
			0, 0, 0, 0x10, 'm', 'f', 'h', 'd',
			0, 0, 0, 0, // FullBox.
			0, 0, 0, 0, // Sequence number.
			0, 0, 0, 0x60, 't', 'r', 'a', 'f', // Video traf.
			0, 0, 0, 0x10, 't', 'f', 'h', 'd', // Video tfhd.
			0, 2, 0, 0, // Track id.
			0, 0, 0, 1, // Sample size.
			0, 0, 0, 0x14, 't', 'f', 'd', 't', // Video tfdt.
			1, 0, 0, 0, // Track id.
			0, 0, 0, 0, 0, 0, 0xea, 0x60, // BaseMediaDecodeTime.
			0, 0, 0, 0x34, 't', 'r', 'u', 'n', // Video trun.
			1, 0, 0xf, 1, // FullBox.
			0, 0, 0, 2, // Sample count.
			0, 0, 0, 0xc8, // Data offset.
			0, 0, 0x2e, 0xdf, // Entry1 sample duration.
			0, 0, 0, 4, // Entry1 sample size.
			0, 0, 0, 0, // Entry1 sample flags.
			0xff, 0xff, 0xe8, 0x90, // 1 Entry SampleCompositionTimeOffset
			0, 0, 0x23, 0x28, // 2 Entry sample duration.
			0, 0, 0, 4, // 2 Entry sample size.
			0, 1, 0, 0, // 2 Entry sample flags.
			0xff, 0xff, 0xdc, 0xd8, // Entry2 SampleCompositionTimeOffset
			0, 0, 0, 0x48, 't', 'r', 'a', 'f', // Audio traf.
			0, 0, 0, 0x10, 't', 'f', 'h', 'd', // Audio tfhd.
			0, 2, 0, 0, // Track id.
			0, 0, 0, 2, // Sample size.
			0, 0, 0, 0x14, 't', 'f', 'd', 't', // Audio tfdt.
			1, 0, 0, 0, // Track id.
			0, 0, 0, 0, 0, 0, 0x78, 0x96, // BaseMediaDecodeTime.
			0, 0, 0, 0x1c, 't', 'r', 'u', 'n', // Audio trun.
			0, 0, 3, 1, // FullBox.
			0, 0, 0, 1, // Sample count.
			0, 0, 0, 0xd0, // Data offset.
			0, 0, 0x11, 0x3a, // Entry sample duration.
			0, 0, 0, 4, // Entry sample size.
			0, 0, 0, 0x14, 'm', 'd', 'a', 't',
			'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', // Samples
		}
		require.Equal(t, expected, actual)
	})
}

func TestDurationGoToMp4(t *testing.T) {
	cases := []struct {
		input    int64
		scale    int64
		expected int64
	}{
		{
			input:    100000,
			scale:    VideoTimescale,
			expected: 9,
		},
		{
			input:    100000000,
			scale:    VideoTimescale,
			expected: 9000,
		},
		{
			input:    100000000000,
			scale:    VideoTimescale,
			expected: 9000000,
		},
		{
			input:    100000000000000, // 3 days.
			scale:    VideoTimescale,
			expected: 9000000000,
		},
		{
			input:    1000000000000000, // 30 days.
			scale:    VideoTimescale,
			expected: 90000000000,
		},
		{
			input:    10000000000000000, // 300 days.
			scale:    VideoTimescale,
			expected: 900000000000,
		},
		{
			input:    100000000000000000, // 3000 days.
			scale:    VideoTimescale,
			expected: 9000000000000,
		},
		{
			input:    1000000000000000000, // 30000 days.
			scale:    VideoTimescale,
			expected: 90000000000000,
		},
	}
	for i, tc := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			require.Equal(t, tc.expected, NanoToTimescale(tc.input, tc.scale))
		})
	}
}
