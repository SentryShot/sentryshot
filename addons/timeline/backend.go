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

package timeline

import (
	"context"
	"encoding/json"
	"fmt"
	"nvr"
	"nvr/pkg/ffmpeg"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func init() {
	nvr.RegisterLogSource([]string{"timeline"})
	nvr.RegisterMonitorRecSavedHook(onRecSaved)
	nvr.RegisterMigrationMonitorHook(migrate)
}

func onRecSaved(r *monitor.Recorder, recPath string, recData storage.RecordingData) {
	id := r.Config.ID()
	logf := func(level log.Level, format string, a ...interface{}) {
		r.Log.Level(level).Src("timeline").Monitor(id).Msgf(format, a...)
	}

	tempPath := recPath + "_timeline_tmp.mp4"
	timelinePath := recPath + "_timeline.mp4"
	opts := argOpts{
		logLevel:   r.Config.LogLevel(),
		inputPath:  recPath + ".mp4",
		outputPath: tempPath,
	}

	config, err := parseConfig(r.Config)
	if err != nil {
		logf(log.LevelError, "could not parse config: %w")
	}

	args := genArgs(opts, *config)

	logf(log.LevelInfo, "generating video: %v", strings.Join(args, " "))
	cmd := exec.Command(r.Env.FFmpegBin, args...)

	logFunc := func(msg string) {
		logf(log.FFmpegLevel(r.Config.LogLevel()), "process: %v", msg)
	}

	process := r.NewProcess(cmd).
		StdoutLogger(logFunc).
		StderrLogger(logFunc)

	recDuration := recData.End.Sub(recData.Start)
	ctx, cancel := context.WithTimeout(context.Background(), recDuration)
	defer cancel()

	if err := process.Start(ctx); err != nil {
		logf(log.LevelError, "could not generate video: %v", args)
		return
	}

	if err := os.Rename(tempPath, timelinePath); err != nil {
		logf(log.LevelError, "could not rename temp file: %v", err)
	}
}

type argOpts struct {
	logLevel   string
	inputPath  string
	outputPath string
}

const defaultScale = "8"

func genArgs(opts argOpts, c config) []string {
	scale := ffmpeg.ParseScaleString(c.scale)
	if scale == "" {
		scale = defaultScale
	}
	crf := parseQuality(c.quality)
	fps := parseFrameRate(c.frameRate)

	args := []string{
		"-n", "-loglevel", opts.logLevel,
		"-threads", "1", "-discard", "nokey",
		"-i", opts.inputPath, "-an",
		"-c:v", "libx264", "-x264-params", "keyint=4",
		"-preset", "veryfast", "-tune", "fastdecode", "-crf", crf,
		"-vsync", "vfr", "-vf",
	}

	filters := "mpdecimate,fps=" + fps + ",mpdecimate"
	if scale != "1" {
		filters += ",scale='iw/" + scale + ":ih/" + scale + "'"
	}

	args = append(args, filters)

	args = append(args, "-movflags", "empty_moov+default_base_moof+frag_keyframe", opts.outputPath)

	return args
}

func parseQuality(q string) string {
	switch q {
	case "1":
		return "18"
	case "2":
		return "21"
	case "3":
		return "24"
	case "4":
		return "27"
	case "5":
		return "30"
	case "6":
		return "33"
	case "7":
		return "36"
	case "8":
		return "39"
	case "9":
		return "42"
	case "10":
		return "45"
	case "11":
		return "48"
	case "12":
		return "51"
	}
	return "27"
}

const defaultFrameRate = "6"

func parseFrameRate(rate string) string {
	fpm, err := strconv.ParseFloat(rate, 64)
	if err != nil || fpm <= 0 {
		return defaultFrameRate
	}

	fps := fpm / 60
	return strconv.FormatFloat(fps, 'f', 4, 32)
}

type config struct {
	scale     string
	quality   string
	frameRate string
}

type rawConfigV1 struct {
	Scale     string `json:"scale"`
	Quality   string `json:"quality"`
	FrameRate string `json:"frameRate"`
}

func parseConfig(conf monitor.Config) (*config, error) {
	var rawConf rawConfigV1
	rawTimeline := conf["timeline"]
	if rawTimeline != "" {
		err := json.Unmarshal([]byte(rawTimeline), &rawConf)
		if err != nil {
			return nil, fmt.Errorf("unmarshal doods: %w", err)
		}
	}
	return &config{
		scale:     rawConf.Scale,
		quality:   rawConf.Quality,
		frameRate: rawConf.FrameRate,
	}, nil
}

const currentConfigVersion = 1

func migrate(conf monitor.Config) error {
	configVersion, _ := strconv.Atoi(conf["timelineConfigVersion"])

	if configVersion < 1 {
		if err := migrateV0toV1(conf); err != nil {
			return fmt.Errorf("timeline v0 to v1: %w", err)
		}
	}

	conf["timelineConfigVersion"] = strconv.Itoa(currentConfigVersion)
	return nil
}

func migrateV0toV1(conf monitor.Config) error {
	config := rawConfigV1{
		Scale:     conf["timelineScale"],
		Quality:   conf["timelineQuality"],
		FrameRate: conf["timelineFrameRate"],
	}

	delete(conf, "timelineScale")
	delete(conf, "timelineQuality")
	delete(conf, "timelineFrameRate")

	rawConfig, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal raw config: %w", err)
	}
	conf["timeline"] = string(rawConfig)
	return nil
}
