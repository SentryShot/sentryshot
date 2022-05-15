// Copyright 2020-2022 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; either version 2 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package alert

import (
	"encoding/json"
	"fmt"
	"nvr"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"strconv"
	"time"
)

// Hook Alert hook.
type Hook func(*monitor.Monitor, *storage.Event, []byte)

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

func (a *alerter) onEvent(m *monitor.Monitor, event *storage.Event) {
	go func() {
		m.Mu.Lock()
		id := m.Config.ID()
		rawConfig := m.Config["alert"]
		m.Mu.Unlock()

		err := a.processEvent(m, event, id, rawConfig)
		if err != nil {
			m.Log.Error().Src("alert").Monitor(id).Msgf("%v", err)
		}
	}()
}

func (a *alerter) processEvent(
	m *monitor.Monitor,
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
		hook(m, event, nil)
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

func logAlert(m *monitor.Monitor, event *storage.Event, _ []byte) {
	m.Mu.Lock()
	id := m.Config.ID()
	m.Mu.Unlock()

	d := bestDetection(*event)

	m.Log.Info().
		Src("alert").
		Monitor(id).
		Msgf("label:%v score:%v", d.Label, d.Score)
}
