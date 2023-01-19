// SPDX-License-Identifier: GPL-2.0-or-later

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
