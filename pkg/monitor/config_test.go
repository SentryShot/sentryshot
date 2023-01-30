package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCensorLog(t *testing.T) {
	cases := map[string]struct {
		config   RawConfig
		input    string
		expected string
	}{
		"emptyConfig": {
			RawConfig{},
			"a b c",
			"a b c",
		},
		"mainInput": {
			RawConfig{"mainInput": "a"},
			"a b c",
			"$MainInput b c",
		},
		"subInput": {
			RawConfig{"subInput": "b"},
			"a b c",
			"a $SubInput c",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			actual := NewConfig(tc.config).CensorLog(tc.input)
			require.Equal(t, tc.expected, actual)
		})
	}
}
