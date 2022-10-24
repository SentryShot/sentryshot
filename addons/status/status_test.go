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

package status

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"nvr/pkg/log"
	"nvr/pkg/storage"

	"github.com/shirou/gopsutil/v3/mem"
	"github.com/stretchr/testify/require"
)

func mockCPU(_ context.Context, _ time.Duration, _ bool) ([]float64, error) {
	return []float64{11}, nil
}

func mockRAM() (*mem.VirtualMemoryStat, error) {
	return &mem.VirtualMemoryStat{
		UsedPercent: 22.0,
	}, nil
}

func mockDisk() (storage.DiskUsage, error) {
	return storage.DiskUsage{
		Percent:   33,
		Formatted: "44",
	}, nil
}

func mockCPUErr(_ context.Context, _ time.Duration, _ bool) ([]float64, error) {
	return nil, errors.New("")
}

func mockRAMErr() (*mem.VirtualMemoryStat, error) {
	return &mem.VirtualMemoryStat{}, errors.New("")
}

func mockDiskErr() (storage.DiskUsage, error) {
	return storage.DiskUsage{}, errors.New("mock")
}

func TestUpdate(t *testing.T) {
	cases := map[string]struct {
		cpu           cpuFunc
		ram           ramFunc
		disk          diskFunc
		expectedError bool
		expectedValue string
	}{
		"cpuErr": {mockCPUErr, mockRAM, mockDisk, true, "{0 0 0 }"},
		"ramErr": {mockCPU, mockRAMErr, mockDisk, true, "{0 0 0 }"},
		"ok":     {mockCPU, mockRAM, mockDisk, false, "{11 22 0 }"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := system{
				cpu:  tc.cpu,
				ram:  tc.ram,
				disk: tc.disk,
			}

			ctx, cancel := context.WithTimeout(context.TODO(), 100*time.Millisecond)
			defer cancel()

			actualError := s.updateCPUAndRAM(ctx)
			gotError := actualError != nil
			require.Equal(t, gotError, tc.expectedError)

			actualValue := fmt.Sprintf("%v", s.getStatus())
			require.Equal(t, actualValue, tc.expectedValue)
		})
	}
}

func TestUpdateCPUAndRAM(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		s := system{cpu: mockCPU, ram: mockRAM}

		err := s.updateCPUAndRAM(context.Background())
		require.NoError(t, err)

		expected := status{
			CPUUsage: 11,
			RAMUsage: 22,
		}
		require.Equal(t, expected, s.status)
	})
	t.Run("cpuErr", func(t *testing.T) {
		s := system{cpu: mockCPUErr, ram: mockRAM}

		err := s.updateCPUAndRAM(context.Background())
		require.Error(t, err)
	})
	t.Run("diskErr", func(t *testing.T) {
		s := system{cpu: mockCPU, ram: mockRAMErr}

		err := s.updateCPUAndRAM(context.Background())
		require.Error(t, err)
	})
}

func TestUpdateDiskError(t *testing.T) {
	logs := make(chan string)
	logf := func(_ log.Level, format string, a ...interface{}) {
		logs <- fmt.Sprintf(format, a...)
	}
	s := system{
		disk:           mockDiskErr,
		isUpdatingDisk: true,
		logf:           logf,
	}

	go s.updateDisk()
	require.Equal(t, "could not update disk usage: mock", <-logs)
}

func TestLoop(t *testing.T) {
	s := system{
		cpu:  mockCPU,
		ram:  mockRAM,
		disk: mockDisk,
	}

	ctx, cancel := context.WithTimeout(context.TODO(), 100*time.Millisecond)
	defer cancel()

	s.StatusLoop(ctx)
}
