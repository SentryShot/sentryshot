package mp4muxer

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"nvr/pkg/video/gortsplib/pkg/h264"
	"nvr/pkg/video/hls"

	"github.com/stretchr/testify/require"
)

func TestWriteVideo(t *testing.T) {
	startTime := 1 * int64(time.Hour)
	videoSample3 := &hls.VideoSample{
		PTS:        startTime + 70000,
		DTS:        startTime + 80000,
		AVCC:       []byte{0x4, 0x5},
		IdrPresent: true,
		NextDTS:    startTime,
	}
	videoSample2 := &hls.VideoSample{
		PTS:     startTime + 50000,
		DTS:     startTime + 60000,
		AVCC:    []byte{0x2, 0x3},
		NextDTS: videoSample3.DTS,
	}
	videoSample1 := &hls.VideoSample{
		PTS:        startTime + 30000,
		DTS:        startTime + 40000,
		AVCC:       []byte{0x0, 0x1},
		IdrPresent: true,
		NextDTS:    videoSample2.DTS,
	}

	audioSample2 := &hls.AudioSample{
		AU:      []byte{0x8, 0x9},
		PTS:     startTime + 20000,
		NextPTS: startTime,
	}
	audioSample1 := &hls.AudioSample{
		AU:      []byte{0x6, 0x7},
		PTS:     startTime + 10000,
		NextPTS: audioSample2.PTS,
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

	buf := &mockFile{}
	info := hls.StreamInfo{
		VideoTrackExist: true,
		VideoSPS:        sps,
		VideoSPSP:       spsp,
		VideoWidth:      640,
		VideoHeight:     480,
		AudioTrackExist: true,
	}

	firstSegment := &hls.Segment{
		StartTime:        time.Unix(0, startTime),
		RenderedDuration: 1 * time.Hour,

		ID: 1,
		Parts: []*hls.MuxerPart{
			{
				VideoSamples: []*hls.VideoSample{videoSample1},
				AudioSamples: []*hls.AudioSample{audioSample1},
			},
		},
	}
	secondSegment := &hls.Segment{
		StartTime:        time.Unix(0, int64(2*time.Hour)),
		RenderedDuration: 1 * time.Hour,

		ID: 2,
		Parts: []*hls.MuxerPart{
			{
				VideoSamples: []*hls.VideoSample{videoSample2},
				AudioSamples: []*hls.AudioSample{},
			},
		},
	}
	thirdSegment := &hls.Segment{
		StartTime:        time.Unix(0, int64(3*time.Hour)),
		RenderedDuration: 1 * time.Hour,

		ID: 3,
		Parts: []*hls.MuxerPart{
			{
				VideoSamples: []*hls.VideoSample{videoSample3},
				AudioSamples: []*hls.AudioSample{audioSample2},
			},
		},
	}

	nextSegment := func(prevID uint64) (*hls.Segment, error) {
		switch prevID {
		case 1:
			return secondSegment, nil
		case 2:
			return thirdSegment, nil
		}
		return nil, context.Canceled
	}

	prevSeg, endTime, err := WriteVideo(
		context.Background(), buf, nextSegment, firstSegment, info, 1*time.Hour)
	require.NoError(t, err)
	require.Equal(t, uint64(3), prevSeg)
	require.Equal(t, time.Unix(14400, 0), *endTime)

	expected := []byte{
		0, 0, 0, 0x14, 'f', 't', 'y', 'p',
		'i', 's', 'o', '4',
		0, 0, 2, 0, // Minor version.
		'i', 's', 'o', '4',
		0, 0, 0, 0x12, 'm', 'd', 'a', 't',
		0x0, 0x1, // Video sample 1.
		0x6, 0x7, // Audio sample 1.
		0x2, 0x3, // Video sample 2.
		0x4, 0x5, // Video sample 3.
		0x8, 0x9, // Audio sample 2.
		0, 0, 4, 0xab, 'm', 'o', 'o', 'v',
		0, 0, 0, 0x6c, 'm', 'v', 'h', 'd',
		0, 0, 0, 0, // Fullbox.
		0, 0, 0, 0, // Creation time.
		0, 0, 0, 0, // Modification time.
		0, 0, 3, 0xe8, // Timescale.
		0, 0xa4, 0xcb, 0x80, // Duration.
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
		0, 0, 2, 0x5d, 't', 'r', 'a', 'k',
		0, 0, 0, 0x5c, 't', 'k', 'h', 'd',
		0, 0, 0, 3, // Fullbox.
		0, 0, 0, 0, // Creation time.
		0, 0, 0, 0, // Modification time.
		0, 0, 0, 0, // Track ID.
		0, 0, 0, 0, // Reserved0.
		0, 0xa4, 0xcb, 0x80, // Duration.
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
		0, 0, 1, 0xf9, 'm', 'd', 'i', 'a',
		0, 0, 0, 0x20, 'm', 'd', 'h', 'd',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Creation time.
		0, 0, 0, 0, // Modification time.
		0, 1, 0x5f, 0x90, // Time scale.
		0x39, 0xef, 0x8b, 0, // Duration.
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
		0, 0, 1, 0xa4, 'm', 'i', 'n', 'f',
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
		0, 0, 1, 0x64, 's', 't', 'b', 'l',
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
		0, 0, 0, 0x28, 's', 't', 't', 's',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 3, // Entry count.
		0, 0, 0, 1, // Entry1 sample count.
		0, 0, 0, 1, // Entry1 sample delta.
		0, 0, 0, 1, // Entry2 sample count.
		0, 0, 0, 2, // Entry2 sample delta.
		0, 0, 0, 1, // Entry3 sample count.
		0xff, 0xff, 0xff, 0xf9, // Entry3 sample delta.
		0, 0, 0, 0x18, 's', 't', 's', 's',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 2, // Entry count.
		0, 0, 0, 1, // Entry1.
		0, 0, 0, 3, // Entry2.
		0, 0, 0, 0x28, 'c', 't', 't', 's',
		1, 0, 0, 0, // FullBox.
		0, 0, 0, 3, // Entry count.
		0, 0, 0, 1, // Entry1 sample count.
		0, 0, 0, 0, // Entry1 sample offset
		0, 0, 0, 1, // Entry2 sample count.
		0, 0, 0, 1, // Entry2 sample offset
		0, 0, 0, 1, // Entry3 sample count.
		0, 0, 0, 0, // Entry3 sample offset
		0, 0, 0, 0x28, 's', 't', 's', 'c',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 2, // Entry count.
		0, 0, 0, 1, // Entry1 first chunk.
		0, 0, 0, 1, // Entry1 samples per chunk.
		0, 0, 0, 1, // Entry1 sample description index.
		0, 0, 0, 2, // Entry2 first chunk.
		0, 0, 0, 2, // Entry2 samples per chunk.
		0, 0, 0, 1, // Entry2 sample description index.
		0, 0, 0, 0x20, 's', 't', 's', 'z',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Sample size.
		0, 0, 0, 3, // Sample count.
		0, 0, 0, 2, // Entry1 size.
		0, 0, 0, 2, // Entry2 size.
		0, 0, 0, 2, // Entry3 size.
		0, 0, 0, 0x18, 's', 't', 'c', 'o',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 2, // Entry count.
		0, 0, 0, 0x1c, // Chunk offset1.
		0, 0, 0, 0x20, // Chunk offset2.

		/* Audio trak */
		0, 0, 1, 0xda, 't', 'r', 'a', 'k',
		0, 0, 0, 0x5c, 't', 'k', 'h', 'd',
		0, 0, 0, 3, // FullBox.
		0, 0, 0, 0, // Creation time.
		0, 0, 0, 0, // Modification time.
		0, 0, 0, 1, // Track ID.
		0, 0, 0, 0, // Reserved.
		0, 0xa4, 0xcb, 0x80, // Duration.
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
		0, 0, 1, 0x76, 'm', 'd', 'i', 'a',
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
		0, 0, 1, 0x21, 'm', 'i', 'n', 'f',
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
		0, 0, 0, 0xe1, 's', 't', 'b', 'l',
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
		0, 0, 0, 0, // Sample rate.
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
		0, 0, 0, 0, // Entry1 sample delta.
		0, 0, 0, 0x28, 's', 't', 's', 'c',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 2, // Entry count.
		0, 0, 0, 1, // Entry1 first chunk.
		0, 0, 0, 1, // Entry1 samples per chunk.
		0, 0, 0, 1, // Entry1 sample description index.
		0, 0, 0, 2, // Entry2 first chunk.
		0, 0, 0, 1, // Entry2 samples per chunk.
		0, 0, 0, 1, // Entry2 sample description index.
		0, 0, 0, 0x1c, 's', 't', 's', 'z',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 0, // Sample size.
		0, 0, 0, 2, // Sample count.
		0, 0, 0, 2, // Entry1 size.
		0, 0, 0, 2, // Entry2 size.
		0, 0, 0, 0x18, 's', 't', 'c', 'o',
		0, 0, 0, 0, // FullBox.
		0, 0, 0, 2, // Entry count.
		0, 0, 0, 0x1e, // Chunk offset1.
		0, 0, 0, 0x24, // Chunk offset2.
	}
	require.Equal(t, expected, buf.buf)
}

type mockFile struct {
	buf []byte
	pos int
}

func (m *mockFile) Write(p []byte) (n int, err error) {
	minCap := m.pos + len(p)
	if minCap > cap(m.buf) { // Make sure buf has enough capacity:
		buf2 := make([]byte, len(m.buf), minCap+len(p)) // add some extra
		copy(buf2, m.buf)
		m.buf = buf2
	}
	if minCap > len(m.buf) {
		m.buf = m.buf[:minCap]
	}
	copy(m.buf[m.pos:], p)
	m.pos += len(p)
	return len(p), nil
}

func (m *mockFile) Seek(offset int64, whence int) (int64, error) {
	newPos, offs := 0, int(offset)
	switch whence {
	case io.SeekStart:
		newPos = offs
	case io.SeekCurrent:
		newPos = m.pos + offs
	case io.SeekEnd:
		newPos = len(m.buf) + offs
	}
	if newPos < 0 {
		return 0, errors.New("negative result pos")
	}
	m.pos = newPos
	return int64(newPos), nil
}
