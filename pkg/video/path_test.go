package video

import (
	"context"
	"nvr/pkg/video/hls"
	"sync"
	"testing"
	"time"

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
		pconf.onNewHLSsegment <- hls.Segments{}
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
}
