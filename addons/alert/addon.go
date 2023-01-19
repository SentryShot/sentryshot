// SPDX-License-Identifier: GPL-2.0-or-later

package alert

import (
	"encoding/json"
	"fmt"
	"nvr"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"strconv"
	"time"
)

// Hook Alert hook.
type Hook func(*monitor.Recorder, *storage.Event, []byte)

var addon struct {
	hooks []Hook
}

// RegisterAlertHook registers hook that's called on alerts.
func RegisterAlertHook(hook Hook) {
	addon.hooks = append(addon.hooks, hook)
}

func init() {
	RegisterAlertHook(logAlert)
	a := newAlerter(addon.hooks)

	nvr.RegisterLogSource([]string{"alert"})
	nvr.RegisterMonitorEventHook(a.onEvent)
}

func newAlerter(alertHooks []Hook) *alerter {
	return &alerter{
		alertHooks: alertHooks,
		prevAlerts: map[string]time.Time{},
	}
}

type alerter struct {
	alertHooks []Hook
	prevAlerts map[string]time.Time // map[monitorID]prevAlert.
}

func (a *alerter) onEvent(r *monitor.Recorder, event *storage.Event) {
	go func() {
		id := r.Config.ID()
		rawConfig := r.Config.Get("alert")

		err := a.processEvent(r, event, id, rawConfig)
		if err != nil {
			r.Logger.Log(log.Entry{
				Level:     log.LevelError,
				Src:       "alert",
				MonitorID: id,
				Msg:       err.Error(),
			})
		}
	}()
}

func (a *alerter) processEvent(
	r *monitor.Recorder,
	event *storage.Event,
	id string,
	rawConfig string,
) error {
	if rawConfig == "" {
		return nil
	}

	var config Config
	err := json.Unmarshal([]byte(rawConfig), &config)
	if err != nil {
		return fmt.Errorf("could not unmarshal config: %w", err)
	}
	config.fillMissing()

	if config.Enable != "true" {
		return nil
	}

	cooldownFloat, err := strconv.ParseFloat(config.Cooldown, 64)
	if err != nil {
		return fmt.Errorf("could not parse cooldown: %w", err)
	}

	cooldown := time.Duration(cooldownFloat * float64(time.Minute))
	if a.prevAlerts[id].Add(cooldown).After(time.Now()) {
		return nil
	}

	threshold, err := strconv.ParseFloat(config.Threshold, 64)
	if err != nil {
		return fmt.Errorf("could not parse threshold: %w", err)
	}

	d := bestDetection(*event)
	if d.Score < threshold {
		return nil
	}

	a.prevAlerts[id] = time.Now()

	for _, hook := range a.alertHooks {
		hook(r, event, nil)
	}

	return nil
}

// Config is a monitor alert config.
type Config struct {
	Enable    string `json:"enable"`
	Threshold string `json:"threshold"`
	Cooldown  string `json:"cooldown"`
}

func (c *Config) fillMissing() {
	if c.Enable == "" {
		c.Enable = "false"
	}
	if c.Threshold == "" {
		c.Threshold = "50"
	}
	if c.Cooldown == "" {
		c.Cooldown = "30"
	}
}

func bestDetection(e storage.Event) storage.Detection {
	var best storage.Detection
	for _, d := range e.Detections {
		if d.Score > best.Score {
			best = d
		}
	}
	return best
}

func logAlert(r *monitor.Recorder, event *storage.Event, _ []byte) {
	monitorID := r.Config.ID()
	d := bestDetection(*event)
	r.Logger.Log(log.Entry{
		Level:     log.LevelInfo,
		Src:       "alert",
		MonitorID: monitorID,
		Msg:       fmt.Sprintf("label:%v score:%v", d.Label, d.Score),
	})
}
