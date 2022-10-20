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
	"os"
	"strings"
)

func modifyTemplates(pageFiles map[string]string) error {
	js, exists := pageFiles["settings.js"]
	if !exists {
		return fmt.Errorf("doods: settings.js: %w", os.ErrNotExist)
	}

	pageFiles["settings.js"] = modifySettingsjs(js)
	return nil
}

func modifySettingsjs(tpl string) string {
	const importStatement = `import { doods } from "./doods.mjs"
`
	const target = "logLevel: fieldTemplate.select("

	tpl = strings.ReplaceAll(tpl, target, "doods: doods(),"+target)
	return importStatement + tpl
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
