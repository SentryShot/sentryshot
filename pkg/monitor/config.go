// SPDX-License-Identifier: GPL-2.0-or-later

package monitor

import "strings"

// RawConfigs map of RawConfig.
type RawConfigs map[string]RawConfig

// RawConfig raw config.
type RawConfig map[string]string

// Configs Monitor configurations.
type Configs map[string]Config

// Config Monitor configuration.
type Config struct {
	v map[string]string
}

// NewConfig creates a new immutable monitor config.
func NewConfig(c RawConfig) Config {
	return Config{v: c}
}

// Get config value by key.
func (c Config) Get(key string) string {
	return c.v[key]
}

func (c Config) enabled() bool {
	return c.v["enable"] == "true"
}

// ID returns the monitor ID.
func (c Config) ID() string {
	return c.v["id"]
}

// Name returns the monitor name.
func (c Config) Name() string {
	return c.v["name"]
}

// InputOpts returns the monitor input options.
func (c Config) InputOpts() string {
	return c.v["inputOptions"]
}

func (c Config) audioEnabled() bool {
	switch c.v["audioEncoder"] {
	case "":
		return false
	case "none":
		return false
	}
	return true
}

// AudioEncoder returns the monitor audio encoder.
func (c Config) AudioEncoder() string {
	return c.v["audioEncoder"]
}

// VideoEncoder returns the monitor audio encoder.
func (c Config) VideoEncoder() string {
	return c.v["videoEncoder"]
}

// MainInput returns the main input url.
func (c Config) MainInput() string {
	return c.v["mainInput"]
}

// SubInput returns the sub input url.
func (c Config) SubInput() string {
	return c.v["subInput"]
}

// SubInputEnabled if sub input is available.
func (c Config) SubInputEnabled() bool {
	return c.SubInput() != ""
}

// video length is seconds.
func (c Config) videoLength() string {
	return c.v["videoLength"]
}

func (c Config) alwaysRecord() bool {
	return c.v["alwaysRecord"] == "true"
}

// TimestampOffset returns the timestamp offset.
func (c Config) TimestampOffset() string {
	return c.v["timestampOffset"]
}

// LogLevel returns the ffmpeg log level.
func (c Config) LogLevel() string {
	return c.v["logLevel"]
}

// Hwaccel returns the ffmpeg hwaccel.
func (c Config) Hwaccel() string {
	return c.v["hwaccel"]
}

// CensorLog replaces sensitive monitor config values.
func (c Config) CensorLog(msg string) string {
	if c.MainInput() != "" {
		msg = strings.ReplaceAll(msg, c.MainInput(), "$MainInput")
	}
	if c.SubInput() != "" {
		msg = strings.ReplaceAll(msg, c.SubInput(), "$SubInput")
	}
	return msg
}
