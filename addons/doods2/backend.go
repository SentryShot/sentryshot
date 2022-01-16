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
	"strings"
	"sync"
	"time"
)

func init() {
	nvr.RegisterMonitorInputProcessHook(onInputProcessStart)
}

func onInputProcessStart(ctx context.Context, i *monitor.InputProcess, args *[]string) {
	m := i.M
	if m.Config["doodsEnable"] != "true" {
		return
	}
	if useSubStream(m) != i.IsSubInput() {
		return
	}

	modifyArgs(args, m)

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

func modifyArgs(args *[]string, m *monitor.Monitor) {
	pipePath := filepath.Join(m.Env.SHMDir, "doods", m.Config.ID(), "main.fifo")

	newArgs := []string{
		"-c:v", "copy", "-map", "0:v", "-f", "fifo", "-fifo_format", "mpegts",
		"-drop_pkts_on_overflow", "1", "-attempt_recovery", "1",
		"-restart_with_keyframe", "1", "-recovery_wait_time", "1", pipePath,
	}
	*args = append(*args, newArgs...)
}

func start(ctx context.Context, input *monitor.InputProcess) error {
	detectorName := input.M.Config["doodsDetectorName"]
	detector, err := detectorByName(detectorName)
	if err != nil {
		return fmt.Errorf("could not get detector: %w", err)
	}

	config, err := parseConfig(input.M)
	if err != nil {
		return fmt.Errorf("could not parse config: %w", err)
	}

	i := newInstance(addon.sendRequest, input.M, *config)

	if err := i.prepareEnvironment(); err != nil {
		return fmt.Errorf("could not prepare environment: %w", err)
	}

	outputWidth := int(detector.Width)
	outputHeight := int(detector.Height)

	inputs, err := parseInputs(
		input.Size(),
		input.M.Config["doodsCrop"],
		outputWidth,
		outputHeight,
		detectorName)
	if err != nil {
		return fmt.Errorf("could not parse inputs: %w", err)
	}

	outputs, reverseValues, err := calculateOutputs(inputs)
	if err != nil {
		return fmt.Errorf("could not calculate ffmpeg outputs: %w", err)
	}
	i.outputs = *outputs
	i.reverseValues = *reverseValues

	maskPath, err := i.generateMask(input.M.Config["doodsMask"])
	if err != nil {
		return fmt.Errorf("could not generate mask: %w", err)
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
	delay           time.Duration
	detectorName    string
}

func parseConfig(m *monitor.Monitor) (*config, error) {
	var t thresholds
	if err := json.Unmarshal([]byte(m.Config["doodsThresholds"]), &t); err != nil {
		return nil, fmt.Errorf("could not unmarshal thresholds: %w", err)
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
		return nil, fmt.Errorf("could not parse doodsDuration: %w", err)
	}
	recDuration := time.Duration(recDurationFloat * float64(time.Second))

	timestampOffset, err := strconv.Atoi(m.Config["timestampOffset"])
	if err != nil {
		return nil, fmt.Errorf("could not parse timestamp offset %w", err)
	}

	delay, err := strconv.Atoi(m.Config["doodsDelay"])
	if err != nil {
		return nil, fmt.Errorf("could not parse doodsDelay %w", err)
	}

	return &config{
		eventDuration:   duration,
		recDuration:     recDuration,
		thresholds:      t,
		timestampOffset: time.Duration(timestampOffset) * time.Millisecond,
		delay:           time.Duration(delay) * time.Millisecond,
		detectorName:    m.Config["doodsDetectorName"],
	}, nil
}

func newInstance(sendRequest sendRequestFunc, m *monitor.Monitor, c config) *instance {
	return &instance{
		c:           c,
		wg:          m.WG,
		monitorID:   m.Config.ID(),
		monitorName: m.Config.Name(),
		log:         m.Log,
		logLevel:    m.Config.LogLevel(),
		sendEvent:   m.SendEvent,

		env: *m.Env,

		newProcess:  ffmpeg.NewProcess,
		runFFmpeg:   runFFmpeg,
		startReader: startInstance,
		runInstance: runInstance,
		sendRequest: sendRequest,

		encoder: png.Encoder{
			CompressionLevel: png.BestSpeed,
		},
	}
}

type instance struct {
	c           config
	monitorID   string
	monitorName string
	log         *log.Logger
	logLevel    string
	sendEvent   monitor.SendEventFunc

	outputs       outputs
	ffArgs        []string
	reverseValues reverseValues

	env storage.ConfigEnv
	wg  *sync.WaitGroup

	newProcess  ffmpeg.NewProcessFunc
	runFFmpeg   runFFmpegFunc
	startReader startReaderFunc
	runInstance runInstanceFunc
	sendRequest sendRequestFunc
	encoder     png.Encoder
}

func (i *instance) fifoDir() string {
	return i.env.SHMDir + "/doods/" + i.monitorID
}

func (i *instance) mainPipe() string {
	return i.fifoDir() + "/main.fifo"
}

func (i *instance) prepareEnvironment() error {
	if err := os.MkdirAll(i.fifoDir(), 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("could not make directory for pipe: %w", err)
	}
	if err := ffmpeg.MakePipe(i.mainPipe()); err != nil {
		return fmt.Errorf("could not make main pipe: %w", err)
	}

	return nil
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
	size string,
	rawCrop string,
	outputWidth int,
	outputHeight int,
	detectorName string,
) (*inputs, error) {
	split := strings.Split(size, "x")
	inputWidth, err := strconv.ParseFloat(split[0], 64)
	if err != nil {
		return nil, fmt.Errorf("could not parse input width: %w %v", err, split)
	}
	inputHeight, err := strconv.ParseFloat(split[1], 64)
	if err != nil {
		return nil, fmt.Errorf("could not parse input height: %w %v", err, split)
	}

	var crop [3]float64
	if err := json.Unmarshal([]byte(rawCrop), &crop); err != nil {
		return nil, fmt.Errorf("could not Unmarshal crop values: %w", err)
	}

	grayMode := false
	if len(detectorName) > 5 && detectorName[0:5] == "gray_" {
		grayMode = true
	}

	return &inputs{
		inputWidth:   inputWidth,
		inputHeight:  inputHeight,
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
		return "", fmt.Errorf("could not unmarshal doodsMask: %w", err)
	}

	if !m.Enable {
		return "", nil
	}

	w := i.outputs.scaledWidth
	h := i.outputs.scaledHeight

	path := filepath.Join(i.env.SHMDir, "doods", i.monitorID+"_mask.png")

	polygon := m.Area.ToAbs(w, h)
	mask := ffmpeg.CreateMask(w, h, polygon)

	if err := ffmpeg.SaveImage(path, mask); err != nil {
		return "", fmt.Errorf("could not save mask: %w", err)
	}

	return path, nil
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

	args = append(args, "-i", i.mainPipe())

	var filter string
	filter += ",pad=" + paddedWidth + ":" + paddedHeight + ":0:0"
	filter += ",crop=" + outputWidth + ":" + outputHeight + ":" + o.cropX + ":" + o.cropY

	if grayMode {
		filter += ",hue=s=0"
	}

	if maskPath == "" {
		args = append(args, "-filter")
		args = append(args, "fps=fps="+fps+
			",scale="+scaledWidth+":"+scaledHeight+filter)
	} else {
		args = append(args, "-i", maskPath, "-filter_complex")
		args = append(args, "[0:v]fps=fps="+fps+
			",scale="+scaledWidth+":"+scaledHeight+"[bg];[bg][1:v]overlay"+filter)
	}

	args = append(args, "-f", "rawvideo")
	args = append(args, "-pix_fmt", "rgb24", "-")

	return args
}

func (i instance) startFFmpeg(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			i.wg.Done()
			i.log.Info().Src("doods").Monitor(i.monitorID).Msg("process stopped")
			return
		}
		if err := i.runFFmpeg(ctx, i); err != nil {
			i.log.Error().
				Src("doods").
				Monitor(i.monitorID).
				Msgf("process crashed: %v", err)

			time.Sleep(1 * time.Second)
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
		return fmt.Errorf("stderr: %w", err)
	}

	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()

	i.wg.Add(1)
	go i.startReader(ctx2, i, stdout)

	i.log.Info().
		Src("doods").
		Monitor(i.monitorID).
		Msgf("starting process: %v", cmd)

	if err = process.Start(ctx); err != nil {
		return fmt.Errorf("detector crashed: %w", err)
	}
	cancel()
	return nil
}

type startReaderFunc func(context.Context, instance, io.Reader)

func startInstance(ctx context.Context, i instance, stdout io.Reader) {
	for {
		if ctx.Err() != nil {
			i.log.Info().
				Src("doods").
				Monitor(i.monitorID).
				Msg("client stopped")

			i.wg.Done()
			return
		}
		if err := i.runInstance(ctx, i, stdout); err != nil {
			i.log.Error().
				Src("doods").
				Monitor(i.monitorID).
				Msgf("client crashed: %v", err)

			time.Sleep(1 * time.Second)
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
			return nil
		}
		if _, err := io.ReadAtLeast(stdout, inputBuffer, i.outputs.frameSize); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("could not read from stdout: %w", err)
		}
		t := time.Now().Add(-i.c.timestampOffset).Add(-i.c.delay)

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

		i.log.Info().
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
