// SPDX-License-Identifier: GPL-2.0-or-later

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
	"nvr/pkg/video/gortsplib/pkg/h264"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

func init() {
	nvr.RegisterMonitorInputProcessHook(onInputProcessStart)
}

func onInputProcessStart(ctx context.Context, i *monitor.InputProcess, _ *[]string) {
	id := i.Config.ID()
	logf := func(level log.Level, format string, a ...interface{}) {
		i.Logger.Log(log.Entry{
			Level:     level,
			Src:       "doods",
			MonitorID: id,
			Msg:       fmt.Sprintf(format, a...),
		})
	}

	config, enable, err := parseConfig(i.Config)
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
			logf(log.LevelError, "could not start: %v", err)
		}
	}()
}

func start(
	ctx context.Context,
	input *monitor.InputProcess,
	config config,
	logf log.Func,
) error {
	detector, err := detectorByName(config.detectorName)
	if err != nil {
		return fmt.Errorf("get detector: %w", err)
	}

	videoTrack, err := input.VideoTrack(ctx)
	if err != nil {
		return fmt.Errorf("get video track: %w", err)
	}

	var spsp h264.SPS
	err = spsp.Unmarshal(videoTrack.SPS)
	if err != nil {
		return fmt.Errorf("unmarshal spsp: %w", err)
	}

	inputs := inputs{
		inputWidth:   float64(spsp.Width()),
		inputHeight:  float64(spsp.Height()),
		outputWidth:  float64(detector.Width),
		outputHeight: float64(detector.Height),
	}
	outputs, reverseValues, err := calculateOutputs(config, inputs)
	if err != nil {
		return fmt.Errorf("calculate ffmpeg outputs: %w", err)
	}

	i := newInstance(addon.sendRequest, input, config, addon.previewCache, logf)

	i.outputs = *outputs
	i.reverseValues = *reverseValues

	i.ffArgs = generateFFmpegArgs(
		*outputs,
		config,
		input.RTSPprotocol(),
		input.RTSPaddress(),
	)

	i.wg.Add(1)
	go i.startProcess(ctx)

	return nil
}

type instance struct {
	c         config
	wg        *sync.WaitGroup
	env       storage.ConfigEnv
	logf      log.Func
	sendEvent monitor.SendEventFunc

	outputs       outputs
	ffArgs        []string
	reverseValues reverseValues

	newProcess   ffmpeg.NewProcessFunc
	startReader  startReaderFunc
	sendRequest  sendRequestFunc
	encoder      png.Encoder
	previewCache *previewCache

	// watchdogTimer restarts process if it stops outputting frames.
	watchdogTimer *time.Timer
}

func newInstance(
	sendRequest sendRequestFunc,
	i *monitor.InputProcess,
	c config,
	previewCache *previewCache,
	logf log.Func,
) *instance {
	return &instance{
		c:         c,
		wg:        i.WG,
		env:       i.Env,
		logf:      logf,
		sendEvent: i.SendEvent,

		newProcess:  ffmpeg.NewProcess,
		startReader: startReader,
		sendRequest: sendRequest,
		encoder: png.Encoder{
			CompressionLevel: png.BestSpeed,
		},
		previewCache: previewCache,
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

func generateFFmpegArgs(
	out outputs,
	c config,
	rtspProtocol string,
	rtspAddress string,
) []string {
	// Output minimal
	// ffmpeg -rtsp_transport tcp -i rtsp://x -filter
	//   'fps=fps=3,scale=320:260,pad=320:320:0:0,crop:300:300:10:10'
	//   -f rawvideo -pix_fmt rgb24 -
	//
	// Output maximal
	// ffmpeg -hwaccel x -rtsp_transport tcp -i rtsp://x -filter
	//   'fps=fps=3,scale=320:260,pad=320:320:0:0,crop:300:300:10:10,hue=s=0'
	//   -f rawvideo -pix_fmt rgb24 -
	//
	// Padding is done after scaling for higher efficiency.
	// Cropping must come after padding.

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

	args = append(args,
		"-filter", "fps=fps="+fps+",scale="+scaledWidth+":"+scaledHeight+filter)

	args = append(args,
		"-f", "rawvideo", "-pix_fmt", "rgb24", "-")

	return args
}

func (i *instance) startProcess(parentCtx context.Context) {
	defer i.wg.Done()

	for {
		ctx, cancel := context.WithCancel(parentCtx)
		err := i.runProcess(ctx, cancel)
		if err != nil && !errors.Is(err, context.Canceled) {
			i.logf(log.LevelError, "detector crashed: %v", err)
		} else {
			i.logf(log.LevelInfo, "detector stopped")
		}
		cancel()

		select {
		case <-parentCtx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (i *instance) runProcess(ctx context.Context, cancel context.CancelFunc) error {
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

	i.watchdogTimer = time.AfterFunc(10*time.Second, func() {
		if ctx.Err() != nil {
			return
		}
		i.logf(log.LevelError, "watchdog: process stopped outputting frames, restarting")
		cancel()
	})

	i.wg.Add(1)
	go i.startReader(ctx, cancel, i, stdout)

	i.logf(log.LevelInfo, "starting process: %v", cmd)

	err = process.Start(ctx) // Blocks until process exists.
	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("process crashed: %w", err)
	}
	return nil
}

type startReaderFunc func(
	context.Context,
	context.CancelFunc,
	*instance,
	io.Reader,
)

func startReader(
	ctx context.Context,
	cancel context.CancelFunc,
	i *instance,
	stdout io.Reader,
) {
	defer i.wg.Done()

	err := i.runReader(ctx, stdout)
	if !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
		i.logf(log.LevelError, "instance crashed: %v", err)
	} else {
		i.logf(log.LevelInfo, "instance stopped")
	}
	cancel()
}

func (i *instance) runReader(ctx context.Context, stdout io.Reader) error {
	eventDuration := ffmpeg.FeedRateToDuration(i.c.feedRate)

	img := NewRGB24(image.Rect(0, 0, i.outputs.width, i.outputs.height))
	inputBuffer := make([]byte, i.outputs.frameSize)
	tmpBuffer := []byte{}
	outputBuffer := []byte{}

	for {
		if _, err := io.ReadAtLeast(stdout, inputBuffer, i.outputs.frameSize); err != nil {
			return fmt.Errorf("read stdout: %w", err)
		}
		t := time.Now().Add(-i.c.timestampOffset)
		i.watchdogTimer.Reset(10 * time.Second)

		img.Pix = inputBuffer
		b := bytes.NewBuffer(tmpBuffer)
		if err := i.encoder.Encode(b, img); err != nil {
			return fmt.Errorf("encode frame: %w", err)
		}
		outputBuffer = b.Bytes()
		i.previewCache.Set(i.c.monitorID, outputBuffer)

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
			return fmt.Errorf("send frame: %w", err)
		}

		parsed := parseDetections(i.c.minSize, i.c.maxSize, i.c.mask.Area, i.reverseValues, *detections)
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
			return fmt.Errorf("send event: %w", err)
		}
	}
}

func parseDetections(
	minSize float64,
	maxSize float64,
	mask ffmpeg.Polygon,
	reverse reverseValues,
	detections detections,
) []storage.Detection {
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

		top := convY(detection.Top)
		left := convX(detection.Left)
		bottom := convY(detection.Bottom)
		right := convX(detection.Right)

		height := bottom - top
		width := right - left

		sizePercent := float64(width*height) / 100
		if sizePercent < minSize {
			continue
		}
		if maxSize != 0 && sizePercent > maxSize {
			continue
		}

		centerY := top + (height / 2)
		centerX := left + (width / 2)

		centerInsideMask := ffmpeg.VertexInsidePoly(centerY, centerX, mask)
		if centerInsideMask {
			continue
		}

		d := storage.Detection{
			Label: label,
			Score: score,
			Region: &storage.Region{
				Rect: &ffmpeg.Rect{top, left, bottom, right},
			},
		}
		parsed = append(parsed, d)
	}
	return parsed
}
