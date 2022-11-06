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

package log

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestLogger(t *testing.T) (func(), *Logger) {
	logger := &Logger{
		feed:  make(chan Entry),
		sub:   make(chan chan Entry),
		unsub: make(chan chan Entry),
		wg:    &sync.WaitGroup{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	logger.Start(ctx)

	return cancel, logger
}

func TestLoggerMSG(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		cancel, logger := newTestLogger(t)
		defer cancel()

		go func() {
			time.Sleep(10 * time.Millisecond)
			logger.Log(Entry{
				Level: LevelInfo, Src: "s1", MonitorID: "m1", Msg: "1",
			})
			logger.Log(Entry{
				Level: LevelWarning, Src: "s2", MonitorID: "m2", Msg: "2",
			})
			logger.Log(Entry{
				Level: LevelError, Src: "s3", MonitorID: "m3", Msg: "3",
			})
			logger.Log(Entry{
				Level: LevelDebug, Src: "s4", MonitorID: "m4", Msg: "4",
			})
		}()

		feed, cancel2 := logger.Subscribe()
		defer cancel2()

		actual := []Entry{<-feed, <-feed, <-feed, <-feed}
		for i := 0; i < len(actual); i++ {
			actual[i].Time = 0
		}

		expected := []Entry{
			{Level: LevelInfo, Src: "s1", MonitorID: "m1", Msg: "1"},
			{Level: LevelWarning, Src: "s2", MonitorID: "m2", Msg: "2"},
			{Level: LevelError, Src: "s3", MonitorID: "m3", Msg: "3"},
			{Level: LevelDebug, Src: "s4", MonitorID: "m4", Msg: "4"},
		}

		require.Equal(t, actual, expected)
	})

	panics := map[string]Entry{
		"levelPanic": {
			Src: "src",
			Msg: "msg",
		},
		"srcPanic": {
			Level: LevelDebug,
			Msg:   "msg",
		},
		"msgPanic": {
			Level: LevelDebug,
			Src:   "src",
		},
	}
	for name, log := range panics {
		t.Run(name, func(t *testing.T) {
			cancel, logger := newTestLogger(t)
			defer cancel()
			require.Panics(t, func() {
				logger.Log(log)
			})
		})
	}
}

func TestLogger(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		cancel, logger := newTestLogger(t)
		cancel()
		time.Sleep(10 * time.Millisecond)

		feed, cancel2 := logger.Subscribe()
		<-feed
		_, ok := <-feed
		require.False(t, ok)

		cancel2()
	})
	t.Run("closeChannels", func(t *testing.T) {
		cancel, logger := newTestLogger(t)

		feed1, cancel2 := logger.Subscribe()

		cancel()
		cancel2()

		_, ok := <-feed1
		require.False(t, ok)
	})
	t.Run("unsubBeforePrint", func(t *testing.T) {
		cancel, logger := newTestLogger(t)
		defer cancel()

		feed1, cancel1 := logger.Subscribe()
		feed2, cancel2 := logger.Subscribe()
		cancel2()

		msg := "test"
		logger.Log(Entry{Level: LevelInfo, Src: "test", Msg: msg})
		actual1 := <-feed1
		actual2 := <-feed2
		cancel1()

		require.Equal(t, actual1.Msg, msg)
		require.Equal(t, actual2.Msg, "")
	})
	t.Run("unsubAfterPrint", func(t *testing.T) {
		cancel, logger := newTestLogger(t)
		defer cancel()

		feed, cancel2 := logger.Subscribe()

		newTestEntry := func() Entry {
			return Entry{Level: LevelInfo, Src: ".", Msg: "test"}
		}

		go func() { logger.Log(newTestEntry()) }()
		go func() { logger.Log(newTestEntry()) }()
		go func() { logger.Log(newTestEntry()) }()
		time.Sleep(10 * time.Microsecond)
		cancel2()

		actual := <-feed
		require.Equal(t, actual.Msg, "")
	})
	t.Run("logToWriter", func(t *testing.T) {
		cancel, logger := newTestLogger(t)
		defer cancel()

		ctx, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		newTestEntry := func(level Level) Entry {
			return Entry{Level: level, Src: "src", Msg: "msg"}
		}

		w, writes := newMockWriter()

		go logger.LogToWriter(ctx, w)
		time.Sleep(100 * time.Millisecond)

		go logger.Log(newTestEntry(LevelError))
		require.Equal(t, "[ERROR] Src: msg\n", <-writes)

		go logger.Log(newTestEntry(LevelWarning))
		require.Equal(t, "[WARNING] Src: msg\n", <-writes)

		go logger.Log(newTestEntry(LevelInfo))
		require.Equal(t, "[INFO] Src: msg\n", <-writes)

		go logger.Log(newTestEntry(LevelDebug))
		require.Equal(t, "[DEBUG] Src: msg\n", <-writes)
	})
}

type mockWriter struct {
	writes chan string
}

func newMockWriter() (io.Writer, chan string) {
	writes := make(chan string)
	return &mockWriter{writes: writes}, writes
}

func (w *mockWriter) Write(p []byte) (int, error) {
	w.writes <- string(p)
	return len(p), nil
}
