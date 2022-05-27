package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateEvent(t *testing.T) {
	cases := []struct {
		name     string
		input    Event
		expected error
	}{
		{"working", Event{Time: time.Now(), RecDuration: 1}, nil},
		{"missing Time", Event{RecDuration: 1}, ErrValueMissing},
		{"missing RecDuration", Event{Time: time.Now()}, ErrValueMissing},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.input.Validate()
			require.ErrorIs(t, err, tc.expected)
		})
	}
}
