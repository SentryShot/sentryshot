package watchdog

// Watchdog detects and restarts frozen processes.
// Freeze is detected by polling the output HLS manifest for file updates.

import (
	"context"
	"fmt"
	"nvr"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/video"
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

	d := &watchdog{
		subFunc:  i.SubsribeToHlsSegmentFinalized,
		interval: defaultInterval,
		onFreeze: i.Cancel,
		logf:     logf,
	}
	go d.start(ctx)
}

type watchdog struct {
	subFunc  video.SubscibeToHlsSegmentFinalizedFunc
	interval time.Duration
	onFreeze func()
	logf     func(log.Level, string, ...interface{})
}

func (d *watchdog) start(ctx context.Context) {
	sub, cancel, err := d.subFunc()
	if err != nil {
		d.onFreeze()
		d.logf(log.LevelError, "could not subscribe")
		return
	}
	defer cancel()

	watch := func() {
		select {
		case <-time.After(d.interval):
			// Function didn't return in time.
			d.logf(log.LevelError, "possible freeze detected, restarting..")
			d.onFreeze()
		case <-sub:
		case <-ctx.Done():
		}
	}

	for {
		select {
		case <-time.After(d.interval):
			go watch()
		case <-ctx.Done():
			return
		}
	}
}
