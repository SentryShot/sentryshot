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

package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/ioutil"
	"nvr/pkg/log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Process interface only used for testing.
type Process interface {
	Start(ctx context.Context) error
	SetTimeout(time.Duration)
	SetPrefix(string)
	SetStdoutLogger(*log.Logger)
	SetStderrLogger(*log.Logger)
}

// process manages subprocesses.
type process struct {
	timeout time.Duration
	cmd     *exec.Cmd

	prefix       string
	stdoutLogger *log.Logger
	stderrLogger *log.Logger

	done chan struct{}
}

// NewProcessFunc is used for mocking.
type NewProcessFunc func(*exec.Cmd) Process

// NewProcess return process.
func NewProcess(cmd *exec.Cmd) Process {
	return &process{
		timeout: 1000 * time.Millisecond,
		cmd:     cmd,
	}
}

func (p *process) attachLogger(l *log.Logger, label string, stdPipe func() (io.ReadCloser, error)) error {
	pipe, err := stdPipe()
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(pipe)
	go func() {
		for scanner.Scan() {
			l.Printf("%v%v: %v\n", p.prefix, label, scanner.Text())
		}
	}()
	return nil
}

// Start starts process with context.
func (p *process) Start(ctx context.Context) error {
	if p.stdoutLogger != nil {
		if err := p.attachLogger(p.stdoutLogger, "stdout", p.cmd.StdoutPipe); err != nil {
			return err
		}
	}
	if p.stderrLogger != nil {
		if err := p.attachLogger(p.stderrLogger, "stderr", p.cmd.StderrPipe); err != nil {
			return err
		}
	}

	if err := p.cmd.Start(); err != nil {
		return err
	}

	p.done = make(chan struct{})

	go func() {
		select {
		case <-p.done:
		case <-ctx.Done():
			p.stop()
		}
	}()

	err := p.cmd.Wait()
	close(p.done)

	// FFmpeg seems to return 255 on normal exit.
	if err != nil && err.Error() == "exit status 255" {
		return nil
	}

	return err
}

// Note, canCommandContext to stop process as it would
// kill the process before it has a chance to exit on its own.
func (p *process) stop() {
	p.cmd.Process.Signal(os.Interrupt) //nolint:errcheck

	select {
	case <-p.done:
	case <-time.After(p.timeout):
		p.cmd.Process.Signal(os.Kill) //nolint:errcheck
		<-p.done
	}
}

func (p *process) SetTimeout(timeout time.Duration) {
	p.timeout = timeout
}

func (p *process) SetPrefix(prefix string) {
	p.prefix = prefix
}

func (p *process) SetStdoutLogger(l *log.Logger) {
	p.stdoutLogger = l
}
func (p *process) SetStderrLogger(l *log.Logger) {
	p.stderrLogger = l
}

// MakePipe creates fifo pipe at specified location.
func MakePipe(path string) error {
	os.Remove(path)
	err := syscall.Mkfifo(path, 0600)
	if err != nil {
		return err
	}
	return nil
}

// FFMPEG stores ffmpeg binary location.
type FFMPEG struct {
	command func(...string) *exec.Cmd
}

// New returns FFMPEG.
func New(bin string) *FFMPEG {
	command := func(args ...string) *exec.Cmd {
		return exec.Command(bin, args...)
	}
	return &FFMPEG{command: command}
}

// SizeFromStreamFunc is used for mocking.
type SizeFromStreamFunc func(string) (string, error)

// SizeFromStream uses ffmpeg to grab stream size.
func (f *FFMPEG) SizeFromStream(url string) (string, error) {
	cmd := f.command("-i", url, "-f", "ffmetadata", "-")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %v", stderr.String(), err)
	}

	re := regexp.MustCompile(`\b\d+x\d+\b`)
	// Input "Stream #0:0: Video: h264 (Main), yuv420p(progressive), 720x1280 fps, 30.00"
	// Output "720x1280"

	output := re.FindString(stderr.String())
	if output != "" {
		return output, nil
	}

	return "", fmt.Errorf("no regex match %s", stderr.String())
}

/*
func HWaccels(bin string) ([]string, error) {
	cmd := exec.Command(bin, "-hwaccels")

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return []string{}, fmt.Errorf("%v", err)
	}

	// Input
	//   accels Hardware acceleration methods:
	//   vdpau
	//   vaapi

	// Output ["vdpau", "vaapi"]
	input := strings.TrimSpace(stdout.String())
	lines := strings.Split(input, "\n")

	return lines[1:], nil
}
*/

// CreateMask creates an image mask from a polygon.
// Pixels outside the polygon are masked.
func CreateMask(w int, h int, poly [][2]int) image.Image {
	img := image.NewAlpha(image.Rect(0, 0, w, h))

	for y := 0; y < w; y++ {
		for x := 0; x < h; x++ {
			if vertexInsidePoly(y, x, poly) {
				img.Set(y, x, color.Alpha{0})
			} else {
				img.Set(y, x, color.Alpha{255})
			}
		}
	}
	return img
}

func vertexInsidePoly(x int, y int, poly [][2]int) bool {
	var inside = false
	var j = len(poly) - 1
	for i := 0; i < len(poly); i++ {
		var xi = poly[i][0]
		var yi = poly[i][1]
		var xj = poly[j][0]
		var yj = poly[j][1]

		if ((yi > y) != (yj > y)) && (x < (xj-xi)*(y-yi)/(yj-yi)+xi) {
			inside = !inside
		}
		j = i
	}
	return inside
}

// SaveImage saves image to specified location.
func SaveImage(path string, img image.Image) error {
	os.Remove(path)

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	err = png.Encode(file, img)
	if err != nil {
		return err
	}

	err = file.Close()
	if err != nil {
		return err
	}
	return nil
}

// ParseArgs slices arguments.
func ParseArgs(args string) []string {
	return strings.Split(strings.TrimSpace(args), " ")
}

// WaitForKeyframeFunc is used for mocking.
type WaitForKeyframeFunc func(context.Context, string) (time.Duration, error)

// WaitForKeyframe waits for ffmpeg to update ".m3u8" segment list
// with a new keyframe, and returns that keyframes duration.
// Used to calculate start time of the recording.
func WaitForKeyframe(ctx context.Context, hlsPath string) (time.Duration, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return 0, err
	}
	defer watcher.Close()

	err = watcher.Add(hlsPath)
	if err != nil {
		return 0, err
	}
	for {
		select {
		case <-watcher.Events:
			return getKeyframeDuration(hlsPath)
		case err := <-watcher.Errors:
			return 0, err
		case <-time.After(30 * time.Second):
			return 0, errors.New("timeout")
		case <-ctx.Done():
			return 0, nil
		}
	}
}

func getKeyframeDuration(hlsPath string) (time.Duration, error) {
	/* INPUT
	   #EXTM3U
	   #EXT-X-VERSION:3
	   #EXT-X-ALLOW-CACHE:NO
	   #EXT-X-TARGETDURATION:2
	   #EXT-X-MEDIA-SEQUENCE:251
	   #EXTINF:4.250000,
	   10.ts
	   #EXTINF:3.500000,
	   11.ts
	*/
	// OUTPUT 3500

	m3u8, err := ioutil.ReadFile(hlsPath)
	if err != nil {
		return 0, err
	}

	// Get second to last line. "#EXTINF:3.500000,"
	lines := strings.Split(strings.TrimSpace(string(m3u8)), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("too few lines: %v", m3u8)
	}
	keyframeLine := lines[len(lines)-2]

	keyframeLine = strings.ReplaceAll(keyframeLine, ".", "")
	if len(keyframeLine) < 12 {
		return 0, fmt.Errorf("invalid line: %v", err)
	}

	keyframeInterval, err := strconv.Atoi(keyframeLine[8:12])
	if err != nil {
		return 0, err
	}

	return time.Duration(keyframeInterval) * time.Millisecond, nil
}
