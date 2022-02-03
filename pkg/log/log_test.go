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

package log

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestLogger(t *testing.T) (context.Context, func(), *Logger) {
	logger := NewMockLogger()

	ctx, cancel := context.WithCancel(context.Background())
	logger.Start(ctx)

	return ctx, cancel, logger
}

func TestLogger(t *testing.T) {
	t.Run("msg", func(t *testing.T) {
		_, cancel, logger := newTestLogger(t)
		defer cancel()

		go func() {
			time.Sleep(10 * time.Millisecond)
			logger.Info().Src("s1").Monitor("m1").Time(time.Unix(0, 1000)).Msg("1")
			logger.Warn().Src("s2").Monitor("m2").Time(time.Unix(0, 2000)).Msg("2")
			logger.Error().Src("s3").Monitor("m3").Time(time.Unix(0, 3000)).Msg("3")
			logger.Debug().Src("s4").Monitor("m4").Time(time.Unix(0, 4000)).Msg("4")
		}()

		feed, cancel2 := logger.Subscribe()
		defer cancel2()

		actual := []Log{<-feed, <-feed, <-feed, <-feed}

		expected := []Log{
			{Level: LevelInfo, Src: "s1", Monitor: "m1", Msg: "1", Time: 1},
			{Level: LevelWarning, Src: "s2", Monitor: "m2", Msg: "2", Time: 2},
			{Level: LevelError, Src: "s3", Monitor: "m3", Msg: "3", Time: 3},
			{Level: LevelDebug, Src: "s4", Monitor: "m4", Msg: "4", Time: 4},
		}

		require.Equal(t, actual, expected)
	})
	t.Run("ffmpegLevel", func(t *testing.T) {
		_, cancel, logger := newTestLogger(t)
		defer cancel()

		go func() {
			time.Sleep(10 * time.Millisecond)
			logger.FFmpegLevel("fatal").Time(time.Unix(0, 1000)).Msg("")
			logger.FFmpegLevel("error").Time(time.Unix(0, 2000)).Msg("")
			logger.FFmpegLevel("warning").Time(time.Unix(0, 3000)).Msg("")
			logger.FFmpegLevel("info").Time(time.Unix(0, 4000)).Msg("")
			logger.FFmpegLevel("debug").Time(time.Unix(0, 5000)).Msg("")
		}()

		feed, cancel2 := logger.Subscribe()
		defer cancel2()

		actual := []Log{<-feed, <-feed, <-feed, <-feed, <-feed}

		expected := []Log{
			{Level: LevelError, Time: 1},
			{Level: LevelError, Time: 2},
			{Level: LevelWarning, Time: 3},
			{Level: LevelInfo, Time: 4},
			{Level: LevelDebug, Time: 5},
		}

		require.Equal(t, actual, expected)
	})

	t.Run("unsubBeforePrint", func(t *testing.T) {
		_, cancel, logger := newTestLogger(t)
		defer cancel()

		feed1, cancel1 := logger.Subscribe()
		feed2, cancel2 := logger.Subscribe()
		cancel2()

		msg := "test"
		logger.Info().Msg(msg)
		actual1 := <-feed1
		actual2 := <-feed2
		cancel1()

		require.Equal(t, actual1.Msg, msg)
		require.Equal(t, actual2.Msg, "")
	})
	t.Run("unsubAfterPrint", func(t *testing.T) {
		_, cancel, logger := newTestLogger(t)
		defer cancel()

		feed, cancel2 := logger.Subscribe()

		go func() { logger.Info().Msg("test") }()
		go func() { logger.Info().Msg("test") }()
		go func() { logger.Info().Msg("test") }()
		time.Sleep(10 * time.Microsecond)
		cancel2()

		actual := <-feed
		require.Equal(t, actual.Msg, "")
	})
	t.Run("logToStdout", func(t *testing.T) {
		cs := []string{"-test.run=TestLogToStdout"}
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{"GO_TEST_PROCESS=1"}

		output, err := cmd.CombinedOutput()
		require.NoError(t, err)

		actual := string(output)
		expected := `[ERROR] test
[WARNING] test
[INFO] test
[DEBUG] test
`
		require.Equal(t, actual, expected)
	})
}

func TestLogToStdout(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	ctx, cancel, logger := newTestLogger(t)
	defer cancel()

	go logger.LogToStdout(ctx)
	time.Sleep(1 * time.Millisecond)
	logger.Error().Msg("test")
	logger.Warn().Msg("test")
	logger.Info().Msg("test")
	logger.Debug().Msg("test")
	time.Sleep(5 * time.Millisecond)

	os.Exit(0)
}
