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
	"context"
	"net/http"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"nvr/pkg/web"
	"nvr/pkg/web/auth"
)

type (
	envHook     func(*storage.ConfigEnv)
	logHook     func(*log.Logger)
	authHook    func(*auth.Authenticator)
	storageHook func(*storage.Manager)
	muxHook     func(*http.ServeMux)
	appRunHook  func(context.Context) error
)

type hookList struct {
	onEnv              []envHook
	onAuth             []authHook
	onStorage          []storageHook
	onLog              []logHook
	onMux              []muxHook
	onAppRun           []appRunHook
	template           []web.TemplateHook
	templateSub        []web.TemplateHook
	templateData       []web.TemplateDataFunc
	monitorStart       []monitor.StartHook
	monitorMainProcess []monitor.StartInputHook
	monitorSubProcess  []monitor.StartInputHook
	monitorRecSave     []monitor.RecSaveHook
	logSource          []string
}

var hooks = &hookList{}

// RegisterEnvHook registers hook that's called when environment config is loaded.
func RegisterEnvHook(h envHook) {
	hooks.onEnv = append(hooks.onEnv, h)
}

// RegisterLogHook is used to grab the logger.
func RegisterLogHook(h logHook) {
	hooks.onLog = append(hooks.onLog, h)
}

// RegisterAuthHook is used to grab the authenticator.
func RegisterAuthHook(h authHook) {
	hooks.onAuth = append(hooks.onAuth, h)
}

// RegisterStorageHook is used to grab the storage manager.
func RegisterStorageHook(h storageHook) {
	hooks.onStorage = append(hooks.onStorage, h)
}

// RegisterMuxHook registers hook used to modifiy routes.
func RegisterMuxHook(h muxHook) {
	hooks.onMux = append(hooks.onMux, h)
}

// RegisterAppRunHook registers hook that's called when app runs.
func RegisterAppRunHook(h appRunHook) {
	hooks.onAppRun = append(hooks.onAppRun, h)
}

// RegisterTplHook registers function used to modify templates.
func RegisterTplHook(h web.TemplateHook) {
	hooks.template = append(hooks.template, h)
}

// RegisterTplSubHook registers function used to modify sub templates.
func RegisterTplSubHook(h web.TemplateHook) {
	hooks.templateSub = append(hooks.templateSub, h)
}

// RegisterTplDataHook registers function thats called
// on page render. Used to modify template data.
func RegisterTplDataHook(h web.TemplateDataFunc) {
	hooks.templateData = append(hooks.templateData, h)
}

// RegisterMonitorStartHook registers hook that's called when the monitor starts.
func RegisterMonitorStartHook(h monitor.StartHook) {
	hooks.monitorStart = append(hooks.monitorStart, h)
}

// RegisterMonitorMainProcessHook registers hook that's called when the main monitor process starts.
func RegisterMonitorMainProcessHook(h monitor.StartInputHook) {
	hooks.monitorMainProcess = append(hooks.monitorMainProcess, h)
}

// RegisterMonitorSubProcessHook registers hook that's called when the sub monitor process starts.
func RegisterMonitorSubProcessHook(h monitor.StartInputHook) {
	hooks.monitorSubProcess = append(hooks.monitorSubProcess, h)
}

// RegisterMonitorRecSaveHook registers hook that's called monitor saves recording.
func RegisterMonitorRecSaveHook(h monitor.RecSaveHook) {
	hooks.monitorRecSave = append(hooks.monitorRecSave, h)
}

// RegisterLogSource adds log source.
func RegisterLogSource(s []string) {
	hooks.logSource = append(hooks.logSource, s...)
}

func (h *hookList) env(env *storage.ConfigEnv) {
	for _, hook := range h.onEnv {
		hook(env)
	}
}

func (h *hookList) log(log *log.Logger) {
	for _, hook := range h.onLog {
		hook(log)
	}
}

func (h *hookList) auth(a *auth.Authenticator) {
	for _, hook := range h.onAuth {
		hook(a)
	}
}

func (h *hookList) storage(s *storage.Manager) {
	for _, hook := range h.onStorage {
		hook(s)
	}
}

func (h *hookList) mux(mux *http.ServeMux) {
	for _, hook := range h.onMux {
		hook(mux)
	}
}

func (h *hookList) appRun(ctx context.Context) error {
	for _, hook := range h.onAppRun {
		if err := hook(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (h *hookList) tplHooks() web.TemplateHooks {
	tplHook := func(pageFiles map[string]string) error {
		for _, hook := range h.template {
			if err := hook(pageFiles); err != nil {
				return err
			}
		}
		return nil
	}
	tplSubHook := func(pageFiles map[string]string) error {
		for _, hook := range h.templateSub {
			if err := hook(pageFiles); err != nil {
				return err
			}
		}
		return nil
	}

	return web.TemplateHooks{
		Tpl: tplHook,
		Sub: tplSubHook,
	}
}

func (h *hookList) monitor() monitor.Hooks {
	startHook := func(ctx context.Context, m *monitor.Monitor) {
		for _, hook := range h.monitorStart {
			hook(ctx, m)
		}
	}
	startMainHook := func(ctx context.Context, m *monitor.Monitor, args *string) {
		for _, hook := range h.monitorMainProcess {
			hook(ctx, m, args)
		}
	}
	startSubHook := func(ctx context.Context, m *monitor.Monitor, args *string) {
		for _, hook := range h.monitorSubProcess {
			hook(ctx, m, args)
		}
	}
	recSaveHook := func(m *monitor.Monitor, args *string) {
		for _, hook := range h.monitorRecSave {
			hook(m, args)
		}
	}

	return monitor.Hooks{
		Start:     startHook,
		StartMain: startMainHook,
		StartSub:  startSubHook,
		RecSave:   recSaveHook,
	}
}
