package video

// reader is an entity that can read a stream.
type reader interface {
	close()
	onReaderAccepted()
	onReaderPacketRTP(int, []byte)
}

type closer interface {
	close()
}
