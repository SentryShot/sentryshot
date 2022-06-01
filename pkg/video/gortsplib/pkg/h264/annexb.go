package h264

// MaxNALUSize is the maximum size of a NALU.
// with a 250 Mbps H264 video, the maximum NALU size is 2.2MB.
const MaxNALUSize = 3 * 1024 * 1024

func annexBEncodeSize(nalus [][]byte) int {
	n := 0
	for _, nalu := range nalus {
		n += 4 + len(nalu)
	}
	return n
}

// AnnexBEncode encodes NALUs into the Annex-B stream format.
func AnnexBEncode(nalus [][]byte) ([]byte, error) {
	buf := make([]byte, annexBEncodeSize(nalus))
	pos := 0

	for _, nalu := range nalus {
		pos += copy(buf[pos:], []byte{0x00, 0x00, 0x00, 0x01})
		pos += copy(buf[pos:], nalu)
	}

	return buf, nil
}
