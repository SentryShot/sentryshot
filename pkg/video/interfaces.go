package video

import "github.com/pion/rtp/v2"

// reader is an entity that can read a stream.
type reader interface {
	close()
	onReaderAccepted()
	onReaderPacketRTP(int, *rtp.Packet)
}

type closer interface {
	close()
}
