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

package nvr

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"nvr/pkg/group"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"nvr/pkg/system"
	"nvr/pkg/web"
	"nvr/pkg/web/auth"
)

// Run .
func Run(envPath string) error {
	app, err := newApp(envPath, hooks)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())

	fatal := make(chan error, 1)
	go func() { fatal <- app.run(ctx) }()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err = <-fatal:
	case signal := <-stop:
		app.log.Printf("\nReceived %v stopping.\n", signal)
	}

	app.monitorManager.StopAll()
	app.log.Println("Monitors stopped.")

	cancel()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	if err := app.server.Shutdown(ctx2); err != nil {
		return err
	}
	return err
}

func newApp(envPath string, hooks *hookList) (*app, error) { //nolint:funlen
	logger := log.NewLogger()

	envYAML, err := ioutil.ReadFile(envPath)
	if err != nil {
		return nil, fmt.Errorf("could not read env.yaml: %v", err)
	}

	env, err := storage.NewConfigEnv(envPath, envYAML)
	if err != nil {
		return nil, fmt.Errorf("could not get environment config: %v", err)
	}

	hooks.env(env)

	general, err := storage.NewConfigGeneral(env.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("could not get general config: %v", err)
	}

	monitorConfigDir := filepath.Join(env.ConfigDir, "monitors")
	monitorManager, err := monitor.NewManager(monitorConfigDir, env, logger, hooks.monitor())
	if err != nil {
		return nil, fmt.Errorf("could not create monitor manager: %v", err)
	}

	groupConfigDir := filepath.Join(env.ConfigDir, "groups")
	groupManager, err := group.NewManager(groupConfigDir)
	if err != nil {
		return nil, fmt.Errorf("could not create monitor manager: %v", err)
	}

	usersConfigPath := filepath.Join(env.ConfigDir, "users.json")
	a, err := auth.NewBasicAuthenticator(usersConfigPath, logger)
	if err != nil {
		return nil, err
	}

	storageManager := storage.NewManager(env.StorageDir, general, logger)

	recordingsDir := filepath.Join(env.StorageDir, "recordings")
	crawler := storage.NewCrawler(recordingsDir)

	sys := system.New(storageManager.Usage, logger)

	timeZone, err := system.TimeZone()
	if err != nil {
		return nil, err
	}

	templateData := web.TemplateData{
		Status:  sys.Status,
		General: general.Get,
	}

	templatesDir := filepath.Join(env.WebDir, "templates")
	t, err := web.NewTemplater(templatesDir, a, templateData, hooks.tpl)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	mux.Handle("/live", a.User(t.Render("live.tpl")))
	mux.Handle("/recordings", a.User(t.Render("recordings.tpl")))
	mux.Handle("/settings", a.User(t.Render("settings.tpl")))
	mux.Handle("/settings.js", a.User(t.Render("settings.js")))
	mux.Handle("/logs", a.Admin(t.Render("logs.tpl")))
	mux.Handle("/debug", a.Admin(t.Render("debug.tpl")))
	mux.Handle("/logout", web.Logout())

	staticDir := filepath.Join(env.WebDir, "static")
	mux.Handle("/static/", a.User(web.Static(staticDir)))
	mux.Handle("/storage/", a.User(web.Storage(env.StorageDir)))
	mux.Handle("/hls/", a.User(web.HLS(env)))

	mux.Handle("/api/system/status", a.User(web.Status(sys)))
	mux.Handle("/api/system/timeZone", a.User(web.TimeZone(timeZone)))

	mux.Handle("/api/general", a.Admin(web.General(general)))
	mux.Handle("/api/general/set", a.Admin(a.CSRF(web.GeneralSet(general))))

	mux.Handle("/api/users", a.Admin(web.Users(a)))
	mux.Handle("/api/user/set", a.Admin(a.CSRF(web.UserSet(a))))
	mux.Handle("/api/user/delete", a.Admin(a.CSRF(web.UserDelete(a))))
	mux.Handle("/api/user/myToken", a.Admin(a.MyToken()))

	mux.Handle("/api/monitor/list", a.User(web.MonitorList(monitorManager.MonitorList)))
	mux.Handle("/api/monitor/configs", a.Admin(web.MonitorConfigs(monitorManager)))
	mux.Handle("/api/monitor/restart", a.Admin(a.CSRF(web.MonitorRestart(monitorManager))))
	mux.Handle("/api/monitor/set", a.Admin(a.CSRF(web.MonitorSet(monitorManager))))
	mux.Handle("/api/monitor/delete", a.Admin(a.CSRF(web.MonitorDelete(monitorManager))))

	mux.Handle("/api/group/configs", a.User(web.GroupConfigs(groupManager)))
	mux.Handle("/api/group/set", a.Admin(a.CSRF(web.GroupSet(groupManager))))
	mux.Handle("/api/group/delete", a.Admin(a.CSRF(web.GroupDelete(groupManager))))

	mux.Handle("/api/recording/query", a.User(web.RecordingQuery(crawler, logger)))
	mux.Handle("/api/logs", a.Admin(web.Logs(logger, a)))

	server := &http.Server{Addr: ":" + env.Port, Handler: mux}

	return &app{
		log:            logger,
		env:            env,
		monitorManager: monitorManager,
		storage:        storageManager,
		system:         sys,
		server:         server,
	}, nil
}

type app struct {
	log            *log.Logger
	env            *storage.ConfigEnv
	monitorManager *monitor.Manager
	storage        *storage.Manager
	system         *system.System
	server         *http.Server
}

func (a *app) run(ctx context.Context) error {
	go a.log.Start(ctx)
	go a.log.LogToStdout(ctx)
	time.Sleep(10 * time.Millisecond)
	a.log.Println("starting..")

	if err := a.env.PrepareEnvironment(); err != nil {
		return fmt.Errorf("could not prepare environment: %v", err)
	}

	// Start monitors
	for _, monitor := range a.monitorManager.Monitors {
		if err := monitor.Start(); err != nil {
			a.monitorManager.StopAll()
			return fmt.Errorf("could not start monitor: %v", err)
		}
	}

	go a.storage.PurgeLoop(ctx, 10*time.Minute)

	go a.system.StatusLoop(ctx)

	return a.server.ListenAndServe()
}
