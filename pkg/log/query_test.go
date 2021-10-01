package log

import (
	"fmt"
	"testing"
	"time"
)

func TestQuery(t *testing.T) {
	msg1 := Log{
		Level:   LevelError,
		Time:    4000,
		Src:     "s1",
		Monitor: "m1",
		Msg:     "msg1",
	}
	msg2 := Log{
		Level: LevelWarning,
		Time:  3000,
		Src:   "s1",
		Msg:   "msg2",
	}
	msg3 := Log{
		Level:   LevelInfo,
		Time:    2000,
		Src:     "s2",
		Monitor: "m2",
		Msg:     "msg3",
	}
	/*msg4 := Log{
		Level: LevelDebug,
		Time:  1000,
		Src:   "s2",
		Msg:   "msg4",
	}*/

	ctx, cancel, logger := newTestLogger(t)
	defer cancel()

	go logger.LogToDB(ctx)

	// Populate database.
	time.Sleep(1 * time.Millisecond)
	logger.Error().Src("s1").Monitor("m1").Time(time.Unix(0, 4000000)).Msg("msg1")
	logger.Warn().Src("s1").Time(time.Unix(0, 3000000)).Msg("msg2")
	logger.Info().Src("s2").Monitor("m2").Time(time.Unix(0, 2000000)).Msg("msg3")
	//logger.Debug().Src("s2").Time(time.Unix(0, 1000000)).Msg("msg4")
	time.Sleep(1 * time.Millisecond)

	cases := []struct {
		name     string
		input    Query
		expected *[]Log
	}{
		{
			name: "singleLevel",
			input: Query{
				Levels:  []Level{LevelWarning},
				Sources: []string{"s1"},
			},
			expected: &[]Log{msg2},
		},
		{
			name: "multipleLevels",
			input: Query{
				Levels:  []Level{LevelError, LevelWarning},
				Sources: []string{"s1"},
			},
			expected: &[]Log{msg1, msg2},
		},
		{
			name: "multipleSources",
			input: Query{
				Levels:  []Level{LevelError, LevelInfo},
				Sources: []string{"s1", "s2"},
			},
			expected: &[]Log{msg1, msg3},
		},
		{
			name: "singleMonitor",
			input: Query{
				Levels:   []Level{LevelError, LevelInfo},
				Sources:  []string{"s1", "s2"},
				Monitors: []string{"m1"},
			},
			expected: &[]Log{msg1},
		},
		{
			name: "multipleMonitors",
			input: Query{
				Levels:   []Level{LevelError, LevelInfo},
				Sources:  []string{"s1", "s2"},
				Monitors: []string{"m1", "m2"},
			},
			expected: &[]Log{msg1, msg3},
		},
		{
			name: "all",
			input: Query{
				Levels:  []Level{LevelError, LevelWarning, LevelInfo, LevelDebug},
				Sources: []string{"s1", "s2"},
			},
			expected: &[]Log{msg1, msg2, msg3},
		},
		{
			name: "limit",
			input: Query{
				Levels:  []Level{LevelError, LevelWarning, LevelInfo, LevelDebug},
				Sources: []string{"s1", "s2"},
				Limit:   2,
			},
			expected: &[]Log{msg1, msg2},
		},
		{
			name: "time",
			input: Query{
				Levels:  []Level{LevelError, LevelWarning, LevelInfo, LevelDebug},
				Sources: []string{"s1", "s2"},
				Time:    4000,
			},
			expected: &[]Log{msg2, msg3},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs, err := logger.Query(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			actual := fmt.Sprintf("%v", logs)
			expected := fmt.Sprintf("%v", tc.expected)

			if actual != expected {
				t.Fatalf("\nexpected:\n%v.\ngot:\n%v", expected, actual)
			}
		})
	}
}
