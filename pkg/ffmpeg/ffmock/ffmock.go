package ffmock

import (
	"context"
	"errors"
	"nvr/pkg/ffmpeg"
	"nvr/pkg/log"
	"os/exec"
	"time"
)

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

func (m mockProcess) Start(ctx context.Context) error {
	if m.c.Sleep != 0 {
		select {
		case <-time.After(m.c.Sleep):
		case <-ctx.Done():
		}
	}
	if m.c.ReturnErr {
		return errors.New("mock")
	}
	return nil
}

func (m mockProcess) Stop() {
	if m.c.OnStop != nil {
		m.c.OnStop()
	}
}
func (m mockProcess) SetTimeout(time.Duration)    {}
func (m mockProcess) SetPrefix(string)            {}
func (m mockProcess) SetStdoutLogger(*log.Logger) {}
func (m mockProcess) SetStderrLogger(*log.Logger) {}

// NewProcess returns Sleeps for 15ms before returning.
var NewProcess = NewProcessMocker(MockProcessConfig{
	ReturnErr: false,
	Sleep:     15 * time.Millisecond,
})

// NewProcessNil returns nil
var NewProcessNil = NewProcessMocker(MockProcessConfig{
	ReturnErr: false,
})

// NewProcessErr returns error.
var NewProcessErr = NewProcessMocker(MockProcessConfig{
	ReturnErr: true,
})
