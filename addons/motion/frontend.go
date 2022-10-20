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

package motion

import (
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"strings"
)

func modifyTemplates(pageFiles map[string]string) error {
	js, exists := pageFiles["settings.js"]
	if !exists {
		return fmt.Errorf("motion: settings.js %w", os.ErrNotExist)
	}

	pageFiles["settings.js"] = modifySettingsjs(js)
	return nil
}

func modifySettingsjs(tpl string) string {
	const importStatement = `import { motion } from "./motion.mjs"
`
	const target = "logLevel: fieldTemplate.select("

	tpl = strings.ReplaceAll(tpl, target, "motion: motion(),"+target)
	return importStatement + tpl
}

//go:embed motion.mjs
var motionMjsFile string

func serveMotionMjs() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/javascript")
		if _, err := w.Write([]byte(motionMjsFile)); err != nil {
			http.Error(w, "could not write: "+err.Error(), http.StatusInternalServerError)
		}
	})
}
