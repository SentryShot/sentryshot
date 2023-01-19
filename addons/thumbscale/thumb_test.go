// SPDX-License-Identifier: GPL-2.0-or-later

package thumbscale

import (
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
				Config: monitor.NewConfig(monitor.RawConfig{
					"thumbScale": tc.input,
				}),
			}
			args := " -frames"
			onRecSave(r, &args)

			require.Equal(t, tc.output, args)
		})
	}
}
