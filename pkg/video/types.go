package video

import (
	"fmt"
	"nvr/pkg/log"
	"time"

	"github.com/pion/rtp"
)

// data is the data unit routed across the server.
// it must contain one or more of the following:
// - a single RTP packet
// - a group of H264 NALUs (grouped by timestamp)
// - a single AAC AU.
type data struct {
	trackID   int
	rtpPacket *rtp.Packet

	ptsEqualsDTS bool
	pts          time.Duration
	h264NALUs    [][]byte
}

type closer interface {
	close()
}

func sendLogf(
	logger *log.Logger,
	conf PathConf,
	level log.Level,
	prefix string,
	format string,
	a ...interface{},
) {
	processName := func() string {
		if conf.IsSub {
			return "sub"
		}
		return "main"
	}()
	msg := fmt.Sprintf("%v %v: %v", prefix, processName, fmt.Sprintf(format, a...))
	logger.Log(log.Entry{
		Level:     level,
		Src:       "monitor",
		MonitorID: conf.MonitorID,
		Msg:       msg,
	})
}
