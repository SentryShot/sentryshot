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

package log

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func newTestLogger(t *testing.T) (context.Context, func(), *Logger) {
	logger := NewLogger()

	ctx, cancel := context.WithCancel(context.Background())
	go logger.Start(ctx)

	cancelFunc := func() {
		cancel()
	}

	return ctx, cancelFunc, logger
}

func TestLogger(t *testing.T) {
	t.Run("unsubBeforePrint", func(t *testing.T) {
		_, cancel, logger := newTestLogger(t)
		defer cancel()

		feed1, cancel1 := logger.Subscribe()
		feed2, cancel2 := logger.Subscribe()
		cancel2()

		msg := "test"
		logger.Info().Msg(msg)
		actual1 := <-feed1
		actual2 := <-feed2
		cancel1()

		if actual1.Msg != msg {
			t.Fatalf("expected: %v, got %v", msg, actual1.Msg)
		}

		if actual2.Msg != "" {
			t.Fatalf("expected nil got: %v", actual2.Msg)
		}
	})
	t.Run("unsubAfterPrint", func(t *testing.T) {
		_, cancel, logger := newTestLogger(t)
		defer cancel()

		feed, cancel2 := logger.Subscribe()

		go func() { logger.Info().Msg("test") }()
		go func() { logger.Info().Msg("test") }()
		go func() { logger.Info().Msg("test") }()
		time.Sleep(10 * time.Microsecond)
		cancel2()

		actual := <-feed
		if actual.Msg != "" {
			t.Fatalf("expected: nil, got %v", actual.Msg)
		}
	})
	t.Run("logToStdout", func(t *testing.T) {
		cs := []string{"-test.run=TestLogToStdout"}
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{"GO_TEST_PROCESS=1"}
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command failed: %v", err)
		}
		actual := string(output)
		expected := `[ERROR] test
[WARNING] test
[INFO] test
[DEBUG] test
`

		if actual != expected {
			t.Fatalf("\nexpected:\n%v.\ngot:\n%v.", expected, actual)
		}
	})
}

func TestLogToStdout(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	ctx, cancel, logger := newTestLogger(t)
	defer cancel()

	go logger.LogToStdout(ctx)
	time.Sleep(1 * time.Millisecond)
	logger.Error().Msg("test")
	logger.Warn().Msg("test")
	logger.Info().Msg("test")
	logger.Debug().Msg("test")
	time.Sleep(1 * time.Millisecond)

	os.Exit(0)
}
