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

package nvr

import (
	"context"
	stdLog "log"
	"net/http"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"nvr/pkg/web"
	"nvr/pkg/web/auth"
)

type (
	envHook       func(storage.ConfigEnv)
	logHook       func(*log.Logger)
	authHook      func(auth.Authenticator)
	templaterHook func(*web.Templater)
	storageHook   func(*storage.Manager)
	muxHook       func(*http.ServeMux)
	appRunHook    func(context.Context) error
)

type hookList struct {
	newAuthenticator    auth.NewAuthenticatorFunc
	onAuth              []authHook
	onEnv               []envHook
	onLog               []logHook
	onTemplater         []templaterHook
	onStorage           []storageHook
	onMux               []muxHook
	onAppRun            []appRunHook
	template            []web.TemplateHook
	templateSub         []web.TemplateHook
	templateData        []web.TemplateDataFunc
	monitorStart        []monitor.StartHook
	monitorInputProcess []monitor.StartInputHook
	monitorEvent        []monitor.EventHook
	monitorRecSave      []monitor.RecSaveHook
	monitorRecSaved     []monitor.RecSavedHook
	migrationMonitor    []monitor.MigationHook
	logSource           []string
}

var hooks = &hookList{}

// SetAuthenticator is used to set the authenticator.
func SetAuthenticator(a auth.NewAuthenticatorFunc) {
	if hooks.newAuthenticator != nil {
		stdLog.Fatalf("\n\nERROR: Only a single autentication addon is allowed.\n\n")
	}
	hooks.newAuthenticator = a
}

// RegisterAuthHook is used to grab the authenticator.
func RegisterAuthHook(h authHook) {
	hooks.onAuth = append(hooks.onAuth, h)
}

// RegisterEnvHook registers hook that's called when environment config is loaded.
func RegisterEnvHook(h envHook) {
	hooks.onEnv = append(hooks.onEnv, h)
}

// RegisterLogHook is used to grab the logger.
func RegisterLogHook(h logHook) {
	hooks.onLog = append(hooks.onLog, h)
}

// RegisterTemplaterHook is used to grab the templater.
func RegisterTemplaterHook(h templaterHook) {
	hooks.onTemplater = append(hooks.onTemplater, h)
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

// RegisterMonitorInputProcessHook registers hook that's
// called when the monitor input process starts.
func RegisterMonitorInputProcessHook(h monitor.StartInputHook) {
	hooks.monitorInputProcess = append(hooks.monitorInputProcess, h)
}

// RegisterMonitorEventHook registers hook that's called on every event.
func RegisterMonitorEventHook(h monitor.EventHook) {
	hooks.monitorEvent = append(hooks.monitorEvent, h)
}

// RegisterMonitorRecSaveHook registers hook that's called when monitor saves recording.
func RegisterMonitorRecSaveHook(h monitor.RecSaveHook) {
	hooks.monitorRecSave = append(hooks.monitorRecSave, h)
}

// RegisterMonitorRecSavedHook registers hook that's called after monitor have saved recording.
func RegisterMonitorRecSavedHook(h monitor.RecSavedHook) {
	hooks.monitorRecSaved = append(hooks.monitorRecSaved, h)
}

// RegisterMigrationMonitorHook is called when each monitor config is loaded.
func RegisterMigrationMonitorHook(h monitor.MigationHook) {
	hooks.migrationMonitor = append(hooks.migrationMonitor, h)
}

// RegisterLogSource adds log source.
func RegisterLogSource(s []string) {
	hooks.logSource = append(hooks.logSource, s...)
}

func (h *hookList) env(env storage.ConfigEnv) {
	for _, hook := range h.onEnv {
		hook(env)
	}
}

func (h *hookList) log(log *log.Logger) {
	for _, hook := range h.onLog {
		hook(log)
	}
}

func (h *hookList) auth(a auth.Authenticator) {
	for _, hook := range h.onAuth {
		hook(a)
	}
}

func (h *hookList) templater(t *web.Templater) {
	for _, hook := range h.onTemplater {
		hook(t)
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

func (h *hookList) monitor() *monitor.Hooks {
	startHook := func(ctx context.Context, m *monitor.Monitor) {
		for _, hook := range h.monitorStart {
			hook(ctx, m)
		}
	}
	startInputHook := func(ctx context.Context, i *monitor.InputProcess, args *[]string) {
		for _, hook := range h.monitorInputProcess {
			hook(ctx, i, args)
		}
	}
	eventHook := func(r *monitor.Recorder, event *storage.Event) {
		for _, hook := range h.monitorEvent {
			hook(r, event)
		}
	}
	recSaveHook := func(r *monitor.Recorder, args *string) {
		for _, hook := range h.monitorRecSave {
			hook(r, args)
		}
	}
	recSavedHook := func(r *monitor.Recorder, recPath string, recData storage.RecordingData) {
		for _, hook := range h.monitorRecSaved {
			hook(r, recPath, recData)
		}
	}
	migrateHook := func(conf monitor.Config) error {
		for _, hook := range h.migrationMonitor {
			err := hook(conf)
			if err != nil {
				return err
			}
		}
		return nil
	}

	return &monitor.Hooks{
		Start:      startHook,
		StartInput: startInputHook,
		Event:      eventHook,
		RecSave:    recSaveHook,
		RecSaved:   recSavedHook,
		Migrate:    migrateHook,
	}
}
