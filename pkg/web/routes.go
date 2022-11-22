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
	"net/http"
	"nvr/pkg/group"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"nvr/pkg/web/auth"
	"nvr/web/static"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/gorilla/websocket"
)

const jsonContentType = "application/json"

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
		w.Header().Set("Content-Type", jsonContentType)
		err := json.NewEncoder(w).Encode(timeZone)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
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

		w.Header().Set("Content-Type", jsonContentType)
		err := json.NewEncoder(w).Encode(general.Get())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
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

		var config map[string]string
		err := json.NewDecoder(r.Body).Decode(&config)
		if err != nil {
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

		w.Header().Set("Content-Type", jsonContentType)
		err := json.NewEncoder(w).Encode(a.UsersList())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
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

		var req auth.SetUserRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

		for _, r := range req.Username {
			if unicode.IsUpper(r) {
				http.Error(
					w,
					fmt.Sprintf("username cannot contain uppercase letters: %q", string(r)),
					http.StatusBadRequest,
				)
				return
			}
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

// MonitorList returns a censored monitor list.
func MonitorList(monitorInfo func() monitor.RawConfigs) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", jsonContentType)
		err := json.NewEncoder(w).Encode(monitorInfo())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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

		w.Header().Set("Content-Type", jsonContentType)
		err := json.NewEncoder(w).Encode(c.MonitorConfigs())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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

		err := m.RestartMonitor(id)
		if err != nil {
			http.Error(w, fmt.Sprintf("could not restart monitor: %v", err),
				http.StatusInternalServerError)
		}
	})
}

// MonitorSet handler to set monitor configuration.
func MonitorSet(m *monitor.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}

		var c monitor.RawConfig
		err := json.NewDecoder(r.Body).Decode(&c)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := checkIDandName(c); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err = m.MonitorSet(c["id"], c)
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

		w.Header().Set("Content-Type", jsonContentType)
		err := json.NewEncoder(w).Encode(m.Configs())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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

		var g group.Config
		err := json.NewDecoder(r.Body).Decode(&g)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := checkIDandNameGroup(g); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err = m.GroupSet(g["id"], g); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}

// Errors.
var (
	ErrEmptyValue     = errors.New("value cannot be empty")
	ErrContainsSpaces = errors.New("value cannot contain spaces")
	ErrIDTooLong      = errors.New("id cannot be longer than 24 bytes")
)

func checkIDandName(c monitor.RawConfig) error {
	switch {
	case c["id"] == "":
		return fmt.Errorf("id: %w", ErrEmptyValue)
	case containsSpaces(c["id"]):
		return fmt.Errorf("id: %w", ErrContainsSpaces)
	case c["name"] == "":
		return fmt.Errorf("name: %w", ErrEmptyValue)
	case containsSpaces(c["name"]):
		return fmt.Errorf("name: %w", ErrContainsSpaces)
	case len(c["id"]) > 24:
		return ErrIDTooLong
	default:
		return nil
	}
}

func checkIDandNameGroup(input map[string]string) error {
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
func RecordingVideo(logger *log.Logger, recordingsDir string) http.Handler {
	videoReaderCache := storage.NewVideoCache()
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
		path := filepath.Join(recordingsDir, recPath)
		// Sanitize path.
		if containsDotDot(path) {
			http.Error(w, "invalid recording ID", http.StatusBadRequest)
			return
		}

		mp4Path := path + ".mp4"

		_, err = os.Stat(mp4Path)
		if err == nil { // File exist.
			http.ServeFile(w, r, mp4Path)
			return
		}
		if !errors.Is(err, os.ErrNotExist) {
			http.Error(w, "stat mp4 file", http.StatusInternalServerError)
			return
		}

		video, err := storage.NewVideoReader(path, videoReaderCache)
		if err != nil {
			logger.Log(log.Entry{
				Level: log.LevelError,
				Src:   "app",
				Msg:   fmt.Sprintf("video request: %v", err),
			})
			http.Error(w, "see logs for details", http.StatusInternalServerError)
		}
		defer video.Close()

		ServeMP4Content(w, r, video.ModTime(), video.Size(), video)
	})
}

func containsDotDot(v string) bool {
	if !strings.Contains(v, "..") {
		return false
	}
	for _, ent := range strings.FieldsFunc(v, isSlashRune) {
		if ent == ".." {
			return true
		}
	}
	return false
}

func isSlashRune(r rune) bool { return r == '/' || r == '\\' }

// RecordingQuery handles recording query.
func RecordingQuery(crawler *storage.Crawler, logger *log.Logger) http.Handler { //nolint:funlen
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
			logger.Log(log.Entry{
				Level: log.LevelError,
				Src:   "app",
				Msg:   fmt.Sprintf("crawler: could not process recording query: %v", err),
			})
			http.Error(w, "could not process recording query", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", jsonContentType)
		err = json.NewEncoder(w).Encode(recordings)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
			var entry log.Entry
			select {
			case entry = <-feed:
			case <-logger.Ctx.Done():
				return
			}

			if !log.LevelInLevels(entry.Level, q.Levels) {
				continue
			}
			if !log.StringInStrings(entry.Src, q.Sources) {
				continue
			}

			// Validate auth before each message.
			auth := a.ValidateRequest(r)
			if !auth.IsValid || !auth.User.IsAdmin {
				return
			}

			if err := c.WriteJSON(entry); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	})
}

// LogQuery handles log queries.
func LogQuery(logStore *log.Store) http.Handler { //nolint:funlen
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
			Time:    log.UnixMicro(timeInt),
			Limit:   limitInt,
		}

		logs, err := logStore.Query(q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", jsonContentType)
		err = json.NewEncoder(w).Encode(logs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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

		w.Header().Set("Content-Type", jsonContentType)
		err := json.NewEncoder(w).Encode(l.Sources())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}

func containsSpaces(s string) bool {
	return strings.Contains(s, " ")
}
