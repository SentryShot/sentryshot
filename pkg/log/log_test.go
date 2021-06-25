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

func newTestLogger() (context.Context, func(), *Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := NewLogger()
	go logger.Start(ctx)

	return ctx, cancel, logger
}

func TestLogger(t *testing.T) {
	t.Run("print", func(t *testing.T) {
		_, cancel, logger := newTestLogger()
		defer cancel()

		feed, cancel2 := logger.Subscribe()
		defer cancel2()

		cases := []struct {
			name     string
			printer  func(...interface{})
			msg      string
			expected string
		}{
			{"Print", logger.Print, "test", "test"},
			{"Println", logger.Println, "test", "test\n"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				go tc.printer(tc.msg)
				actual := <-feed
				if actual != tc.expected {
					t.Fatalf("expected: %v, got %v", tc.msg, actual)
				}
			})
		}
	})
	t.Run("printf", func(t *testing.T) {
		_, cancel, logger := newTestLogger()
		defer cancel()

		feed, cancel2 := logger.Subscribe()
		defer cancel2()

		cases := []struct {
			name     string
			printer  func(string, ...interface{})
			format   string
			msg      string
			expected string
		}{
			{"Printf", logger.Printf, "%s", "test", "test"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				go tc.printer(tc.format, tc.msg)

				actual := <-feed
				if actual != tc.expected {
					t.Fatalf("expected: %v, got %v", tc.msg, actual)
				}
			})
		}
	})
	t.Run("unsubBeforePrint", func(t *testing.T) {
		_, cancel, logger := newTestLogger()
		defer cancel()

		feed1, cancel1 := logger.Subscribe()
		feed2, cancel2 := logger.Subscribe()
		cancel2()

		msg := "test"
		logger.Print(msg)
		actual1 := <-feed1
		actual2 := <-feed2
		cancel1()

		if actual1 != msg {
			t.Fatalf("expected: %v, got %v", msg, actual1)
		}

		if actual2 != "" {
			t.Fatalf("expected nil got: %v", actual2)
		}
	})
	t.Run("unsubAfterPrint", func(t *testing.T) {
		_, cancel, logger := newTestLogger()
		defer cancel()

		feed, cancel2 := logger.Subscribe()

		go func() { logger.Print("test") }()
		go func() { logger.Print("test") }()
		go func() { logger.Print("test") }()
		time.Sleep(10 * time.Microsecond)
		cancel2()

		actual := <-feed
		if actual != "" {
			t.Fatalf("expected: nil, got %v", actual)
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
		expected := "log test"

		if actual != expected {
			t.Fatalf("expected: %v, got: %v", expected, actual)
		}
	})
}

func TestLogToStdout(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	ctx, cancel, logger := newTestLogger()
	defer cancel()

	go logger.LogToStdout(ctx)
	time.Sleep(1 * time.Millisecond)
	logger.Print("test")
	time.Sleep(1 * time.Millisecond)

	os.Exit(0)
}
