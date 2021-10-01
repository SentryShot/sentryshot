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

package motion

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"nvr"
	"nvr/pkg/ffmpeg"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func init() {
	nvr.RegisterMonitorMainProcessHook(main)
	nvr.RegisterMonitorSubProcessHook(sub)
	nvr.RegisterLogSource([]string{"motion"})
}

func main(ctx context.Context, m *monitor.Monitor, args *string) {
	if m.Config["motionDetection"] != "true" || m.SubInputEnabled() {
		return
	}
	*args += genArgs(m)

	if err := onMonitorStart(ctx, m); err != nil {
		m.Log.Error().
			Src("motion").
			Monitor(m.ID()).
			Msgf("failed to start %v", err)
	}
}

func sub(ctx context.Context, m *monitor.Monitor, args *string) {
	if m.Config["motionDetection"] != "true" || !m.SubInputEnabled() {
		return
	}
	*args += genArgs(m)

	if err := onMonitorStart(ctx, m); err != nil {
		m.Log.Error().
			Src("motion").
			Monitor(m.ID()).
			Msgf("failed to start %v", err)
	}
}

func genArgs(m *monitor.Monitor) string {
	pipePath := m.Env.SHMDir + "/motion/" + m.ID() + "/main.fifo"

	return " -c:v copy -map 0:v -f fifo -fifo_format mpegts" +
		" -drop_pkts_on_overflow 1 -attempt_recovery 1" +
		" -restart_with_keyframe 1 -recovery_wait_time 1 " + pipePath
}

func onMonitorStart(ctx context.Context, m *monitor.Monitor) error {
	if m.Config["motionDetection"] != "true" {
		return nil
	}

	a := newAddon(m)

	if err := os.MkdirAll(a.zonesDir(), 0700); err != nil && err != os.ErrExist {
		return fmt.Errorf("could not make directory for zones: %v", err)
	}

	if err := ffmpeg.MakePipe(a.mainPipe()); err != nil {
		return fmt.Errorf("could not make main pipe: %v", err)
	}

	var err error
	a.zones, err = a.unmarshalZones()
	if err != nil {
		return fmt.Errorf("could not unmarshal zones: %v", err)
	}

	a.duration, err = ffmpeg.FeedRateToDuration(a.m.Config["motionFeedRate"])
	if err != nil {
		return fmt.Errorf("could not parse duration: %v", err)
	}

	scale := parseScale(m.Config["motionFrameScale"])
	masks, err := a.generateMasks(a.zones, scale)
	if err != nil {
		return fmt.Errorf("could not generate mask: %v", err)
	}

	detectorArgs := a.generateDetectorArgs(masks, m.Config["hwaccel"], scale)

	durationInt, err := strconv.Atoi(a.m.Config["motionDuration"])
	if err != nil {
		return fmt.Errorf("could not parse motionDuration: %v", err)
	}
	a.recDuration = time.Duration(durationInt) * time.Second

	go a.startDetector(ctx, detectorArgs)

	return nil
}

type area []ffmpeg.Point
type zone struct {
	Enable    bool    `json:"enable"`
	Threshold float64 `json:"threshold"`
	Area      area    `json:"area"`
}

func (zone zone) calculatePolygon(w int, h int) ffmpeg.Polygon {
	polygon := make(ffmpeg.Polygon, len(zone.Area))
	for i, point := range zone.Area {
		px := point[0]
		py := point[1]
		polygon[i] = [2]int{int(float32(w) * (float32(px) / 100)), int(float32(h) * (float32(py) / 100))}
	}

	return polygon
}

type addon struct {
	m   *monitor.Monitor
	env *storage.ConfigEnv

	zones       []zone
	duration    time.Duration
	recDuration time.Duration
}

func newAddon(m *monitor.Monitor) addon {
	return addon{
		m:   m,
		env: m.Env,
	}
}

func (a addon) fifoDir() string {
	return a.env.SHMDir + "/motion/"
}

func (a addon) zonesDir() string {
	return a.fifoDir() + a.m.ID()
}

func (a addon) mainPipe() string {
	return a.fifoDir() + a.m.ID() + "/main.fifo"
}

func (a addon) unmarshalZones() ([]zone, error) {
	var zones []zone
	err := json.Unmarshal([]byte(a.m.Config["motionZones"]), &zones)

	return zones, err
}

func (zone zone) generateMask(w int, h int) image.Image {
	polygon := zone.calculatePolygon(w, h)

	return ffmpeg.CreateMask(w, h, polygon)
}

func (a addon) generateMasks(zones []zone, scale string) ([]string, error) {
	masks := make([]string, 0, len(zones))
	for i, zone := range zones {
		if !zone.Enable {
			continue
		}

		var size []string
		if a.m.SubInputEnabled() {
			size = strings.Split(a.m.Config["sizeMain"], "x")
		} else {
			size = strings.Split(a.m.Config["sizeSub"], "x")
		}
		w, _ := strconv.Atoi(size[0])
		h, _ := strconv.Atoi(size[1])

		s, _ := strconv.Atoi(scale)

		mask := zone.generateMask(w/s, h/s)
		maskPath := a.zonesDir() + "/zone" + strconv.Itoa(i) + ".png"
		masks = append(masks, maskPath)
		if err := ffmpeg.SaveImage(maskPath, mask); err != nil {
			return nil, fmt.Errorf("could not save mask: %v", err)
		}
	}
	return masks, nil
}

func (a addon) generateDetectorArgs(masks []string, hwaccel string, scale string) []string {
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

	if hwaccel != "" {
		args = append(args, ffmpeg.ParseArgs("-hwaccel "+hwaccel)...)
	}

	args = append(args, "-i", a.mainPipe())
	for _, mask := range masks {
		args = append(args, "-i", mask)
	}
	args = append(args, "-filter_complex")

	feedrate := a.m.Config["motionFeedRate"]
	filter := "[0:v]fps=fps=" + feedrate + ",scale=iw/" + scale + ":ih/" + scale + ",split=" + strconv.Itoa(len(masks))

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

func (a addon) startDetector(ctx context.Context, args []string) {
	a.m.WG.Add(1)

	for {
		if ctx.Err() != nil {
			a.m.WG.Done()
			a.m.Log.Info().
				Src("motion").
				Monitor(a.m.ID()).
				Msg("detector stopped")

			return
		}
		if err := a.detectorProcess(ctx, args); err != nil {
			a.m.Log.Error().
				Src("motion").
				Monitor(a.m.ID()).
				Msg(err.Error())

			time.Sleep(1 * time.Second)
		}
	}
}

func (a addon) detectorProcess(ctx context.Context, args []string) error {
	cmd := exec.Command(a.env.FFmpegBin, args...)

	process := ffmpeg.NewProcess(cmd)
	process.SetPrefix("motion: process:")
	process.SetStdoutLogger(a.m.Log)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr: %v", err)
	}

	a.m.Log.Info().
		Src("motion").
		Monitor(a.m.ID()).
		Msgf("starting detector: %v", cmd)

	go a.parseFFmpegOutput(stderr)

	err = process.Start(ctx)

	if err != nil {
		return fmt.Errorf("detector crashed: %v", err)
	}
	return nil
}

func (a addon) parseFFmpegOutput(stderr io.Reader) {
	output := bufio.NewScanner(stderr)
	p := newParser()
	for output.Scan() {
		line := output.Text()

		id, score := p.parseLine(line)

		if score == 0 {
			continue
		}

		//m.Log.Println(id, score)
		if a.zones[id].Threshold < score {
			a.sendTrigger(id, score)
		}
	}
}

func (a addon) sendTrigger(id int, score float64) {
	now := time.Now().Local()
	timestamp := fmt.Sprintf("%v:%v:%v", now.Hour(), now.Minute(), now.Second())

	a.m.Log.Info().
		Src("motion").
		Monitor(a.m.ID()).
		Msgf("trigger id:%v score:%.2f time:%v\n", id, score, timestamp)

	a.m.Trigger <- monitor.Event{
		Detections: []monitor.Detection{
			{
				Score: score,
			},
		},
		Time:        time.Now(),
		Duration:    a.duration,
		RecDuration: a.recDuration,
	}
}

/*
func drainReader(r io.Reader) {
	b := make([]byte, 1024)
	for {
		if _, err := r.Read(b); err != nil {
			return
		}
	}
}
*/

func parseScale(scale string) string {
	switch strings.ToLower(scale) {
	case "full":
		return "1"
	case "half":
		return "2"
	case "third":
		return "3"
	case "quarter":
		return "4"
	case "sixth":
		return "6"
	case "eighth":
		return "8"
	default:
		return "1"
	}
}

type parser struct {
	segment *string
}

func newParser() parser {
	segment := ""
	return parser{
		segment: &segment,
	}
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
