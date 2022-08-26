package video

import (
	"context"
	"sync"
	"testing"

	"nvr/pkg/log"

	"github.com/stretchr/testify/require"
)

type cancelFunc func()

func newTestServer(t *testing.T) (*Server, cancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}

	logger := log.NewMockLogger()
	if err := logger.Start(ctx); err != nil {
		require.NoError(t, err)
	}

	pathManager := newPathManager(&wg, logger)

	s := &Server{
		rtspAddress: "127.0.0.1:8554",
		hlsAddress:  "127.0.0.1:8888",
		pathManager: pathManager,
		wg:          &wg,
	}

	s.pathManager.start(ctx)

	cancelFunc := func() {
		cancel()
		wg.Wait()
	}

	return s, cancelFunc
}

func TestNewPath(t *testing.T) {
	p, cancel := newTestServer(t)
	defer cancel()

	c := PathConf{MonitorID: "x"}

	actual, cancel2, err := p.NewPath("mypath", c)
	require.NoError(t, err)
	actual.StreamInfo = nil
	actual.SubscribeToHlsSegmentFinalized = nil

	expected := ServerPath{
		HlsAddress:   "http://127.0.0.1:8888/hls/mypath/index.m3u8",
		RtspAddress:  "rtsp://127.0.0.1:8554/mypath",
		RtspProtocol: "tcp",
	}
	require.Equal(t, expected, *actual)

	require.True(t, p.PathExist("mypath"))
	cancel2()

	require.False(t, p.PathExist("mypath"))
}
