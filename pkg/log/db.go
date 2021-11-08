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
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	dbAPIversion = "1"
	// dbAPIversion = "-1" // Testing.
)

const defaultMaxKeys = 100000

// NewDB new log database.
func NewDB(dbPath string, wg *sync.WaitGroup) *DB {
	return &DB{
		dbPath:  dbPath,
		maxKeys: defaultMaxKeys,

		wg:     wg,
		saveWG: &sync.WaitGroup{},
	}
}

// DB log database.
type DB struct {
	dbPath  string
	maxKeys int

	db *bolt.DB
	wg *sync.WaitGroup

	// Wait for last log to be saved before losing db.
	saveWG *sync.WaitGroup
}

// Init initialize database.
func (logDB *DB) Init(ctx context.Context) error {
	dbOpts := &bolt.Options{
		Timeout: 1 * time.Second,
	}

	db, err := bolt.Open(logDB.dbPath, 0o600, dbOpts)
	if err != nil {
		return fmt.Errorf("could not open database: %w: %v", err, logDB.dbPath)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(dbAPIversion))
		return err
	})
	if err != nil {
		db.Close()
		return fmt.Errorf("could not create bucket: %v, %w", dbAPIversion, err)
	}

	logDB.db = db

	logDB.wg.Add(1)
	go func() {
		<-ctx.Done()
		logDB.saveWG.Wait()
		db.Close()
		logDB.wg.Done()
	}()

	return nil
}

// SaveLogs saves logs from the logger into the database.
func (logDB *DB) SaveLogs(ctx context.Context, l *Logger) {
	feed, cancel := l.Subscribe()
	defer cancel()

	logDB.saveWG.Add(1)
	for {
		select {
		case <-ctx.Done():
			logDB.saveWG.Done()
			return
		case log := <-feed:
			if err := logDB.saveLog(log); err != nil {
				fmt.Fprintf(os.Stderr, "could not save log: %v %v", log.Msg, err)
				l.Error().Src("app").Msgf("could not save log: '%v' %v", log.Msg, err)
			}
		}
	}
}

func (logDB *DB) saveLog(log Log) error {
	key := encodeKey(uint64(log.Time))
	value := encodeValue(log)

	return logDB.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(dbAPIversion))

		if b.Stats().KeyN >= logDB.maxKeys {
			if err := deleteFirstKey(b); err != nil {
				return fmt.Errorf("could not delete first key: %w", err)
			}
		}
		return b.Put(key, value)
	})
}

func deleteFirstKey(b *bolt.Bucket) error {
	k, _ := b.Cursor().First()
	return b.Delete(k)
}

// Query database query.
type Query struct {
	Levels   []Level
	Time     UnixMillisecond
	Sources  []string
	Monitors []string
	Limit    int
}

// Query logs in database.
func (logDB *DB) Query(q Query) (*[]Log, error) {
	var logs []Log

	err := logDB.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(dbAPIversion))
		c := b.Cursor()

		var log Log
		filterLog := func(rawLog []byte) error {
			if err := json.Unmarshal(rawLog, &log); err != nil {
				return fmt.Errorf("could not unmarshal log: %w", err)
			}

			if !levelInLevels(log.Level, q.Levels) {
				return nil
			}
			if !stringInStrings(log.Src, q.Sources) {
				return nil
			}
			if !stringInStrings(log.Monitor, q.Monitors) {
				return nil
			}

			logs = append(logs, log)
			return nil
		}

		if q.Time == 0 {
			_, value := c.Last()
			if err := filterLog(value); err != nil {
				return err
			}
		} else {
			c.Seek(encodeKey(uint64(q.Time)))
		}

		limit := q.Limit
		if limit == 0 {
			limit = defaultMaxKeys
		}

		for len(logs) < limit {
			key, value := c.Prev()
			if key == nil {
				return nil
			}
			if err := filterLog(value); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &logs, nil
}

func levelInLevels(level Level, levels []Level) bool {
	if levels == nil {
		return true
	}
	for _, l := range levels {
		if l == level {
			return true
		}
	}
	return false
}

func stringInStrings(source string, sources []string) bool {
	if sources == nil {
		return true
	}
	for _, src := range sources {
		if src == source {
			return true
		}
	}
	return false
}

func encodeKey(key uint64) []byte {
	output := make([]byte, 8)
	binary.BigEndian.PutUint64(output, key)
	return output
}

func encodeValue(log Log) []byte {
	value, _ := json.Marshal(log)
	return value
}
