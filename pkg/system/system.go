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

package system

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"
)

// ErrNoTimeZone could not determine time zone.
var ErrNoTimeZone = errors.New("could not determine time zone")

// TimeZone returns system time zone location.
func TimeZone() (string, error) {
	// Try golang's built-in function.
	zone := time.Now().Location().String()
	if zone != "Local" {
		return zone, nil
	}

	// Fallback 1
	file, _ := os.ReadFile("/etc/timezone")
	zone = string(file)
	if zone != "" {
		return strings.TrimSpace(zone), nil
	}

	// Fallback 2
	localtime, _ := os.ReadFile("/etc/localtime")
	fileSystem := os.DirFS("/usr/share/zoneinfo")
	fs.WalkDir(fileSystem, ".", func(filePath string, d fs.DirEntry, err error) error { //nolint:errcheck
		if err != nil || d.IsDir() {
			return nil
		}
		file, err := fs.ReadFile(fileSystem, filePath)
		if err != nil {
			return nil
		}
		if string(file) == string(localtime) {
			dir, city := path.Split(filePath)
			region := path.Base(dir)
			zone = city

			switch region {
			case "zoneinfo":
			case "posix":
			default:
				zone = region + "/" + city
			}
		}
		return nil
	})
	if zone == "" {
		return "", ErrNoTimeZone
	}
	return strings.TrimSpace(zone), nil
}
