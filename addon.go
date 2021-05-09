// Copyright 2020-2021 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; version 2.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package nvr

import (
	"nvr/pkg/monitor"
	"nvr/pkg/web"
)

var (
	tplHooks              []web.Hook
	monitorStartHooks     []monitor.StartHook
	monitorStartMainHooks []monitor.StartMainHook
)

// RegisterTplHook registers hook that's called on page render.
func RegisterTplHook(h web.Hook) {
	tplHooks = append(tplHooks, h)
}

// RegisterMonitorHook registers hook that's called when the monitor starts.
func RegisterMonitorStartHook(h monitor.StartHook) {
	monitorStartHooks = append(monitorStartHooks, h)
}

// RegisterMonitorHook registers hook that's called when the main monitor process starts.
func RegisterMonitorStartProcessHook(h monitor.StartMainHook) {
	monitorStartMainHooks = append(monitorStartMainHooks, h)
}

func tplHook(pageFiles map[string]string) error {
	for _, hook := range tplHooks {
		if err := hook(pageFiles); err != nil {
			return err
		}
	}
	return nil
}

func monitorHooks() monitor.Hooks {
	startHook := func(m *monitor.Monitor) {
		for _, hook := range monitorStartHooks {
			hook(m)
		}
	}
	startMainHook := func(m *monitor.Monitor, args *string) {
		for _, hook := range monitorStartMainHooks {
			hook(m, args)
		}
	}

	return monitor.Hooks{
		Start:     startHook,
		StartMain: startMainHook,
	}
}
