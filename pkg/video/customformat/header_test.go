package customformat

import (
	"testing"

	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/mpeg4audio"

	"github.com/stretchr/testify/require"
)

func TestHeaderGetTracks(t *testing.T) {
	videoSPS := []byte{103, 0, 0, 0, 172, 217, 0}
	videoPPS := []byte{1}
	header := Header{
		VideoSPS:    videoSPS,
		VideoPPS:    videoPPS,
		AudioConfig: []byte{20, 10, 0, 0},
	}

	videoTrack, audioTrack, err := header.GetTracks()
	require.NoError(t, err)

	expectedVideoTrack := &gortsplib.TrackH264{SPS: videoSPS, PPS: videoPPS}
	require.Equal(t, expectedVideoTrack, videoTrack)

	expectedAudioTrack := &gortsplib.TrackMPEG4Audio{
		Config: &mpeg4audio.Config{
			Type:               2,
			SampleRate:         16000,
			ChannelCount:       1,
			DependsOnCoreCoder: true,
		},
	}
	require.Equal(t, expectedAudioTrack, audioTrack)
}
