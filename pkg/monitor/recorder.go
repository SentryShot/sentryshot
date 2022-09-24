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

package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"nvr/pkg/ffmpeg"
	"nvr/pkg/log"
	"nvr/pkg/storage"
	"nvr/pkg/video/customformat"
	"nvr/pkg/video/hls"
	"nvr/pkg/video/mp4muxer"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Recorder creates and saves new recordings.
type Recorder struct {
	Config      *Config
	MonitorLock *sync.Mutex

	events     *storage.Events
	eventsLock sync.Mutex
	eventChan  chan storage.Event

	logf       logFunc
	runProcess runRecordingProcessFunc
	NewProcess ffmpeg.NewProcessFunc

	input *InputProcess
	Env   storage.ConfigEnv
	Log   *log.Logger
	wg    *sync.WaitGroup
	hooks Hooks

	prevSeg uint64
}

func newRecorder(m *Monitor) *Recorder {
	monitorID := m.Config.ID()
	logf := func(level log.Level, format string, a ...interface{}) {
		m.Log.Level(level).Src("recorder").Monitor(monitorID).Msgf(format, a...)
	}
	return &Recorder{
		Config:      m.Config,
		MonitorLock: &m.Lock,
		events:      &storage.Events{},
		eventsLock:  sync.Mutex{},
		eventChan:   make(chan storage.Event),

		logf:       logf,
		runProcess: runRecordingProcess,
		NewProcess: ffmpeg.NewProcess,

		input: m.mainInput,
		Env:   m.Env,
		Log:   m.Log,
		wg:    m.WG,
		hooks: m.hooks,
	}
}

func (r *Recorder) start(ctx context.Context) { //nolint:funlen
	var recCtx context.Context
	recCancel := func() {}
	isRecording := false
	triggerTimer := &time.Timer{}
	onRecExit := make(chan error)

	startRecording := func() {
		onRecExit <- r.runProcess(recCtx, r)
	}

	recStopped := func() {
		triggerTimer.Stop()
		isRecording = false
		r.logf(log.LevelInfo, "recording stopped")
	}

	var timerEnd time.Time
	for {
		select {
		case <-ctx.Done():
			recCancel()
			if isRecording {
				// Recording was active and is now canceled. Clean up.
				<-onRecExit
				recStopped()
			}
			r.wg.Done()
			return

		case event := <-r.eventChan: // Incomming events.
			r.hooks.Event(r, &event)
			r.eventsLock.Lock()
			*r.events = append(*r.events, event)
			r.eventsLock.Unlock()

			end := event.Time.Add(event.RecDuration)
			if end.After(timerEnd) {
				timerEnd = end
			}

			if isRecording {
				// Update timer.
				triggerTimer.Stop()
				triggerTimer = time.NewTimer(time.Until(timerEnd))
				continue
			}

			// Start new recording.
			triggerTimer = time.NewTimer(time.Until(timerEnd))
			recCtx, recCancel = context.WithCancel(ctx)
			isRecording = true
			go startRecording()

		case <-triggerTimer.C:
			r.logf(log.LevelInfo, "trigger reached end, stopping recording")
			recCancel()

		case err := <-onRecExit:
			if recCtx.Err() != nil {
				// Recording was canceled and stopped.
				recStopped()
				continue
			}

			if err != nil && !errors.Is(err, context.Canceled) {
				// Recording crached. Wait a second and start it again.
				r.logf(log.LevelError, "recording process: %v", err)
				go func() {
					select {
					case <-ctx.Done():
						onRecExit <- nil
					case <-time.After(1 * time.Second):
						go startRecording()
					}
				}()
				continue
			}

			// Recording reached videoLength and stopped normally.
			// The trigger is still active so start it again.
			go startRecording()
		}
	}
}

type runRecordingProcessFunc func(context.Context, *Recorder) error

func runRecordingProcess(ctx context.Context, r *Recorder) error {
	r.MonitorLock.Lock()
	monitorID := r.Config.ID()
	videoLengthStr := r.Config.videoLength()
	timestampOffsetInt, err := strconv.Atoi(r.Config.TimestampOffset())
	r.MonitorLock.Unlock()
	if err != nil {
		return fmt.Errorf("parse timestamp offset %w", err)
	}

	muxer, err := r.input.HLSMuxer()
	if err != nil {
		return fmt.Errorf("get muxer: %w", err)
	}

	firstSegment, err := muxer.NextSegment(r.prevSeg)
	if err != nil {
		return fmt.Errorf("first segment: %w", err)
	}

	offset := 0 + time.Duration(timestampOffsetInt)*time.Millisecond
	startTime := firstSegment.StartTime.Add(-offset)

	fileDir := filepath.Join(
		r.Env.RecordingsDir(),
		startTime.Format("2006/01/02/")+monitorID,
	)
	filePath := filepath.Join(
		fileDir,
		startTime.Format("2006-01-02_15-04-05_")+monitorID,
	)
	basePath := filepath.Base(filePath)

	err = os.MkdirAll(fileDir, 0o755)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("make directory for video: %w", err)
	}

	videoLengthFloat, err := strconv.ParseFloat(videoLengthStr, 64)
	if err != nil {
		return fmt.Errorf("parse video length: %w", err)
	}
	videoLength := time.Duration(videoLengthFloat * float64(time.Minute))

	r.logf(log.LevelInfo, "starting recording: %v", basePath)

	info, err := r.input.StreamInfo(ctx)
	if err != nil {
		return fmt.Errorf("stream info: %w", err)
	}

	go r.generateThumbnail(filePath, firstSegment, *info)

	prevSeg, endTime, err := generateVideo(
		ctx, filePath, muxer.NextSegment, firstSegment, *info, videoLength)
	if err != nil {
		return fmt.Errorf("write video: %w", err)
	}
	r.prevSeg = prevSeg
	r.logf(log.LevelInfo, "video generated: %v", basePath)

	go r.saveRecording(filePath, startTime, *endTime)

	return nil
}

// ErrSkippedSegment skipped segment.
var ErrSkippedSegment = errors.New("skipped segment")

type nextSegmentFunc func(uint64) (*hls.Segment, error)

func generateVideo( //nolint:funlen
	ctx context.Context,
	filePath string,
	nextSegment nextSegmentFunc,
	firstSegment *hls.Segment,
	info hls.StreamInfo,
	maxDuration time.Duration,
) (uint64, *time.Time, error) {
	prevSeg := firstSegment.ID
	startTime := firstSegment.StartTime
	stopTime := firstSegment.StartTime.Add(maxDuration)
	endTime := startTime

	metaPath := filePath + ".meta"
	mdatPath := filePath + ".mdat"

	meta, err := os.OpenFile(metaPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, nil, err
	}
	defer meta.Close()

	mdat, err := os.OpenFile(mdatPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, nil, err
	}
	defer mdat.Close()

	header := customformat.Header{
		VideoSPS:    info.VideoSPS,
		VideoPPS:    info.VideoPPS,
		AudioConfig: info.AudioTrackConfig,
		StartTime:   startTime.UnixNano(),
	}

	w, err := customformat.NewWriter(meta, mdat, header)
	if err != nil {
		return 0, nil, err
	}

	writeSegment := func(seg *hls.Segment) error {
		if err := w.WriteSegment(seg); err != nil {
			return err
		}
		prevSeg = seg.ID
		endTime = seg.StartTime.Add(seg.RenderedDuration)
		return nil
	}

	if err := writeSegment(firstSegment); err != nil {
		return 0, nil, err
	}

	for {
		if ctx.Err() != nil {
			return prevSeg, &endTime, nil
		}

		seg, err := nextSegment(prevSeg)
		if err != nil {
			return prevSeg, &endTime, nil
		}

		if seg.ID != prevSeg+1 {
			return 0, nil, fmt.Errorf("%w: expected: %v got %v",
				ErrSkippedSegment, prevSeg+1, seg.ID)
		}

		if err := writeSegment(seg); err != nil {
			return 0, nil, err
		}

		if seg.StartTime.After(stopTime) {
			return prevSeg, &endTime, nil
		}
	}
}

// The first h264 frame in firstSegment is wrapped in a mp4
// container and piped into FFmpeg and then converted to jpeg.
func (r *Recorder) generateThumbnail(
	filePath string,
	firstSegment *hls.Segment,
	info hls.StreamInfo,
) {
	videoBuffer := &bytes.Buffer{}
	err := mp4muxer.GenerateThumbnailVideo(videoBuffer, firstSegment, info)
	if err != nil {
		r.logf(log.LevelError, "generate thumbnail video: %v", err)
		return
	}

	r.MonitorLock.Lock()
	logLevel := r.Config.LogLevel()
	r.MonitorLock.Unlock()

	thumbPath := filePath + ".jpeg"
	args := "-n -threads 1 -loglevel " + logLevel +
		" -i -" + // Input.
		" -frames:v 1 " + thumbPath // Output.

	r.logf(log.LevelInfo, "generating thumbnail: %v", args)

	r.hooks.RecSave(r, &args)

	cmd := exec.Command(r.Env.FFmpegBin, ffmpeg.ParseArgs(args)...)
	cmd.Stdin = videoBuffer

	ffLogLevel := log.FFmpegLevel(logLevel)
	logFunc := func(msg string) {
		r.logf(ffLogLevel, "thumbnail process: %v", msg)
	}
	process := r.NewProcess(cmd).
		StdoutLogger(logFunc).
		StderrLogger(logFunc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := process.Start(ctx); err != nil {
		r.logf(log.LevelError, "generate thumbnail, args: %v error: %v", args, err)
		return
	}
	r.logf(log.LevelInfo, "thumbnail generated: %v", filepath.Base(thumbPath))
}

func (r *Recorder) saveRecording(
	filePath string,
	startTime time.Time,
	endTime time.Time,
) {
	r.logf(log.LevelInfo, "saving recording: %v", filepath.Base(filePath))

	r.eventsLock.Lock()
	events := r.events.QueryAndPrune(startTime, endTime)
	r.eventsLock.Unlock()

	data := storage.RecordingData{
		Start:  startTime,
		End:    endTime,
		Events: events,
	}
	json, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		r.logf(log.LevelError, "marshal event data: %w", err)
		return
	}

	dataPath := filePath + ".json"
	if err := os.WriteFile(dataPath, json, 0o600); err != nil {
		r.logf(log.LevelError, "write event data: %w", err)
		return
	}

	go r.hooks.RecSaved(r, filePath, data)

	r.logf(log.LevelInfo, "recording saved: %v", filepath.Base(dataPath))
}

func (r *Recorder) sendEvent(event storage.Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}
	r.eventChan <- event
	return nil
}
