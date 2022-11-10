package watchdog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"nvr/pkg/log"

	"github.com/stretchr/testify/require"
)

type stubMuxer struct{}

func (m *stubMuxer) WaitForSegFinalized() {}

func newStubMuxer(err error) func() (muxer, error) {
	return func() (muxer, error) {
		return &stubMuxer{}, err
	}
}

func newTestWatchdog(t *testing.T) (watchdog, chan string) {
	logs := make(chan string)
	logFunc := func(_ log.Level, format string, a ...interface{}) {
		logs <- fmt.Sprintf(format, a...)
	}

	d := watchdog{
		interval: 10 * time.Millisecond,
		muxer:    newStubMuxer(nil),
		onFreeze: func() {},
		logf:     logFunc,
	}

	return d, logs
}

func TestWatchdog(t *testing.T) {
	t.Run("freeze", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		d, logs := newTestWatchdog(t)

		done := make(chan struct{})
		d.onFreeze = func() {
			close(done)
		}

		go d.start(ctx)
		require.Equal(t, "possible freeze detected, restarting..", <-logs)
		<-done
	})
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		d, _ := newTestWatchdog(t)
		d.start(ctx)
	})
}
