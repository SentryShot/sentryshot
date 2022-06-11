package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"nvr/pkg/ffmpeg"
	"nvr/pkg/storage"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

func (m *Monitor) startRecorder(ctx context.Context) {
	var triggerTimeout *time.Timer
	var timeout time.Time

	for {
		select {
		case <-ctx.Done():
			if triggerTimeout != nil {
				triggerTimeout.Stop()
			}
			m.WG.Done()
			return
		case event := <-m.eventChan: // Wait for event.
			m.hooks.Event(m, &event)
			m.eventsMu.Lock()
			m.events = append(m.events, event)
			m.eventsMu.Unlock()

			end := event.Time.Add(event.RecDuration)
			if end.After(timeout) {
				timeout = end
			}

			m.Mu.Lock()
			if m.recording {
				triggerTimeout.Reset(time.Until(timeout))
				m.Mu.Unlock()
				continue
			}

			ctx2, cancel := context.WithCancel(ctx)

			// Stops recording when timeout is reached.
			triggerTimeout = time.AfterFunc(time.Until(timeout), func() {
				m.Log.Info().
					Src("recorder").
					Monitor(m.Config.ID()).
					Msg("trigger reached end, stopping recording")

				cancel()
			})
			m.recording = true
			m.Mu.Unlock()

			m.WG.Add(1)
			go m.startRecording(ctx2, m)
		}
	}
}

type startRecordingFunc func(context.Context, *Monitor)

func startRecording(ctx context.Context, m *Monitor) {
	for {
		if ctx.Err() != nil {
			m.Mu.Lock()

			m.recording = false
			m.Log.Info().
				Src("recorder").
				Monitor(m.Config.ID()).
				Msg("recording stopped")

			m.WG.Done()
			m.Mu.Unlock()
			return
		}
		err := m.runRecordingProcess(ctx, m)
		if err != nil && !errors.Is(err, context.Canceled) {
			m.Log.Error().
				Src("recorder").
				Monitor(m.Config.ID()).
				Msgf("recording process: %v", err)

			time.Sleep(1 * time.Second)
			continue
		}
	}
}

type runRecordingProcessFunc func(context.Context, *Monitor) error

func runRecordingProcess(ctx context.Context, m *Monitor) error {
	segmentDuration, err := m.mainInput.waitForNewHLSsegment(ctx, 2)
	if err != nil {
		return fmt.Errorf("get keyframe duration: %w", err)
	}

	timestampOffsetInt, err := strconv.Atoi(m.Config["timestampOffset"])
	if err != nil {
		return fmt.Errorf("parse timestamp offset %w", err)
	}

	offset := segmentDuration + time.Duration(timestampOffsetInt)*time.Millisecond
	startTime := time.Now().UTC().Add(-offset)

	id := m.Config.ID()

	fileDir := filepath.Join(m.Env.RecordingsDir(), startTime.Format("2006/01/02/")+id)
	filePath := filepath.Join(fileDir, startTime.Format("2006-01-02_15-04-05_")+id)

	if err := os.MkdirAll(fileDir, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("make directory for video: %w", err)
	}

	args, err := m.generateRecorderArgs(filePath)
	if err != nil {
		return err
	}
	cmd := exec.Command(m.Env.FFmpegBin, ffmpeg.ParseArgs(args)...)

	m.Mu.Lock()
	logFunc := func(msg string) {
		m.Log.FFmpegLevel(m.Config.LogLevel()).
			Src("recorder").
			Monitor(id).
			Msgf("recording process:%v", cmd)
	}
	m.Mu.Unlock()

	process := m.NewProcess(cmd).
		Timeout(10 * time.Second).
		StdoutLogger(logFunc).
		StderrLogger(logFunc)

	m.Log.Info().Src("recorder").Monitor(id).Msgf("starting recording: %v", cmd)

	err = process.Start(ctx)

	go m.saveRecording(filePath, startTime)

	if err != nil {
		return fmt.Errorf("crashed: %w", err)
	}
	return nil
}

func (m *Monitor) generateRecorderArgs(filePath string) (string, error) {
	videoLength, err := strconv.ParseFloat(m.Config.videoLength(), 64)
	if err != nil {
		return "", fmt.Errorf("parse video length: %w", err)
	}
	videoLengthSec := strconv.Itoa((int(videoLength * 60)))

	args := "-y -threads 1 -loglevel " + m.Config.LogLevel() +
		" -live_start_index -2" + // HLS segment to start from.
		" -i " + m.mainInput.HLSaddress() + // Input.
		" -t " + videoLengthSec + // Max video length.
		" -c:v copy " + filePath + ".mp4" // Output.

	return args, nil
}

func (m *Monitor) saveRecording(filePath string, startTime time.Time) {
	id := m.Config.ID()

	err := m.saveRec(filePath, startTime)
	if err != nil {
		m.Log.Error().Src("recorder").Monitor(id).Msgf("could not save recording: %v", err)
	} else {
		m.Log.Info().Src("recorder").Monitor(id).Msg("recording finished")
	}
}

func (m *Monitor) saveRec(filePath string, startTime time.Time) error {
	videoPath := filePath + ".mp4"
	thumbPath := filePath + ".jpeg"
	dataPath := filePath + ".json"

	abort := func() {
		os.Remove(videoPath)
		os.Remove(thumbPath)
	}

	m.Log.Info().Src("recorder").Monitor(m.Config.ID()).Msgf("saving recording: %v", videoPath)

	args := "-n -threads 1 -loglevel " + m.Config.LogLevel() +
		" -i " + videoPath + // Input.
		" -frames:v 1 " + thumbPath // Output.

	m.hooks.RecSave(m, &args)

	cmd := exec.Command(m.Env.FFmpegBin, ffmpeg.ParseArgs(args)...)

	logFunc := func(msg string) {
		m.Log.FFmpegLevel(m.Config.LogLevel()).
			Src("recorder").
			Monitor(m.Config.ID()).
			Msgf("thumbnail process: %v", msg)
	}
	process := m.NewProcess(cmd).
		StdoutLogger(logFunc).
		StderrLogger(logFunc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := process.Start(ctx); err != nil {
		abort()
		return fmt.Errorf("generate thumbnail, args: %v error: %w", args, err)
	}

	duration, err := m.videoDuration(videoPath)
	if err != nil {
		abort()
		return fmt.Errorf("get video duration of: %v: %w", videoPath, err)
	}

	endTime := startTime.Add(duration)

	m.eventsMu.Lock()
	e := queryEvents(m.events, startTime, endTime)
	m.eventsMu.Unlock()

	data := storage.RecordingData{
		Start:  startTime,
		End:    endTime,
		Events: e,
	}
	json, _ := json.MarshalIndent(data, "", "    ")
	if err := os.WriteFile(dataPath, json, 0o600); err != nil {
		return fmt.Errorf("write events file: %w", err)
	}

	go m.hooks.RecSaved(m, filePath, data)
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
