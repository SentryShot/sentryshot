package hls

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNextSegment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	playlist := newPlaylist(ctx, 3)
	go playlist.start()

	seg5 := &Segment{ID: 5}
	seg6 := &Segment{ID: 6}

	playlist.onSegmentFinalized(seg5)
	playlist.onSegmentFinalized(seg6)

	cases := map[string]struct {
		prevID   uint64
		expected *Segment
	}{
		"before": {3, seg5},
		"ok":     {4, seg5},
		"ok2":    {5, seg6},
		"after":  {7, seg5},
		"after2": {999, seg5},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			seg, err := playlist.nextSegment(tc.prevID)
			require.NoError(t, err)
			require.Equal(t, tc.expected, seg)
		})
	}
	t.Run("blocking", func(t *testing.T) {
		seg7 := &Segment{ID: 7}
		done := make(chan struct{})
		go func() {
			seg, err := playlist.nextSegment(6)
			require.NoError(t, err)
			require.Equal(t, seg7, seg)
			close(done)
		}()

		playlist.onSegmentFinalized(seg7)
		<-done
	})
}
