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

type logFunc func(log.Level, string, ...interface{})

func onInputProcessStart(ctx context.Context, i *monitor.InputProcess, _ *[]string) {
	id := i.M.Config.ID()

	logf := func(level log.Level, format string, a ...interface{}) {
		i.M.Log.Level(level).Src("doods").Monitor(id).Msgf(format, a...)
	}

	config, enable, err := parseConfig(i.M.Config)
	if err != nil {
		logf(log.LevelError, "could not parse config: %v", err)
		return
	}
	if !enable || config.useSubStream != i.IsSubInput() {
		return
	}
	config.fillMissing()
	if err := config.validate(); err != nil {
		logf(log.LevelError, "config: %v", err)
	}

	if err := start(ctx, i, *config, logf); err != nil {
		logf(log.LevelError, "could not start: %v", err)
	}
}

func start(
	ctx context.Context,
	input *monitor.InputProcess,
	config config,
	logf logFunc,
) error {
	detector, err := detectorByName(config.detectorName)
	if err != nil {
		return fmt.Errorf("get detector: %w", err)
	}

	i := newInstance(addon.sendRequest, input, config, logf)

	inputs := inputs{
		inputWidth:   float64(input.Width()),
		inputHeight:  float64(input.Height()),
		outputWidth:  float64(detector.Width),
		outputHeight: float64(detector.Height),
	}
	outputs, reverseValues, err := calculateOutputs(config, inputs)
	if err != nil {
		return fmt.Errorf("calculate ffmpeg outputs: %w", err)
	}
	i.outputs = *outputs
	i.reverseValues = *reverseValues

	maskPath, err := i.generateMask(config.mask)
	if err != nil {
		return fmt.Errorf("generate mask: %w", err)
	}

	i.ffArgs = generateFFmpegArgs(
		*outputs,
		config,
		input.RTSPprotocol(),
		input.RTSPaddress(),
		maskPath,
	)

	i.wg.Add(1)
	go i.startFFmpeg(ctx)

	return nil
}

type instance struct {
	c         config
	logf      logFunc
	sendEvent monitor.SendEventFunc
	warmup    time.Duration

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

func newInstance(
	sendRequest sendRequestFunc,
	i *monitor.InputProcess,
	c config,
	logf logFunc,
) *instance {
	return &instance{
		c:         c,
		wg:        i.M.WG,
		logf:      logf,
		sendEvent: i.M.SendEvent,
		warmup:    10 * time.Second,

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

type inputs struct {
	inputWidth   float64
	inputHeight  float64
	outputWidth  float64
	outputHeight float64
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

func calculateOutputs(c config, i inputs) (*outputs, *reverseValues, error) { //nolint:funlen
	if i.inputWidth < i.outputWidth {
		return nil, nil, fmt.Errorf("input width is less than output width, %v/%v %w",
			i.inputWidth, i.outputWidth, ErrInvalidConfig)
	}
	if i.inputHeight < i.outputHeight {
		return nil, nil, fmt.Errorf("input height is less than output height, %v/%v %w",
			i.inputHeight, i.outputHeight, ErrInvalidConfig)
	}

	paddedWidth := i.outputWidth * 100 / c.cropSize
	paddedHeight := i.outputHeight * 100 / c.cropSize

	cropOutX := paddedWidth * c.cropX / 100
	cropOutY := paddedHeight * c.cropY / 100

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
		newMin := paddedWidth * c.cropX / 100
		newMax := paddedWidth * (c.cropX + c.cropSize) / 100
		newRange := newMax - newMin
		return float32((float64(input)*newRange + newMin) / paddedWidth)
	}
	uncropYfunc := func(input float32) float32 {
		newMin := paddedHeight * c.cropY / 100
		newMax := (paddedHeight * (c.cropY + c.cropSize) / 100)
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

func (i *instance) generateMask(m mask) (string, error) {
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

	maskPath := filepath.Join(tempDir, i.c.monitorID+"_mask.png")

	polygon := m.Area.ToAbs(w, h)
	mask := ffmpeg.CreateMask(w, h, polygon)

	if err := ffmpeg.SaveImage(maskPath, mask); err != nil {
		return "", fmt.Errorf("save mask: %w", err)
	}

	return maskPath, nil
}

func generateFFmpegArgs( //nolint:funlen
	out outputs,
	c config,
	rtspProtocol string,
	rtspAddress string,
	maskPath string,
) []string {
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

	fps := strconv.FormatFloat(c.feedRate, 'f', -1, 64)
	scaledWidth := strconv.Itoa(out.scaledWidth)
	scaledHeight := strconv.Itoa(out.scaledHeight)

	// Padding cannot be equal to input in some cases. ffmpeg bug?
	paddedWidth := strconv.Itoa(out.paddedWidth + 1)
	paddedHeight := strconv.Itoa(out.paddedHeight + 1)

	outputWidth := strconv.Itoa(out.width)
	outputHeight := strconv.Itoa(out.height)

	var args []string

	args = append(args, "-y", "-threads", "1", "-loglevel", c.ffmpegLogLevel)

	if c.hwaccel != "" {
		args = append(args, ffmpeg.ParseArgs("-hwaccel "+c.hwaccel)...)
	}

	args = append(args, "-rtsp_transport", rtspProtocol, "-i", rtspAddress)

	var filter string
	filter += ",pad=" + paddedWidth + ":" + paddedHeight + ":0:0"
	filter += ",crop=" + outputWidth + ":" + outputHeight + ":" + out.cropX + ":" + out.cropY

	if c.grayMode {
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
			i.logf(log.LevelInfo, "process stopped")
			return
		}
		if err := i.runFFmpeg(ctx, i); err != nil {
			i.logf(log.LevelError, "process crashed: %v", err)
			time.Sleep(3 * time.Second)
		}
	}
}

type runFFmpegFunc func(context.Context, instance) error

func runFFmpeg(ctx context.Context, i instance) error {
	cmd := exec.Command(i.env.FFmpegBin, i.ffArgs...)

	processLogFunc := func(msg string) {
		i.logf(log.FFmpegLevel(i.c.ffmpegLogLevel), "process: %v", msg)
	}

	process := i.newProcess(cmd).
		StderrLogger(processLogFunc)

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
		i.logf(log.LevelError, "watchdog: process stopped outputting frames, restarting")
		process.Stop()
	})

	i.wg.Add(1)
	go i.startInstance(ctx2, i, stdout)

	i.logf(log.LevelInfo, "starting process: %v", cmd)

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
			i.logf(log.LevelInfo, "instance stopped")
			i.wg.Done()
			return
		}
		err := i.runInstance(ctx, i, stdout)
		if err != nil && !errors.Is(err, context.Canceled) {
			i.logf(log.LevelError, "instance crashed: %v", err)
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
			}
		}
	}
}

type runInstanceFunc func(context.Context, instance, io.Reader) error

func runInstance(ctx context.Context, i instance, stdout io.Reader) error {
	eventDuration := ffmpeg.FeedRateToDuration(i.c.feedRate)

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

		ctx2, cancel := context.WithTimeout(ctx, eventDuration*2)
		defer cancel()
		detections, err := i.sendRequest(ctx2, request)
		if err != nil {
			return fmt.Errorf("could not send frame: %w", err)
		}

		parsed := parseDetections(i.reverseValues, *detections)
		if len(parsed) == 0 {
			continue
		}

		i.logf(log.LevelDebug, "trigger: label:%v score:%.1f",
			parsed[0].Label, parsed[0].Score)

		err = i.sendEvent(storage.Event{
			Time:        t,
			Detections:  parsed,
			Duration:    eventDuration,
			RecDuration: i.c.recDuration,
		})
		if err != nil {
			i.logf(log.LevelError, "could not send event: %v", err)
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
