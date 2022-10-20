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
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"nvr"
	"nvr/pkg/ffmpeg"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

func init() {
	nvr.RegisterMonitorInputProcessHook(onInputProcessStart)
	nvr.RegisterLogSource([]string{"motion"})

	nvr.RegisterTplHook(modifyTemplates)
	nvr.RegisterAppRunHook(func(_ context.Context, app *nvr.App) error {
		app.Mux.Handle("/motion.mjs", app.Auth.Admin(serveMotionMjs()))
		return nil
	})
}

func onInputProcessStart(ctx context.Context, i *monitor.InputProcess, _ *[]string) {
	i.MonitorLock.Lock()
	defer i.MonitorLock.Unlock()

	if i.Config.SubInputEnabled() != i.IsSubInput() {
		return
	}

	id := i.Config.ID()
	logf := func(level log.Level, format string, a ...interface{}) {
		i.Log.Level(level).Src("motion").Monitor(id).Msgf(format, a...)
	}

	config, enable, err := parseConfig(*i.Config)
	if err != nil {
		logf(log.LevelError, "could not parse config: %v", err)
		return
	}
	if !enable {
		return
	}

	i.WG.Add(1)
	go func() {
		defer i.WG.Done()
		// Wait for monitor to start.
		select {
		case <-time.After(10 * time.Second):
		case <-ctx.Done():
			return
		}
		if err := start(ctx, i, *config, logf); err != nil {
			logf(log.LevelError, "failed to start %v", err)
		}
	}()
}

func start(
	ctx context.Context,
	i *monitor.InputProcess,
	config config,
	logf log.Func,
) error {
	ctx2, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	streamInfo, err := i.StreamInfo(ctx2)
	if err != nil {
		return fmt.Errorf("stream info: %w", err)
	}
	width := streamInfo.VideoWidth
	height := streamInfo.VideoHeight

	d, err := newDetector(i, config, logf)
	if err != nil {
		return err
	}

	masks, err := generateMasks(
		d.config.zones,
		d.zonesDir,
		width,
		height,
		config.scale,
	)
	if err != nil {
		return fmt.Errorf("generate mask: %w", err)
	}

	args := generateArgs(masks, config, i.RTSPprotocol(), i.RTSPaddress())

	d.wg.Add(1)
	go d.startDetector(ctx, args)
	return nil
}

type detector struct {
	sendEvent monitor.SendEventFunc
	wg        *sync.WaitGroup
	env       storage.ConfigEnv
	logf      log.Func
	config    config
	zonesDir  string
}

func newDetector(i *monitor.InputProcess, conf config, logf log.Func) (*detector, error) {
	zonesDir := filepath.Join(os.TempDir(), "motion", conf.monitorID)
	err := os.MkdirAll(zonesDir, 0o700)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("make directory for zones: %w", err)
	}

	return &detector{
		sendEvent: i.SendEvent,
		wg:        i.WG,
		env:       i.Env,
		logf:      logf,
		config:    conf,
		zonesDir:  zonesDir,
	}, nil
}

func generateMasks(
	zones []zoneConfig,
	zonesDir string,
	width int,
	height int,
	scale int,
) ([]string, error) {
	masks := make([]string, 0, len(zones))
	for i, zone := range zones {
		if !zone.Enable {
			continue
		}

		mask := zone.generateMask(width/scale, height/scale)
		maskPath := zonesDir + "/zone" + strconv.Itoa(i) + ".png"
		masks = append(masks, maskPath)
		if err := ffmpeg.SaveImage(maskPath, mask); err != nil {
			return nil, fmt.Errorf("could not save mask: %w", err)
		}
	}
	return masks, nil
}

func generateArgs(
	masks []string,
	c config,
	rtspProtocol string,
	rtspAddress string,
) []string {
	var args []string

	// Final command will look something like this.
	/*	ffmpeg -hwaccel x -y -i rtsp://ip -i zone0.png -i zone1.png \
		-filter_complex "[0:v]fps=fps=3,scale=ih/2:iw/2,split=2[in1][in2]; \
		[in1][1:v]overlay,metadata=add:key=id:value=0,select='gte(scene\,0)',metadata=print[out1]; \
		[in2][2:v]overlay,metadata=add:key=id:value=1,select='gte(scene\,0)',metadata=print[out2]" \
		-map "[out1]" -f null - \
		-map "[out2]" -f null -
	*/

	args = append(args, "-y")

	if c.hwaccel != "" {
		args = append(args, ffmpeg.ParseArgs("-hwaccel "+c.hwaccel)...)
	}

	args = append(args, "-rtsp_transport", rtspProtocol, "-i", rtspAddress)

	for _, mask := range masks {
		args = append(args, "-i", mask)
	}
	args = append(args, "-filter_complex")

	scale := strconv.Itoa(c.scale)
	filter := "[0:v]fps=fps=" + c.feedRate +
		",scale=iw/" + scale + ":ih/" + scale + ",split=" + strconv.Itoa(len(masks))

	for i := range masks {
		filter += "[in" + strconv.Itoa(i) + "]"
	}

	for index := range masks {
		i := strconv.Itoa(index)
		filter += ";[in" + i + "][" + strconv.Itoa(index+1)
		filter += ":v]overlay"
		filter += ",metadata=add:key=id:value=" + i
		filter += ",select='gte(scene\\,0)'"
		filter += ",metadata=print[out" + i + "]"
	}
	args = append(args, filter)

	for index := range masks {
		i := strconv.Itoa(index)
		args = append(args, "-map", "[out"+i+"]", "-f", "null", "-")
	}

	return args
}

func (d detector) startDetector(ctx context.Context, args []string) {
	for {
		if ctx.Err() != nil {
			d.wg.Done()
			d.logf(log.LevelInfo, "detector stopped")

			return
		}
		if err := d.detectorProcess(ctx, args); err != nil {
			d.logf(log.LevelError, "%v", err)
			select {
			case <-ctx.Done():
			case <-time.After(1 * time.Second):
			}
		}
	}
}

func (d detector) detectorProcess(ctx context.Context, args []string) error {
	cmd := exec.Command(d.env.FFmpegBin, args...)

	processLogFunc := func(msg string) {
		d.logf(log.FFmpegLevel(d.config.logLevel), msg)
	}

	process := ffmpeg.NewProcess(cmd).
		StdoutLogger(processLogFunc)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr: %w", err)
	}

	d.logf(log.LevelInfo, "starting detector: %v", args)

	go d.parseFFmpegOutput(stderr)

	err = process.Start(ctx)

	if err != nil {
		return fmt.Errorf("detector crashed: %w", err)
	}

	return nil
}

func (d detector) parseFFmpegOutput(stderr io.Reader) {
	output := bufio.NewScanner(stderr)
	p := newParser()
	for output.Scan() {
		line := output.Text()

		id, score := p.parseLine(line)

		if score == 0 {
			continue
		}

		// m.Log.Println(id, score)
		if d.config.zones[id].Threshold < score {
			d.sendTrigger(id, score)
		}
	}
}

func (d detector) sendTrigger(id int, score float64) {
	d.logf(log.LevelDebug, "trigger id:%v score:%.2f\n", id, score)

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

type parser struct {
	segment *string
}

func newParser() parser {
	return parser{segment: new(string)}
}

// Stitch several lines into a segment.
/*	[Parsed_metadata_5 @ 0x] frame:35   pts:39      pts_time:19.504x
	[Parsed_metadata_5 @ 0x] id=0
	[Parsed_metadata_5 @ 0x] lavfi.scene_score=0.008761
*/
func (p parser) parseLine(line string) (int, float64) {
	*p.segment += "\n" + line
	endOfSegment := strings.Contains(line, "lavfi.scene_score")
	if endOfSegment {
		s := *p.segment
		*p.segment = line
		return parseSegment(s)
	}
	return 0, 0
}

func parseSegment(segment string) (int, float64) {
	// Input
	// [Parsed_metadata_12 @ 0x] id=3
	// [Parsed_metadata_12 @ 0x] lavfi.scene_score=0.050033

	// Output ["", 3, 0.05033]
	re := regexp.MustCompile(`\bid=(\d+)\b\n.*lavfi.scene_score=(\d.\d+)`)
	match := re.FindStringSubmatch(segment)

	if match == nil {
		return 0, 0
	}

	id, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, 0
	}

	score, err := strconv.ParseFloat(match[2], 64)
	if err != nil {
		return 0, 0
	}

	return id, score * 100
}
