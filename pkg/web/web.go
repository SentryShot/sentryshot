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

package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"nvr/pkg/web/auth"
	"path/filepath"
	"strings"
	"time"

	tpls "nvr/web/templates"
)

type (
	templates map[string]*template.Template

	// TemplateHook .
	TemplateHook func(map[string]string) error

	// TemplateDataFunc is a function thats called on page
	// render and allows template data to be modified.
	TemplateDataFunc func(template.FuncMap, string)
)

// TemplateHooks .
type TemplateHooks struct {
	Tpl TemplateHook
	Sub TemplateHook
}

// Templater is used to render html from templates.
type Templater struct {
	auth              auth.Authenticator
	templates         templates
	templateDataFuncs []TemplateDataFunc

	lastModified time.Time
}

// NewTemplater return template renderer.
func NewTemplater(a auth.Authenticator, hooks TemplateHooks) (*Templater, error) {
	pageFiles := tpls.PageFiles
	if err := hooks.Tpl(pageFiles); err != nil {
		return nil, err
	}

	includeFiles := tpls.IncludeFiles
	if err := hooks.Sub(includeFiles); err != nil {
		return nil, err
	}

	templates := make(map[string]*template.Template)
	for fileName, page := range pageFiles {
		t := template.New(fileName)
		t, err := t.Parse(page)
		if err != nil {
			return nil, fmt.Errorf("could not parse page: %w", err)
		}

		for _, include := range includeFiles {
			t, err = t.Parse(include)
			if err != nil {
				return nil, fmt.Errorf("could not parse include: %w", err)
			}
		}
		templates[fileName] = t
	}

	return &Templater{
		auth:         a,
		templates:    templates,
		lastModified: time.Now().UTC(),
	}, nil
}

// RegisterTemplateDataFuncs .
func (templater *Templater) RegisterTemplateDataFuncs(dataFuncs ...TemplateDataFunc) {
	templater.templateDataFuncs = append(
		templater.templateDataFuncs, dataFuncs...)
}

// Render executes a template.
func (templater *Templater) Render(page string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t, exists := templater.templates[page]
		if !exists {
			http.Error(w, "could not find template for page: "+page, http.StatusInternalServerError)
			return
		}

		if strings.Contains(page, ".js") {
			w.Header().Set("content-type", "text/javascript")
		}

		data := make(template.FuncMap)

		data["currentPage"] = strings.Title(strings.TrimSuffix(page, filepath.Ext(page)))

		auth := templater.auth.ValidateRequest(r)
		data["user"] = auth.User

		if page == "debug.tpl" {
			tls := r.Header["X-Forwarded-Proto"]
			if len(tls) != 0 {
				data["tls"] = tls[0]
			}
		}

		for _, dataFunc := range templater.templateDataFuncs {
			dataFunc(data, page)
		}

		var b bytes.Buffer
		if err := t.Execute(&b, data); err != nil {
			http.Error(w, "could not execute template "+err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := io.WriteString(w, b.String()); err != nil {
			http.Error(w, "could not write string", http.StatusInternalServerError)
			return
		}
	})
}
