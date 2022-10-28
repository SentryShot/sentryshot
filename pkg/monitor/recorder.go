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
	runSession runRecordingFunc
	NewProcess ffmpeg.NewProcessFunc

	input *InputProcess
	Env   storage.ConfigEnv
	Log   *log.Logger
	wg    *sync.WaitGroup
	hooks Hooks

	sleep   time.Duration
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
		runSession: runRecording,
		NewProcess: ffmpeg.NewProcess,

		input: m.mainInput,
		Env:   m.Env,
		Log:   m.Log,
		wg:    m.WG,
		hooks: m.hooks,

		sleep: 3 * time.Second,
	}
}

func (r *Recorder) start(ctx context.Context) {
	defer r.wg.Done()

	var sessionCtx context.Context
	var cancelSession context.CancelFunc
	isRecording := false
	triggerTimer := &time.Timer{}
	var onSessionExit chan struct{}

	var timerEnd time.Time
	for {
		select {
		case <-ctx.Done():
			if cancelSession != nil {
				cancelSession()
			}
			if isRecording {
				// Wait for session to exit.
				<-onSessionExit
			}
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
				r.logf(log.LevelDebug, "new event, already recording, updating timer")
				triggerTimer = time.NewTimer(time.Until(timerEnd))
				continue
			}

			r.logf(log.LevelDebug, "starting recording session")
			isRecording = true
			triggerTimer = time.NewTimer(time.Until(timerEnd))
			onSessionExit = make(chan struct{})
			sessionCtx, cancelSession = context.WithCancel(ctx)
			go func() {
				r.runRecordingSession(sessionCtx)
				close(onSessionExit)
			}()

		case <-triggerTimer.C:
			r.logf(log.LevelDebug, "timer reached end, canceling session")
			cancelSession()

		case <-onSessionExit:
			// Recording was canceled and stopped.
			isRecording = false
			continue
		}
	}
}

func (r *Recorder) runRecordingSession(ctx context.Context) {
	defer r.logf(log.LevelDebug, "session stopped")
	for {
		err := r.runSession(ctx, r)
		if err != nil {
			r.logf(log.LevelError, "recording crashed: %v", err)
			select {
			case <-ctx.Done():
				// Session is canceled.
				return
			case <-time.After(r.sleep):
				r.logf(log.LevelDebug, "recovering after crash")
			}
		} else {
			r.logf(log.LevelInfo, "recording finished")
			if ctx.Err() != nil {
				// Session is canceled.
				return
			}
			// Recoding reached videoLength and exited normally. The timer
			// is still active, so continue the loop and start another one.
			continue
		}
	}
}

type runRecordingFunc func(context.Context, *Recorder) error

func runRecording(ctx context.Context, r *Recorder) error {
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

	r.logf(log.LevelInfo, "generating thumbnail: %v", thumbPath)

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
	r.logf(log.LevelDebug, "thumbnail generated: %v", filepath.Base(thumbPath))
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
		r.logf(log.LevelError, "write event data: %v", err)
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
