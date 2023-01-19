// SPDX-License-Identifier: GPL-2.0-or-later

package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// Recordings are stored in the following format
//
// <Year>
// └── <Month>
//     └── <Day>
//         ├── Monitor1
//         └── Monitor2
//             ├── YYYY-MM-DD_hh-mm-ss_monitor2.jpeg  // Thumbnail.
//             ├── YYYY-MM-DD_hh-mm-ss_monitor2.mp4   // Video.
//             └── YYYY-MM-DD_hh-mm-ss_monitor2.json  // Event data.
//
// Event data is only generated If video was saved successfully.
// The job of these functions are to on-request find and return recording IDs.

// CrawlerQuery query of recordings for crawler to find.
type CrawlerQuery struct {
	Time     string
	Limit    int
	Reverse  bool
	Monitors []string

	// If event data should be read from file and included.
	IncludeData bool

	// Query scoped cache to avoid reading the same directory twice.
	cache queryCache
}

type queryCache map[string][]dir

// Crawler crawls through storage looking for recordings.
type Crawler struct {
	fs fs.FS
}

// NewCrawler creates new crawler.
func NewCrawler(fileSystem fs.FS) *Crawler {
	return &Crawler{fs: fileSystem}
}

// ErrInvalidValue invalid value.
var ErrInvalidValue = errors.New("invalid value")

// RecordingByQuery finds best matching recording and
// returns limit number of subsequent videos.
func (c *Crawler) RecordingByQuery(q *CrawlerQuery) ([]Recording, error) {
	q.cache = make(queryCache)
	var recordings []Recording

	var file *dir
	var err error
	for len(recordings) < q.Limit { // Run until limit is reached.
		isFirstFile := file == nil
		if isFirstFile {
			file, err = c.findVideo(q)
			if err != nil {
				return nil, err
			}
		}

		// If last file is reached.
		if file == nil {
			return recordings, nil
		}

		if !isFirstFile || file.name == q.Time {
			file, err = file.sibling()
			if err != nil {
				return nil, err
			}
		}

		// If last file is reached.
		if file == nil {
			return recordings, nil
		}

		data := func() *RecordingData {
			if q.IncludeData {
				return readDataFile(file.fs)
			}
			return nil
		}()

		recordings = append(recordings, Recording{
			ID:   filepath.Base(file.path),
			Data: data,
		})
	}
	return recordings, nil
}

func readDataFile(fileSystem fs.FS) *RecordingData {
	rawData, err := fs.ReadFile(fileSystem, ".")
	if err != nil {
		return nil
	}
	var data RecordingData
	err = json.Unmarshal(rawData, &data)
	if err != nil {
		return nil
	}
	return &data
}

func (c *Crawler) findVideo(q *CrawlerQuery) (*dir, error) {
	if len(q.Time) < 10 {
		return nil, fmt.Errorf("time: %v: %w", q.Time, ErrInvalidValue)
	}

	yearMonthDay := []string{
		q.Time[:4],   // Year.
		q.Time[5:7],  // Month.
		q.Time[8:10], // Day.
	}

	root := &dir{
		fs:    c.fs,
		path:  "",
		depth: 0,
		query: q,
	}

	// Try to find exact file.
	current := root
	var parent *dir
	var err error
	for _, val := range yearMonthDay {
		parent = current
		current, err = current.childByExactName(val)
		if err != nil {
			return nil, err
		}
		if current == nil {
			// Exact match could not be found.
			child, err := parent.childByName(val)
			if err != nil {
				return nil, err
			}
			if child == nil {
				return parent.sibling()
			}
			return child.findFileDeep()
		}
	}

	// If exact match found, return sibling of match.
	file, err := current.childByExactName(q.Time)
	if err != nil {
		return nil, err
	}
	if file != nil {
		return file.sibling()
	}

	// If inexact file found, return match.
	file, err = current.childByName(q.Time)
	if err != nil {
		return nil, err
	}
	if file != nil {
		return file, nil
	}

	return current.sibling()
}

type dir struct {
	fs     fs.FS
	name   string
	path   string
	depth  int
	parent *dir
	query  *CrawlerQuery
}

const (
	monitorDepth = 3
	recDepth     = 5
)

// children of current directory. Special case if depth == monitorDepth.
func (d *dir) children() ([]dir, error) {
	cache := d.query.cache
	cached, exist := cache[d.path]
	if exist {
		return cached, nil
	}

	if d.depth == monitorDepth {
		thumbnails, err := d.findAllFiles()
		if err != nil {
			return nil, err
		}

		sort.Slice(thumbnails, func(i, j int) bool {
			return thumbnails[i].name < thumbnails[j].name
		})

		cache[d.path] = thumbnails
		return cache[d.path], nil
	}

	files, err := fs.ReadDir(d.fs, ".")
	if err != nil {
		return nil, err
	}

	var children []dir
	for _, file := range files {
		if !file.IsDir() {
			continue
		}
		path := filepath.Join(d.path, file.Name())
		fileFS, err := fs.Sub(d.fs, file.Name())
		if err != nil {
			return nil, fmt.Errorf("child fs: %v: %w", path, err)
		}

		children = append(children, dir{
			fs:     fileFS,
			name:   file.Name(),
			path:   path,
			parent: d,
			depth:  d.depth + 1,
			query:  d.query,
		})
	}
	cache[d.path] = children
	return cache[d.path], nil
}

// ErrUnexpectedDir unexpected directory.
var ErrUnexpectedDir = errors.New("unexpected directory")

// findAllFiles finds all json files beloning to
// selected monitors in decending directories.
// Only called by `children()`.
func (d *dir) findAllFiles() ([]dir, error) {
	monitorDirs, err := fs.ReadDir(d.fs, ".")
	if err != nil {
		return nil, fmt.Errorf("read day directory: %v %w", d.path, err)
	}

	var allFiles []dir
	for _, entry := range monitorDirs {
		if len(d.query.Monitors) != 0 && !d.monitorSelected(entry.Name()) {
			continue
		}

		monitorPath := filepath.Join(d.path, entry.Name())
		monitorFS, err := fs.Sub(d.fs, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("monitor fs: %v: %w", monitorPath, err)
		}

		files, err := fs.ReadDir(monitorFS, ".")
		if err != nil {
			return nil, fmt.Errorf("read monitor directory: %v: %w", monitorPath, err)
		}
		for _, file := range files {
			if file.IsDir() {
				return nil, fmt.Errorf("%v: %w", monitorPath, ErrUnexpectedDir)
			}
			if !strings.Contains(file.Name(), ".json") {
				continue
			}
			jsonPath := filepath.Join(monitorPath, file.Name())
			path := strings.TrimSuffix(jsonPath, ".json")

			fileFS, err := fs.Sub(monitorFS, file.Name())
			if err != nil {
				return nil, fmt.Errorf("file fs: %v: %w", jsonPath, err)
			}

			allFiles = append(allFiles, dir{
				fs:     fileFS,
				name:   strings.TrimSuffix(file.Name(), ".json"),
				path:   path,
				parent: d,
				depth:  d.depth + 2,
				query:  d.query,
			})
		}
	}
	return allFiles, nil
}

func (d *dir) monitorSelected(monitor string) bool {
	for _, m := range d.query.Monitors {
		if m == monitor {
			return true
		}
	}
	return false
}

// childByName Returns next or previous child.
func (d *dir) childByName(name string) (*dir, error) {
	children, err := d.children()
	if err != nil {
		return nil, err
	}

	if d.query.Reverse {
		return d.nextChildByName(name, children)
	}
	return d.prevChildByName(name, children)
}

// nextChildByName iterates though children and returns the
// first child with a name alphabetically before supplied name.
func (d *dir) nextChildByName(name string, children []dir) (*dir, error) {
	for _, child := range children {
		if child.name > name {
			return &child, nil
		}
	}
	return nil, nil
}

// prevChildByName iterates though children in reverse and returns the
// first child with a name alphabetically after supplied name.
func (d *dir) prevChildByName(name string, children []dir) (*dir, error) {
	for i := len(children) - 1; i >= 0; i-- { // Reverse range.
		child := children[i]
		if child.name < name {
			return &child, nil
		}
	}
	return nil, nil
}

// childByExactName returns child of current directory by exact name.
// returns nil if child doesn't exist.
func (d *dir) childByExactName(name string) (*dir, error) {
	children, err := d.children()
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		if child.name == name {
			return &child, nil
		}
	}

	return nil, nil
}

// findFileDeep returns the newest or oldest file in all decending directories.
func (d *dir) findFileDeep() (*dir, error) {
	current := d
	for current.depth < recDepth {
		children, err := current.children()
		if err != nil {
			return nil, err
		}
		if len(children) == 0 {
			if d.depth == 0 {
				return nil, nil
			}
			sibling, err := current.sibling()
			if err != nil {
				return nil, err
			}
			if sibling == nil {
				return nil, nil
			}
			return sibling.findFileDeep()
		}
		if d.query.Reverse {
			current = &children[0] // First child.
		} else {
			current = &children[len(children)-1] // Last child.
		}
	}
	return current, nil
}

// ErrNoSibling could not find sibling.
var ErrNoSibling = errors.New("could not find sibling")

// sibling Returns next or previous sibling.
// Will climb to parent directories.
func (d *dir) sibling() (*dir, error) {
	if d.depth == 0 {
		return nil, nil
	}

	siblings, err := d.parent.children()
	if err != nil {
		return nil, err
	}

	for i, sibling := range siblings {
		if sibling == *d {
			// Next
			if d.query.Reverse {
				if i < len(siblings)-1 {
					return siblings[i+1].findFileDeep()
				}
				return d.parent.sibling()
			}
			// Previous
			if i > 0 {
				return siblings[i-1].findFileDeep()
			}
			return d.parent.sibling()
		}
	}
	return nil, fmt.Errorf("%v: %w", d.path, ErrNoSibling)
}

// ErrInvalidRecordingID invalid recording ID.
var ErrInvalidRecordingID = errors.New("invalid recording ID")

// RecordingIDToPath converts recording ID to path.
func RecordingIDToPath(id string) (string, error) {
	if len(id) < 20 {
		return "", ErrInvalidRecordingID
	}
	if id[4] != '-' || id[7] != '-' ||
		id[10] != '_' || id[13] != '-' ||
		id[16] != '-' || id[19] != '_' {
		return "", fmt.Errorf("%w: %v", ErrInvalidRecordingID, id)
	}

	year := id[0:4]
	month := id[5:7]
	day := id[8:10]
	monitorID := id[20:]

	return filepath.Join(year, month, day, monitorID, id), nil
}
