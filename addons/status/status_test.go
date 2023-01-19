// SPDX-License-Identifier: GPL-2.0-or-later

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

func stubCPU(_ context.Context, _ time.Duration, _ bool) ([]float64, error) {
	return []float64{11}, nil
}

func stubRAM() (*mem.VirtualMemoryStat, error) {
	return &mem.VirtualMemoryStat{
		UsedPercent: 22.0,
	}, nil
}

func stubCPUErr(_ context.Context, _ time.Duration, _ bool) ([]float64, error) {
	return nil, errors.New("")
}

func stubRAMErr() (*mem.VirtualMemoryStat, error) {
	return &mem.VirtualMemoryStat{}, errors.New("")
}

func TestUpdate(t *testing.T) {
	cases := map[string]struct {
		cpu           cpuFunc
		ram           ramFunc
		expectedError bool
		expectedValue string
	}{
		"cpuErr": {stubCPUErr, stubRAM, true, "{0 0 0 }"},
		"ramErr": {stubCPU, stubRAMErr, true, "{0 0 0 }"},
		"ok":     {stubCPU, stubRAM, false, "{11 22 0 }"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := system{
				cpu: tc.cpu,
				ram: tc.ram,
				diskCached: func() (storage.DiskUsage, time.Duration) {
					return storage.DiskUsage{}, 0
				},
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
		s := system{cpu: stubCPU, ram: stubRAM}

		err := s.updateCPUAndRAM(context.Background())
		require.NoError(t, err)

		expected := status{
			CPUUsage: 11,
			RAMUsage: 22,
		}
		require.Equal(t, expected, s.status)
	})
	t.Run("cpuErr", func(t *testing.T) {
		s := system{cpu: stubCPUErr, ram: stubRAM}

		err := s.updateCPUAndRAM(context.Background())
		require.Error(t, err)
	})
	t.Run("diskErr", func(t *testing.T) {
		s := system{cpu: stubCPU, ram: stubRAMErr}

		err := s.updateCPUAndRAM(context.Background())
		require.Error(t, err)
	})
}

func stubDiskGet() (storage.DiskUsage, error) {
	return storage.DiskUsage{
		Percent:   33,
		Formatted: "44",
	}, nil
}

func TestUpdateDiskError(t *testing.T) {
	logs := make(chan string)
	logf := func(_ log.Level, format string, a ...interface{}) {
		logs <- fmt.Sprintf(format, a...)
	}
	s := system{
		diskCached: func() (storage.DiskUsage, time.Duration) {
			return storage.DiskUsage{}, 1 * time.Hour
		},
		disk: func(time.Duration) (storage.DiskUsage, error) {
			return storage.DiskUsage{}, errors.New("stub")
		},
		logf: logf,
	}

	s.updateDiskUnsafe()
	require.Equal(t, "could not get disk usage: stub", <-logs)
}
