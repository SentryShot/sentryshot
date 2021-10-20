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

package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"nvr/pkg/web/auth"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type (
	templates map[string]*template.Template

	// TemplateHook .
	TemplateHook func(map[string]string) error

	// TemplateDataFunc is a function thats called on page
	// render and allows template data to be modified.
	TemplateDataFunc func(template.FuncMap, string)
)

// Templater is used to render html from templates.
type Templater struct {
	auth              *auth.Authenticator
	templates         templates
	templateDataFuncs []TemplateDataFunc

	lastModified time.Time
}

// NewTemplater return template renderer.
func NewTemplater(path string, a *auth.Authenticator, hook TemplateHook) (*Templater, error) {
	pageFiles, err := readDir(path)
	if err != nil {
		return nil, err
	}

	if err := hook(pageFiles); err != nil {
		return nil, err
	}

	includeFiles, err := readDir(path + "/includes")
	if err != nil {
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

// readDir reads and returns contents of files in given directory.
func readDir(dir string) (map[string]string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("could not read directory: %w", err)
	}

	fileContents := make(map[string]string)
	for _, file := range files {
		if !file.IsDir() && []rune(file.Name())[0] != []rune(".")[0] { // Check if file is hidden.
			b, err := ioutil.ReadFile(dir + "/" + file.Name())
			if err != nil {
				return nil, fmt.Errorf("could not read file: %w", err)
			}
			fileContents[file.Name()] = string(b)
		}
	}

	if len(fileContents) == 0 {
		return nil, fmt.Errorf("%v: %w", dir, os.ErrNotExist)
	}

	return fileContents, nil
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

		auth := templater.auth.ValidateAuth(r.Header.Get("Authorization"))
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
