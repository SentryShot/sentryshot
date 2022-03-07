package watchdog

// Watchdog detects and restarts frozen processes.
// Freeze is detected by polling the output HLS manifest for file updates.

import (
	"context"
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
	// Function that must return before interval ends.
	watchFunc := func() {
		i.WaitForNewHLSsegment(ctx, 0) //nolint:errcheck
	}

	d := &watchdog{
		monitorID:   i.M.Config.ID(),
		processName: i.ProcessName(),
		watchFunc:   watchFunc,
		interval:    defaultInterval,
		onFreeze:    i.Cancel,

		log: i.M.Log,
	}
	go d.start(ctx)
}

type watchdog struct {
	monitorID   string
	processName string
	watchFunc   func()
	interval    time.Duration
	onFreeze    func()

	log *log.Logger
}

func (d *watchdog) start(ctx context.Context) {
	watch := func() {
		returned := make(chan struct{})
		go func() {
			d.watchFunc()
			close(returned)
		}()

		select {
		case <-time.After(d.interval):
			// Function didn't return in time.
			d.log.Error().
				Src("watchdog").
				Monitor(d.monitorID).
				Msgf("%v process: possible freeze detected, restarting..",
					d.processName)

			d.onFreeze()
		case <-returned:
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
