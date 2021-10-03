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
	"errors"
	"fmt"
	"io/ioutil"
	"nvr/pkg/log"
	"nvr/pkg/storage"
	"os"
	"path"
	"path/filepath"
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

type (
	cpuFunc  func(context.Context, time.Duration, bool) ([]float64, error)
	ramFunc  func() (*mem.VirtualMemoryStat, error)
	diskFunc func() (storage.DiskUsage, error)
)

// System .
type System struct {
	cpu  cpuFunc
	ram  ramFunc
	disk diskFunc

	status   Status
	duration time.Duration

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

		duration: 10 * time.Second,

		log: log,
	}
}

func (s *System) update(ctx context.Context) error {
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
				return
			}
			if err := s.update(ctx); err != nil {
				s.log.Error().Src("app").Msgf("could not update system status: %v", err)
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

// ErrNoTimeZone could not determine time zone.
var ErrNoTimeZone = errors.New("could not determine time zone")

// TimeZone returns system time zone location.
func TimeZone() (string, error) {
	// Try golang's built-in function.
	zone := time.Now().Location().String()
	if zone != "Local" {
		return zone, nil
	}

	// Fallback 1
	data, _ := ioutil.ReadFile("/etc/timezone")
	zone = string(data)
	if zone != "" {
		return strings.TrimSpace(zone), nil
	}

	// Fallback 2
	localtime, _ := ioutil.ReadFile("/etc/localtime")
	_ = filepath.Walk("/usr/share/zoneinfo", func(filePath string, file os.FileInfo, err error) error {
		if err != nil || file.IsDir() {
			return err
		}
		data, _ := ioutil.ReadFile(filePath)
		if string(data) == string(localtime) {
			dir, city := path.Split(filePath)
			region := path.Base(dir)
			zone = city

			switch region {
			case "zoneinfo":
			case "posix":
			default:
				zone = region + "/" + city
			}
		}
		return nil
	})
	if zone != "" {
		return strings.TrimSpace(zone), nil
	}

	return "", ErrNoTimeZone
}
