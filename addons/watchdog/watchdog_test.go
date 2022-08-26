package watchdog

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"nvr/pkg/log"
	"nvr/pkg/video"
	"nvr/pkg/video/hls"

	"github.com/stretchr/testify/require"
)

func newTestWatchdog(t *testing.T) (watchdog, chan string) {
	subFunc := func() (chan []*hls.Segment, video.CancelFunc, error) {
		return make(chan []*hls.Segment), func() {}, nil
	}

	logs := make(chan string)
	logFunc := func(_ log.Level, format string, a ...interface{}) {
		logs <- fmt.Sprintf(format, a...)
	}

	d := watchdog{
		interval: 10 * time.Millisecond,
		subFunc:  subFunc,
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
	t.Run("subErr", func(t *testing.T) {
		d, logs := newTestWatchdog(t)
		d.subFunc = func() (chan []*hls.Segment, video.CancelFunc, error) {
			return make(chan []*hls.Segment), func() {}, errors.New("mock")
		}
		go d.start(context.Background())
		require.Equal(t, "could not subscribe", <-logs)
	})
}
