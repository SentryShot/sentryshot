package video

import (
	"context"
	"sync"
	"testing"
	"time"

	"nvr/pkg/video/hls"

	"github.com/stretchr/testify/require"
)

func TestPathConf(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pconf := PathConf{}
		pconf.start(ctx)

		wg := sync.WaitGroup{}

		wg.Add(2)
		go func() {
			pconf.WaitForNewHLSsegment(ctx, 0)
			wg.Done()
		}()
		go func() {
			pconf.WaitForNewHLSsegment(ctx, 0)
			wg.Done()
		}()

		time.Sleep(10 * time.Millisecond)
		pconf.onNewHLSsegment <- []hls.SegmentOrGap{}
		wg.Wait()
	})

	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		pconf := PathConf{}
		pconf.start(ctx)

		_, err := pconf.WaitForNewHLSsegment(ctx, 0)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("canceled2", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		pconf := PathConf{}
		pconf.start(ctx)

		wg := sync.WaitGroup{}

		wg.Add(1)
		go func() {
			pconf.WaitForNewHLSsegment(ctx, 0)
			wg.Done()
		}()

		time.Sleep(10 * time.Millisecond)
		cancel()

		wg.Wait()
	})
}
