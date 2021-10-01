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

// API inspired by zerolog https://github.com/rs/zerolog

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3" // sqlite3 driver.
)

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

	e.logger.feed <- log
}

// Msgf sends the event with formatted msg added as the message field.
func (e *Event) Msgf(format string, v ...interface{}) {
	e.Msg(fmt.Sprintf(format, v...))
}

// Feed defines feed of logs.
type Feed <-chan Log
type logFeed chan Log

// Logger logs.
type Logger struct {
	feed  logFeed      // feed of logs.
	sub   chan logFeed // subscribe requests.
	unsub chan logFeed // unsubscribe requests.

	wg     *sync.WaitGroup
	db     *sql.DB
	dbPath string
}

// NewLogger starts and returns Logger.
func NewLogger(dbPath string, wg *sync.WaitGroup) (*Logger, error) {
	if err := checkDB(dbPath); err != nil {
		return nil, err
	}

	// Move db
	return &Logger{
		feed:  make(logFeed),
		sub:   make(chan logFeed),
		unsub: make(chan logFeed),

		wg:     wg,
		dbPath: dbPath,
	}, nil
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

const dbAPIversion = -1 // testing

func checkDB(dbPath string) error {
	if !dirExist(dbPath) {
		return createDB(dbPath)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("could not open database: %v", err)
	}
	defer db.Close()

	rows, err := db.Query("PRAGMA user_version;")
	if err != nil {
		return err
	}
	defer rows.Close()

	var version int
	rows.Next()
	if err = rows.Scan(&version); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if version != dbAPIversion {
		return fmt.Errorf("invalid database version: %v", dbPath)
	}

	return nil
}

func createDB(dbPath string) error {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("could not create database: %v", err)
	}
	defer db.Close()

	sqlStmt := "create table logs (" +
		"time INTEGER not null," +
		" level INTEGER not null," +
		" src TEXT not null," +
		" monitor TEXT," +
		" msg TEXT not null);"

	_, err = db.Exec(sqlStmt)
	if err != nil {
		return fmt.Errorf("could not create table in database: %v", err)
	}

	_, err = db.Exec("PRAGMA user_version = " + strconv.Itoa(dbAPIversion))
	if err != nil {
		return fmt.Errorf("could set database api version: %v", err)
	}

	return nil
}

// Start logger.
func (l *Logger) Start(ctx context.Context) error {
	db, err := sql.Open("sqlite3", l.dbPath)
	if err != nil {
		return fmt.Errorf("could not open database: %v", err)
	}
	l.db = db

	l.wg.Add(1)
	go func() {
		subs := map[logFeed]struct{}{}
		for {
			select {
			case <-ctx.Done():
				db.Close()
				l.wg.Done()
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
	}()
	return nil
}

// CancelFunc cancels log feed subsciption.
type CancelFunc func()

// Subscribe returns a new chan with log feed and a CancelFunc.
func (l *Logger) Subscribe() (<-chan Log, CancelFunc) {
	feed := make(logFeed)
	l.sub <- feed

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
	for {
		select {
		case log := <-feed:
			printLog(log)
		case <-ctx.Done():
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
		output += strings.Title(log.Src) + ": "
	}

	output += log.Msg
	fmt.Println(output)
}

// LogToDB prints log feed sqlite database.
func (l *Logger) LogToDB(ctx context.Context) {
	feed, cancel := l.Subscribe()
	defer cancel()
	for {
		select {
		case log := <-feed:
			if err := saveLogToDB(log, l.db); err != nil {
				fmt.Fprintf(os.Stderr, "could not save log: %v %v", log.Msg, err)
				l.Error().Src("app").Msgf("could not save log: '%v' %v", log.Msg, err)
			}
		case <-ctx.Done():
			return
		}
	}
}

const maxRows = "100000"

func saveLogToDB(log Log, db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("could not start transaction: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Insert log.
	insertStmt, err := tx.Prepare("insert into logs(time, level, src, monitor, msg) values(?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare: %v", err)
	}
	defer insertStmt.Close()

	// GO1.17 replace UnixNano with UnixMicro.
	_, err = insertStmt.Exec(log.Time, log.Level, log.Src, log.Monitor, log.Msg)
	if err != nil {
		return fmt.Errorf("exec: %v", err)
	}

	// Maintain table size.
	sqlStmt := "DELETE FROM logs WHERE NOT rowid IN " +
		"(SELECT rowid FROM `logs` ORDER BY `time` DESC LIMIT " + maxRows + ");"

	if _, err = tx.Exec(sqlStmt); err != nil {
		return fmt.Errorf("prepare: %v", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit transtaction: %v", err)
	}

	return nil
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

func dirExist(path string) bool {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false
		}
		return false
	}
	return true
}
