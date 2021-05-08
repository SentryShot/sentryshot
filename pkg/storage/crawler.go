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

package storage

import (
	"io/ioutil"
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
//             └── YYYY-MM-DD_hh-mm-ss_monitor2.json  // Event data, Not implemented.
//
// Thumbnail is only generated If video was saved successfully.
// The job of these functions are to on-request
// find and return recording paths.

// Crawler crawls through storage looking for recordings.
type Crawler struct {
	path string
}

// NewCrawler creates new crawler.
func NewCrawler(path string) *Crawler {
	return &Crawler{
		path: path,
	}
}

// RecordingByQuery finds best matching recording and
// returns limit number of subsequent videos
func (c *Crawler) RecordingByQuery(limit int, query string) ([]Recording, error) {
	var recordings []Recording

	var file *dir
	for len(recordings) < limit {
		firstFile := file == nil
		if firstFile {
			file = c.findVideo(query)
		}

		if file.name == query || !firstFile {
			file = file.prevSibling()
		}

		if file.isNil() {
			break
		}

		recordings = append(recordings, newVideo(c.cleanPath(file.path)))
	}
	return recordings, nil
}

// Removes storageDir from input and replaces it with "storage"
func (c *Crawler) cleanPath(input string) string {
	storageDirLen := len(c.path)
	return "storage" + input[storageDirLen:]
}

type dir struct {
	name   string
	path   string
	depth  int
	parent *dir
}

func (d *dir) isNil() bool {
	return *d == dir{}
}

func (c *Crawler) findVideo(id string) *dir {
	query := []string{
		id[:4],   // Year.
		id[5:7],  // Month.
		id[8:10], // Day.
	}

	root := &dir{
		path:  c.path,
		depth: 0,
	}

	current := root
	for _, val := range query {
		parent := current
		current = current.childByName(val)
		if current.isNil() {
			return parent.prevChildByName(val).latestFile()
		}
	}

	file := current.childByName(id)
	if !file.isNil() {
		return file
	}

	return current.prevChildByName(id)
}

// Recording contains identifier and path.
// ".mp4",".jpeg" or ".json" can be appended to the
// path to get the video, thumbnail or data file.
type Recording struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

func newVideo(path string) Recording {
	return Recording{
		ID:   filepath.Base(path),
		Path: path,
	}
}

const monitorDepth = 3

// children returns children of current directory.
func (d *dir) children() []dir {
	if d.depth == monitorDepth {
		thumbnails := d.findAllThumbnails()

		sort.Slice(thumbnails, func(i, j int) bool {
			return thumbnails[i].name < thumbnails[j].name
		})
		return thumbnails
	}

	files, err := ioutil.ReadDir(d.path)
	if err != nil {
		return []dir{}
	}

	var children []dir
	for _, file := range files {
		if file.IsDir() {
			children = append(children, dir{
				name:   file.Name(),
				path:   d.path + "/" + file.Name(),
				parent: d,
				depth:  d.depth + 1,
			})
		}
	}
	return children
}

// findAllThumbnails finds all jpeg files in decending directories.
func (d *dir) findAllThumbnails() []dir {
	var thumbnails []dir
	filepath.Walk(d.path, func(path string, file os.FileInfo, _ error) error { //nolint:errcheck
		if !file.IsDir() && strings.Contains(file.Name(), ".jpeg") {
			thumbnails = append(thumbnails, dir{
				name:   strings.TrimSuffix(file.Name(), ".jpeg"),
				path:   strings.TrimSuffix(path, ".jpeg"),
				parent: d,
				depth:  d.depth + 2,
			})
		}
		return nil
	})
	return thumbnails
}

// prevChildByName iterates though children in reverse and returns the
// first child with a name alphabetically after supplied name.
func (d *dir) prevChildByName(name string) *dir {
	children := d.children()

	for i := len(children) - 1; i >= 0; i-- { // Reverse range.
		child := children[i]
		if child.name < name {
			return &child
		}
	}

	return &dir{}
}

// childByName returns child of current directory by name.
// returns nil if child doesn't exist.
func (d *dir) childByName(name string) *dir {
	children := d.children()
	for _, child := range children {
		if child.name == name {
			return &child
		}
	}

	return &dir{}
}

// latestFile returns the newest file in decending directories.
func (d *dir) latestFile() *dir {
	file := d
	for file.depth < monitorDepth+2 {
		children := file.children()
		if len(children) == 0 {
			if d.depth == 0 {
				return &dir{}
			}
			return file.prevSibling().latestFile()
		}
		file = &children[len(children)-1]
	}
	return file
}

// prevSibling returns the previus sibling alphabetically.
func (d *dir) prevSibling() *dir {
	if d.depth == 0 {
		return &dir{}
	}

	siblings := d.parent.children()

	for i, sibling := range siblings {
		if sibling == *d {
			if i > 0 {
				return siblings[i-1].latestFile()
			}
			return d.parent.prevSibling()
		}
	}
	return d.parent.prevSibling()
}
