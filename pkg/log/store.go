// SPDX-License-Identifier: GPL-2.0-or-later

package log

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// chunk {
//     file.data
//     file.msg
// }
//
// file.data {
//     version uint8
//     []data
// }
//
// data {
//     time      uint64
//     src       [srcMaxLength]byte
//     monitorID [idMaxLength]byte
//     msgOffset uint32
//     msgSize   uint16
//     level     uint8
// }

// 166 minutes or 27.7 hours.
const (
	chunkDuration = 1000000 * second
	second        = 100000
)

const (
	chunkAPIVersion   = 0
	chunkIDLenght     = 5
	chunkHeaderLength = 1
)

const (
	dataSize     = 47
	srcMaxLength = 8
	idMaxLength  = 24
)

// Store custom log store.
type Store struct {
	logDir  string
	encoder *chunkEncoder

	// Keep track of the previous entry time to ensure
	// that the next entry will have a later time.
	prevEntryTime UnixMicro

	// Wait for last log to be saved before exiting.
	saveWG *sync.WaitGroup
	wg     *sync.WaitGroup

	logf func(string, ...interface{})

	getDiskSpace getDiskSpaceFunc
	minDiskUsage int64
}

const (
	kilobyte int64 = 1000
	megabyte       = kilobyte * 1000
)

type getDiskSpaceFunc func() (int64, error)

// NewStore new log store.
func NewStore(
	logDir string,
	wg *sync.WaitGroup,
	getDiskSpace getDiskSpaceFunc,
) (*Store, error) {
	err := os.MkdirAll(logDir, 0o770)
	if err != nil {
		return nil, fmt.Errorf("make log directory: %w", err)
	}

	logf := func(format string, a ...interface{}) {
		msg := fmt.Sprintf(format, a...)
		fmt.Printf("log store warning: %s\n", msg)
	}
	return &Store{
		logDir:       logDir,
		saveWG:       &sync.WaitGroup{},
		wg:           wg,
		logf:         logf,
		getDiskSpace: getDiskSpace,
		minDiskUsage: 100 * megabyte,
	}, nil
}

// SaveLogs saves logs from the logger into the database.
func (s *Store) SaveLogs(ctx context.Context, logger *Logger) {
	s.wg.Add(1)
	go func() {
		feed, cancel := logger.Subscribe()
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				if s.encoder != nil {
					s.encoder.close()
				}
				s.wg.Done()
				return
			case log := <-feed:
				err := s.saveLog(log)
				if err != nil {
					fmt.Printf("could not save log: %v %v\n", log.Msg, err)
				}
			}
		}
	}()
}

// PurgeLoop purges logs every hour.
func (s *Store) PurgeLoop(ctx context.Context, logger *Logger) {
	s.wg.Add(1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				s.wg.Done()
				return
			case <-time.After(1 * time.Hour):
				if err := s.purge(); err != nil {
					logger.Log(Entry{
						Level: LevelError,
						Src:   "app",
						Msg:   fmt.Sprintf("could not purge logs: %v", err),
					})
				}
			}
		}
	}()
}

func (s *Store) saveLog(entry Entry) error {
	chunkID, err := timeToID(entry.Time)
	if err != nil {
		return fmt.Errorf("time to ID: %w", err)
	}

	if s.encoder == nil || chunkID != s.encoder.chunkID {
		if s.encoder != nil {
			s.encoder.close()
		}

		var err error
		s.encoder, s.prevEntryTime, err = newChunkEncoder(s.logDir, chunkID)
		if err != nil {
			return fmt.Errorf("new chunk encoder: %w", err)
		}
	}
	if entry.Time <= s.prevEntryTime {
		entry.Time = s.prevEntryTime + 1
	}

	err = s.encoder.encode(entry)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	s.prevEntryTime = entry.Time
	return nil
}

// Query database query.
type Query struct {
	Levels   []Level
	Time     UnixMicro
	Sources  []string
	Monitors []string
	Limit    int
}

// Query logs in database.
func (s *Store) Query(q Query) ([]Entry, error) {
	chunkIDs, err := s.listChunksBefore(q.Time)
	if err != nil {
		return nil, fmt.Errorf("list chunks before: %w", err)
	}

	var entries []Entry
	for i := len(chunkIDs) - 1; i >= 0; i-- {
		chunkID := chunkIDs[i]
		err := s.queryChunk(q, &entries, chunkID)
		if err != nil {
			s.logf("query chunk %q: %v", chunkID, err)
		}
		// Time is only relevant for the first iteration.
		q.Time = 0
	}

	return entries, nil
}

func (s *Store) queryChunk(q Query, entries *[]Entry, chunkID string) error {
	decoder, err := newChunkDecoder(s.logDir, chunkID)
	if err != nil {
		return fmt.Errorf("create decoder: %w", err)
	}
	defer decoder.close()

	index := decoder.lastIndex()
	if q.Time != 0 {
		index, err = decoder.search(q.Time)
		if err != nil {
			return fmt.Errorf("seek: %w", err)
		}
		index--
	}

	for index >= 0 && (q.Limit == 0 || len(*entries) < q.Limit) {
		entry, _, err := decoder.decode(index)
		if err != nil {
			return err
		}
		if entry == nil {
			// Last entry.
			return nil
		}
		index--

		if !LevelInLevels(entry.Level, q.Levels) ||
			!StringInStrings(entry.Src, q.Sources) ||
			!StringInStrings(entry.MonitorID, q.Monitors) {
			entryChunkID, err := timeToID(entry.Time)
			if err != nil || chunkID != entryChunkID {
				continue
			}
			continue
		}
		*entries = append(*entries, *entry)
	}

	return nil
}

func (s *Store) listChunksBefore(time UnixMicro) ([]string, error) {
	chunks, err := s.listChunks()
	if err != nil {
		return nil, err
	}

	if time == 0 {
		return chunks, nil
	}

	beforeID, err := timeToID(time)
	if err != nil {
		return nil, fmt.Errorf("time to ID: %w", err)
	}

	var filtered []string
	for _, chunk := range chunks {
		if strings.Compare(chunk, beforeID) <= 0 {
			filtered = append(filtered, chunk)
		}
	}
	return filtered, nil
}

func (s *Store) listChunks() ([]string, error) {
	files, err := os.ReadDir(s.logDir)
	if err != nil {
		return nil, fmt.Errorf("stat log dir: %w", err)
	}

	var chunks []string
	for _, file := range files {
		name := file.Name()
		if len(name) < chunkIDLenght+5 || filepath.Ext(name) != ".data" {
			continue
		}
		chunks = append(chunks, name[:chunkIDLenght])
	}

	return chunks, nil
}

// purges a single chunk if needed.
func (s *Store) purge() error {
	dirSize, err := dirSize(s.logDir)
	if err != nil {
		return fmt.Errorf("dir size: %w", err)
	}
	diskSpace, err := s.getDiskSpace()
	if err != nil {
		return fmt.Errorf("get disk space: %w", err)
	}

	if dirSize <= (diskSpace/100) || dirSize <= s.minDiskUsage {
		return nil
	}

	chunks, err := s.listChunks()
	if err != nil {
		return fmt.Errorf("list chunks: %w", err)
	}
	if len(chunks) == 0 {
		return nil
	}

	chunkToRemove := chunks[0]
	dataPath, msgPath := chunkIDToPaths(s.logDir, chunkToRemove)

	err = os.Remove(dataPath)
	if err != nil {
		return fmt.Errorf("remove %q %w", dataPath, err)
	}
	os.Remove(msgPath)
	if err != nil {
		return fmt.Errorf("remove %q %w", msgPath, err)
	}

	return nil
}

func dirSize(path string) (int64, error) {
	files, err := os.ReadDir(path)
	if err != nil {
		return 0, fmt.Errorf("read dir: %w", err)
	}
	var total int64
	for _, file := range files {
		info, err := file.Info()
		if err != nil {
			return 0, fmt.Errorf("file info: %w", err)
		}
		total += info.Size()
	}
	return total, nil
}

func chunkIDToPaths(logDir, chunkID string) (string, string) {
	dataPath := filepath.Join(logDir, chunkID+".data")
	msgPath := filepath.Join(logDir, chunkID+".msg")
	return dataPath, msgPath
}

type chunkDecoder struct {
	nEntries int
	dataFile io.ReadSeekCloser
	msgFile  io.ReadSeekCloser
}

// ErrUnknownChunkVersion unknown chuck api version.
var ErrUnknownChunkVersion = errors.New("unknown chunk api version")

// Must be closed.
func newChunkDecoder(logDir, chunkID string) (*chunkDecoder, error) {
	dataPath, msgPath := chunkIDToPaths(logDir, chunkID)

	dataFile, err := os.OpenFile(dataPath, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}

	version := make([]byte, 1)
	_, err = io.ReadFull(dataFile, version)
	if err != nil {
		return nil, fmt.Errorf("read version: %w", err)
	}
	if version[0] != 0 {
		return nil, ErrUnknownChunkVersion
	}

	dataStat, err := dataFile.Stat()
	if err != nil {
		dataFile.Close()
		return nil, err
	}

	msgFile, err := os.OpenFile(msgPath, os.O_RDONLY, 0)
	if err != nil {
		dataFile.Close()
		return nil, err
	}

	return &chunkDecoder{
		msgFile:  msgFile,
		dataFile: dataFile,
		nEntries: calculateNEntries(dataStat.Size()),
	}, nil
}

func calculateNEntries(size int64) int {
	return int((size - chunkHeaderLength) / dataSize)
}

func calculateDataEnd(size int64) int64 {
	nEntries := calculateNEntries(size)
	return int64(chunkHeaderLength + (nEntries * dataSize))
}

func (c *chunkDecoder) close() {
	c.dataFile.Close()
	c.msgFile.Close()
}

func (c *chunkDecoder) lastIndex() int {
	return c.nEntries - 1
}

// Binary search.
func (c *chunkDecoder) search(time UnixMicro) (int, error) {
	l, r := 0, c.nEntries-1
	for l <= r {
		i := (l + r) / 2

		entry, _, err := c.decode(i)
		if err != nil {
			return 0, fmt.Errorf("decode: %w", err)
		}

		switch {
		case entry.Time < time:
			l = i + 1
		case entry.Time > time:
			r = i - 1
		default:
			return i, nil
		}
	}
	return l, nil
}

func (c *chunkDecoder) decode(index int) (*Entry, uint32, error) {
	entryPos := int64(chunkHeaderLength + (index * dataSize))
	_, err := c.dataFile.Seek(entryPos, io.SeekStart)
	if err != nil {
		return nil, 0, fmt.Errorf("seek: %w", err)
	}

	rawEntry := make([]byte, dataSize)
	_, err = io.ReadFull(c.dataFile, rawEntry)
	if err != nil {
		return nil, 0, fmt.Errorf("read full: %w", err)
	}

	entry, msgOffset, err := decodeEntry(rawEntry, c.msgFile)
	if err != nil {
		return nil, 0, fmt.Errorf("decode entry: %w", err)
	}
	return entry, msgOffset, nil
}

type writeSeekCloser interface {
	io.Writer
	io.Seeker
	io.Closer
}

type chunkEncoder struct {
	chunkID  string
	dataFile writeSeekCloser
	msgFile  writeSeekCloser
	msgPos   uint32
}

// Must be closed.
func newChunkEncoder(logDir, chunkID string) (*chunkEncoder, UnixMicro, error) {
	dataPath, msgPath := chunkIDToPaths(logDir, chunkID)

	dataEnd := int64(chunkHeaderLength)
	dataFileSize := getFileSize(dataPath)
	msgPos := uint32(0)
	var prevEntryTime UnixMicro
	if dataFileSize == 0 {
		err := os.WriteFile(dataPath, []byte{chunkAPIVersion}, 0o600)
		if err != nil {
			return nil, 0, fmt.Errorf("write version: %w", err)
		}
	} else {
		decoder, err := newChunkDecoder(logDir, chunkID)
		if err != nil {
			return nil, 0, fmt.Errorf("new chunk decoder: %w", err)
		}
		defer decoder.close()

		i := decoder.lastIndex()

		lastEntry, msgOffset, err := decoder.decode(i)
		if err != nil {
			return nil, 0, err
		}

		prevEntryTime = lastEntry.Time
		dataEnd = calculateDataEnd(dataFileSize)
		msgPos = msgOffset + uint32(len(lastEntry.Msg)) + 1
	}

	dataFile, err := os.OpenFile(dataPath, os.O_WRONLY, 0)
	if err != nil {
		return nil, 0, fmt.Errorf("open data file: %w", err)
	}

	_, err = dataFile.Seek(dataEnd, io.SeekStart)
	if err != nil {
		dataFile.Close()
		return nil, 0, fmt.Errorf("seek to data end: %w", err)
	}

	msgFile, err := os.OpenFile(msgPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		dataFile.Close()
		return nil, 0, fmt.Errorf("open msg file: %w", err)
	}

	_, err = msgFile.Seek(int64(msgPos), io.SeekStart)
	if err != nil {
		dataFile.Close()
		msgFile.Close()
		return nil, 0, fmt.Errorf("seek to msg end: %w", err)
	}

	encoder := &chunkEncoder{
		chunkID:  chunkID,
		msgFile:  msgFile,
		dataFile: dataFile,
		msgPos:   msgPos,
	}
	return encoder, prevEntryTime, nil
}

func getFileSize(path string) int64 {
	stat, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return stat.Size()
}

func (c *chunkEncoder) encode(entry Entry) error {
	buf := make([]byte, dataSize)
	err := encodeEntry(buf, entry, c.msgFile, &c.msgPos)
	if err != nil {
		return fmt.Errorf("encode entry: %w", err)
	}
	if _, err = c.dataFile.Write(buf); err != nil {
		return err
	}
	return nil
}

func (c *chunkEncoder) close() {
	c.dataFile.Close()
	c.msgFile.Close()
}

// Errors.
var (
	ErrSrcTooLong       = errors.New("source too long")
	ErrMonitorIDTooLong = errors.New("monitor ID too long")
)

func encodeEntry(buf []byte, entry Entry, msgFile io.Writer, msgOffset *uint32) error {
	srcLength := len(entry.Src)
	if srcLength > srcMaxLength {
		return ErrSrcTooLong
	}

	idLength := len(entry.MonitorID)
	if idLength > idMaxLength {
		return ErrMonitorIDTooLong
	}

	// Write message and newline.
	_, err := msgFile.Write(append([]byte(entry.Msg), byte('\n')))
	if err != nil {
		return fmt.Errorf("write msg: %w", err)
	}
	// Time.
	binary.BigEndian.PutUint64(buf[:8], uint64(entry.Time))
	// Source.
	copy(buf[8:16], append(
		[]byte(entry.Src),
		bytes.Repeat([]byte{' '}, srcMaxLength-srcLength)...,
	))
	// Monitor ID.
	copy(buf[16:40], append(
		[]byte(entry.MonitorID),
		bytes.Repeat([]byte{' '}, idMaxLength-idLength)...,
	))
	// Message offset and size.
	binary.BigEndian.PutUint32(buf[40:44], *msgOffset)
	binary.BigEndian.PutUint16(buf[44:46], uint16(len(entry.Msg)))
	// Level.
	buf[46] = byte(entry.Level)

	*msgOffset += uint32(len(entry.Msg)) + 1

	return nil
}

func decodeEntry(buf []byte, msgFile io.ReadSeeker) (*Entry, uint32, error) {
	msgOffset := binary.BigEndian.Uint32(buf[40:44])
	msgSize := binary.BigEndian.Uint16(buf[44:46])

	_, err := msgFile.Seek(int64(msgOffset), io.SeekStart)
	if err != nil {
		return nil, 0, fmt.Errorf("seek: %w", err)
	}

	msgBuf := make([]byte, msgSize)
	_, err = io.ReadFull(msgFile, msgBuf)
	if err != nil {
		return nil, 0, fmt.Errorf("read: %w", err)
	}

	return &Entry{
		Time:      UnixMicro(binary.BigEndian.Uint64(buf[:8])),
		Src:       strings.TrimSpace(string(buf[8:16])),
		MonitorID: strings.TrimSpace(string(buf[16:40])),
		Level:     Level(buf[46]),
		Msg:       string(msgBuf),
	}, msgOffset, nil
}

// ErrInvalidTime invalid time.
var ErrInvalidTime = errors.New("invalid time")

var padInt = "%0" + strconv.Itoa(chunkIDLenght) + "d"

// timeToID returns the first x digits in a UnixMilli timestamp as string.
// Output is padded with zeros if needed.
func timeToID(time UnixMicro) (string, error) {
	shifted := uint64(time) / chunkDuration
	padded := fmt.Sprintf(padInt, shifted)
	if len(padded) > chunkIDLenght {
		return "", fmt.Errorf("%w: %v", ErrInvalidTime, time)
	}
	return padded, nil
}

// LevelInLevels returns true if level is in levels or if levels is empty.
func LevelInLevels(level Level, levels []Level) bool {
	if len(levels) == 0 {
		return true
	}
	for _, l := range levels {
		if l == level {
			return true
		}
	}
	return false
}

// StringInStrings returns true if source is in sources or if sources is empty.
func StringInStrings(source string, sources []string) bool {
	if len(sources) == 0 {
		return true
	}
	for _, src := range sources {
		if src == source {
			return true
		}
	}
	return false
}
