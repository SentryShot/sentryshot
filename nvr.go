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
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"nvr/pkg/system"
	"nvr/pkg/web"
	"nvr/pkg/web/auth"
)

// Run .
func Run(configDir string) error { //nolint:funlen
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := log.NewLogger(ctx)

	go logger.LogToStdout(ctx)
	time.Sleep(10 * time.Millisecond)
	logger.Println("starting..")

	envConfig, err := storage.NewConfigEnv(configDir)
	if err != nil {
		return fmt.Errorf("could not get environment config: %v", err)
	}

	generalConfig, err := storage.NewConfigGeneral(configDir)
	if err != nil {
		return fmt.Errorf("could not get general config: %v", err)
	}

	if err := envConfig.PrepareEnvironment(); err != nil {
		return fmt.Errorf("could not prepare environment: %v", err)
	}

	monitorManager, err := monitor.NewMonitorManager("./configs/monitors", envConfig, logger, monitorHook)
	if err != nil {
		return fmt.Errorf("could not create monitor manager: %v", err)
	}

	// Start monitors
	for _, monitor := range monitorManager.Monitors {
		if err := monitor.Start(); err != nil {
			monitorManager.StopAll()
			return fmt.Errorf("could not start monitor: %v", err)
		}
	}

	a, err := auth.NewBasicAuthenticator("./configs", logger)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup

	storageManager := storage.NewManager(envConfig.StorageDir, generalConfig, logger)
	go storageManager.PurgeLoop(ctx, 10*time.Minute)

	crawler := storage.NewCrawler(envConfig.StorageDir)

	status := system.New(storageManager.Usage, logger)
	go status.StatusLoop(ctx)
	timeZone, err := status.TimeZone()
	if err != nil {
		return fmt.Errorf("could not get timezone: %v", err)
	}

	templateData := web.TemplateData{
		Status:  status.Status,
		General: generalConfig.Get,
	}
	t, err := web.NewTemplater(a, templateData, tplHook)
	if err != nil {
		return err
	}

	http.Handle("/live", a.User(t.Render("live.tpl")))
	http.Handle("/recordings", a.User(t.Render("recordings.tpl")))
	http.Handle("/settings", a.User(t.Render("settings.tpl")))
	http.Handle("/settings.js", a.User(t.Render("settings.js")))
	http.Handle("/logs", a.Admin(t.Render("logs.tpl")))
	http.Handle("/debug", a.Admin(t.Render("debug.tpl")))

	http.Handle("/static/", a.User(web.Static()))
	http.Handle("/storage/", a.User(web.Storage()))
	http.Handle("/hls/", a.User(web.HLS(envConfig)))

	http.Handle("/api/system/status", a.User(web.Status(status)))
	http.Handle("/api/system/timeZone", a.User(web.TimeZone(timeZone)))
	http.Handle("/api/general", a.Admin(web.General(generalConfig)))
	http.Handle("/api/general/set", a.Admin(a.CSRF(web.GeneralSet(generalConfig))))
	http.Handle("/api/users", a.Admin(web.Users(a)))
	http.Handle("/api/user/set", a.Admin(a.CSRF(web.UserSet(a))))
	http.Handle("/api/user/delete", a.Admin(a.CSRF(web.UserDelete(a))))
	http.Handle("/api/user/myToken", a.Admin(a.MyToken()))
	http.Handle("/api/monitor/list", a.User(web.MonitorList(monitorManager)))
	http.Handle("/api/monitor/configs", a.Admin(web.MonitorConfigs(monitorManager)))
	http.Handle("/api/monitor/restart", a.Admin(a.CSRF(web.MonitorRestart(monitorManager))))
	http.Handle("/api/monitor/set", a.Admin(a.CSRF(web.MonitorSet(monitorManager))))
	http.Handle("/api/monitor/delete", a.Admin(a.CSRF(web.MonitorDelete(monitorManager))))
	http.Handle("/api/recording/query", a.User(web.RecordingQuery(crawler)))
	http.Handle("/api/logs", a.Admin(web.Logs(logger, a)))

	server := &http.Server{Addr: ":" + envConfig.Port, Handler: nil}

	fatal := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatal <- fmt.Errorf("server crashed: %v", err)
		}
	}()

	// Graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	shutdown := func() error {
		monitorManager.StopAll()
		logger.Println("Monitors stopped.")

		cancel()
		wg.Wait()

		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		err := server.Shutdown(ctx2)
		cancel2()

		return err
	}

	select {
	case signal := <-stop:
		logger.Printf("\nReceived %v stopping.\n", signal)
		return shutdown()
	case err = <-fatal:
		if err2 := shutdown(); err2 != nil {
			logger.Println(err2.Error() + "\n")
		}
		return err
	}
}
