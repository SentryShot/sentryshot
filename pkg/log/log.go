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
	"fmt"
)

// Logger logs.
type Logger struct {
	feed  chan string      // feed of logs.
	sub   chan chan string // subscribe requests.
	unsub chan chan string // unsubscribe requests.
}

// NewLogger starts and returns Logger.
func NewLogger(ctx context.Context) *Logger {
	logger := &Logger{
		feed:  make(chan string),
		sub:   make(chan chan string),
		unsub: make(chan chan string),
	}
	go logger.start(ctx)
	return logger
}

func (l *Logger) start(ctx context.Context) {
	subs := map[chan string]struct{}{}
	for {
		select {
		case <-ctx.Done():
			return

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
}

// CancelFunc cancels log feed subsciption.
type CancelFunc func()

// Subscribe returns a new chan with log feed and a CancelFunc.
func (l *Logger) Subscribe() (<-chan string, CancelFunc) {
	feed := make(chan string)
	l.sub <- feed

	cancel := func() {
		l.unSubscribe(feed)
	}
	return feed, cancel
}

func (l *Logger) unSubscribe(feed chan string) {
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
	for {
		select {
		case msg := <-feed:
			fmt.Print("log " + msg)
		case <-ctx.Done():
			return
		}
	}
}

// Print formats using the default formats for its operands and writes to
// the log feed. Spaces are added between operands when neither is a string.
func (l *Logger) Print(v ...interface{}) {
	l.feed <- fmt.Sprint(v...)
}

// Printf formats according to a format specifier and writes to the log feed.
func (l *Logger) Printf(format string, v ...interface{}) {
	l.feed <- fmt.Sprintf(format, v...)
}

// Println formats using the default formats for its operands
// and writes to the log feed. Spaces are always added
// between operands and a newline is appended.
func (l *Logger) Println(v ...interface{}) {
	l.feed <- fmt.Sprintln(v...)
}
