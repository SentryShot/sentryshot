package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateEvent(t *testing.T) {
	cases := map[string]struct {
		input    Event
		expected error
	}{
		"working":            {Event{Time: time.Now(), RecDuration: 1}, nil},
		"missingTime":        {Event{RecDuration: 1}, ErrValueMissing},
		"missingRecDuration": {Event{Time: time.Now()}, ErrValueMissing},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := tc.input.Validate()
			require.ErrorIs(t, err, tc.expected)
		})
	}
}
