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

// API inspired by zerolog https://github.com/rs/zerolog

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Func logging function.
type Func func(level Level, format string, a ...interface{})

// Level defines log level.
type Level uint8

// Logging constants matching FFmpeg.
const (
	LevelError   Level = 16
	LevelWarning Level = 24
	LevelInfo    Level = 32
	LevelDebug   Level = 48
)

// UnixMillisecond .
type UnixMillisecond uint64

// Entry defines log entry.
type Entry struct {
	Level     Level
	Msg       string          // Message
	Src       string          // Source.
	MonitorID string          // Source monitor id.
	Time      UnixMillisecond // Timestamp. Do not set manually.
}

// GetTime entry timestamp as time.GetTime.
func (e Entry) GetTime() time.Time {
	return time.Unix(0, int64(e.Time*1000))
}

func (e Entry) String() string {
	var b strings.Builder

	switch e.Level {
	case LevelError:
		b.WriteString("[ERROR] ")
	case LevelWarning:
		b.WriteString("[WARNING] ")
	case LevelInfo:
		b.WriteString("[INFO] ")
	case LevelDebug:
		b.WriteString("[DEBUG] ")
	}

	if e.MonitorID != "" {
		b.WriteString(e.MonitorID + ": ")
	}

	srcTitle := cases.Title(language.Und).String(e.Src)
	b.WriteString(srcTitle + ": ")

	b.WriteString(e.Msg)
	return b.String()
}

// FFmpegLevel converts ffmpeg log level to Level.
func FFmpegLevel(logLevel string) Level {
	switch logLevel {
	case "quiet":
	case "fatal", "error":
		return LevelError
	case "warning":
		return LevelWarning
	case "info":
		return LevelInfo
	case "debug":
		return LevelDebug
	}
	return LevelDebug
}

// ILogger Logger interface.
type ILogger interface {
	Log(log Entry)
}

// Logger .
type Logger struct {
	feed  chan Entry      // feed of logs.
	sub   chan chan Entry // subscribe requests.
	unsub chan chan Entry // unsubscribe requests.

	wg      *sync.WaitGroup
	Ctx     context.Context
	sources []string
}

var defaultSources = []string{"app", "auth", "monitor", "recorder"}

// NewLogger starts and returns Logger.
func NewLogger(wg *sync.WaitGroup, addonSources []string) *Logger {
	return &Logger{
		feed:  make(chan Entry),
		sub:   make(chan chan Entry),
		unsub: make(chan chan Entry),

		wg:      wg,
		sources: append(defaultSources, addonSources...),
	}
}

// Log to logger.
func (l *Logger) Log(log Entry) {
	if log.Level == 0 {
		panic(fmt.Sprintf("log level cannot be 0: %v", log))
	}
	if log.Src == "" {
		panic(fmt.Sprintf("log source cannot be empty: %v", log))
	}
	// MonitorID can be empty.
	if log.Msg == "" {
		panic(fmt.Sprintf("log message cannot be empty: %v", log))
	}

	select {
	case <-l.Ctx.Done():
	case l.feed <- log:
	}
}

// Sources Returns log sources.
func (l *Logger) Sources() []string {
	return l.sources
}

// Start logger.
func (l *Logger) Start(ctx context.Context) error {
	l.Ctx = ctx

	l.wg.Add(1)
	go func() {
		subs := map[chan Entry]struct{}{}
		for {
			select {
			case <-ctx.Done():
				// Only exit if everyone has unsubscribed.
				if len(subs) == 0 {
					l.wg.Done()
					return
				}
				time.Sleep(50 * time.Millisecond)

			case ch := <-l.sub:
				subs[ch] = struct{}{}

			case ch := <-l.unsub:
				close(ch)
				delete(subs, ch)

			case msg := <-l.feed:
				for ch := range subs {
					ch <- msg
				}
			}
		}
	}()
	return nil
}

// CancelFunc cancels log feed subsciption.
type CancelFunc func()

// Subscribe returns a new chan with log feed and a CancelFunc.
func (l *Logger) Subscribe() (<-chan Entry, CancelFunc) {
	feed := make(chan Entry)

	select {
	case <-l.Ctx.Done():
		close(feed)
		return feed, func() {}
	case l.sub <- feed:
	}

	cancel := func() {
		l.unSubscribe(feed)
	}
	return feed, cancel
}

func (l *Logger) unSubscribe(feed chan Entry) {
	// Read feed until unsub request is accepted.
	for {
		select {
		case l.unsub <- feed:
			return
		case <-feed:
		}
	}
}

// LogToWriter prints log feed to writer.
func (l *Logger) LogToWriter(ctx context.Context, out io.Writer) {
	feed, cancel := l.Subscribe()
	defer cancel()

	l.wg.Add(1)
	for {
		select {
		case entry := <-feed:
			fmt.Fprintln(out, entry)
		case <-ctx.Done():
			l.wg.Done()
			return
		}
	}
}

type testLogger chan string

func (logger *testLogger) Log(log Entry) {
	if *logger != nil {
		*logger <- log.Msg
	}
}

// NewMockLogger creates a ILogger used for testing.
func NewMockLogger() (ILogger, chan string) {
	logger := make(testLogger)
	return &logger, logger
}

// NewDummyLogger creates a dummy logger.
func NewDummyLogger() ILogger {
	return new(testLogger)
}
