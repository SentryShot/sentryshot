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
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func newTestDB(t *testing.T) (*DB, func()) {
	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)

	dbPath := filepath.Join(tempDir, "logs.db")
	logDB := NewDB(dbPath, &sync.WaitGroup{})

	ctx, cancel := context.WithCancel(context.Background())
	logDB.Init(ctx)
	require.NoError(t, err)

	return logDB, cancel
}

func TestQuery(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		msg1 := Entry{
			Level:     LevelError,
			Src:       "s1",
			MonitorID: "m1",
			Msg:       "msg1",
			Time:      4000,
		}
		msg2 := Entry{
			Level: LevelWarning,
			Src:   "s1",
			Msg:   "msg2",
			Time:  3000,
		}
		msg3 := Entry{
			Level:     LevelInfo,
			Src:       "s2",
			MonitorID: "m2",
			Msg:       "msg3",
			Time:      2000,
		}
		/*msg4 := Log{
			Level: LevelDebug,
			Src:   "s2",
			Msg:   "msg4",
			Time:  1000,
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

		cases := map[string]struct {
			input    Query
			expected *[]Entry
		}{
			"singleLevel": {
				input: Query{
					Levels:  []Level{LevelWarning},
					Sources: []string{"s1"},
				},
				expected: &[]Entry{msg2},
			},
			"multipleLevels": {
				input: Query{
					Levels:  []Level{LevelError, LevelWarning},
					Sources: []string{"s1"},
				},
				expected: &[]Entry{msg1, msg2},
			},
			"singleSource": {
				input: Query{
					Levels:  []Level{LevelError, LevelInfo},
					Sources: []string{"s1"},
				},
				expected: &[]Entry{msg1},
			},
			"multipleSources": {
				input: Query{
					Levels:  []Level{LevelError, LevelInfo},
					Sources: []string{"s1", "s2"},
				},
				expected: &[]Entry{msg1, msg3},
			},
			"singleMonitor": {
				input: Query{
					Levels:   []Level{LevelError, LevelInfo},
					Sources:  []string{"s1", "s2"},
					Monitors: []string{"m1"},
				},
				expected: &[]Entry{msg1},
			},
			"multipleMonitors": {
				input: Query{
					Levels:   []Level{LevelError, LevelInfo},
					Sources:  []string{"s1", "s2"},
					Monitors: []string{"m1", "m2"},
				},
				expected: &[]Entry{msg1, msg3},
			},
			"all": {
				input: Query{
					Levels:  []Level{LevelError, LevelWarning, LevelInfo, LevelDebug},
					Sources: []string{"s1", "s2"},
				},
				expected: &[]Entry{msg1, msg2, msg3},
			},
			"none": {
				input:    Query{},
				expected: &[]Entry{msg1, msg2, msg3},
			},
			"limit": {
				input: Query{
					Levels:  []Level{LevelError, LevelWarning, LevelInfo, LevelDebug},
					Sources: []string{"s1", "s2"},
					Limit:   2,
				},
				expected: &[]Entry{msg1, msg2},
			},
			"limit2": {
				input: Query{
					Levels: []Level{LevelInfo},
					Limit:  1,
				},
				expected: &[]Entry{msg3},
			},
			"exactTime": {
				input: Query{
					Levels:  []Level{LevelError, LevelWarning, LevelInfo, LevelDebug},
					Sources: []string{"s1", "s2"},
					Time:    4000,
				},
				expected: &[]Entry{msg2, msg3},
			},
			"time": {
				input: Query{
					Levels:  []Level{LevelError, LevelWarning, LevelInfo, LevelDebug},
					Sources: []string{"s1", "s2"},
					Time:    3500,
				},
				expected: &[]Entry{msg2, msg3},
			},
		}

		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				logs, err := logDB.Query(tc.input)
				require.NoError(t, err)

				require.Equal(t, tc.expected, logs)
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
		require.NoError(t, err)

		_, err = logDB.Query(Query{})
		var e *json.SyntaxError
		require.ErrorAs(t, err, &e)

		err = logDB.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(dbAPIversion))
			return b.Put([]byte("valid"), []byte(encodeValue(Entry{})))
		})
		require.NoError(t, err)

		_, err = logDB.Query(Query{})
		require.ErrorAs(t, err, &e)
	})
}

func TestDB(t *testing.T) {
	t.Run("maxKeys", func(t *testing.T) {
		logDB, cancel := newTestDB(t)
		defer cancel()

		logDB.maxKeys = 3

		logDB.db.View(func(tx *bolt.Tx) error {
			keyN := tx.Bucket([]byte(dbAPIversion)).Stats().KeyN
			require.Equal(t, keyN, 0, "database is not empty")
			return nil
		})

		logDB.saveLog(Entry{Time: 1})
		logDB.saveLog(Entry{Time: 2})
		logDB.saveLog(Entry{Time: 3})
		logDB.saveLog(Entry{Time: 4})
		logDB.saveLog(Entry{Time: 5})

		logDB.db.View(func(tx *bolt.Tx) error {
			keyN := tx.Bucket([]byte(dbAPIversion)).Stats().KeyN
			require.Equal(t, logDB.maxKeys, keyN)
			return nil
		})
	})
	t.Run("mkdir", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		newDir := filepath.Join(tempDir, "test")
		require.NoDirExists(t, newDir)

		dbPath := filepath.Join(newDir, "logs.db")
		logDB := NewDB(dbPath, &sync.WaitGroup{})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err = logDB.Init(ctx)
		require.NoError(t, err)

		require.DirExists(t, newDir)
	})
	t.Run("mkdirError", func(t *testing.T) {
		logDB := NewDB("/dev/null/nil", &sync.WaitGroup{})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := logDB.Init(ctx)
		require.Error(t, err)
	})
	t.Run("openDBerr", func(t *testing.T) {
		logDB := &DB{
			dbPath: "/dev/null",
		}
		err := logDB.Init(context.Background())
		require.Error(t, err)
	})
}
