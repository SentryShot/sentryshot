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
	tplHooks     []web.Hook
	monitorHooks []monitor.Hook
)

// RegisterTplHook registers hook that's called on page render.
func RegisterTplHook(h web.Hook) {
	tplHooks = append(tplHooks, h)
}

// RegisterMonitorHook registers hook that's called when the main monitor process starts.
func RegisterMonitorHook(h monitor.Hook) {
	monitorHooks = append(monitorHooks, h)
}

func tplHook(pageFiles map[string]string) error {
	for _, hook := range tplHooks {
		if err := hook(pageFiles); err != nil {
			return err
		}
	}
	return nil
}

func monitorHook(m *monitor.Monitor, args *string) {
	for _, hook := range monitorHooks {
		hook(m, args)
	}
}
