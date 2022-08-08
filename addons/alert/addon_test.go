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

package alert

import (
	"encoding/json"
	"testing"
	"time"

	"nvr/pkg/monitor"
	"nvr/pkg/storage"

	"github.com/stretchr/testify/require"
)

func rawConf(t *testing.T, config Config) string {
	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	return string(rawConfig)
}

func TestProcessEvent(t *testing.T) {
	cases := map[string]struct {
		config    string
		event     *storage.Event
		passEvent bool
		err       bool
	}{
		"ok": {
			rawConf(t, Config{
				Enable:    "true",
				Threshold: "50",
				Cooldown:  "0",
			}),
			&storage.Event{
				Detections: []storage.Detection{
					{Score: 49},
					{Score: 51},
				},
			},
			true,
			false,
		},
		"nilConfig": {
			"",
			&storage.Event{},
			false,
			false,
		},
		"emptyConfig": {
			"{}",
			&storage.Event{},
			false,
			false,
		},
		"unmarshalErr": {
			"{",
			&storage.Event{},
			false,
			true,
		},
		"disable": {
			rawConf(t, Config{
				Enable:    "false",
				Threshold: "0",
				Cooldown:  "0",
			}),
			&storage.Event{},
			false,
			false,
		},
		"parseCooldownErr": {
			rawConf(t, Config{
				Enable:    "true",
				Threshold: "0",
				Cooldown:  "x",
			}),
			&storage.Event{},
			false,
			true,
		},
		"parseThresholdErr": {
			rawConf(t, Config{
				Enable:    "true",
				Threshold: "x",
				Cooldown:  "0",
			}),
			&storage.Event{},
			false,
			true,
		},
		"threshold": {
			rawConf(t, Config{
				Enable:    "true",
				Threshold: "100",
				Cooldown:  "0",
			}),
			&storage.Event{
				Detections: []storage.Detection{
					{Score: 99},
				},
			},
			false,
			false,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var outEvent *storage.Event
			onEvent := func(_ *monitor.Recorder, event *storage.Event, _ []byte) {
				outEvent = event
			}

			a := newAlerter([]Hook{onEvent})

			err := a.processEvent(nil, tc.event, "", tc.config)
			require.Equal(t, err != nil, tc.err)

			if tc.passEvent {
				require.Equal(t, tc.event, outEvent)
			}
		})
	}

	t.Run("cooldown", func(t *testing.T) {
		var outEvent *storage.Event
		onEvent := func(_ *monitor.Recorder, event *storage.Event, _ []byte) {
			outEvent = event
		}

		a := newAlerter([]Hook{onEvent})

		event1 := &storage.Event{
			Detections: []storage.Detection{
				{Score: 50},
			},
		}
		event2 := &storage.Event{
			Detections: []storage.Detection{
				{Score: 51},
			},
		}

		config := rawConf(t, Config{
			Enable:    "true",
			Threshold: "0",
			Cooldown:  "1",
		})

		err := a.processEvent(nil, event1, "", config)
		require.NoError(t, err)
		require.Equal(t, outEvent, event1)

		err = a.processEvent(nil, event2, "", config)
		require.NoError(t, err)
		require.Equal(t, outEvent, event1)

		a.prevAlerts = map[string]time.Time{}
		err = a.processEvent(nil, event2, "", config)
		require.NoError(t, err)
		require.Equal(t, outEvent, event2)
	})
}
