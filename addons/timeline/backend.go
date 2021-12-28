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

package timeline

import (
	"context"
	"nvr"
	"nvr/pkg/ffmpeg"
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
}

func onRecSaved(m *monitor.Monitor, recPath string, recData storage.RecordingData) {
	tempPath := recPath + "_timeline_tmp.mp4"
	timelinePath := recPath + "_timeline.mp4"
	opts := argOpts{
		logLevel:   m.Config.LogLevel(),
		inputPath:  recPath + ".mp4",
		outputPath: tempPath,
		scale:      m.Config["timelineScale"],
		quality:    m.Config["timelineQuality"],
		frameRate:  m.Config["timelineFrameRate"],
	}
	args := genArgs(opts)

	m.Log.Info().
		Src("timeline").
		Monitor(m.Config.ID()).
		Msgf("generating video: %v", strings.Join(args, " "))

	cmd := exec.Command(m.Env.FFmpegBin, args...)

	logFunc := func(msg string) {
		m.Log.FFmpegLevel(m.Config.LogLevel()).
			Src("timeline").
			Monitor(m.Config.ID()).
			Msgf("process: %v", msg)
	}

	process := m.NewProcess(cmd).
		StdoutLogger(logFunc).
		StderrLogger(logFunc)

	recDuration := recData.End.Sub(recData.Start)
	ctx, cancel := context.WithTimeout(context.Background(), recDuration)
	defer cancel()

	if err := process.Start(ctx); err != nil {
		m.Log.Error().
			Src("timeline").
			Monitor(m.Config.ID()).
			Msgf("could not generate video: %v", args)
		return
	}

	if err := os.Rename(tempPath, timelinePath); err != nil {
		m.Log.Error().
			Src("timeline").
			Monitor(m.Config.ID()).
			Msgf("could not rename temp file: %v", err)
	}
}

type argOpts struct {
	logLevel   string
	inputPath  string
	outputPath string
	scale      string
	quality    string
	frameRate  string
}

func genArgs(opts argOpts) []string {
	s := ffmpeg.ParseScaleString(opts.scale)
	crf := parseQuality(opts.quality)
	fps := parseFrameRate(opts.frameRate)

	args := []string{
		"-n", "-loglevel", opts.logLevel,
		"-threads", "1", "-discard", "nokey",
		"-i", opts.inputPath, "-an",
		"-c:v", "libx264", "-x264-params", "keyint=4",
		"-preset", "veryfast", "-tune", "fastdecode", "-crf", crf,
		"-vsync", "vfr", "-vf",
	}

	filters := "mpdecimate,fps=" + fps + ",mpdecimate"
	if s != "1" {
		filters += ",scale='iw/" + s + ":ih/" + s + "'"
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

const defaultRate = "6"

func parseFrameRate(rate string) string {
	fpm, err := strconv.ParseFloat(rate, 64)
	if err != nil || fpm <= 0 {
		return defaultRate
	}

	fps := fpm / 60
	return strconv.FormatFloat(fps, 'f', 4, 32)
}
