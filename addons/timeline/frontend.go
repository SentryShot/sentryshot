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

package timeline

import (
	_ "embed"
	"fmt"
	"net/http"
	"nvr"
	"nvr/pkg/web"
	"nvr/pkg/web/auth"
	"os"
	"strings"
)

func init() {
	var addon struct {
		a auth.Authenticator
		t *web.Templater
	}
	nvr.RegisterTplSubHook(modifySubTemplates)
	nvr.RegisterTplHook(modifyTemplates)
	nvr.RegisterAuthHook(func(a auth.Authenticator) {
		addon.a = a
	})
	nvr.RegisterTemplaterHook(func(t *web.Templater) {
		addon.t = t
	})
	nvr.RegisterMuxHook(func(mux *http.ServeMux) {
		mux.Handle("/timeline", addon.a.User(addon.t.Render("timeline.tpl")))
		mux.Handle("/timeline.mjs", addon.a.User(serveTimelineMjs()))
	})
}

func modifySubTemplates(subFiles map[string]string) error {
	tpl, exists := subFiles["sidebar.tpl"]
	if !exists {
		return fmt.Errorf("timeline: sidebar.tpl: %w", os.ErrNotExist)
	}

	subFiles["sidebar.tpl"] = modifySidebar(tpl)
	return nil
}

func modifySidebar(tpl string) string {
	target := `<a href="recordings" id="nav-link-recordings" class="nav-link">
				<img class="icon" src="static/icons/feather/film.svg" />
				<span class="nav-text">Recordings</span>
			</a>`
	timelineButton := `<a href="timeline" id="nav-link-timeline" class="nav-link">
				<img class="icon" src="static/icons/feather/activity.svg" />
				<span class="nav-text">Timeline</span>
			</a>`

	return strings.ReplaceAll(tpl, target, target+timelineButton)
}

//go:embed timeline.tpl
var timelineTplFile string

func modifyTemplates(pageFiles map[string]string) error {
	js, exists := pageFiles["settings.js"]
	if !exists {
		return fmt.Errorf("timeline: settings.js: %w", os.ErrNotExist)
	}
	pageFiles["settings.js"] = modifySettingsjs(js)

	pageFiles["timeline.tpl"] = timelineTplFile
	return nil
}

func modifySettingsjs(tpl string) string {
	const target = "logLevel: fieldTemplate.select("

	const javascript = `
		timelineScale:fieldTemplate.select(
			"Timeline scale",
			["full", "half", "third", "quarter", "sixth", "eighth"],
			"eighth",
		),
		timelineQuality: fieldTemplate.select(
			"Timeline quality",
			["1", "2", "3", "4", "5", "6", "7", "8"],
			"4"
		),
		timelineFrameRate: newField(
			[inputRules.notEmpty],
			{
				errorField: true,
				input: "number",
				min: "0",
				max: "30",
			},
			{
				label: "Timeline frames per minute",
				placeholder: "0-30",
				initial: 15,
			}
		),`

	return strings.ReplaceAll(tpl, target, javascript+target)
}

//go:embed timeline.mjs
var timelineMjsFile []byte

func serveTimelineMjs() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/javascript")
		if _, err := w.Write(timelineMjsFile); err != nil {
			http.Error(w, "could not write: "+err.Error(), http.StatusInternalServerError)
		}
	})
}
