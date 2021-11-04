package watchdog

// Watchdog detects and restarts frozen processes.
// Freeze is detected by polling the output HLS manifest for file updates.

import (
	"context"
	"errors"
	"fmt"
	"nvr"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"time"

	"github.com/fsnotify/fsnotify"
)

func init() {
	nvr.RegisterMonitorInputProcessHook(onInputProcessStart)
}

const defaultInterval = 10 * time.Second

func onInputProcessStart(ctx context.Context, i *monitor.InputProcess, _ *[]string) {
	d := &watchdog{
		monitorID:   i.M.Config.ID(),
		processName: i.ProcessName(),
		hlsPath:     i.HlsPath(),
		interval:    defaultInterval,
		onFreeze:    i.Cancel,

		log: i.M.Log,
	}
	go d.start(ctx)
}

type watchdog struct {
	monitorID   string
	processName string
	hlsPath     string
	interval    time.Duration
	onFreeze    func()

	log *log.Logger
}

// ErrFreeze possible freeze detected.
var ErrFreeze = errors.New("possible freeze detected")

func (d *watchdog) start(ctx context.Context) {
	watchFile := func() error {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return err
		}
		defer watcher.Close()

		err = watcher.Add(d.hlsPath)
		if err != nil {
			return err
		}
		for {
			select {
			case <-watcher.Events: // file updated, process not frozen.
				return nil
			case <-time.After(d.interval):
				return fmt.Errorf("%w, restarting", ErrFreeze)
			case err := <-watcher.Errors:
				return err
			case <-ctx.Done():
				return nil
			}
		}
	}
	for {
		select {
		case <-time.After(d.interval):
		case <-ctx.Done():
			return
		}
		go func() {
			if err := watchFile(); err != nil {
				d.log.Error().
					Src("watchdog").
					Monitor(d.monitorID).
					Msgf("%v process: %v", d.processName, err)

				d.onFreeze()
			}
		}()
	}
}
