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
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func newTestDB(t *testing.T) (*DB, func()) {
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatalf("could not create tempoary directory: %v", err)
	}
	dbPath := filepath.Join(tempDir, "logs.db")

	logDB := NewDB(dbPath, &sync.WaitGroup{})

	ctx, cancel := context.WithCancel(context.Background())
	if err := logDB.Init(ctx); err != nil {
		t.Fatal(err)
	}

	return logDB, cancel
}

func TestQuery(t *testing.T) {
	t.Run("working", func(t *testing.T) {
		msg1 := Log{
			Level:   LevelError,
			Time:    4000,
			Src:     "s1",
			Monitor: "m1",
			Msg:     "msg1",
		}
		msg2 := Log{
			Level: LevelWarning,
			Time:  3000,
			Src:   "s1",
			Msg:   "msg2",
		}
		msg3 := Log{
			Level:   LevelInfo,
			Time:    2000,
			Src:     "s2",
			Monitor: "m2",
			Msg:     "msg3",
		}
		/*msg4 := Log{
			Level: LevelDebug,
			Time:  1000,
			Src:   "s2",
			Msg:   "msg4",
		}*/

		logDB, cancel := newTestDB(t)
		defer cancel()

		// Populate database.
		time.Sleep(1 * time.Millisecond)
		logDB.saveLog(msg1)
		logDB.saveLog(msg2)
		logDB.saveLog(msg3)
		// logDB.saveLog(msg4)
		time.Sleep(10 * time.Millisecond)

		cases := []struct {
			name     string
			input    Query
			expected *[]Log
		}{
			{
				name: "singleLevel",
				input: Query{
					Levels:  []Level{LevelWarning},
					Sources: []string{"s1"},
				},
				expected: &[]Log{msg2},
			},
			{
				name: "multipleLevels",
				input: Query{
					Levels:  []Level{LevelError, LevelWarning},
					Sources: []string{"s1"},
				},
				expected: &[]Log{msg1, msg2},
			},
			{
				name: "singleSource",
				input: Query{
					Levels:  []Level{LevelError, LevelInfo},
					Sources: []string{"s1"},
				},
				expected: &[]Log{msg1},
			},
			{
				name: "multipleSources",
				input: Query{
					Levels:  []Level{LevelError, LevelInfo},
					Sources: []string{"s1", "s2"},
				},
				expected: &[]Log{msg1, msg3},
			},
			{
				name: "singleMonitor",
				input: Query{
					Levels:   []Level{LevelError, LevelInfo},
					Sources:  []string{"s1", "s2"},
					Monitors: []string{"m1"},
				},
				expected: &[]Log{msg1},
			},
			{
				name: "multipleMonitors",
				input: Query{
					Levels:   []Level{LevelError, LevelInfo},
					Sources:  []string{"s1", "s2"},
					Monitors: []string{"m1", "m2"},
				},
				expected: &[]Log{msg1, msg3},
			},
			{
				name: "all",
				input: Query{
					Levels:  []Level{LevelError, LevelWarning, LevelInfo, LevelDebug},
					Sources: []string{"s1", "s2"},
				},
				expected: &[]Log{msg1, msg2, msg3},
			},
			{
				name: "limit",
				input: Query{
					Levels:  []Level{LevelError, LevelWarning, LevelInfo, LevelDebug},
					Sources: []string{"s1", "s2"},
					Limit:   2,
				},
				expected: &[]Log{msg1, msg2},
			},
			{
				name: "limit2",
				input: Query{
					Levels: []Level{LevelInfo},
					Limit:  1,
				},
				expected: &[]Log{msg3},
			},

			{
				name: "exactTime",
				input: Query{
					Levels:  []Level{LevelError, LevelWarning, LevelInfo, LevelDebug},
					Sources: []string{"s1", "s2"},
					Time:    4000,
				},
				expected: &[]Log{msg2, msg3},
			},
			{
				name: "time",
				input: Query{
					Levels:  []Level{LevelError, LevelWarning, LevelInfo, LevelDebug},
					Sources: []string{"s1", "s2"},
					Time:    3500,
				},
				expected: &[]Log{msg2, msg3},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				logs, err := logDB.Query(tc.input)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				actual := fmt.Sprintf("%v", logs)
				expected := fmt.Sprintf("%v", tc.expected)

				if actual != expected {
					t.Fatalf("\nexpected:\n%v.\ngot:\n%v", expected, actual)
				}
			})
		}
	})
	t.Run("unmarshalErr", func(t *testing.T) {
		logDB, cancel := newTestDB(t)
		defer cancel()

		err := logDB.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(dbAPIversion))
			return b.Put([]byte("invalid"), []byte("nil"))
		})
		if err != nil {
			t.Fatal(err)
		}

		if _, err := logDB.Query(Query{}); err == nil {
			t.Fatalf("expected: error, got: nil.")
		}

		err = logDB.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(dbAPIversion))
			return b.Put([]byte("valid"), []byte(encodeValue(Log{})))
		})
		if err != nil {
			t.Fatalf("could not add valid value to db: %v", err)
		}

		if _, err := logDB.Query(Query{}); err == nil {
			t.Fatalf("expected: error, got: nil.")
		}
	})
}

func TestDB(t *testing.T) {
	t.Run("maxKeys", func(t *testing.T) {
		logDB, cancel := newTestDB(t)
		defer cancel()

		logDB.maxKeys = 3

		logDB.db.View(func(tx *bolt.Tx) error {
			if tx.Bucket([]byte(dbAPIversion)).Stats().KeyN != 0 {
				t.Fatalf("database is not empty")
			}
			return nil
		})

		logDB.saveLog(Log{Time: 1})
		logDB.saveLog(Log{Time: 2})
		logDB.saveLog(Log{Time: 3})
		logDB.saveLog(Log{Time: 4})
		logDB.saveLog(Log{Time: 5})

		logDB.db.View(func(tx *bolt.Tx) error {
			keyN := tx.Bucket([]byte(dbAPIversion)).Stats().KeyN
			if keyN != logDB.maxKeys {
				t.Fatalf("expected: %v number of keys, got %v", logDB.maxKeys, keyN)
			}
			return nil
		})
	})
	t.Run("openDBerr", func(t *testing.T) {
		logDB := &DB{
			dbPath: "/dev/null",
		}
		if err := logDB.Init(context.Background()); err == nil {
			t.Fatal("expected: error, got: nil")
		}
	})
}
