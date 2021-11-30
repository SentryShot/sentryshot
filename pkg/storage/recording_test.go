package storage

import (
	"errors"
	"testing"
	"time"
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
			if err := tc.input.Validate(); !errors.Is(err, tc.expected) {
				t.Fatalf("expected: %v, got: %v", tc.expected, err)
			}
		})
	}
}
