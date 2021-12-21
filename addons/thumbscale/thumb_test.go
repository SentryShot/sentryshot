package thumbscale

import (
	"nvr/pkg/monitor"
	"sync"
	"testing"
)

func TestOnRecSave(t *testing.T) {
	cases := []struct{ input, output string }{
		{"", " -frames"},
		{"full", " -frames"},
		{"half", " -vf scale='iw/2:ih/2' -frames"},
	}
	for _, tc := range cases {
		m := &monitor.Monitor{
			Mu: sync.Mutex{},
			Config: map[string]string{
				"thumbScale": tc.input,
			},
		}
		args := " -frames"
		onRecSave(m, &args)

		if args != tc.output {
			t.Fatalf("%v\nexpected:\n%v.\ngot:\n%v", tc.input, tc.output, args)
		}
	}
}
