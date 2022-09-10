package watchdog

// Watchdog detects and restarts frozen processes.
// Freeze is detected by polling the output HLS manifest for file updates.

import (
	"context"
	"fmt"
	"nvr"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"time"
)

func init() {
	nvr.RegisterMonitorInputProcessHook(onInputProcessStart)
	nvr.RegisterLogSource([]string{"watchdog"})
}

const defaultInterval = 15 * time.Second

func onInputProcessStart(ctx context.Context, i *monitor.InputProcess, _ *[]string) {
	monitorID := i.Config.ID()
	processName := i.ProcessName()

	logf := func(level log.Level, format string, a ...interface{}) {
		format = fmt.Sprintf("%v process: %s", processName, format)
		i.Log.Level(level).Src("watchdog").Monitor(monitorID).Msgf(format, a...)
	}

	muxer := func() (muxer, error) {
		return i.HLSMuxer()
	}

	d := &watchdog{
		muxer:    muxer,
		interval: defaultInterval,
		onFreeze: i.Cancel,
		logf:     logf,
	}
	go d.start(ctx)
}

type muxer interface {
	WaitForSegFinalized()
}

type watchdog struct {
	muxer    func() (muxer, error)
	interval time.Duration
	onFreeze func()
	logf     func(log.Level, string, ...interface{})
}

func (d *watchdog) start(ctx context.Context) {
	// Warmup.
	select {
	case <-time.After(d.interval):
	case <-ctx.Done():
		return
	}

	keepAlive := make(chan struct{})
	go func() {
		for {
			select {
			case <-time.After(1 * time.Second):
			case <-ctx.Done():
				return
			}

			muxer, err := d.muxer()
			if err != nil {
				d.logf(log.LevelError, "could not get muxer")
				continue
			}

			muxer.WaitForSegFinalized()
			select {
			case <-ctx.Done():
				return
			case keepAlive <- struct{}{}:
			}
		}
	}()

	for {
		select {
		case <-time.After(d.interval):
			d.logf(log.LevelError, "possible freeze detected, restarting..")
			d.onFreeze()
		case <-keepAlive:
		case <-ctx.Done():
			return
		}
	}
}
