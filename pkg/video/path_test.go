package video

import (
	"context"
	"sync"
	"testing"
	"time"

	"nvr/pkg/video/hls"

	"github.com/stretchr/testify/require"
)

func TestPathConfStart(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		wg := sync.WaitGroup{}

		pconf := PathConf{}
		pconf.start(ctx, &wg)

		sub, cancel2, err := pconf.subscibeToHlsSegmentFinalized()
		require.NoError(t, err)

		done := make(chan struct{})
		go func() {
			<-sub
			close(done)
			cancel2()
		}()

		time.Sleep(10 * time.Millisecond)
		pconf.onHlsSegmentFinalized([]hls.SegmentOrGap{})
		<-done

		cancel()
		wg.Wait()
	})

	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		wg := sync.WaitGroup{}
		pconf := PathConf{}
		pconf.start(ctx, &wg)

		_, _, err := pconf.subscibeToHlsSegmentFinalized()
		require.ErrorIs(t, err, context.Canceled)
		wg.Wait()
	})
}
