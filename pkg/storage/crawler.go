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

package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
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
// Thumbnail is only generated If video was saved successfully.
// The job of these functions are to on-request
// find and return recording paths.

// CrawlerQuery query of recordings for crawler to find.
type CrawlerQuery struct {
	Time     string
	Limit    int
	Reverse  bool
	Monitors []string
	Data     bool // If data should be read from file and included.
	cache    queryCache
}

type queryCache map[string][]dir

// Crawler crawls through storage looking for recordings.
type Crawler struct {
	path string
}

// NewCrawler creates new crawler.
func NewCrawler(path string) *Crawler {
	return &Crawler{
		path: filepath.Clean(path),
	}
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

		recordings = append(recordings, c.newRecording(file.path, q.Data))
	}
	return recordings, nil
}

// Removes storageDir from input and replaces it with "storage".
func (c *Crawler) cleanPath(input string) string {
	storageDirLen := len(filepath.Clean(c.path))
	return "storage/recordings" + input[storageDirLen:]
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
		path:  c.path,
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

func (c *Crawler) newRecording(rawPath string, data bool) Recording {
	path := c.cleanPath(rawPath)
	var d *RecordingData
	if data {
		d = readDataFile(rawPath + ".json")
	}
	return Recording{
		ID:   filepath.Base(path),
		Path: path,
		Data: d,
	}
}

func readDataFile(path string) *RecordingData {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var data RecordingData
	if err := json.Unmarshal(file, &data); err != nil {
		return nil
	}
	return &data
}

type dir struct {
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
		thumbnails, err := d.findAllThumbnails()
		if err != nil {
			return nil, err
		}

		sort.Slice(thumbnails, func(i, j int) bool {
			return thumbnails[i].name < thumbnails[j].name
		})

		cache[d.path] = thumbnails
		return cache[d.path], nil
	}

	files, err := fs.ReadDir(os.DirFS(d.path), ".")
	if err != nil {
		return nil, err
	}

	var children []dir
	for _, file := range files {
		if file.IsDir() {
			children = append(children, dir{
				name:   file.Name(),
				path:   filepath.Join(d.path, file.Name()),
				parent: d,
				depth:  d.depth + 1,
				query:  d.query,
			})
		}
	}
	cache[d.path] = children
	return cache[d.path], nil
}

// ErrUnexpectedDir unexpected directory.
var ErrUnexpectedDir = errors.New("unexpected directory")

// findAllThumbnails finds all jpeg files beloning to
// selected monitors in decending directories.
// Only called by `children()`.
func (d *dir) findAllThumbnails() ([]dir, error) {
	monitorDirs, err := fs.ReadDir(os.DirFS(d.path), ".")
	if err != nil {
		return nil, fmt.Errorf("could not read day directory: %v %w", d.path, err)
	}

	var thumbnails []dir
	for _, m := range monitorDirs {
		if len(d.query.Monitors) != 0 && !d.monitorSelected(m.Name()) {
			continue
		}
		path := filepath.Join(d.path, m.Name())
		files, err := fs.ReadDir(os.DirFS(path), ".")
		if err != nil {
			return nil, fmt.Errorf("could not read monitor directory: %v: %w", path, err)
		}
		for _, file := range files {
			if file.IsDir() {
				return nil, fmt.Errorf("%v: %w", path, ErrUnexpectedDir)
			}
			if !strings.Contains(file.Name(), ".jpeg") {
				continue
			}
			thumbPath := filepath.Join(path, file.Name())
			thumbnails = append(thumbnails, dir{
				name:   strings.TrimSuffix(file.Name(), ".jpeg"),
				path:   strings.TrimSuffix(thumbPath, ".jpeg"),
				parent: d,
				depth:  d.depth + 2,
				query:  d.query,
			})
		}
	}
	return thumbnails, nil
}

func (d *dir) monitorSelected(monitor string) bool {
	for _, m := range d.query.Monitors {
		if m == monitor {
			return true
		}
	}
	return false
}

// childByName Returns next or previus child.
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

// sibling Returns next or previus sibling. Will climb.
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
			// Previus
			if i > 0 {
				return siblings[i-1].findFileDeep()
			}
			return d.parent.sibling()
		}
	}
	return nil, fmt.Errorf("%v: %w", d.path, ErrNoSibling)
}
