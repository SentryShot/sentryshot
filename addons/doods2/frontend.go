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

package doods

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"nvr"
	"nvr/pkg/web/auth"
	"os"
	"strings"
)

func init() {
	var a auth.Authenticator
	nvr.RegisterAuthHook(func(auth auth.Authenticator) {
		a = auth
	})
	nvr.RegisterTplHook(modifyTemplates)
	nvr.RegisterMuxHook(func(mux *http.ServeMux) {
		mux.Handle("/doods.mjs", a.Admin(serveDoodsMjs()))
	})
}

func modifyTemplates(pageFiles map[string]string) error {
	js, exists := pageFiles["settings.js"]
	if !exists {
		return fmt.Errorf("doods: settings.js: %w", os.ErrNotExist)
	}

	pageFiles["settings.js"] = modifySettingsjs(js)
	return nil
}

func modifySettingsjs(tpl string) string {
	const target = "logLevel: fieldTemplate.select("

	importStatement := `import { doodsThresholds, doodsCrop, doodsMask } from "./doods.mjs"`
	tpl = strings.ReplaceAll(tpl, target, javascript()+target)

	return importStatement + tpl
}

func javascript() string {
	var detectors string
	for _, detector := range addon.detectorList {
		detectors += `"` + detector.Name + `", `
	}

	return `
	doodsEnable: fieldTemplate.toggle(
		"DOODS enable",
		"false"
	),
	doodsDetectorName: fieldTemplate.select(
		"DOODS detector",
		[` + detectors + `],
		"default"
	),
	doodsThresholds: doodsThresholds(),
	doodsCrop: doodsCrop(),
	doodsMask: doodsMask(),
	doodsFeedRate: newField(
		[inputRules.notEmpty, inputRules.noSpaces],
		{
			errorField: true,
			input: "number",
			min: "0",
		},
		{
			label: "DOODS feed rate (fps)",
			placeholder: "",
			initial: "2",
		}
	),
	doodsDuration: fieldTemplate.integer(
		"DOODS trigger duration (sec)",
		"",
		"120"
	),
	doodsUseSubStream: fieldTemplate.toggle(
		"DOODS use sub stream",
		"true"
	),
	`
}

//go:embed doods.mjs
var doodsMjsFile string
var doodsMjsCache string

func serveDoodsMjs() http.Handler {
	if doodsMjsCache == "" {
		data, _ := json.Marshal(addon.detectorList)
		detectorsJSON := string(data)
		doodsMjsCache = strings.Replace(doodsMjsFile, "$detectorsJSON", detectorsJSON, 1)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/javascript")
		if _, err := w.Write([]byte(doodsMjsCache)); err != nil {
			http.Error(w, "could not write: "+err.Error(), http.StatusInternalServerError)
		}
	})
}
