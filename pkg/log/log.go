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
	"sync"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Func logging function.
type Func func(level Level, format string, a ...interface{})

// Level defines log level.
type Level uint8

// Logging constants, matching ffmpeg.
const (
	LevelError   Level = 16
	LevelWarning Level = 24
	LevelInfo    Level = 32
	LevelDebug   Level = 48
)

// UnixMillisecond .
type UnixMillisecond uint64

// Event defines log event.
type Event struct {
	level   Level
	time    UnixMillisecond // Timestamp.
	src     string          // Source.
	monitor string          // Source monitor id.

	logger *Logger
}

// Log defines log entry.
type Log struct {
	Level   Level
	Time    UnixMillisecond // Timestamp.
	Msg     string          // Message
	Src     string          // Source.
	Monitor string          // Source monitor id.
}

// Level starts a new message with provided level.
// You must call Msg on the returned event in order to send the event.
func (l *Logger) Level(level Level) *Event {
	return &Event{
		level:  level,
		time:   UnixMillisecond(time.Now().UnixNano() / 1000),
		logger: l,
	}
}

// GetTime as time.Time.
func (l Log) GetTime() time.Time {
	return time.Unix(0, int64(l.Time*1000))
}

// Error starts a new message with error level.
// You must call Msg on the returned event in order to send the event.
func (l *Logger) Error() *Event {
	return &Event{
		level:  LevelError,
		time:   UnixMillisecond(time.Now().UnixNano() / 1000),
		logger: l,
	}
}

// Warn starts a new message with warn level.
// You must call Msg on the returned event in order to send the event.
func (l *Logger) Warn() *Event {
	return &Event{
		level:  LevelWarning,
		time:   UnixMillisecond(time.Now().UnixNano() / 1000),
		logger: l,
	}
}

// Info starts a new message with info level.
// You must call Msg on the returned event in order to send the event.
func (l *Logger) Info() *Event {
	return &Event{
		level:  LevelInfo,
		time:   UnixMillisecond(time.Now().UnixNano() / 1000),
		logger: l,
	}
}

// Debug starts a new message with debug level.
// You must call Msg on the returned event in order to send the event.
func (l *Logger) Debug() *Event {
	return &Event{
		level:  LevelDebug,
		time:   UnixMillisecond(time.Now().UnixNano() / 1000),
		logger: l,
	}
}

// FFmpegLevel start a new message with converted ffmpeg log level.
// You must call Msg on the returned event in order to send the event.
func (l *Logger) FFmpegLevel(logLevel string) *Event {
	return &Event{
		level:  FFmpegLevel(logLevel),
		time:   UnixMillisecond(time.Now().UnixNano() / 1000),
		logger: l,
	}
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

// Src sets event source.
func (e *Event) Src(source string) *Event {
	e.src = source
	return e
}

// Monitor sets event monitor.
func (e *Event) Monitor(monitorID string) *Event {
	e.monitor = monitorID
	return e
}

// Time sets event time.
func (e *Event) Time(t time.Time) *Event {
	e.time = UnixMillisecond(t.UnixNano() / 1000)
	return e
}

// Msg sends the *Event with msg added as the message field.
func (e *Event) Msg(msg string) {
	log := Log{
		Time:    e.time,
		Level:   e.level,
		Msg:     msg,
		Src:     e.src,
		Monitor: e.monitor,
	}
	select {
	case <-e.logger.Ctx.Done():
	case e.logger.feed <- log:
	}
}

// Msgf sends the event with formatted msg added as the message field.
func (e *Event) Msgf(format string, v ...interface{}) {
	e.Msg(fmt.Sprintf(format, v...))
}

// Feed defines feed of logs.
type (
	Feed    <-chan Log
	logFeed chan Log
)

var defaultSources = []string{"app", "auth", "monitor", "recorder"}

// NewLogger starts and returns Logger.
func NewLogger(wg *sync.WaitGroup, addonSources []string) *Logger {
	return &Logger{
		feed:  make(logFeed),
		sub:   make(chan logFeed),
		unsub: make(chan logFeed),

		wg:      wg,
		sources: append(defaultSources, addonSources...),
	}
}

// NewMockLogger used for testing.
func NewMockLogger() *Logger {
	return &Logger{
		feed:  make(logFeed),
		sub:   make(chan logFeed),
		unsub: make(chan logFeed),
		wg:    &sync.WaitGroup{},
	}
}

// Logger .
type Logger struct {
	feed  logFeed      // feed of logs.
	sub   chan logFeed // subscribe requests.
	unsub chan logFeed // unsubscribe requests.

	wg      *sync.WaitGroup
	Ctx     context.Context
	sources []string
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
		subs := map[logFeed]struct{}{}
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
func (l *Logger) Subscribe() (<-chan Log, CancelFunc) {
	feed := make(logFeed)

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

func (l *Logger) unSubscribe(feed logFeed) {
	// Read feed until unsub request is accepted.
	for {
		select {
		case l.unsub <- feed:
			return
		case <-feed:
		}
	}
}

// LogToStdout prints log feed to Stdout.
func (l *Logger) LogToStdout(ctx context.Context) {
	feed, cancel := l.Subscribe()
	defer cancel()

	l.wg.Add(1)
	for {
		select {
		case log := <-feed:
			printLog(log)
		case <-ctx.Done():
			l.wg.Done()
			return
		}
	}
}

func printLog(log Log) {
	var output string

	switch log.Level {
	case LevelError:
		output += "[ERROR] "
	case LevelWarning:
		output += "[WARNING] "
	case LevelInfo:
		output += "[INFO] "
	case LevelDebug:
		output += "[DEBUG] "
	}

	if log.Monitor != "" {
		output += log.Monitor + ": "
	}
	if log.Src != "" {
		srcTitle := cases.Title(language.Und).String(log.Src)
		output += srcTitle + ": "
	}

	output += log.Msg
	fmt.Println(output)
}
