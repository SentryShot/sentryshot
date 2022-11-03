package motion

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateFFmpegArgs(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		c := config{
			logLevel: "2",
			feedRate: "5",
			scale:    6,
		}
		actual := generateFFmpegArgs(c, "3", "4")

		expected := []string{
			"-y", "-threads", "1", "-loglevel", "2",
			"-rtsp_transport", "3", "-i", "4",
			"-vf", "fps=fps=5,scale=iw/6:ih/6",
			"-f", "rawvideo", "-pix_fmt", "gray", "-",
		}
		require.Equal(t, expected, actual)
	})

	t.Run("maximal", func(t *testing.T) {
		c := config{
			logLevel: "2",
			hwaccel:  "3",
			feedRate: "6",
			scale:    7,
		}
		actual := generateFFmpegArgs(c, "4", "5")

		expected := []string{
			"-y", "-threads", "1", "-loglevel", "2", "-hwaccel", "3",
			"-rtsp_transport", "4", "-i", "5",
			"-vf", "fps=fps=6,scale=iw/7:ih/7",
			"-f", "rawvideo", "-pix_fmt", "gray", "-",
		}
		require.Equal(t, expected, actual)
	})
}
