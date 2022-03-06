package h264

// EncodeAnnexB encodes NALUs into the Annex-B stream format.
func EncodeAnnexB(nalus [][]byte) ([]byte, error) {
	var ret []byte

	for _, nalu := range nalus {
		ret = append(ret, []byte{0x00, 0x00, 0x00, 0x01}...)
		ret = append(ret, nalu...)
	}

	return ret, nil
}
