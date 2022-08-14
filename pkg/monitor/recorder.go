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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"nvr/pkg/ffmpeg"
	"nvr/pkg/log"
	"nvr/pkg/storage"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Recorder creates and saves new recordings.
type Recorder struct {
	Config      Config
	MonitorLock *sync.Mutex

	events     storage.Events
	eventsLock sync.Mutex
	eventChan  chan storage.Event

	logf          logFunc
	runProcess    runRecordingProcessFunc
	NewProcess    ffmpeg.NewProcessFunc
	videoDuration ffmpeg.VideoDurationFunc

	input *InputProcess
	Env   storage.ConfigEnv
	Log   *log.Logger
	wg    *sync.WaitGroup
	hooks Hooks
}

func newRecorder(m *Monitor) *Recorder {
	monitorID := m.Config.ID()
	logf := func(level log.Level, format string, a ...interface{}) {
		m.Log.Level(level).Src("recorder").Monitor(monitorID).Msgf(format, a...)
	}
	return &Recorder{
		Config:      m.Config,
		MonitorLock: &m.Lock,
		eventsLock:  sync.Mutex{},
		eventChan:   make(chan storage.Event),

		logf:          logf,
		runProcess:    runRecordingProcess,
		NewProcess:    ffmpeg.NewProcess,
		videoDuration: ffmpeg.New(m.Env.FFmpegBin).VideoDuration,

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
			r.events = append(r.events, event)
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
				// Recording process was canceled and exited.
				recStopped()
				continue
			}

			if err != nil && !errors.Is(err, context.Canceled) {
				// Recording process crached. Wait a second and start it again.
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

			// Recording process reached videoLength and exited normally.
			// The trigger is still active so start it again.
			go startRecording()
		}
	}
}

type runRecordingProcessFunc func(context.Context, *Recorder) error

func runRecordingProcess(ctx context.Context, r *Recorder) error {
	segmentDuration, err := r.input.WaitForNewHLSsegment(ctx, 2)
	if err != nil {
		return fmt.Errorf("get keyframe duration: %w", err)
	}

	r.MonitorLock.Lock()
	monitorID := r.Config.ID()
	timestampOffsetInt, err := strconv.Atoi(r.Config.TimestampOffset())
	r.MonitorLock.Unlock()
	if err != nil {
		return fmt.Errorf("parse timestamp offset %w", err)
	}

	offset := segmentDuration + time.Duration(timestampOffsetInt)*time.Millisecond
	startTime := time.Now().UTC().Add(-offset)

	fileDir := filepath.Join(
		r.Env.RecordingsDir(),
		startTime.Format("2006/01/02/")+monitorID,
	)
	filePath := filepath.Join(
		fileDir,
		startTime.Format("2006-01-02_15-04-05_")+monitorID,
	)

	err = os.MkdirAll(fileDir, 0o755)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("make directory for video: %w", err)
	}

	r.MonitorLock.Lock()
	logLevel := log.FFmpegLevel(r.Config.LogLevel())
	args, err := r.generateRecorderArgs(filePath)
	r.MonitorLock.Unlock()
	if err != nil {
		return err
	}
	cmd := exec.Command(r.Env.FFmpegBin, ffmpeg.ParseArgs(args)...)

	logFunc := func(msg string) {
		r.logf(logLevel, "recording process:%v", cmd)
	}

	process := r.NewProcess(cmd).
		Timeout(10 * time.Second).
		StdoutLogger(logFunc).
		StderrLogger(logFunc)

	r.logf(log.LevelInfo, "starting recording: %v", cmd)

	err = process.Start(ctx)

	go r.saveRecording(filePath, startTime)

	if err != nil {
		return fmt.Errorf("crashed: %w", err)
	}
	return nil
}

func (r *Recorder) generateRecorderArgs(filePath string) (string, error) {
	videoLength, err := strconv.ParseFloat(r.Config.videoLength(), 64)
	if err != nil {
		return "", fmt.Errorf("parse video length: %w", err)
	}
	videoLengthSec := strconv.Itoa((int(videoLength * 60)))

	args := "-y -threads 1 -loglevel " + r.Config.LogLevel() +
		" -live_start_index -2" + // HLS segment to start from.
		" -i " + r.input.HLSaddress() + // Input.
		" -t " + videoLengthSec + // Max video length.
		" -c:v copy " + filePath + ".mp4" // Output.

	return args, nil
}

func (r *Recorder) saveRecording(filePath string, startTime time.Time) {
	err := r.saveRec(filePath, startTime)
	if err != nil {
		r.logf(log.LevelError, "could not save recording: %v", err)
	} else {
		r.logf(log.LevelInfo, "recording finished")
	}
}

func (r *Recorder) saveRec(filePath string, startTime time.Time) error {
	videoPath := filePath + ".mp4"
	thumbPath := filePath + ".jpeg"
	dataPath := filePath + ".json"

	r.logf(log.LevelInfo, "saving recording: %v", videoPath)

	abort := func() {
		os.Remove(videoPath)
		os.Remove(thumbPath)
	}

	r.MonitorLock.Lock()
	logLevel := log.FFmpegLevel(r.Config.LogLevel())
	args := "-n -threads 1 -loglevel " + r.Config.LogLevel() +
		" -i " + videoPath + // Input.
		" -frames:v 1 " + thumbPath // Output.
	r.MonitorLock.Unlock()

	r.hooks.RecSave(r, &args)

	cmd := exec.Command(r.Env.FFmpegBin, ffmpeg.ParseArgs(args)...)

	logFunc := func(msg string) {
		r.logf(logLevel, "thumbnail process: %v", msg)
	}
	process := r.NewProcess(cmd).
		StdoutLogger(logFunc).
		StderrLogger(logFunc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := process.Start(ctx); err != nil {
		abort()
		return fmt.Errorf("generate thumbnail, args: %v error: %w", args, err)
	}

	duration, err := r.videoDuration(videoPath)
	if err != nil {
		abort()
		return fmt.Errorf("get video duration of: %v: %w", videoPath, err)
	}

	endTime := startTime.Add(duration)

	r.eventsLock.Lock()
	e := queryEvents(r.events, startTime, endTime)
	r.eventsLock.Unlock()

	data := storage.RecordingData{
		Start:  startTime,
		End:    endTime,
		Events: e,
	}
	json, _ := json.MarshalIndent(data, "", "    ")
	if err := os.WriteFile(dataPath, json, 0o600); err != nil {
		return fmt.Errorf("write events file: %w", err)
	}

	go r.hooks.RecSaved(r, filePath, data)
	return nil
}

func (r *Recorder) sendEvent(event storage.Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}
	r.eventChan <- event
	return nil
}

func queryEvents(e storage.Events, start time.Time, end time.Time) storage.Events {
	newEvents := storage.Events{}
	returnEvents := storage.Events{}
	for _, event := range e {
		if event.Time.Before(start) { // Discard events before start time.
			continue
		}
		newEvents = append(newEvents, event) //nolint:staticcheck

		if event.Time.Before(end) {
			returnEvents = append(returnEvents, event)
		}
	}
	e = newEvents //nolint:ineffassign,staticcheck
	return returnEvents
}
