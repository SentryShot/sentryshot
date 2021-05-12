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
	"nvr/pkg/storage"
	"nvr/pkg/system"
	"nvr/pkg/web/auth"
	"path/filepath"
	"strings"
	"time"
)

// Hook addon template hook.
type Hook func(map[string]string) error

// Templater is used to render html from templates.
type Templater struct {
	auth      *auth.Authenticator
	data      TemplateData
	templates map[string]*template.Template

	lastModified time.Time
}

// TemplateData is used when rendering templates.
type TemplateData struct {
	Status  func() system.Status
	General func() storage.GeneralConfig
}

// NewTemplater experimental.
func NewTemplater(a *auth.Authenticator, data TemplateData, hook Hook) (Templater, error) {
	path := "./web/templates"

	pageFiles, err := readDir(path)
	if err != nil {
		return Templater{}, err
	}

	if err := hook(pageFiles); err != nil {
		return Templater{}, err
	}

	includeFiles, err := readDir(path + "/includes")
	if err != nil {
		return Templater{}, err
	}

	templates := make(map[string]*template.Template)
	for fileName, page := range pageFiles {
		t := template.New(fileName)
		t, err := t.Parse(page)
		if err != nil {
			return Templater{}, fmt.Errorf("could not parse page: %v", err)
		}

		for _, include := range includeFiles {
			t, err = t.Parse(include)
			if err != nil {
				return Templater{}, fmt.Errorf("could not parse include: %v", err)
			}
		}
		templates[fileName] = t
	}

	return Templater{
		auth:         a,
		data:         data,
		templates:    templates,
		lastModified: time.Now().UTC(),
	}, nil
}

// readDir reads and returns contents of files in given directory.
func readDir(dir string) (map[string]string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("could not read directory: %v", err)
	}

	var fileContents = make(map[string]string)
	for _, file := range files {
		if !file.IsDir() && []rune(file.Name())[0] != []rune(".")[0] { // Check if file is hidden.
			b, err := ioutil.ReadFile(dir + "/" + file.Name())
			if err != nil {
				return nil, fmt.Errorf("could not read file: %v", err)
			}
			fileContents[file.Name()] = string(b)
		}
	}
	return fileContents, nil
}

// Render executes a template.
func (templater Templater) Render(page string) http.Handler {
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
		data["status"] = templater.data.Status()
		data["general"] = templater.data.General()

		auth := templater.auth.ValidateAuth(r.Header.Get("Authorization"))
		data["user"] = auth.User

		tls := r.Header["X-Forwarded-Proto"]
		if len(tls) != 0 {
			data["tls"] = tls[0]
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
