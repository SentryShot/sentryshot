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

package thumbscale

import (
	"sync"
	"testing"

	"nvr/pkg/monitor"

	"github.com/stretchr/testify/require"
)

func TestOnRecSave(t *testing.T) {
	cases := map[string]struct{ input, output string }{
		"empty": {"", " -frames"},
		"full":  {"full", " -frames"},
		"half":  {"half", " -vf scale='iw/2:ih/2' -frames"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := &monitor.Recorder{
				MonitorLock: &sync.Mutex{},
				Config: map[string]string{
					"thumbScale": tc.input,
				},
			}
			args := " -frames"
			onRecSave(r, &args)

			require.Equal(t, tc.output, args)
		})
	}
}
