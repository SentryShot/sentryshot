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

package motion

import (
	"context"
	"errors"
	"fmt"
	"io"
	"nvr"
	"nvr/pkg/ffmpeg"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

func init() {
	nvr.RegisterMonitorInputProcessHook(onInputProcessStart)
	nvr.RegisterLogSource([]string{"motion"})

	nvr.RegisterTplHook(modifyTemplates)
	nvr.RegisterAppRunHook(func(_ context.Context, app *nvr.App) error {
		app.Router.Handle("/motion.mjs", app.Auth.Admin(serveMotionMjs()))
		return nil
	})
}

func onInputProcessStart(ctx context.Context, i *monitor.InputProcess, _ *[]string) {
	if i.Config.SubInputEnabled() != i.IsSubInput() {
		return
	}

	id := i.Config.ID()
	logf := func(level log.Level, format string, a ...interface{}) {
		i.Logger.Log(log.Entry{
			Level:     level,
			Src:       "motion",
			MonitorID: id,
			Msg:       fmt.Sprintf(format, a...),
		})
	}

	config, enable, err := parseConfig(i.Config)
	if err != nil {
		logf(log.LevelError, "could not parse config: %v", err)
		return
	}
	if !enable {
		return
	}

	i.WG.Add(1)
	go start(ctx, i, *config, logf)
}

func start(
	ctx context.Context,
	i *monitor.InputProcess,
	config config,
	logf log.Func,
) {
	defer i.WG.Done()

	// Wait for the monitor to start.
	select {
	case <-time.After(10 * time.Second):
	case <-ctx.Done():
		return
	}

	for {
		if ctx.Err() != nil {
			return
		}

		ctx2, cancel := context.WithCancel(ctx)

		if err := run(ctx2, cancel, i, config, logf); err != nil {
			logf(log.LevelError, "%v", err)
		}

		select {
		case <-time.After(3 * time.Second):
		case <-ctx.Done():
			return
		}
	}
}

func run(
	ctx context.Context,
	cancel context.CancelFunc,
	i *monitor.InputProcess,
	config config,
	logf log.Func,
) error {
	infoCtx, infoCancel := context.WithTimeout(ctx, 30*time.Second)
	defer infoCancel()
	streamInfo, err := i.StreamInfo(infoCtx)
	if err != nil {
		return fmt.Errorf("stream info: %w", err)
	}
	width := streamInfo.VideoWidth
	height := streamInfo.VideoHeight

	d, err := newDetector(i, config, logf, width, height)
	if err != nil {
		return fmt.Errorf("create detector: %w", err)
	}

	args := generateFFmpegArgs(config, i.RTSPprotocol(), i.RTSPaddress())
	cmd := exec.Command(i.Env.FFmpegBin, args...)

	processLogFunc := func(msg string) {
		logf(log.FFmpegLevel(config.logLevel), fmt.Sprintf("process: %v", msg))
	}

	process := ffmpeg.NewProcess(cmd).
		StderrLogger(processLogFunc)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout: %w", err)
	}

	logf(log.LevelInfo, "starting process: %v", cmd)

	i.WG.Add(1)
	go d.startFrameReader(cancel, i.WG, stdout)

	err = process.Start(ctx)
	if err != nil {
		return fmt.Errorf("process crashed: %w", err)
	}
	return nil
}

func generateFFmpegArgs(
	c config,
	rtspProtocol string,
	rtspAddress string,
) []string {
	// Output.
	//	ffmpeg -loglevel info -hwaccel x -y -rtsp_transport tcp -i rtsp://ip
	//    -vf "fps=fps=3,scale=ih/2:iw/2" -f rawvideo -pix_fmt gray -

	var args []string

	args = append(args, "-y", "-threads", "1", "-loglevel", c.logLevel)

	if c.hwaccel != "" {
		args = append(args, ffmpeg.ParseArgs("-hwaccel "+c.hwaccel)...)
	}

	args = append(args, "-rtsp_transport", rtspProtocol, "-i", rtspAddress)

	scale := strconv.Itoa(c.scale)
	args = append(args, "-vf", "fps=fps="+c.feedRate+",scale=iw/"+scale+":ih/"+scale)
	args = append(args, "-f", "rawvideo", "-pix_fmt", "gray", "-")

	return args
}

type detector struct {
	sendEvent monitor.SendEventFunc
	logf      log.Func
	config    config

	frameSize int
	zones     zones
}

var errScaleInvalid = errors.New("scale invalid")

func newDetector(
	i *monitor.InputProcess,
	conf config,
	logf log.Func,
	width int,
	height int,
) (*detector, error) {
	if width%conf.scale != 0 {
		return nil, fmt.Errorf("%w: cannot divide width by scale %v/%v",
			errScaleInvalid, width, conf.scale)
	}
	if height%conf.scale != 0 {
		return nil, fmt.Errorf("%w: cannot divide height by scale %v/%v",
			errScaleInvalid, height, conf.scale)
	}

	width /= conf.scale
	height /= conf.scale

	zones := make([]*zone, len(conf.zones))
	for i, zoneConfig := range conf.zones {
		if !zoneConfig.Enable {
			continue
		}
		zones[i] = newZone(width, height, zoneConfig)
	}

	return &detector{
		sendEvent: i.SendEvent,
		logf:      logf,
		config:    conf,

		frameSize: width * height,
		zones:     zones,
	}, nil
}

func (d detector) startFrameReader(
	cancel context.CancelFunc,
	wg *sync.WaitGroup,
	stdout io.Reader,
) {
	defer wg.Done()
	err := d.runFrameReader(stdout)
	if err != nil && !errors.Is(err, io.EOF) {
		d.logf(log.LevelError, "frame reader: %v", err)
	}
	cancel()
}

func (d detector) runFrameReader(stdout io.Reader) error {
	firstFrame := true
	frameBuf := make([]uint8, d.frameSize)
	prevFrameBuf := make([]uint8, d.frameSize)
	diffBuf := make([]uint8, d.frameSize)

	onActive := func(zone int, score float64) {
		d.logf(log.LevelDebug, "detection: zone:%v score:%.2f", zone, score)
		t := time.Now().Add(-d.config.timestampOffset)
		d.sendEvent(storage.Event{ //nolint:errcheck
			Detections: []storage.Detection{
				{Score: score},
			},
			Time:        t,
			Duration:    d.config.duration,
			RecDuration: d.config.recDuration,
		})
	}

	for {
		_, err := io.ReadFull(stdout, frameBuf)
		if err != nil {
			return fmt.Errorf("read frame: %w", err)
		}

		if firstFrame {
			prevFrameBuf, frameBuf = frameBuf, prevFrameBuf
			firstFrame = false
			continue
		}

		d.zones.analyze(frameBuf, prevFrameBuf, diffBuf, onActive)
		prevFrameBuf, frameBuf = frameBuf, prevFrameBuf
	}
}
