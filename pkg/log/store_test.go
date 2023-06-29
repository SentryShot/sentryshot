// SPDX-License-Identifier: GPL-2.0-or-later

package log

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestStore(t testing.TB, logDir string) *Store {
	if logDir == "" {
		logDir = t.TempDir()
	}
	logDB, err := NewStore(logDir, &sync.WaitGroup{}, nil)
	require.NoError(t, err)

	return logDB
}

func TestQuery(t *testing.T) {
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

	store := newTestStore(t, "")

	// Populate database.
	time.Sleep(1 * time.Millisecond)
	store.saveLog(msg3)
	store.saveLog(msg2)
	store.saveLog(msg1)
	// logDB.saveLog(msg4)
	time.Sleep(10 * time.Millisecond)

	cases := map[string]struct {
		input    Query
		expected []Entry
	}{
		"singleLevel": {
			input: Query{
				Levels:  []Level{LevelWarning},
				Sources: []string{"s1"},
			},
			expected: []Entry{msg2},
		},
		"multipleLevels": {
			input: Query{
				Levels:  []Level{LevelError, LevelWarning},
				Sources: []string{"s1"},
			},
			expected: []Entry{msg1, msg2},
		},
		"singleSource": {
			input: Query{
				Levels:  []Level{LevelError, LevelInfo},
				Sources: []string{"s1"},
			},
			expected: []Entry{msg1},
		},
		"multipleSources": {
			input: Query{
				Levels:  []Level{LevelError, LevelInfo},
				Sources: []string{"s1", "s2"},
			},
			expected: []Entry{msg1, msg3},
		},
		"singleMonitor": {
			input: Query{
				Levels:   []Level{LevelError, LevelInfo},
				Sources:  []string{"s1", "s2"},
				Monitors: []string{"m1"},
			},
			expected: []Entry{msg1},
		},
		"multipleMonitors": {
			input: Query{
				Levels:   []Level{LevelError, LevelInfo},
				Sources:  []string{"s1", "s2"},
				Monitors: []string{"m1", "m2"},
			},
			expected: []Entry{msg1, msg3},
		},
		"all": {
			input: Query{
				Levels:  []Level{LevelError, LevelWarning, LevelInfo, LevelDebug},
				Sources: []string{"s1", "s2"},
			},
			expected: []Entry{msg1, msg2, msg3},
		},
		"none": {
			input:    Query{},
			expected: []Entry{msg1, msg2, msg3},
		},
		"limit": {
			input: Query{
				Levels:  []Level{LevelError, LevelWarning, LevelInfo, LevelDebug},
				Sources: []string{"s1", "s2"},
				Limit:   2,
			},
			expected: []Entry{msg1, msg2},
		},
		"limit2": {
			input: Query{
				Levels: []Level{LevelInfo},
				Limit:  1,
			},
			expected: []Entry{msg3},
		},
		"exactTime": {
			input: Query{
				Levels:  []Level{LevelError, LevelWarning, LevelInfo, LevelDebug},
				Sources: []string{"s1", "s2"},
				Time:    4000,
			},
			expected: []Entry{msg2, msg3},
		},
		"time": {
			input: Query{
				Levels:  []Level{LevelError, LevelWarning, LevelInfo, LevelDebug},
				Sources: []string{"s1", "s2"},
				Time:    3500,
			},
			expected: []Entry{msg2, msg3},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			logs, err := store.Query(tc.input)
			require.NoError(t, err)

			require.Equal(t, tc.expected, logs)
		})
	}

	t.Run("noEntries", func(t *testing.T) {
		store := newTestStore(t, "")
		entries, err := store.Query(Query{})
		require.NoError(t, err)
		require.Equal(t, 0, len(entries))
	})

	t.Run("writeAndRead", func(t *testing.T) {
		store := newTestStore(t, "")

		msg1 := Entry{Time: 1}
		msg2 := Entry{Time: 2}
		msg3 := Entry{Time: 3}

		store.saveLog(msg1)
		entries, err := store.Query(Query{})
		require.NoError(t, err)
		require.Equal(t, msg1, entries[0])

		store.saveLog(msg2)
		entries, err = store.Query(Query{})
		require.NoError(t, err)
		require.Equal(t, []Entry{msg2, msg1}, entries)

		store.saveLog(msg3)
		entries, err = store.Query(Query{})
		require.NoError(t, err)
		require.Equal(t, []Entry{msg3, msg2, msg1}, entries)
	})
	t.Run("multipleChunks", func(t *testing.T) {
		store := newTestStore(t, "")

		msg1 := Entry{Time: 1}
		msg2 := Entry{Time: chunkDuration}
		msg3 := Entry{Time: chunkDuration * 2}

		store.saveLog(msg1)
		store.saveLog(msg2)
		store.saveLog(msg3)

		entries, err := store.Query(Query{})
		require.NoError(t, err)
		require.Equal(t, []Entry{msg3, msg2, msg1}, entries)
	})
	t.Run("recoverMsgPos", func(t *testing.T) {
		logDir := t.TempDir()

		store := newTestStore(t, logDir)
		store.saveLog(Entry{Time: 1, Msg: "a"})

		store = newTestStore(t, logDir)
		store.saveLog(Entry{Time: 2, Msg: "b"})

		expected := []Entry{
			{Time: 2, Msg: "b"},
			{Time: 1, Msg: "a"},
		}
		actual, err := store.Query(Query{})
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		expectedFile := []byte{'a', '\n', 'b', '\n'}
		actualFile, err := os.ReadFile(filepath.Join(logDir, "00000.msg"))
		require.NoError(t, err)
		require.Equal(t, expectedFile, actualFile)
	})
	t.Run("order", func(t *testing.T) {
		logDir := t.TempDir()

		store := newTestStore(t, logDir)
		store.saveLog(Entry{Time: 100})

		store = newTestStore(t, logDir)
		store.saveLog(Entry{Time: 90})
		store.saveLog(Entry{Time: 120})
		store.saveLog(Entry{Time: 0})

		expected := []Entry{
			{Time: 121},
			{Time: 120},
			{Time: 101},
			{Time: 100},
		}
		actual, err := store.Query(Query{})
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	})
	t.Run("search", func(t *testing.T) {
		store := newTestStore(t, "")

		msg1 := Entry{Time: 1}
		msg2 := Entry{Time: 2}
		msg3 := Entry{Time: 3}
		msg4 := Entry{Time: 4}
		msg5 := Entry{Time: chunkDuration}
		msg6 := Entry{Time: chunkDuration + 1}
		msg7 := Entry{Time: chunkDuration + 2}
		msg8 := Entry{Time: chunkDuration * 2}
		msg9 := Entry{Time: chunkDuration*2 + 1}

		store.saveLog(msg1)
		store.saveLog(msg2)
		store.saveLog(msg3)
		store.saveLog(msg4)
		store.saveLog(msg5)
		store.saveLog(msg6)
		store.saveLog(msg7)
		store.saveLog(msg8)
		store.saveLog(msg9)

		cases := []struct {
			input  UnixMicro
			output []Entry
		}{
			{0, []Entry{msg9, msg8, msg7, msg6, msg5, msg4, msg3, msg2, msg1}},
			{msg9.Time + 1, []Entry{msg9, msg8, msg7, msg6, msg5, msg4, msg3, msg2, msg1}},
			{msg9.Time, []Entry{msg8, msg7, msg6, msg5, msg4, msg3, msg2, msg1}},
			{msg8.Time, []Entry{msg7, msg6, msg5, msg4, msg3, msg2, msg1}},
			{msg8.Time - 1, []Entry{msg7, msg6, msg5, msg4, msg3, msg2, msg1}},
			{msg7.Time + 1, []Entry{msg7, msg6, msg5, msg4, msg3, msg2, msg1}},
			{msg7.Time, []Entry{msg6, msg5, msg4, msg3, msg2, msg1}},
			{msg6.Time, []Entry{msg5, msg4, msg3, msg2, msg1}},
			{msg5.Time, []Entry{msg4, msg3, msg2, msg1}},
			{msg5.Time - 1, []Entry{msg4, msg3, msg2, msg1}},
			{msg4.Time + 1, []Entry{msg4, msg3, msg2, msg1}},
			{msg4.Time, []Entry{msg3, msg2, msg1}},
			{msg3.Time, []Entry{msg2, msg1}},
			{msg2.Time, []Entry{msg1}},
			{msg1.Time, nil},
		}

		for _, tc := range cases {
			actual, err := store.Query(Query{Time: tc.input})
			require.NoError(t, err)
			require.Equal(t, tc.output, actual)
		}
	})
}

func TestNewStore(t *testing.T) {
	t.Run("mkdir", func(t *testing.T) {
		tempDir := t.TempDir()
		newDir := filepath.Join(tempDir, "test")
		require.NoDirExists(t, newDir)

		_, err := NewStore(newDir, &sync.WaitGroup{}, nil)
		require.NoError(t, err)

		require.DirExists(t, newDir)
	})
}

func TestEncodeAndDecodeEntry(t *testing.T) {
	testEntry := Entry{
		Level:     LevelDebug,
		Src:       "abcdefgh",
		MonitorID: "aabbccddeeffgghhiijjkkll",
		Time:      5,
		Msg:       "a",
	}

	t.Run("encode", func(t *testing.T) {
		buf := make([]byte, dataSize)
		msgBuf := &writeSeeker{}
		msgPos := uint32(0)
		err := encodeEntry(buf, testEntry, msgBuf, &msgPos)
		require.NoError(t, err)

		expected := []byte{
			0, 0, 0, 0, 0, 0, 0, 5, // Time.
			'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', // Src.
			'a', 'a', 'b', 'b', 'c', 'c', 'd', 'd', // Monitor ID.
			'e', 'e', 'f', 'f', 'g', 'g', 'h', 'h',
			'i', 'i', 'j', 'j', 'k', 'k', 'l', 'l',
			0, 0, 0, 0, // Message offset.
			0, 1, // Message size.
			48, // Level.
		}
		require.Equal(t, expected, buf)
		require.Equal(t, msgBuf.buf, []byte{'a', '\n'})
		require.Equal(t, uint32(len(testEntry.Msg)+1), msgPos)
	})

	t.Run("decode", func(t *testing.T) {
		buf := make([]byte, dataSize)
		msgBuf := &writeSeeker{}

		msgPos := uint32(10)
		_, err := msgBuf.Seek(int64(msgPos), io.SeekStart)

		require.NoError(t, err)
		err = encodeEntry(buf, testEntry, msgBuf, &msgPos)
		require.NoError(t, err)

		entry, _, err := decodeEntry(buf, bytes.NewReader(msgBuf.buf))
		require.NoError(t, err)
		require.Equal(t, testEntry, *entry)
	})
}

func TestTimeToID(t *testing.T) {
	cases := []struct {
		input  UnixMicro
		output string
	}{
		{0, "00000"},
		{1000334455000111, "10003"},
		{1122334455000111, "11223"},
		{chunkDuration - 1, "00000"},
		{chunkDuration, "00001"},
		{chunkDuration + 1, "00001"},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(int(tc.input)), func(t *testing.T) {
			id, err := timeToID(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.output, id)
		})
	}

	t.Run("error", func(t *testing.T) {
		id, err := timeToID(12345678901234567)
		require.Zero(t, id)
		require.ErrorIs(t, err, ErrInvalidTime)
	})
}

type writeSeeker struct {
	buf []byte
	pos int
}

func (m *writeSeeker) Write(p []byte) (int, error) {
	minCap := m.pos + len(p)
	if minCap > cap(m.buf) {
		buf2 := make([]byte, len(m.buf), minCap+len(p))
		copy(buf2, m.buf)
		m.buf = buf2
	}
	if minCap > len(m.buf) {
		m.buf = m.buf[:minCap]
	}
	copy(m.buf[m.pos:], p)
	m.pos += len(p)
	return len(p), nil
}

func (m *writeSeeker) Seek(offset int64, whence int) (int64, error) {
	if whence != io.SeekStart {
		panic("not implemented")
	}
	newPos := int(offset)
	if newPos < 0 {
		return 0, errors.New("negative result pos")
	}
	m.pos = newPos
	return int64(newPos), nil
}

func BenchmarkDBInsert(b *testing.B) {
	store := newTestStore(b, "")

	testEntry := Entry{
		Level:     LevelDebug,
		Msg:       "....................................",
		Src:       "monitor",
		MonitorID: "abcde",
	}

	b.ResetTimer()
	for i := 0; i < 100000; i++ {
		testEntry.Time = UnixMicro(i)
		err := store.saveLog(testEntry)
		require.NoError(b, err)
	}
}

func BenchmarkDBQuery(b *testing.B) {
	store := newTestStore(b, "")

	testEntry := Entry{
		Level:     LevelDebug,
		Msg:       "....................................",
		Src:       "monitor",
		MonitorID: "abcde",
	}

	for i := 0; i < 10000; i++ {
		testEntry.Time = UnixMicro(i)
		err := store.saveLog(testEntry)
		require.NoError(b, err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entries, err := store.Query(Query{
			Limit:    1,
			Monitors: []string{""},
		})
		require.NoError(b, err)
		require.Equal(b, 0, len(entries))
	}
}

func TestNewChunkDecoder(t *testing.T) {
	t.Run("versionErr", func(t *testing.T) {
		logDir := t.TempDir()
		chunkID := "0"

		err := os.WriteFile(filepath.Join(logDir, "0.data"), []byte{255}, 0o600)
		require.NoError(t, err)

		_, err = newChunkDecoder(logDir, chunkID)
		require.ErrorIs(t, err, ErrUnknownChunkVersion)
	})
}

func TestNewChunkEncoder(t *testing.T) {
	t.Run("versionErr", func(t *testing.T) {
		logDir := t.TempDir()
		chunkID := "0"

		err := os.WriteFile(filepath.Join(logDir, "0.data"), []byte{255}, 0o600)
		require.NoError(t, err)

		_, _, err = newChunkEncoder(logDir, chunkID)
		require.ErrorIs(t, err, ErrUnknownChunkVersion)
	})
}

func TestPurge(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		stubGetDiskSpace := func() (int64, error) {
			return 10000, nil
		}
		logDir := t.TempDir()
		s := Store{
			logDir:       logDir,
			getDiskSpace: stubGetDiskSpace,
			minDiskUsage: 0,
		}

		writeTestChunk(t, logDir, "00000")
		writeTestChunk(t, logDir, "11111")

		files := listFiles(t, logDir)
		expected := []string{"00000.data", "00000.msg", "11111.data", "11111.msg"}
		require.Equal(t, expected, files)

		require.NoError(t, s.purge())

		files = listFiles(t, logDir)
		expected = []string{"11111.data", "11111.msg"}
		require.Equal(t, expected, files)
	})
	t.Run("diskSpace", func(t *testing.T) {
		stubGetDiskSpace := func() (int64, error) {
			return 10000, nil
		}
		logDir := t.TempDir()
		s := Store{
			logDir:       logDir,
			getDiskSpace: stubGetDiskSpace,
			minDiskUsage: 0,
		}

		writeTestChunk(t, logDir, "00000")
		writeTestChunk(t, logDir, "11111")
		require.Equal(t, 2, chunkCount(t, logDir))

		require.NoError(t, s.purge())
		require.NoError(t, s.purge())
		require.Equal(t, 1, chunkCount(t, logDir))
	})
	t.Run("minDiskUsage", func(t *testing.T) {
		stubGetDiskSpace := func() (int64, error) {
			return 0, nil
		}
		logDir := t.TempDir()
		s := Store{
			logDir:       logDir,
			getDiskSpace: stubGetDiskSpace,
			minDiskUsage: 100,
		}

		writeTestChunk(t, logDir, "00000")
		writeTestChunk(t, logDir, "11111")
		require.Equal(t, 2, chunkCount(t, logDir))

		require.NoError(t, s.purge())
		require.NoError(t, s.purge())
		require.Equal(t, 1, chunkCount(t, logDir))
	})
	t.Run("noFiles", func(t *testing.T) {
		stubGetDiskSpace := func() (int64, error) {
			return 0, nil
		}
		logDir := t.TempDir()
		s := Store{
			logDir:       logDir,
			getDiskSpace: stubGetDiskSpace,
			minDiskUsage: 0,
		}
		require.Equal(t, 0, chunkCount(t, logDir))
		require.NoError(t, s.purge())
	})
	t.Run("diskSpaceErr", func(t *testing.T) {
		stubError := errors.New("stub")
		stubGetDiskSpace := func() (int64, error) {
			return 0, stubError
		}
		logDir := t.TempDir()
		s := Store{
			logDir:       logDir,
			getDiskSpace: stubGetDiskSpace,
			minDiskUsage: 0,
		}
		writeTestChunk(t, logDir, "00000")

		err := s.purge()
		require.ErrorIs(t, err, stubError)
	})
}

// Each chunk is 100 bytes.
func writeTestChunk(t *testing.T, logDir, chunkID string) {
	t.Helper()
	dataPath, msgPath := chunkIDToPaths(logDir, chunkID)

	err := os.WriteFile(dataPath, bytes.Repeat([]byte{0}, 50), 0o600)
	require.NoError(t, err)

	err = os.WriteFile(msgPath, bytes.Repeat([]byte{0}, 50), 0o600)
	require.NoError(t, err)
}

func chunkCount(t *testing.T, logDir string) int {
	t.Helper()
	s := Store{logDir: logDir}
	chunks, err := s.listChunks()
	require.NoError(t, err)
	return len(chunks)
}

func listFiles(t *testing.T, path string) []string {
	files, err := os.ReadDir(path)
	require.NoError(t, err)

	var fileNames []string
	for _, file := range files {
		fileNames = append(fileNames, file.Name())
	}
	return fileNames
}
