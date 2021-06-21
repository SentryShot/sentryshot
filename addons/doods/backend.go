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

package doods

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"nvr"
	"nvr/addons/doods/odrpc"
	"nvr/pkg/ffmpeg"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
)

func init() {
	nvr.RegisterMonitorStartHook(func(ctx context.Context, m *monitor.Monitor) {
		if err := onMonitorStart(ctx, m); err != nil {
			m.Log.Printf("%v: doods: %v\n", m.Name(), err)
		}
	})
	nvr.RegisterMonitorStartProcessHook(modifyMainArgs)
}

func modifyMainArgs(_ context.Context, m *monitor.Monitor, args *string) {
	if m.Config["doodsEnable"] != "true" {
		return
	}

	pipePath := m.Env.SHMDir + "/doods/" + m.ID() + "/main.fifo"

	*args += " -c:v copy -map 0:v -f fifo -fifo_format mpegts" +
		" -drop_pkts_on_overflow 1 -attempt_recovery 1" +
		" -restart_with_keyframe 1 -recovery_wait_time 1 " + pipePath
}

func onMonitorStart(ctx context.Context, m *monitor.Monitor) error {
	if m.Config["doodsEnable"] != "true" {
		return nil
	}

	detector, err := detectorByName(m.Config["doodsDetector"])
	if err != nil {
		return fmt.Errorf("could not get detectory: %v", err)
	}

	config, err := parseConfig(m, doodsIP, detector)
	if err != nil {
		return fmt.Errorf("could not parse config: %v", err)
	}

	a := newAddon(m, config)

	if err := a.prepareEnvironment(); err != nil {
		return fmt.Errorf("could not prepare environment: %v", err)
	}

	ffmpegArgs := a.generateFFmpegArgs(m.Config)
	a.wg.Add(1)
	go a.newFFmpeg(ffmpegArgs).start(ctx)

	return nil
}

type thresholds map[string]float64

type doodsConfig struct {
	ip           string
	width        int
	height       int
	duration     time.Duration
	recDuration  time.Duration
	thresholds   thresholds
	detectorName string
}

func parseConfig(m *monitor.Monitor, ip string, detector odrpc.Detector) (*doodsConfig, error) {
	scale := m.Config["doodsFrameScale"]

	width, height, err := parseSize(m.Size(), scale, detector)
	if err != nil {
		return nil, fmt.Errorf("could not parse size: %v", err)
	}

	var t thresholds
	if err := json.Unmarshal([]byte(m.Config["doodsThresholds"]), &t); err != nil {
		return nil, fmt.Errorf("could not unmarshal thresholds: %v", err)
	}
	for key, thresh := range t {
		if thresh == -1 {
			delete(t, key)
		}
	}

	feedRate := m.Config["doodsFeedRate"]
	duration, err := ffmpeg.FeedRateToDuration(feedRate)
	if err != nil {
		return nil, err
	}

	recDurationFloat, err := strconv.ParseFloat(m.Config["doodsDuration"], 64)
	if err != nil {
		return nil, fmt.Errorf("could not parse doodsDuration: %v", err)
	}
	recDuration := time.Duration(recDurationFloat * float64(time.Second))

	config := &doodsConfig{
		ip: ip,

		width:  width,
		height: height,

		thresholds:   t,
		duration:     duration,
		recDuration:  recDuration,
		detectorName: m.Config["doodsDetectorName"],
	}
	return config, nil
}

func parseSize(size string, scale string, detector odrpc.Detector) (int, int, error) {
	split := strings.Split(size, "x")
	width, err := strconv.Atoi(split[0])
	if err != nil {
		return 0, 0, err
	}
	height, err := strconv.Atoi(split[1])
	if err != nil {
		return 0, 0, err
	}

	// If detector has a required size.
	if detector.Width > 0 && detector.Height > 0 {
		width = int(detector.Width)
		height = int(detector.Height)
		return width, height, nil
	}

	ratio := textToScale(scale)
	width /= ratio
	height /= ratio

	return width, height, nil
}

func textToScale(input string) int {
	switch strings.ToLower(input) {
	case "full":
		return 1
	case "half":
		return 2
	case "third":
		return 3
	case "quarter":
		return 4
	case "sixth":
		return 6
	case "eighth":
		return 8
	default:
		return 1
	}
}

func newAddon(m *monitor.Monitor, c *doodsConfig) *addon {
	return &addon{
		c:       c,
		wg:      m.WG,
		id:      m.ID(),
		name:    m.Name(),
		log:     m.Log,
		trigger: m.Trigger,

		env: m.Env,

		runFFmpeg: runFFmpeg,
	}
}

type addon struct {
	c       *doodsConfig
	id      string
	wg      *sync.WaitGroup
	name    string
	log     *log.Logger
	trigger monitor.Trigger

	env *storage.ConfigEnv

	runFFmpeg runFFmpegFunc
}

func (a *addon) fifoDir() string {
	return a.env.SHMDir + "/doods/" + a.id
}

func (a *addon) mainPipe() string {
	return a.fifoDir() + "/main.fifo"
}

func (a *addon) prepareEnvironment() error {
	if err := os.MkdirAll(a.fifoDir(), 0700); err != nil && err != os.ErrExist {
		return fmt.Errorf("could not make directory for pipe: %v", err)
	}
	if err := ffmpeg.MakePipe(a.mainPipe()); err != nil {
		return fmt.Errorf("could not make main pipe: %v", err)
	}

	return nil
}

func (a *addon) generateFFmpegArgs(config monitor.Config) []string {
	// Output
	// ffmpeg -hwaccel x -i main.pipe -filter 'fps=fps=3,scale=ih/2:iw/2' -f rawvideo -pix_fmt rgb24 -

	logLevel := config["logLevel"]
	width := strconv.Itoa(a.c.width)
	height := strconv.Itoa(a.c.height)
	scale := width + "x" + height

	fps := config["doodsFeedRate"]

	var args []string

	args = append(args, "-y", "-loglevel", logLevel)

	hwaccel := config["hwaccel"]
	if hwaccel != "" {
		args = append(args, ffmpeg.ParseArgs("-hwaccel "+hwaccel)...)
	}

	args = append(args, "-i", a.mainPipe(), "-filter")
	args = append(args, "fps=fps="+fps+",scale="+scale)
	args = append(args, "-f", "rawvideo")
	args = append(args, "-pix_fmt", "rgb24", "-")

	return args
}

func (a *addon) newFFmpeg(args []string) *ffmpegConfig {
	return &ffmpegConfig{
		a:    a,
		args: args,

		d: &doodsClient{
			a:         a,
			c:         a.c,
			runClient: runClient,
			encoder: png.Encoder{
				CompressionLevel: png.BestSpeed,
			},
			sendFrame: sendFrame,
		},

		newProcess:  ffmpeg.NewProcess,
		runFFmpeg:   runFFmpeg,
		startClient: startClient,
	}
}

type ffmpegConfig struct {
	a    *addon
	d    *doodsClient
	args []string

	runFFmpeg   runFFmpegFunc
	newProcess  newProcessFunc
	startClient startClientFunc
}

type runFFmpegFunc func(context.Context, *ffmpegConfig) error
type newProcessFunc func(*exec.Cmd) ffmpeg.Process

func (f *ffmpegConfig) start(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			f.a.wg.Done()
			f.a.log.Printf("%v: doods: process stopped\n", f.a.name)
			return
		}
		if err := f.runFFmpeg(ctx, f); err != nil {
			f.a.log.Printf("%v: doods: process crashed: %v\n", f.a.name, err)
			time.Sleep(1 * time.Second)
		}
	}
}

func runFFmpeg(ctx context.Context, f *ffmpegConfig) error {
	cmd := exec.Command(f.a.env.FFmpegBin, f.args...)
	process := f.newProcess(cmd)
	process.SetPrefix(f.a.name + ": doods: process: ")
	process.SetStderrLogger(f.a.log)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stderr: %v", err)
	}

	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()

	f.d.stdout = stdout
	f.a.wg.Add(1)
	go f.startClient(ctx2, f.d)

	f.a.log.Printf("%v: doods: starting process: %v\n", f.a.name, cmd)
	err = process.Start(ctx)

	if err != nil {
		return fmt.Errorf("detector crashed: %v", err)
	}
	cancel()
	return nil
}

type doodsClient struct {
	a *addon
	c *doodsConfig

	stdout io.Reader
	stream *odrpc.OdrpcDetectStreamClient

	runClient runClientFunc
	encoder   png.Encoder
	sendFrame sendFrameFunc
}

type startClientFunc func(context.Context, *doodsClient)
type runClientFunc func(context.Context, *doodsClient) error
type sendFrameFunc func(*doodsClient, time.Time, *bytes.Buffer) error

func startClient(ctx context.Context, d *doodsClient) {
	for {
		if ctx.Err() != nil {
			d.a.log.Printf("%v: doods: client stopped\n", d.a.name)
			d.a.wg.Done()
			return
		}
		if err := d.runClient(ctx, d); err != nil {
			d.a.log.Printf("%v: doods: client crashed: %v\n", d.a.name, err)
			time.Sleep(1 * time.Second)
		}
	}
}

var dialOptions = []grpc.DialOption{
	grpc.WithBlock(),
	grpc.WithInsecure(),
}

func runClient(ctx context.Context, d *doodsClient) error {
	ctx2, cancel := context.WithTimeout(ctx, 3000*time.Millisecond)
	defer cancel()

	conn, err := grpc.DialContext(ctx2, d.c.ip, dialOptions...)
	if err != nil {
		return fmt.Errorf("could not connect to server: %v", err)
	}
	defer conn.Close()

	rpcClient := odrpc.NewOdrpcClient(conn)

	d.stream, err = rpcClient.DetectStream(ctx)
	if err != nil {
		return fmt.Errorf("could not open stream: %v", err)
	}
	if err := d.readFrames(ctx); err != nil {
		return fmt.Errorf("could not read frames: %v", err)
	}

	if err := d.stream.CloseSend(); err != nil {
		return fmt.Errorf("could not close stream: %v", err)
	}

	return nil
}

func (d *doodsClient) readFrames(ctx context.Context) error {
	rect := image.Rect(0, 0, d.c.width, d.c.height)
	frameSize := d.c.width * d.c.height * 3

	tmp := make([]byte, frameSize)
	for {
		if ctx.Err() != nil {
			return nil
		}
		if _, err := io.ReadAtLeast(d.stdout, tmp, frameSize); err != nil {
			if err == io.EOF {
				fmt.Println("eof")
				return nil
			}
			return fmt.Errorf("could not read from stdout: %v", err)
		}
		t := time.Now()

		img := NewRGB24(rect)
		img.Pix = tmp

		var b bytes.Buffer
		_ = d.encoder.Encode(&b, img)

		err := d.sendFrame(d, t, &b)
		if err != nil {
			return fmt.Errorf("could not send frame: %v", err)
		}
	}
}

func sendFrame(d *doodsClient, t time.Time, b *bytes.Buffer) error {
	request := &odrpc.DetectRequest{
		DetectorName: d.c.detectorName,
		Data:         b.Bytes(),
		Detect: map[string]float32{
			"*": 10,
		},
	}
	//fmt.Println("sending")
	if err := d.stream.Send(request); err != nil {
		return fmt.Errorf("could not send: %v", err)
	}

	response, err := d.stream.Recv()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not receive: %v", err)
	}

	d.parseDetections(t, response.Detections)
	return nil
}

func (d *doodsClient) parseDetections(t time.Time, detections []*odrpc.Detection) {
	if len(detections) == 0 {
		return
	}

	filtered := []monitor.Detection{}

	for _, detection := range detections {
		score := float64(detection.GetConfidence())
		label := detection.GetLabel()

		for name, thresh := range d.c.thresholds {
			if label != name || score < thresh {
				continue
			}

			conv := func(input float32) int {
				return int(input * 100)
			}

			d := monitor.Detection{
				Label: label,
				Score: score,
				Region: &monitor.Region{
					Rect: &ffmpeg.Rect{
						conv(detection.GetTop()),
						conv(detection.GetLeft()),
						conv(detection.GetBottom()),
						conv(detection.GetRight()),
					},
				},
			}
			filtered = append(filtered, d)
		}
	}

	if len(filtered) != 0 {
		now := time.Now().Local()
		timestamp := fmt.Sprintf("%v:%v:%v", now.Hour(), now.Minute(), now.Second())

		d.a.log.Printf("%v: doods: trigger: label:%v score:%.1f time:%v\n",
			d.a.name, filtered[0].Label, filtered[0].Score, timestamp)

		d.a.trigger <- monitor.Event{
			Time:        t,
			Detections:  filtered,
			Duration:    d.c.duration,
			RecDuration: d.c.recDuration,
		}
	}
}
