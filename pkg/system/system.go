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

package system

import (
	"errors"
	"os"
	"path"
	"path/filepath"
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
	data, _ := os.ReadFile("/etc/timezone")
	zone = string(data)
	if zone != "" {
		return strings.TrimSpace(zone), nil
	}

	// Fallback 2
	localtime, _ := os.ReadFile("/etc/localtime")
	_ = filepath.Walk("/usr/share/zoneinfo", func(filePath string, file os.FileInfo, err error) error {
		if err != nil || file.IsDir() {
			return err
		}
		data, _ := os.ReadFile(filePath)
		if string(data) == string(localtime) {
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
	if zone != "" {
		return strings.TrimSpace(zone), nil
	}

	return "", ErrNoTimeZone
}
