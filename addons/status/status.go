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

package status

import (
	"context"
	"fmt"
	"html/template"
	"nvr"
	"nvr/pkg/log"
	"nvr/pkg/storage"
	"nvr/pkg/web/auth"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

func init() {
	var addon struct {
		log     *log.Logger
		storage *storage.Manager
		auth    *auth.Authenticator
		sys     *system
	}

	nvr.RegisterLogHook(func(l *log.Logger) {
		addon.log = l
	})

	nvr.RegisterStorageHook(func(s *storage.Manager) {
		addon.storage = s
	})

	nvr.RegisterAuthHook(func(a *auth.Authenticator) {
		addon.auth = a
	})

	nvr.RegisterAppRunHook(func(ctx context.Context) error {
		addon.sys = newSystem(addon.storage.Usage, addon.log)
		go addon.sys.StatusLoop(ctx)
		return nil
	})

	nvr.RegisterTplDataHook(func(data template.FuncMap, _ string) {
		data["status"] = addon.sys.getStatus()
	})

	nvr.RegisterTplSubHook(modifySubTemplate)
}

type status struct {
	CPUUsage           int    `json:"cpuUsage"`
	RAMUsage           int    `json:"ramUsage"`
	DiskUsage          int    `json:"diskUsage"`
	DiskUsageFormatted string `json:"diskUsageFormatted"`
}

type (
	cpuFunc  func(context.Context, time.Duration, bool) ([]float64, error)
	ramFunc  func() (*mem.VirtualMemoryStat, error)
	diskFunc func() (storage.DiskUsage, error)
)

type system struct {
	cpu  cpuFunc
	ram  ramFunc
	disk diskFunc

	status   status
	duration time.Duration

	log *log.Logger
	mu  sync.Mutex
}

func newSystem(disk diskFunc, log *log.Logger) *system {
	return &system{
		cpu:  cpu.PercentWithContext,
		ram:  mem.VirtualMemory,
		disk: disk,

		duration: 10 * time.Second,

		log: log,
	}
}

func (s *system) update(ctx context.Context) error {
	cpuUsage, err := s.cpu(ctx, s.duration, false)
	if err != nil {
		return fmt.Errorf("could not get cpu usage %w", err)
	}
	ramUsage, err := s.ram()
	if err != nil {
		return fmt.Errorf("could not get ram usage %w", err)
	}
	diskUsage, err := s.disk()
	if err != nil {
		return fmt.Errorf("could not get disk usage %w", err)
	}

	s.mu.Lock()
	s.status = status{
		CPUUsage:           int(cpuUsage[0]),
		RAMUsage:           int(ramUsage.UsedPercent),
		DiskUsage:          diskUsage.Percent,
		DiskUsageFormatted: diskUsage.Formatted,
	}
	s.mu.Unlock()

	return nil
}

// StatusLoop updates system status until context is canceled.
func (s *system) StatusLoop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.update(ctx); err != nil {
			s.log.Error().Src("app").Msgf("could not update system status: %v", err)
		}
	}
}

func (s *system) getStatus() status {
	defer s.mu.Unlock()
	s.mu.Lock()
	return s.status
}

/*func handleStatus(sys *system) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(sys.getStatus()); err != nil {
			http.Error(w, "could not encode json", http.StatusInternalServerError)
		}
	})
}*/

const sidebarHTML = `
	<style>
		#logout {
			margin-bottom: 0;
		}

		.statusbar {
			width: var(--sidebar-width);
			margin-bottom: 0.4rem;
		}
		.statusbar li {
			margin-top: 0.2rem;
		}

		.statusbar-text-container {
			display: flex;
		}
		.statusbar-text {
			padding-right: 0.4rem;
			padding-left: 0.4rem;
			color: var(--color-text);
			font-size: 0.4em;
		}
		.statusbar-number {
			margin-left: auto;
		}
		.statusbar-progressbar {
			height: 0.3rem;
			margin-right: 0.3rem;
			margin-left: 0.3rem;
			padding: 0.05rem;
			background: var(--colorbg);
			border-radius: 0.2rem;
		}
		.statusbar-progressbar span {
			display: block;
			width: 50%;
			height: 100%;
			background: var(--color1);
			border-top-left-radius: 0.5rem;
			border-top-right-radius: 0.2rem;
			border-bottom-right-radius: 0.2rem;
			border-bottom-left-radius: 0.5rem;
		}
	</style>
	<ul class="statusbar">
		<li>
			<div class="statusbar-text-container">
				<span class="statusbar-text">CPU</span>
				<span class="statusbar-text statusbar-number"
					>{{ .status.CPUUsage }}%</span
				>
			</div>
			<div class="statusbar-progressbar">
				<span style="width: {{ .status.CPUUsage }}%"></span>
			</div>
		</li>
		<li>
			<div class="statusbar-text-container">
				<span class="statusbar-text">RAM</span>
				<span class="statusbar-text statusbar-number"
					>{{ .status.RAMUsage }}%</span
				>
			</div>
			<div class="statusbar-progressbar">
				<span style="width: {{ .status.RAMUsage }}%"></span>
			</div>
		</li>
		<li>
			<div class="statusbar-text-container">
				<span class="statusbar-text">DISK</span>
				<span
					style="margin: auto; font-size: 0.35rem"
					class="statusbar-text"
					>{{ .status.DiskUsageFormatted }}</span
				>
				<span class="statusbar-text statusbar-number"
					>{{ .status.DiskUsage }}%</span
				>
			</div>
			<div class="statusbar-progressbar">
				<span style="width: {{ .status.DiskUsage }}%"></span>
			</div>
		</li>
	</ul>`

func modifySubTemplate(pageFiles map[string]string) error {
	const target = "</aside>"

	pageFiles["sidebar.tpl"] = strings.ReplaceAll(
		pageFiles["sidebar.tpl"],
		target,
		sidebarHTML+target,
	)
	return nil
}
