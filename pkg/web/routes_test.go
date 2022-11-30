package web

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCSVParam(t *testing.T) {
	cases := []struct {
		input  string
		output []string
	}{
		{"", nil},
		{"a,b,c", []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			query := url.Values{}
			query.Add("test", tc.input)
			actual := parseCSVParam(query, "test")
			require.Equal(t, tc.output, actual)
		})
	}
}
