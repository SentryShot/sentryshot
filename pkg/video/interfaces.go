package video

import (
	"time"

	"github.com/pion/rtp"
)

type data struct {
	trackID      int
	rtp          *rtp.Packet
	ptsEqualsDTS bool
	h264NALUs    [][]byte
	h264PTS      time.Duration
}

// reader is an entity that can read a stream.
type reader interface {
	close()
	onReaderAccepted()
	onReaderData(*data)
}

type closer interface {
	close()
}
