package headers

import (
	"testing"

	"nvr/pkg/video/gortsplib/pkg/base"

	"github.com/stretchr/testify/require"
)

var casesTransport = []struct {
	name string
	vin  base.HeaderValue
	vout base.HeaderValue
	h    Transport
}{
	{
		"tcp play request / response",
		base.HeaderValue{`RTP/AVP/TCP;interleaved=0-1`},
		base.HeaderValue{`RTP/AVP/TCP;interleaved=0-1`},
		Transport{
			InterleavedIDs: &[2]int{0, 1},
		},
	},
	{
		"dahua rtsp server ssrc with initial spaces",
		base.HeaderValue{`RTP/AVP/TCP;interleaved=0-1;ssrc=     D93FF`},
		base.HeaderValue{`RTP/AVP/TCP;interleaved=0-1;ssrc=000D93FF`},
		Transport{
			InterleavedIDs: &[2]int{0, 1},
			SSRC: func() *uint32 {
				v := uint32(0xD93FF)
				return &v
			}(),
		},
	},
}

func TestTransportUnmarshal(t *testing.T) {
	for _, ca := range casesTransport {
		t.Run(ca.name, func(t *testing.T) {
			var h Transport
			err := h.Unmarshal(ca.vin)
			require.NoError(t, err)
			require.Equal(t, ca.h, h)
		})
	}
}

func TestTransportUnmarshalErrors(t *testing.T) {
	for _, ca := range []struct {
		name string
		hv   base.HeaderValue
		err  string
	}{
		{
			"empty",
			base.HeaderValue{},
			"value not provided",
		},
		{
			"2 values",
			base.HeaderValue{"a", "b"},
			"value provided multiple times ([a b])",
		},
		{
			"invalid keys",
			base.HeaderValue{`key1="k`},
			"apexes not closed (key1=\"k)",
		},
		{
			"invalid interleaved port",
			base.HeaderValue{`RTP/AVP;unicast;interleaved=aa-14187`},
			"invalid ports (aa-14187)",
		},
		{
			"invalid mode",
			base.HeaderValue{`RTP/AVP;unicast;mode=aa`},
			"invalid transport mode: 'aa'",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			var h Transport
			err := h.Unmarshal(ca.hv)
			require.EqualError(t, err, ca.err)
		})
	}
}

func TestTransportMarshal(t *testing.T) {
	for _, ca := range casesTransport {
		t.Run(ca.name, func(t *testing.T) {
			req := ca.h.Marshal()
			require.Equal(t, ca.vout, req)
		})
	}
}
