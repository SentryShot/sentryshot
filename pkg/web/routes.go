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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"nvr/pkg/system"
	"nvr/pkg/web/auth"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
)

// Static serves files from web/static
func Static() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}
		//w.Header().Set("Cache-Control", "max-age=2629800")
		w.Header().Set("Cache-Control", "no-cache")

		h := http.StripPrefix("/static/", http.FileServer(http.Dir("./web/static/")))
		h.ServeHTTP(w, r)
	})
}

// Storage serves files from web/static
func Storage() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}
		h := http.StripPrefix("/storage/", http.FileServer(http.Dir("./storage/")))
		h.ServeHTTP(w, r)
	})
}

// HLS serves files from shmHLS
func HLS(env *storage.ConfigEnv) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")

		h := http.StripPrefix("/hls/", http.FileServer(http.Dir(env.SHMhls())))
		h.ServeHTTP(w, r)
	})
}

// Status returns system status.
func Status(sys *system.System) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(sys.Status()); err != nil {
			http.Error(w, "could not encode json", http.StatusInternalServerError)
		}
	})
}

// TimeZone returns system timeZone.
func TimeZone(timeZone string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(timeZone); err != nil {
			http.Error(w, "could not encode json", http.StatusInternalServerError)
		}
	})
}

// General handler returns general configuration in json format.
func General(general *storage.ConfigGeneral) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}

		j, err := json.Marshal(general.Get())
		if err != nil {
			http.Error(w, "failed to marshal general config", http.StatusInternalServerError)
			return
		}
		if _, err := w.Write(j); err != nil {
			http.Error(w, "could not write data", http.StatusInternalServerError)
		}
	})
}

// GeneralSet handler to set general configuration.
func GeneralSet(general *storage.ConfigGeneral) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		var config storage.GeneralConfig
		if err = json.Unmarshal(body, &config); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if config.DiskSpace == "" {
			http.Error(w, "DiskSpace missing", http.StatusBadRequest)
			return
		}

		err = general.Set(config)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}

// Users returns a censored user list in json format.
func Users(a *auth.Authenticator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}
		j, err := json.Marshal(a.UsersList())
		if err != nil {
			http.Error(w, "failed to marshal user list", http.StatusInternalServerError)
			return
		}
		if _, err := w.Write(j); err != nil {
			http.Error(w, "could not write data", http.StatusInternalServerError)
		}
	})
}

// UserSet handler to set user details.
func UserSet(a *auth.Authenticator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		var user auth.Account
		if err = json.Unmarshal(body, &user); err != nil {
			http.Error(w, "unmarshal error: "+err.Error(), http.StatusBadRequest)
			return
		}

		err = a.UserSet(user)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	})
}

// UserDelete handler to delete user.
func UserDelete(a *auth.Authenticator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}

		name := r.URL.Query().Get("id")
		if name == "" {
			http.Error(w, "id missing", http.StatusBadRequest)
			return
		}

		err := a.UserDelete(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}

// MonitorList returns a censored monitor list with ID, Name and CaptureAudio.
func MonitorList(c *monitor.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}
		u, err := json.Marshal(c.MonitorList())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := w.Write(u); err != nil {
			http.Error(w, "could not write data", http.StatusInternalServerError)
			return
		}
	})
}

// MonitorConfigs returns monitor configurations in json format.
func MonitorConfigs(c *monitor.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}
		u, err := json.Marshal(c.MonitorConfigs())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := w.Write(u); err != nil {
			http.Error(w, "could not write data", http.StatusInternalServerError)
			return
		}
	})
}

// MonitorRestart handler to restart monitor.
func MonitorRestart(m *monitor.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}

		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id missing", http.StatusBadRequest)
			return
		}

		monitor, exists := m.Monitors[id]
		if !exists {
			http.Error(w, "monitor does not exist", http.StatusBadRequest)
			return
		}

		monitor.Stop()
		if err := monitor.Start(); err != nil {
			http.Error(w, "could not restart monitor: "+err.Error(), http.StatusInternalServerError)
		}
	})
}

func containsSpaces(s string) bool {
	return strings.Contains(s, " ")
}

// MonitorSet handler to set monitor configuration.
func MonitorSet(c *monitor.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		var m monitor.Config
		if err = json.Unmarshal(body, &m); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		switch {
		case m["id"] == "":
			http.Error(w, "monitor id cannot be empty", http.StatusBadRequest)
			return
		case containsSpaces(m["id"]):
			http.Error(w, "monitor id cannot contain spaces", http.StatusBadRequest)
			return
		case m["name"] == "":
			http.Error(w, "monitor name cannot be empty", http.StatusBadRequest)
			return
		case containsSpaces(m["name"]):
			http.Error(w, "monitor name cannot contain spaces", http.StatusBadRequest)
			return
		}

		err = c.MonitorSet(m["id"], m)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}

// MonitorDelete handler to delete monitor.
func MonitorDelete(m *monitor.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}

		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id missing", http.StatusBadRequest)
			return
		}

		err := m.MonitorDelete(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}

// RecordingQuery handles recording queries.
func RecordingQuery(s *storage.Crawler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}
		query := r.URL.Query()
		limit := query.Get("limit")
		if limit == "" {
			http.Error(w, "limit missing", http.StatusBadRequest)
			return
		}

		before := query.Get("before")
		if before == "" {
			http.Error(w, "before missing", http.StatusBadRequest)
			return
		}
		if len(before) < 19 {
			http.Error(w, "before to short", http.StatusBadRequest)
			return
		}

		limitInt, err := strconv.Atoi(limit)
		if err != nil {
			http.Error(w, fmt.Sprintf("could not convert n to int: %v", err), http.StatusBadRequest)
			return
		}

		recordings, err := s.RecordingByQuery(limitInt, before)
		if err != nil {
			http.Error(w, "could not process recording query", http.StatusInternalServerError)
			return
		}

		u, err := json.Marshal(recordings)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if _, err := w.Write(u); err != nil {
			http.Error(w, "could not write data", http.StatusInternalServerError)
			return
		}
	})
}

// Logs opens a websocket with system logs.
func Logs(log *log.Logger, a *auth.Authenticator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer c.Close()

		feed, cancel := log.Subscribe()
		defer cancel()

		authHeader := r.Header.Get("Authorization")
		for {
			msg := <-feed

			// Validate auth before each message.
			auth := a.ValidateAuth(authHeader)
			if !auth.IsValid || !auth.User.IsAdmin {
				return
			}

			err := c.WriteMessage(websocket.TextMessage, []byte(msg))
			if err != nil {
				return
			}
		}
	})
}
