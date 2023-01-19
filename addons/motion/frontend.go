// SPDX-License-Identifier: GPL-2.0-or-later

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
