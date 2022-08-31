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

package monitor

// Configs Monitor configurations.
type Configs map[string]Config

// Config Monitor configuration.
type Config map[string]string

func (c Config) enabled() bool {
	return c["enable"] == "true"
}

// ID returns the monitor ID.
func (c Config) ID() string {
	return c["id"]
}

// Name returns the monitor name.
func (c Config) Name() string {
	return c["name"]
}

// InputOpts returns the monitor input options.
func (c Config) InputOpts() string {
	return c["inputOptions"]
}

func (c Config) audioEnabled() bool {
	switch c["audioEncoder"] {
	case "":
		return false
	case "none":
		return false
	}
	return true
}

// AudioEncoder returns the monitor audio encoder.
func (c Config) AudioEncoder() string {
	return c["audioEncoder"]
}

// VideoEncoder returns the monitor audio encoder.
func (c Config) VideoEncoder() string {
	return c["videoEncoder"]
}

// MainInput returns the main input url.
func (c Config) MainInput() string {
	return c["mainInput"]
}

// SubInput returns the sub input url.
func (c Config) SubInput() string {
	return c["subInput"]
}

// SubInputEnabled if sub input is available.
func (c Config) SubInputEnabled() bool {
	return c.SubInput() != ""
}

// video length is seconds.
func (c Config) videoLength() string {
	return c["videoLength"]
}

func (c Config) alwaysRecord() bool {
	return c["alwaysRecord"] == "true"
}

// TimestampOffset returns the timestamp offset.
func (c Config) TimestampOffset() string {
	return c["timestampOffset"]
}

// LogLevel returns the ffmpeg log level.
func (c Config) LogLevel() string {
	return c["logLevel"]
}

// Hwaccel returns the ffmpeg hwaccel.
func (c Config) Hwaccel() string {
	return c["hwaccel"]
}
