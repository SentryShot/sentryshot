package rtpcleaner

import (
	"bytes"
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestRemovePadding(t *testing.T) {
	cleaner := New(false)

	out, err := cleaner.Process(&rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			Marker:         true,
			SequenceNumber: 34572,
			Padding:        true,
		},
		Payload:     bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 64/4),
		PaddingSize: 64,
	})
	require.NoError(t, err)
	require.Equal(t, []*Output{{
		Packet: &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				Marker:         true,
				SequenceNumber: 34572,
			},
			Payload: bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 64/4),
		},
		PTSEqualsDTS: true,
	}}, out)
}

func TestGenericOversized(t *testing.T) {
	cleaner := New(false)

	_, err := cleaner.Process(&rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			Marker:         false,
			SequenceNumber: 34572,
		},
		Payload: bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04, 0x05}, 2050/5),
	})
	require.EqualError(t, err, "payload size (2062) greater than maximum allowed (1472)")
}
