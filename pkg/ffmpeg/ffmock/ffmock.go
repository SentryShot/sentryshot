package ffmock

import (
	"context"
	"errors"
	"nvr/pkg/ffmpeg"
	"os/exec"
	"time"
)

// ErrMock mocking error.
var ErrMock = errors.New("mock")

// MockProcessConfig ProcessMocker config.
type MockProcessConfig struct {
	ReturnErr bool
	Sleep     time.Duration
	OnStop    func()
}

// NewProcessMocker creates process mocker from config.
func NewProcessMocker(c MockProcessConfig) func(*exec.Cmd) ffmpeg.Process {
	return func(*exec.Cmd) ffmpeg.Process {
		return mockProcess{
			c: c,
		}
	}
}

type mockProcess struct {
	c MockProcessConfig
}

func (m mockProcess) Timeout(time.Duration) ffmpeg.Process       { return m }
func (m mockProcess) StdoutLogger(ffmpeg.LogFunc) ffmpeg.Process { return m }
func (m mockProcess) StderrLogger(ffmpeg.LogFunc) ffmpeg.Process { return m }

func (m mockProcess) Start(ctx context.Context) error {
	if m.c.Sleep != 0 {
		select {
		case <-time.After(m.c.Sleep):
		case <-ctx.Done():
		}
	}
	if m.c.ReturnErr {
		return ErrMock
	}
	return nil
}

func (m mockProcess) Stop() {
	if m.c.OnStop != nil {
		m.c.OnStop()
	}
}

// NewProcess returns Sleeps for 15ms before returning.
var NewProcess = NewProcessMocker(MockProcessConfig{
	ReturnErr: false,
	Sleep:     15 * time.Millisecond,
})

// NewProcessNil returns nil.
var NewProcessNil = NewProcessMocker(MockProcessConfig{
	ReturnErr: false,
})

// NewProcessErr returns error.
var NewProcessErr = NewProcessMocker(MockProcessConfig{
	ReturnErr: true,
})
