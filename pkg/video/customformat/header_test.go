package customformat

import (
	"testing"

	"nvr/pkg/video/gortsplib/pkg/h264"
	"nvr/pkg/video/hls"

	"github.com/stretchr/testify/require"
)

func TestHeaderToStreamInfo(t *testing.T) {
	videoSPS := []byte{103, 0, 0, 0, 172, 217, 0}
	audioConfig := []byte{20, 10, 0, 0}
	header := Header{
		VideoSPS:    videoSPS,
		VideoPPS:    []byte{1},
		AudioConfig: audioConfig,
	}

	actual, err := header.ToStreamInfo()
	require.NoError(t, err)

	expected := hls.StreamInfo{
		VideoTrackExist: true,
		VideoSPS:        videoSPS,
		VideoPPS:        []byte{1},
		VideoSPSP: h264.SPS{
			Log2MaxFrameNumMinus4:          1,
			MaxNumRefFrames:                5,
			GapsInFrameNumValueAllowedFlag: true,
			PicHeightInMbsMinus1:           3,
		},
		VideoWidth:        16,
		VideoHeight:       128,
		AudioTrackExist:   true,
		AudioTrackConfig:  audioConfig,
		AudioChannelCount: 1,
		AudioClockRate:    16000,
		AudioType:         2,
	}
	require.Equal(t, expected, *actual)
}
