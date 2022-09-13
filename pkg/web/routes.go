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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"nvr/pkg/group"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"nvr/pkg/web/auth"
	"nvr/web/static"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
)

// Static serves files from `web/static`.
func Static() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}
		// w.Header().Set("Cache-Control", "max-age=2629800")
		w.Header().Set("Cache-Control", "no-cache")

		h := http.StripPrefix("/static/", http.FileServer(http.FS(static.Static)))
		h.ServeHTTP(w, r)
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

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		var config map[string]string
		if err = json.Unmarshal(body, &config); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if config["diskSpace"] == "" {
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
func Users(a auth.Authenticator) http.Handler {
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
func UserSet(a auth.Authenticator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		var req auth.SetUserRequest
		if err = json.Unmarshal(body, &req); err != nil {
			http.Error(w, "unmarshal error: "+err.Error(), http.StatusBadRequest)
			return
		}

		err = a.UserSet(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	})
}

// UserDelete handler to delete user.
func UserDelete(a auth.Authenticator) http.Handler {
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
func MonitorList(monitorList func() monitor.Configs) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}
		u, err := json.Marshal(monitorList())
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

// MonitorSet handler to set monitor configuration.
func MonitorSet(c *monitor.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		var m monitor.Config
		if err = json.Unmarshal(body, &m); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := checkIDandName(m); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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

// GroupConfigs returns group configurations in json format.
func GroupConfigs(m *group.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}
		u, err := json.Marshal(m.Configs())
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

// GroupSet handler to set group configuration.
func GroupSet(m *group.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		var g group.Config
		if err = json.Unmarshal(body, &g); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := checkIDandName(g); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err = m.GroupSet(g["id"], g); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}

// ErrEmptyValue value cannot be empty.
var ErrEmptyValue = errors.New("value cannot be empty")

// ErrContainsSpaces value cannot contain spaces.
var ErrContainsSpaces = errors.New("value cannot contain spaces")

func checkIDandName(input map[string]string) error {
	switch {
	case input["id"] == "":
		return fmt.Errorf("id: %w", ErrEmptyValue)
	case containsSpaces(input["id"]):
		return fmt.Errorf("id: %w", ErrContainsSpaces)
	case input["name"] == "":
		return fmt.Errorf("name: %w", ErrEmptyValue)
	case containsSpaces(input["name"]):
		return fmt.Errorf("name. %w", ErrContainsSpaces)
	default:
		return nil
	}
}

// GroupDelete handler to delete group.
func GroupDelete(m *group.Manager) http.Handler {
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

		err := m.GroupDelete(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}

// RecordingThumbnail serves thumbnail by exact recording ID.
func RecordingThumbnail(recordingsDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}

		recID := r.URL.Path[25:] // Trim "/api/recording/thumbnail/"
		recPath, err := storage.RecordingIDToPath(recID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		thumbPath := filepath.Join(recordingsDir, recPath+".jpeg")

		// ServeFile will sanitize ".."
		http.ServeFile(w, r, thumbPath)
	})
}

// RecordingVideo serves video by exact recording ID.
func RecordingVideo(recordingsDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}

		recID := r.URL.Path[21:] // Trim "/api/recording/video/"
		recPath, err := storage.RecordingIDToPath(recID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		videoPath := filepath.Join(recordingsDir, recPath+".mp4")

		// ServeFile will sanitize ".."
		http.ServeFile(w, r, videoPath)
	})
}

// RecordingQuery handles recording query.
func RecordingQuery(crawler *storage.Crawler, log *log.Logger) http.Handler { //nolint:funlen
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

		limitInt, err := strconv.Atoi(limit)
		if err != nil {
			http.Error(w, fmt.Sprintf("could not convert limit to int: %v", err), http.StatusBadRequest)
			return
		}

		time := query.Get("time")
		if time == "" {
			http.Error(w, "time missing", http.StatusBadRequest)
			return
		}
		if len(time) < 19 {
			http.Error(w, "time value to short", http.StatusBadRequest)
			return
		}
		reverse := query.Get("reverse")

		monitorsCSV := query.Get("monitors")

		var monitors []string
		if monitorsCSV != "" {
			monitors = strings.Split(monitorsCSV, ",")
		}

		var data bool
		if query.Get("data") == "true" {
			data = true
		}

		q := &storage.CrawlerQuery{
			Time:        time,
			Limit:       limitInt,
			Reverse:     reverse == "true",
			Monitors:    monitors,
			IncludeData: data,
		}

		recordings, err := crawler.RecordingByQuery(q)
		if err != nil {
			log.Error().Src("storage").
				Msgf("crawler: could not process recording query: %v", err)

			http.Error(w, "could not process recording query", http.StatusInternalServerError)
			return
		}

		u, err := json.Marshal(recordings)
		if err != nil {
			http.Error(w, "could not marshal data", http.StatusInternalServerError)
			return
		}

		if _, err := w.Write(u); err != nil {
			http.Error(w, "could not write data", http.StatusInternalServerError)
			return
		}
	})
}

// LogFeed opens a websocket with system logs.
func LogFeed(logger *log.Logger, a auth.Authenticator) http.Handler { //nolint:funlen,gocognit
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}
		query := r.URL.Query()

		levelsCSV := query.Get("levels")
		var levels []log.Level
		if levelsCSV != "" {
			for _, levelStr := range strings.Split(levelsCSV, ",") {
				levelInt, err := strconv.Atoi(levelStr)
				if err != nil {
					http.Error(w,
						fmt.Sprintf("invalid levels list: %v %v", levelsCSV, err),
						http.StatusBadRequest)
				}
				levels = append(levels, log.Level(levelInt))
			}
		}

		sourcesCSV := query.Get("sources")
		var sources []string
		if sourcesCSV != "" {
			sources = strings.Split(sourcesCSV, ",")
		}

		q := log.Query{
			Levels:  levels,
			Sources: sources,
		}

		upgrader := websocket.Upgrader{}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer c.Close()

		feed, cancel := logger.Subscribe()
		defer cancel()

		for {
			var l log.Log
			select {
			case l = <-feed:
			case <-logger.Ctx.Done():
				return
			}

			if !log.LevelInLevels(l.Level, q.Levels) {
				continue
			}
			if !log.StringInStrings(l.Src, q.Sources) {
				continue
			}

			// Validate auth before each message.
			auth := a.ValidateRequest(r)
			if !auth.IsValid || !auth.User.IsAdmin {
				return
			}

			raw, err := json.Marshal(l)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}

			if err := c.WriteMessage(websocket.TextMessage, raw); err != nil {
				return
			}
		}
	})
}

// LogQuery handles log queries.
func LogQuery(logDB *log.DB) http.Handler { //nolint:funlen
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

		limitInt, err := strconv.Atoi(limit)
		if err != nil {
			http.Error(w, fmt.Sprintf("could not convert limit to int: %v", err), http.StatusBadRequest)
			return
		}

		levelsCSV := query.Get("levels")
		var levels []log.Level
		if levelsCSV != "" {
			for _, levelStr := range strings.Split(levelsCSV, ",") {
				levelInt, err := strconv.Atoi(levelStr)
				if err != nil {
					http.Error(w,
						fmt.Sprintf("invalid levels list: %v %v", levelsCSV, err),
						http.StatusBadRequest)
				}
				levels = append(levels, log.Level(levelInt))
			}
		}

		sourcesCSV := query.Get("sources")
		var sources []string
		if sourcesCSV != "" {
			sources = strings.Split(sourcesCSV, ",")
		}

		time := query.Get("time")
		timeInt, err := strconv.Atoi(time)
		if err != nil {
			http.Error(w, fmt.Sprintf("could not convert time to int: %v", err), http.StatusBadRequest)
			return
		}

		q := log.Query{
			Levels:  levels,
			Sources: sources,
			Time:    log.UnixMillisecond(timeInt),
			Limit:   limitInt,
		}

		logs, err := logDB.Query(q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		logsJSON, err := json.Marshal(logs)
		if err != nil {
			http.Error(w, fmt.Sprintf("could not marshal data: %v", err), http.StatusInternalServerError)
			return
		}

		if _, err := w.Write(logsJSON); err != nil {
			http.Error(w, fmt.Sprintf("could not write data: %v", err), http.StatusInternalServerError)
			return
		}
	})
}

// LogSources handles list of log sources.
func LogSources(l *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}

		sources, err := json.Marshal(l.Sources())
		if err != nil {
			http.Error(w, fmt.Sprintf("could not marshal data: %v", err), http.StatusInternalServerError)
			return
		}

		if _, err := w.Write(sources); err != nil {
			http.Error(w, fmt.Sprintf("could not write data: %v", err), http.StatusInternalServerError)
			return
		}
	})
}

func containsSpaces(s string) bool {
	return strings.Contains(s, " ")
}
