package watchdog

import (
	"context"
	"sync"
	"testing"
	"time"

	"nvr/pkg/log"

	"github.com/stretchr/testify/require"
)

func newTestWatchdog(t *testing.T) (context.Context, watchdog, log.Feed, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	logger := log.NewMockLogger()
	logger.Start(ctx)

	feed, cancel2 := logger.Subscribe()

	cancel3 := func() {
		cancel2()
		cancel()
	}

	watchFunc := func() {
		time.Sleep(50 * time.Millisecond)
	}

	d := watchdog{
		processName: "x",
		interval:    10 * time.Millisecond,
		watchFunc:   watchFunc,
		onFreeze:    func() {},
		log:         logger,
	}

	return ctx, d, feed, cancel3
}

func TestWatchdog(t *testing.T) {
	t.Run("freeze", func(t *testing.T) {
		ctx, d, feed, cancel := newTestWatchdog(t)
		defer cancel()

		mu := sync.Mutex{}
		mu.Lock()
		d.onFreeze = func() {
			mu.Unlock()
		}

		go d.start(ctx)
		mu.Lock()
		mu.Unlock()

		actual := <-feed
		require.Equal(t, actual.Msg, "x process: possible freeze detected, restarting..")
	})
}
