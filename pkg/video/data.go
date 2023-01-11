package video

import (
	"time"

	"github.com/pion/rtp"
)

// data is the data unit routed across the server.
// it must contain one or more of the following:
// - a single RTP packet
// - a group of H264 NALUs (grouped by timestamp)
// - a single AAC AU.
type data interface {
	getTrackID() int
	getRTPPackets() []*rtp.Packet
	getNTP() time.Time
}

type dataH264 struct {
	trackID    int
	rtpPackets []*rtp.Packet
	ntp        time.Time
	pts        time.Duration
	nalus      [][]byte
}

func (d *dataH264) getTrackID() int {
	return d.trackID
}

func (d *dataH264) getRTPPackets() []*rtp.Packet {
	return d.rtpPackets
}

func (d *dataH264) getNTP() time.Time {
	return d.ntp
}

type dataMPEG4Audio struct {
	trackID    int
	rtpPackets []*rtp.Packet
	ntp        time.Time
	pts        time.Duration
	aus        [][]byte
}

func (d *dataMPEG4Audio) getTrackID() int {
	return d.trackID
}

func (d *dataMPEG4Audio) getRTPPackets() []*rtp.Packet {
	return d.rtpPackets
}

func (d *dataMPEG4Audio) getNTP() time.Time {
	return d.ntp
}
