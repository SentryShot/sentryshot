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

package system

import (
	"context"
	"fmt"
	"io/ioutil"
	"nvr/pkg/log"
	"nvr/pkg/storage"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// Status stores system status.
type Status struct {
	CPUUsage           int    `json:"cpuUsage"`
	RAMUsage           int    `json:"ramUsage"`
	DiskUsage          int    `json:"diskUsage"`
	DiskUsageFormatted string `json:"diskUsageFormatted"`
}

type cpuFunc func(context.Context, time.Duration, bool) ([]float64, error)
type ramFunc func() (*mem.VirtualMemoryStat, error)
type diskFunc func() (storage.DiskUsage, error)

// System .
type System struct {
	cpu  cpuFunc
	ram  ramFunc
	disk diskFunc

	status       Status
	duration     time.Duration
	timeZonePath string

	log *log.Logger
	mu  sync.Mutex
	o   sync.Once
}

// New returns new SystemStatus.
func New(disk diskFunc, log *log.Logger) *System {
	return &System{
		cpu:  cpu.PercentWithContext,
		ram:  mem.VirtualMemory,
		disk: disk,

		duration:     10 * time.Second,
		timeZonePath: "/etc/timezone",

		log: log,
	}
}
func (s *System) update(ctx context.Context) error {
	cpuUsage, err := s.cpu(ctx, s.duration, false)
	if err != nil {
		return fmt.Errorf("could not get cpu usage %v", err)
	}
	ramUsage, err := s.ram()
	if err != nil {
		return fmt.Errorf("could not get ram usage %v", err)
	}
	diskUsage, err := s.disk()
	if err != nil {
		return fmt.Errorf("could not get disk usage %v", err)
	}

	s.mu.Lock()
	s.status = Status{
		CPUUsage:           int(cpuUsage[0]),
		RAMUsage:           int(ramUsage.UsedPercent),
		DiskUsage:          diskUsage.Percent,
		DiskUsageFormatted: diskUsage.Formatted,
	}
	s.mu.Unlock()

	return nil
}

// StatusLoop updates system status until context is canceled.
func (s *System) StatusLoop(ctx context.Context) {
	s.o.Do(func() {
		for {
			if ctx.Err() != nil {
				fmt.Println("status stopped")
				return
			}
			if err := s.update(ctx); err != nil {
				s.log.Printf("could not update system status: %v", err)
			}
		}
	})
}

// Status returns cpu, ram and disk usage.
func (s *System) Status() Status {
	defer s.mu.Unlock()
	s.mu.Lock()
	return s.status
}

func (s *System) timeZone() (string, error) {
	data, err := ioutil.ReadFile(s.timeZonePath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// TimeZone returns system time zone.
func (s *System) TimeZone() (string, error) {
	return s.timeZone()
}
