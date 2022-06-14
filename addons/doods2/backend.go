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

package doods

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"nvr"
	"nvr/pkg/ffmpeg"
	"nvr/pkg/log"
	"nvr/pkg/monitor"
	"nvr/pkg/storage"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

func init() {
	nvr.RegisterMonitorInputProcessHook(onInputProcessStart)
}

func onInputProcessStart(ctx context.Context, i *monitor.InputProcess, _ *[]string) {
	m := i.M
	if m.Config["doodsEnable"] != "true" {
		return
	}
	if useSubStream(m) != i.IsSubInput() {
		return
	}

	if err := start(ctx, i); err != nil {
		m.Log.Error().
			Src("doods").
			Monitor(m.Config.ID()).
			Msgf("could not start: %v", err)
	}
}

func useSubStream(m *monitor.Monitor) bool {
	if m.Config.SubInputEnabled() && m.Config["doodsUseSubStream"] == "true" {
		return true
	}
	return false
}

func start(ctx context.Context, input *monitor.InputProcess) error {
	detectorName := input.M.Config["doodsDetectorName"]
	detector, err := detectorByName(detectorName)
	if err != nil {
		return fmt.Errorf("get detector: %w", err)
	}

	config, err := parseConfig(input.M.Config)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	i := newInstance(addon.sendRequest, input, *config)

	outputWidth := int(detector.Width)
	outputHeight := int(detector.Height)

	inputs, err := parseInputs(
		input.Width(),
		input.Height(),
		input.M.Config["doodsCrop"],
		outputWidth,
		outputHeight,
		detectorName)
	if err != nil {
		return fmt.Errorf("parse inputs: %w", err)
	}

	outputs, reverseValues, err := calculateOutputs(inputs)
	if err != nil {
		return fmt.Errorf("calculate ffmpeg outputs: %w", err)
	}
	i.outputs = *outputs
	i.reverseValues = *reverseValues

	maskPath, err := i.generateMask(input.M.Config["doodsMask"])
	if err != nil {
		return fmt.Errorf("generate mask: %w", err)
	}

	i.ffArgs = i.generateFFmpegArgs(input.M.Config, maskPath, inputs.grayMode)

	i.wg.Add(1)
	go i.startFFmpeg(ctx)

	return nil
}

type config struct {
	eventDuration   time.Duration
	recDuration     time.Duration
	thresholds      thresholds
	timestampOffset time.Duration
	detectorName    string
}

func parseConfig(conf monitor.Config) (*config, error) {
	var t thresholds
	if err := json.Unmarshal([]byte(conf["doodsThresholds"]), &t); err != nil {
		return nil, fmt.Errorf("unmarshal thresholds: %w", err)
	}
	for key, thresh := range t {
		if thresh == -1 {
			delete(t, key)
		}
	}

	feedRate := conf["doodsFeedRate"]
	duration, err := ffmpeg.FeedRateToDuration(feedRate)
	if err != nil {
		return nil, err
	}

	recDurationFloat, err := strconv.ParseFloat(conf["doodsDuration"], 64)
	if err != nil {
		return nil, fmt.Errorf("parse doodsDuration: %w", err)
	}
	recDuration := time.Duration(recDurationFloat * float64(time.Second))

	timestampOffset, err := strconv.Atoi(conf["timestampOffset"])
	if err != nil {
		return nil, fmt.Errorf("parse timestamp offset %w", err)
	}

	return &config{
		eventDuration:   duration,
		recDuration:     recDuration,
		thresholds:      t,
		timestampOffset: time.Duration(timestampOffset) * time.Millisecond,
		detectorName:    conf["doodsDetectorName"],
	}, nil
}

func newInstance(sendRequest sendRequestFunc, i *monitor.InputProcess, c config) *instance {
	mConf := i.M.Config
	return &instance{
		c:            c,
		wg:           i.M.WG,
		monitorID:    mConf.ID(),
		monitorName:  mConf.Name(),
		rtspAddress:  i.RTSPaddress(),
		rtspProtocol: i.RTSPprotocol(),
		log:          i.M.Log,
		logLevel:     mConf.LogLevel(),
		sendEvent:    i.M.SendEvent,
		warmup:       10 * time.Second,

		env: i.M.Env,

		newProcess:    ffmpeg.NewProcess,
		runFFmpeg:     runFFmpeg,
		startInstance: startInstance,
		runInstance:   runInstance,
		sendRequest:   sendRequest,

		encoder: png.Encoder{
			CompressionLevel: png.BestSpeed,
		},
	}
}

type instance struct {
	c            config
	monitorID    string
	monitorName  string
	rtspAddress  string
	rtspProtocol string
	log          *log.Logger
	logLevel     string
	sendEvent    monitor.SendEventFunc
	warmup       time.Duration

	outputs       outputs
	ffArgs        []string
	reverseValues reverseValues

	env storage.ConfigEnv
	wg  *sync.WaitGroup

	newProcess    ffmpeg.NewProcessFunc
	runFFmpeg     runFFmpegFunc
	startInstance startInstanceFunc
	runInstance   runInstanceFunc
	sendRequest   sendRequestFunc
	encoder       png.Encoder

	// watchdogTimer restart process if it stops outputting frames.
	watchdogTimer *time.Timer
}

type inputs struct {
	inputWidth   float64
	inputHeight  float64
	cropX        float64
	cropY        float64
	cropSize     float64
	outputWidth  float64
	outputHeight float64
	grayMode     bool
}

func parseInputs(
	inputWidth int,
	inputHeight int,
	rawCrop string,
	outputWidth int,
	outputHeight int,
	detectorName string,
) (*inputs, error) {
	var crop [3]float64
	if err := json.Unmarshal([]byte(rawCrop), &crop); err != nil {
		return nil, fmt.Errorf("unmarshal crop values: %w", err)
	}

	grayMode := false
	if len(detectorName) > 5 && detectorName[0:5] == "gray_" {
		grayMode = true
	}

	return &inputs{
		inputWidth:   float64(inputWidth),
		inputHeight:  float64(inputHeight),
		cropX:        crop[0],
		cropY:        crop[1],
		cropSize:     crop[2],
		outputWidth:  float64(outputWidth),
		outputHeight: float64(outputHeight),
		grayMode:     grayMode,
	}, nil
}

type outputs struct {
	paddedWidth  int
	paddedHeight int
	scaledWidth  int
	scaledHeight int
	cropX        string
	cropY        string
	width        int
	height       int
	frameSize    int
}

type reverseValues struct {
	paddingYmultiplier float32
	paddingXmultiplier float32
	uncropXfunc        func(float32) float32
	uncropYfunc        func(float32) float32
}

// ErrInvalidConfig .
var ErrInvalidConfig = errors.New("")

func calculateOutputs(i *inputs) (*outputs, *reverseValues, error) { //nolint:funlen
	if i.inputWidth < i.outputWidth {
		return nil, nil, fmt.Errorf("input width is less than output width, %v/%v %w",
			i.inputWidth, i.outputWidth, ErrInvalidConfig)
	}
	if i.inputHeight < i.outputHeight {
		return nil, nil, fmt.Errorf("input height is less than output height, %v/%v %w",
			i.inputHeight, i.outputHeight, ErrInvalidConfig)
	}

	paddedWidth := i.outputWidth * 100 / i.cropSize
	paddedHeight := i.outputHeight * 100 / i.cropSize

	cropOutX := paddedWidth * i.cropX / 100
	cropOutY := paddedHeight * i.cropY / 100

	widthRatio := i.inputWidth / i.outputWidth
	heightRatio := i.inputHeight / i.outputHeight

	scaledWidth := paddedWidth
	scaledHeight := paddedHeight

	var paddingXmultiplier float64 = 1
	var paddingYmultiplier float64 = 1

	if widthRatio > heightRatio {
		scaledHeight = i.inputHeight * paddedWidth / i.inputWidth
		paddingYmultiplier = paddedHeight / scaledHeight
	} else if widthRatio < heightRatio {
		scaledWidth = i.inputWidth * paddedHeight / i.inputHeight
		paddingXmultiplier = paddedWidth / scaledWidth
	}

	if scaledWidth > i.inputWidth {
		return nil, nil, fmt.Errorf("scaled width is greater than input width: %v/%v %w",
			scaledWidth, i.inputWidth, ErrInvalidConfig)
	}

	uncropXfunc := func(input float32) float32 {
		newMin := paddedWidth * i.cropX / 100
		newMax := paddedWidth * (i.cropX + i.cropSize) / 100
		newRange := newMax - newMin
		return float32((float64(input)*newRange + newMin) / paddedWidth)
	}
	uncropYfunc := func(input float32) float32 {
		newMin := paddedHeight * i.cropY / 100
		newMax := (paddedHeight * (i.cropY + i.cropSize) / 100)
		newRange := newMax - newMin
		return float32((float64(input)*newRange + newMin) / paddedHeight)
	}

	return &outputs{
			paddedWidth:  int(paddedWidth),
			paddedHeight: int(paddedHeight),
			scaledWidth:  int(scaledWidth),
			scaledHeight: int(scaledHeight),
			cropX:        strconv.Itoa(int(cropOutX)),
			cropY:        strconv.Itoa(int(cropOutY)),
			width:        int(i.outputWidth),
			height:       int(i.outputHeight),
			frameSize:    int(i.outputWidth * i.outputHeight * 3),
		},
		&reverseValues{
			paddingYmultiplier: float32(paddingYmultiplier),
			paddingXmultiplier: float32(paddingXmultiplier),
			uncropXfunc:        uncropXfunc,
			uncropYfunc:        uncropYfunc,
		}, nil
}

type mask struct {
	Enable bool           `json:"enable"`
	Area   ffmpeg.Polygon `json:"area"`
}

func (i *instance) generateMask(rawMask string) (string, error) {
	var m mask
	if err := json.Unmarshal([]byte(rawMask), &m); err != nil {
		return "", fmt.Errorf("unmarshal doodsMask: %w", err)
	}

	if !m.Enable {
		return "", nil
	}

	w := i.outputs.scaledWidth
	h := i.outputs.scaledHeight

	tempDir := filepath.Join(i.env.TempDir, "doods")
	err := os.MkdirAll(tempDir, 0o700)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("make temporary directory: %v: %w", tempDir, err)
	}

	maskPath := filepath.Join(tempDir, i.monitorID+"_mask.png")

	polygon := m.Area.ToAbs(w, h)
	mask := ffmpeg.CreateMask(w, h, polygon)

	if err := ffmpeg.SaveImage(maskPath, mask); err != nil {
		return "", fmt.Errorf("save mask: %w", err)
	}

	return maskPath, nil
}

func (i *instance) generateFFmpegArgs(c monitor.Config, maskPath string, grayMode bool) []string {
	// Output minimal
	// ffmpeg -i main.pipe -filter
	//   'fps=fps=3,scale=320:260,pad=320:320:0:0,crop:300:300:10:10'
	//   -f rawvideo -pix_fmt rgb24 -
	//
	// Output maximal
	// ffmpeg -hwaccel x -i main.pipe -i mask.png -filter_complex
	//   '[0:v]fps=fps=3,scale=320:260[bg];
	//     [bg][1:v]overlay,pad=320:320:0:0,crop:300:300:10:10,hue=s=0'
	//   -f rawvideo -pix_fmt rgb24 -
	//
	// Padding is done after scaling for higher efficiency.
	// Cropping must come after padding.
	// Mask is overlayed on scaled frame.

	o := i.outputs

	fps := c["doodsFeedRate"]
	scaledWidth := strconv.Itoa(o.scaledWidth)
	scaledHeight := strconv.Itoa(o.scaledHeight)

	// Padding cannot be equal to input in some cases. ffmpeg bug?
	paddedWidth := strconv.Itoa(o.paddedWidth + 1)
	paddedHeight := strconv.Itoa(o.paddedHeight + 1)

	outputWidth := strconv.Itoa(o.width)
	outputHeight := strconv.Itoa(o.height)

	var args []string

	args = append(args, "-y", "-threads", "1", "-loglevel", c.LogLevel())

	if c.Hwacell() != "" {
		args = append(args, ffmpeg.ParseArgs("-hwaccel "+c.Hwacell())...)
	}

	args = append(args, "-rtsp_transport", i.rtspProtocol, "-i", i.rtspAddress)

	var filter string
	filter += ",pad=" + paddedWidth + ":" + paddedHeight + ":0:0"
	filter += ",crop=" + outputWidth + ":" + outputHeight + ":" + o.cropX + ":" + o.cropY

	if grayMode {
		filter += ",hue=s=0"
	}

	if maskPath == "" {
		args = append(args,
			"-filter", "fps=fps="+fps+",scale="+scaledWidth+":"+scaledHeight+filter)
	} else {
		args = append(args,
			"-i", maskPath, "-filter_complex",
			"[0:v]fps=fps="+fps+",scale="+scaledWidth+":"+scaledHeight+"[bg];[bg][1:v]overlay"+filter)
	}

	args = append(args,
		"-f", "rawvideo", "-pix_fmt", "rgb24", "-")

	return args
}

func (i instance) startFFmpeg(ctx context.Context) {
	defer i.wg.Done()

	// Wait for monitor to warm up.
	select {
	case <-ctx.Done():
		return
	case <-time.After(i.warmup):
	}

	for {
		if ctx.Err() != nil {
			i.log.Info().Src("doods").Monitor(i.monitorID).Msg("process stopped")
			return
		}
		if err := i.runFFmpeg(ctx, i); err != nil {
			i.log.Error().
				Src("doods").
				Monitor(i.monitorID).
				Msgf("process crashed: %v", err)
			time.Sleep(3 * time.Second)
		}
	}
}

type runFFmpegFunc func(context.Context, instance) error

func runFFmpeg(ctx context.Context, i instance) error {
	cmd := exec.Command(i.env.FFmpegBin, i.ffArgs...)

	logFunc := func(msg string) {
		i.log.FFmpegLevel(i.logLevel).
			Src("doods").
			Monitor(i.monitorID).
			Msgf("process: %v", msg)
	}

	process := i.newProcess(cmd).
		StderrLogger(logFunc)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("could not get stderr pipe: %w", err)
	}

	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()

	i.watchdogTimer = time.AfterFunc(10*time.Second, func() {
		if ctx.Err() != nil {
			return
		}
		i.log.Error().
			Src("doods").
			Monitor(i.monitorID).
			Msg("watchdog: process stopped outputting frames, restarting")
		process.Stop()
	})

	i.wg.Add(1)
	go i.startInstance(ctx2, i, stdout)

	i.log.Info().
		Src("doods").
		Monitor(i.monitorID).
		Msgf("starting process: %v", cmd)

	err = process.Start(ctx) // Blocks until process exists.
	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("detector crashed: %w", err)
	}
	return nil
}

type startInstanceFunc func(context.Context, instance, io.Reader)

func startInstance(ctx context.Context, i instance, stdout io.Reader) {
	for {
		if ctx.Err() != nil {
			i.log.Info().
				Src("doods").
				Monitor(i.monitorID).
				Msg("instance stopped")

			i.wg.Done()
			return
		}
		err := i.runInstance(ctx, i, stdout)
		if err != nil && !errors.Is(err, context.Canceled) {
			i.log.Error().
				Src("doods").
				Monitor(i.monitorID).
				Msgf("instance crashed: %v", err)

			time.Sleep(3 * time.Second)
		}
	}
}

type runInstanceFunc func(context.Context, instance, io.Reader) error

func runInstance(ctx context.Context, i instance, stdout io.Reader) error {
	img := NewRGB24(image.Rect(0, 0, i.outputs.width, i.outputs.height))
	inputBuffer := make([]byte, i.outputs.frameSize)
	tmpBuffer := []byte{}
	outputBuffer := []byte{}

	for {
		if ctx.Err() != nil {
			return context.Canceled
		}
		if _, err := io.ReadAtLeast(stdout, inputBuffer, i.outputs.frameSize); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("could not read from stdout: %w", err)
		}
		t := time.Now().Add(-i.c.timestampOffset)
		i.watchdogTimer.Reset(10 * time.Second)

		img.Pix = inputBuffer
		b := bytes.NewBuffer(tmpBuffer)
		if err := i.encoder.Encode(b, img); err != nil {
			return fmt.Errorf("could not encode frame: %w", err)
		}
		outputBuffer = b.Bytes()

		request := detectRequest{
			DetectorName: i.c.detectorName,
			Data:         &outputBuffer,
			// Preprocess:   []string{"grayscale"},
			Detect: i.c.thresholds,
		}

		ctx2, cancel := context.WithTimeout(ctx, i.c.eventDuration*2)
		defer cancel()
		detections, err := i.sendRequest(ctx2, request)
		if err != nil {
			return fmt.Errorf("could not send frame: %w", err)
		}

		parsed := parseDetections(i.reverseValues, *detections)
		if len(parsed) == 0 {
			continue
		}

		i.log.Debug().
			Src("doods").
			Monitor(i.monitorID).
			Msgf("trigger: label:%v score:%.1f",
				parsed[0].Label, parsed[0].Score)

		err = i.sendEvent(storage.Event{
			Time:        t,
			Detections:  parsed,
			Duration:    i.c.eventDuration,
			RecDuration: i.c.recDuration,
		})
		if err != nil {
			i.log.Error().Src("doods").Monitor(i.monitorID).Msgf("could not send event: %v", err)
		}
	}
}

func parseDetections(reverse reverseValues, detections detections) []storage.Detection {
	parsed := []storage.Detection{}

	for _, detection := range detections {
		score := float64(detection.Confidence)
		label := detection.Label

		convX := func(input float32) int {
			return int(reverse.uncropXfunc(input) *
				reverse.paddingXmultiplier * 100)
		}
		convY := func(input float32) int {
			return int(reverse.uncropYfunc(input) *
				reverse.paddingYmultiplier * 100)
		}

		d := storage.Detection{
			Label: label,
			Score: score,
			Region: &storage.Region{
				Rect: &ffmpeg.Rect{
					convY(detection.Top),
					convX(detection.Left),
					convY(detection.Bottom),
					convX(detection.Right),
				},
			},
		}
		parsed = append(parsed, d)
	}
	return parsed
}
