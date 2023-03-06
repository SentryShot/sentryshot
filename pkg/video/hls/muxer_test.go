package hls

import (
	"context"
	"testing"
	"time"

	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib"

	"github.com/stretchr/testify/require"
)

// baseline profile without POC
var testSPS = []byte{
	0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
	0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
	0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
	0x20,
}

func TestMuxerFMP4ZeroDuration(t *testing.T) {
	videoTrack := &gortsplib.TrackH264{
		PayloadType:       96,
		SPS:               testSPS,
		PPS:               []byte{0x08},
		PacketizationMode: 1,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewMuxer(
		ctx,
		3,
		1*time.Second,
		0, 50*1024*1024,
		func(log.Level, string, ...interface{}) {},
		videoTrack,
		nil,
	)

	err := m.WriteH264(time.Now(), 0, [][]byte{
		testSPS, // SPS
		{8},     // PPS
		{5},     // IDR
	})
	require.NoError(t, err)

	err = m.WriteH264(time.Now(), 0, [][]byte{
		testSPS, // SPS
		{8},     // PPS
		{5},     // IDR
	})
	require.NoError(t, err)
}
