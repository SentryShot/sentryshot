package gortsplib

import (
	"bytes"
	"net"
	"testing"
	"time"

	"nvr/pkg/video/gortsplib/pkg/base"
	"nvr/pkg/video/gortsplib/pkg/conn"
	"nvr/pkg/video/gortsplib/pkg/headers"
	"nvr/pkg/video/gortsplib/pkg/url"

	"github.com/pion/rtp"
	psdp "github.com/pion/sdp/v3"
	"github.com/stretchr/testify/require"
)

var testRTPPacket = rtp.Packet{
	Header: rtp.Header{
		Version:     2,
		PayloadType: 97,
		CSRC:        []uint32{},
	},
	Payload: []byte{0x01, 0x02, 0x03, 0x04},
}

var testRTPPacketMarshaled = func() []byte {
	byts, _ := testRTPPacket.Marshal()
	return byts
}()

func mustParseURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func invalidURLAnnounceReq(t *testing.T, control string) base.Request {
	return base.Request{
		Method: base.Announce,
		URL:    mustParseURL("rtsp://localhost:8554/teststream"),
		Header: base.Header{
			"CSeq":         base.HeaderValue{"1"},
			"Content-Type": base.HeaderValue{"application/sdp"},
		},
		Body: func() []byte {
			track := &TrackH264{
				PayloadType: 96,
				SPS:         []byte{0x01, 0x02, 0x03, 0x04},
				PPS:         []byte{0x01, 0x02, 0x03, 0x04},
			}
			track.SetControl(control)

			sout := &psdp.SessionDescription{
				SessionName: psdp.SessionName("Stream"),
				Origin: psdp.Origin{
					Username:       "-",
					NetworkType:    "IN",
					AddressType:    "IP4",
					UnicastAddress: "127.0.0.1",
				},
				TimeDescriptions: []psdp.TimeDescription{
					{Timing: psdp.Timing{}}, //nolint:govet
				},
				MediaDescriptions: []*psdp.MediaDescription{
					track.MediaDescription(),
				},
			}

			byts, _ := sout.Marshal()
			return byts
		}(),
	}
}

func TestServerPublishErrorAnnounce(t *testing.T) {
	for _, ca := range []struct {
		name string
		req  base.Request
		err  string
	}{
		{
			"missing content-type",
			base.Request{
				Method: base.Announce,
				URL:    mustParseURL("rtsp://localhost:8554/teststream"),
				Header: base.Header{
					"CSeq": base.HeaderValue{"1"},
				},
			},
			"Content-Type header is missing",
		},
		{
			"invalid content-type",
			base.Request{
				Method: base.Announce,
				URL:    mustParseURL("rtsp://localhost:8554/teststream"),
				Header: base.Header{
					"CSeq":         base.HeaderValue{"1"},
					"Content-Type": base.HeaderValue{"aa"},
				},
			},
			"unsupported Content-Type header '[aa]'",
		},
		{
			"invalid tracks",
			base.Request{
				Method: base.Announce,
				URL:    mustParseURL("rtsp://localhost:8554/teststream"),
				Header: base.Header{
					"CSeq":         base.HeaderValue{"1"},
					"Content-Type": base.HeaderValue{"application/sdp"},
				},
				Body: []byte{0x01, 0x02, 0x03, 0x04},
			},
			"invalid SDP: invalid line: (\x01\x02\x03\x04)",
		},
		{
			"invalid URL 1",
			invalidURLAnnounceReq(t, "rtsp://  aaaaa"),
			"unable to generate track URL",
		},
		{
			"invalid URL 2",
			invalidURLAnnounceReq(t, "rtsp://host"),
			"invalid track URL (rtsp://localhost:8554)",
		},
		{
			"invalid URL 3",
			invalidURLAnnounceReq(t, "rtsp://host/otherpath"),
			"invalid track path: must begin with 'teststream', but is 'otherpath'",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			connClosed := make(chan struct{})

			s := &Server{
				handler: &testServerHandler{
					onConnClose: func(_ *ServerConn, err error) {
						require.EqualError(t, err, ca.err)
						close(connClosed)
					},
					onAnnounce: func(*ServerSession, string, Tracks) (*base.Response, error) {
						return &base.Response{
							StatusCode: base.StatusOK,
						}, nil
					},
				},
				rtspAddress: "localhost:8554",
			}

			err := s.Start()
			require.NoError(t, err)
			defer s.Close()

			nconn, err := net.Dial("tcp", "localhost:8554")
			require.NoError(t, err)
			defer nconn.Close()
			conn := conn.NewConn(nconn)

			_, err = writeReqReadRes(conn, ca.req)
			require.NoError(t, err)

			<-connClosed
		})
	}
}

func TestServerPublishSetupPath(t *testing.T) {
	for _, ca := range []struct {
		name    string
		control string
		url     string
		path    string
		trackID int
	}{
		{
			"normal",
			"trackID=0",
			"rtsp://localhost:8554/teststream/trackID=0",
			"teststream",
			0,
		},
		{
			"unordered id",
			"trackID=2",
			"rtsp://localhost:8554/teststream/trackID=2",
			"teststream",
			0,
		},
		{
			"custom param name",
			"testing=0",
			"rtsp://localhost:8554/teststream/testing=0",
			"teststream",
			0,
		},
		{
			"query",
			"?testing=0",
			"rtsp://localhost:8554/teststream?testing=0",
			"teststream",
			0,
		},
		{
			"subpath",
			"trackID=0",
			"rtsp://localhost:8554/test/stream/trackID=0",
			"test/stream",
			0,
		},
		{
			"subpath and query",
			"?testing=0",
			"rtsp://localhost:8554/test/stream?testing=0",
			"test/stream",
			0,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			s := &Server{
				handler: &testServerHandler{
					onAnnounce: func(_ *ServerSession, _ string, tracks Tracks) (*base.Response, error) {
						// make sure that track URLs are not overridden by NewServerStream()
						stream := NewServerStream(tracks)
						defer stream.Close()

						return &base.Response{
							StatusCode: base.StatusOK,
						}, nil
					},
					onSetup: func(
						_ *ServerSession,
						path string,
						trackID int,
					) (*base.Response, *ServerStream, error) {
						require.Equal(t, ca.path, path)
						require.Equal(t, ca.trackID, trackID)
						return &base.Response{
							StatusCode: base.StatusOK,
						}, nil, nil
					},
				},
				rtspAddress: "localhost:8554",
			}

			err := s.Start()
			require.NoError(t, err)
			defer s.Close()

			nconn, err := net.Dial("tcp", "localhost:8554")
			require.NoError(t, err)
			defer nconn.Close()
			conn := conn.NewConn(nconn)

			track := &TrackH264{
				PayloadType: 96,
				SPS:         []byte{0x01, 0x02, 0x03, 0x04},
				PPS:         []byte{0x01, 0x02, 0x03, 0x04},
			}
			track.SetControl(ca.control)

			sout := &psdp.SessionDescription{
				SessionName: psdp.SessionName("Stream"),
				Origin: psdp.Origin{
					Username:       "-",
					NetworkType:    "IN",
					AddressType:    "IP4",
					UnicastAddress: "127.0.0.1",
				},
				TimeDescriptions: []psdp.TimeDescription{
					{Timing: psdp.Timing{}}, //nolint:govet
				},
				MediaDescriptions: []*psdp.MediaDescription{
					track.MediaDescription(),
				},
			}

			byts, _ := sout.Marshal()

			res, err := writeReqReadRes(conn, base.Request{
				Method: base.Announce,
				URL:    mustParseURL("rtsp://localhost:8554/" + ca.path),
				Header: base.Header{
					"CSeq":         base.HeaderValue{"1"},
					"Content-Type": base.HeaderValue{"application/sdp"},
				},
				Body: byts,
			})
			require.NoError(t, err)
			require.Equal(t, base.StatusOK, res.StatusCode)

			th := &headers.Transport{
				Mode: func() *headers.TransportMode {
					v := headers.TransportModeRecord
					return &v
				}(),
				InterleavedIDs: &[2]int{0, 1},
			}

			res, err = writeReqReadRes(conn, base.Request{
				Method: base.Setup,
				URL:    mustParseURL(ca.url),
				Header: base.Header{
					"CSeq":      base.HeaderValue{"2"},
					"Transport": th.Marshal(),
				},
			})
			require.NoError(t, err)
			require.Equal(t, base.StatusOK, res.StatusCode)
		})
	}
}

func TestServerPublishErrorSetupDifferentPaths(t *testing.T) {
	serverErr := make(chan error)

	s := &Server{
		handler: &testServerHandler{
			onConnClose: func(_ *ServerConn, err error) {
				serverErr <- err
			},
			onAnnounce: func(*ServerSession, string, Tracks) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
			onSetup: func(*ServerSession, string, int) (*base.Response, *ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil, nil
			},
		},
		rtspAddress: "localhost:8554",
	}

	err := s.Start()
	require.NoError(t, err)
	defer s.Close()

	nconn, err := net.Dial("tcp", "localhost:8554")
	require.NoError(t, err)
	defer nconn.Close()
	conn := conn.NewConn(nconn)

	track := &TrackH264{
		PayloadType: 96,
		SPS:         []byte{0x01, 0x02, 0x03, 0x04},
		PPS:         []byte{0x01, 0x02, 0x03, 0x04},
	}

	tracks := Tracks{track}
	tracks.setControls()

	res, err := writeReqReadRes(conn, base.Request{
		Method: base.Announce,
		URL:    mustParseURL("rtsp://localhost:8554/teststream"),
		Header: base.Header{
			"CSeq":         base.HeaderValue{"1"},
			"Content-Type": base.HeaderValue{"application/sdp"},
		},
		Body: tracks.Marshal(),
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	th := &headers.Transport{
		Mode: func() *headers.TransportMode {
			v := headers.TransportModeRecord
			return &v
		}(),
		InterleavedIDs: &[2]int{0, 1},
	}

	res, err = writeReqReadRes(conn, base.Request{
		Method: base.Setup,
		URL:    mustParseURL("rtsp://localhost:8554/test2stream/trackID=0"),
		Header: base.Header{
			"CSeq":      base.HeaderValue{"2"},
			"Transport": th.Marshal(),
		},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusBadRequest, res.StatusCode)

	err = <-serverErr
	require.EqualError(t, err, "invalid track path (test2stream/trackID=0)")
}

func TestServerPublishErrorSetupTrackTwice(t *testing.T) {
	serverErr := make(chan error)

	s := &Server{
		handler: &testServerHandler{
			onConnClose: func(_ *ServerConn, err error) {
				serverErr <- err
			},
			onAnnounce: func(*ServerSession, string, Tracks) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
			onSetup: func(*ServerSession, string, int) (*base.Response, *ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil, nil
			},
		},
		rtspAddress: "localhost:8554",
	}

	err := s.Start()
	require.NoError(t, err)
	defer s.Close()

	nconn, err := net.Dial("tcp", "localhost:8554")
	require.NoError(t, err)
	defer nconn.Close()
	conn := conn.NewConn(nconn)

	track := &TrackH264{
		PayloadType: 96,
		SPS:         []byte{0x01, 0x02, 0x03, 0x04},
		PPS:         []byte{0x01, 0x02, 0x03, 0x04},
	}

	tracks := Tracks{track}
	tracks.setControls()

	res, err := writeReqReadRes(conn, base.Request{
		Method: base.Announce,
		URL:    mustParseURL("rtsp://localhost:8554/teststream"),
		Header: base.Header{
			"CSeq":         base.HeaderValue{"1"},
			"Content-Type": base.HeaderValue{"application/sdp"},
		},
		Body: tracks.Marshal(),
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	th := &headers.Transport{
		Mode: func() *headers.TransportMode {
			v := headers.TransportModeRecord
			return &v
		}(),
		InterleavedIDs: &[2]int{0, 1},
	}

	res, err = writeReqReadRes(conn, base.Request{
		Method: base.Setup,
		URL:    mustParseURL("rtsp://localhost:8554/teststream/trackID=0"),
		Header: base.Header{
			"CSeq":      base.HeaderValue{"2"},
			"Transport": th.Marshal(),
		},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	var sx headers.Session
	err = sx.Unmarshal(res.Header["Session"])
	require.NoError(t, err)

	res, err = writeReqReadRes(conn, base.Request{
		Method: base.Setup,
		URL:    mustParseURL("rtsp://localhost:8554/teststream/trackID=0"),
		Header: base.Header{
			"CSeq":      base.HeaderValue{"3"},
			"Transport": th.Marshal(),
			"Session":   base.HeaderValue{sx.Session},
		},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusBadRequest, res.StatusCode)

	err = <-serverErr
	require.EqualError(t, err, "track 0 has already been setup")
}

func TestServerPublishErrorRecordPartialTracks(t *testing.T) {
	serverErr := make(chan error)

	s := &Server{
		handler: &testServerHandler{
			onConnClose: func(_ *ServerConn, err error) {
				serverErr <- err
			},
			onAnnounce: func(*ServerSession, string, Tracks) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
			onSetup: func(*ServerSession, string, int) (*base.Response, *ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil, nil
			},
			onRecord: func(*ServerSession) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
		},
		rtspAddress: "localhost:8554",
	}

	err := s.Start()
	require.NoError(t, err)
	defer s.Close()

	nconn, err := net.Dial("tcp", "localhost:8554")
	require.NoError(t, err)
	defer nconn.Close()
	conn := conn.NewConn(nconn)

	track1 := &TrackH264{
		PayloadType: 96,
		SPS:         []byte{0x01, 0x02, 0x03, 0x04},
		PPS:         []byte{0x01, 0x02, 0x03, 0x04},
	}

	track2 := &TrackH264{
		PayloadType: 96,
		SPS:         []byte{0x01, 0x02, 0x03, 0x04},
		PPS:         []byte{0x01, 0x02, 0x03, 0x04},
	}

	tracks := Tracks{track1, track2}
	tracks.setControls()

	res, err := writeReqReadRes(conn, base.Request{
		Method: base.Announce,
		URL:    mustParseURL("rtsp://localhost:8554/teststream"),
		Header: base.Header{
			"CSeq":         base.HeaderValue{"1"},
			"Content-Type": base.HeaderValue{"application/sdp"},
		},
		Body: tracks.Marshal(),
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	th := &headers.Transport{
		Mode: func() *headers.TransportMode {
			v := headers.TransportModeRecord
			return &v
		}(),
		InterleavedIDs: &[2]int{0, 1},
	}

	res, err = writeReqReadRes(conn, base.Request{
		Method: base.Setup,
		URL:    mustParseURL("rtsp://localhost:8554/teststream/trackID=0"),
		Header: base.Header{
			"CSeq":      base.HeaderValue{"2"},
			"Transport": th.Marshal(),
		},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	var sx headers.Session
	err = sx.Unmarshal(res.Header["Session"])
	require.NoError(t, err)

	res, err = writeReqReadRes(conn, base.Request{
		Method: base.Record,
		URL:    mustParseURL("rtsp://localhost:8554/teststream"),
		Header: base.Header{
			"CSeq":    base.HeaderValue{"3"},
			"Session": base.HeaderValue{sx.Session},
		},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusBadRequest, res.StatusCode)

	err = <-serverErr
	require.EqualError(t, err, "not all announced tracks have been setup")
}

var oversizedPacketRTPIn = rtp.Packet{
	Header: rtp.Header{
		Version:        2,
		PayloadType:    96,
		Marker:         true,
		SequenceNumber: 34572,
	},
	Payload: bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04, 0x05}, 4096/5),
}

var oversizedPacketsRTPOut = []rtp.Packet{
	{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			Marker:         false,
			SequenceNumber: 34572,
		},
		Payload: mergeBytes(
			[]byte{0x1c, 0x81, 0x02, 0x03, 0x04, 0x05},
			bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04, 0x05}, 290),
			[]byte{0x01, 0x02, 0x03, 0x04},
		),
	},
	{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			Marker:         false,
			SequenceNumber: 34573,
		},
		Payload: mergeBytes(
			[]byte{0x1c, 0x01, 0x05},
			bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04, 0x05}, 291),
			[]byte{0x01, 0x02},
		),
	},
	{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			Marker:         true,
			SequenceNumber: 34574,
		},
		Payload: mergeBytes(
			[]byte{0x1c, 0x41, 0x03, 0x04, 0x05},
			bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04, 0x05}, 235),
		),
	},
}

func mergeBytes(vals ...[]byte) []byte {
	size := 0
	for _, v := range vals {
		size += len(v)
	}
	res := make([]byte, size)

	pos := 0
	for _, v := range vals {
		n := copy(res[pos:], v)
		pos += n
	}

	return res
}

func TestServerPublishErrorInvalidProtocol(t *testing.T) {
	errorRecv := make(chan struct{})

	s := &Server{
		handler: &testServerHandler{
			onConnClose: func(_ *ServerConn, err error) {
				require.EqualError(t, err, "received unexpected interleaved frame")
				close(errorRecv)
			},
			onAnnounce: func(*ServerSession, string, Tracks) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
			onSetup: func(*ServerSession, string, int) (*base.Response, *ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil, nil
			},
			onRecord: func(*ServerSession) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
			onPacketRTP: func(*ServerSession, int, *rtp.Packet) {
				t.Error("should not happen")
			},
		},
		rtspAddress: "localhost:8554",
	}

	err := s.Start()
	require.NoError(t, err)
	defer s.Close()

	nconn, err := net.Dial("tcp", "localhost:8554")
	require.NoError(t, err)
	defer nconn.Close()
	conn := conn.NewConn(nconn)

	track := &TrackH264{
		PayloadType: 96,
		SPS:         []byte{0x01, 0x02, 0x03, 0x04},
		PPS:         []byte{0x01, 0x02, 0x03, 0x04},
	}

	tracks := Tracks{track}
	tracks.setControls()

	res, err := writeReqReadRes(conn, base.Request{
		Method: base.Announce,
		URL:    mustParseURL("rtsp://localhost:8554/teststream"),
		Header: base.Header{
			"CSeq":         base.HeaderValue{"1"},
			"Content-Type": base.HeaderValue{"application/sdp"},
		},
		Body: tracks.Marshal(),
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	err = conn.WriteInterleavedFrame(&base.InterleavedFrame{
		Channel: 0,
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
	}, make([]byte, 1024))
	require.NoError(t, err)

	<-errorRecv
}

func TestServerPublishTimeout(t *testing.T) {
	connClosed := make(chan struct{})
	sessionClosed := make(chan struct{})

	s := &Server{
		handler: &testServerHandler{
			onConnClose: func(*ServerConn, error) {
				close(connClosed)
			},
			onSessionClose: func(*ServerSession, error) {
				close(sessionClosed)
			},
			onAnnounce: func(*ServerSession, string, Tracks) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
			onSetup: func(*ServerSession, string, int) (*base.Response, *ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil, nil
			},
			onRecord: func(*ServerSession) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
		},
		readTimeout: 2 * time.Millisecond,
		rtspAddress: "localhost:8554",
	}

	err := s.Start()
	require.NoError(t, err)
	defer s.Close()

	nconn, err := net.Dial("tcp", "localhost:8554")
	require.NoError(t, err)
	defer nconn.Close()
	conn := conn.NewConn(nconn)

	track := &TrackH264{
		PayloadType: 96,
		SPS:         []byte{0x01, 0x02, 0x03, 0x04},
		PPS:         []byte{0x01, 0x02, 0x03, 0x04},
	}

	tracks := Tracks{track}
	tracks.setControls()

	res, err := writeReqReadRes(conn, base.Request{
		Method: base.Announce,
		URL:    mustParseURL("rtsp://localhost:8554/teststream"),
		Header: base.Header{
			"CSeq":         base.HeaderValue{"1"},
			"Content-Type": base.HeaderValue{"application/sdp"},
		},
		Body: tracks.Marshal(),
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	inTH := &headers.Transport{
		Mode: func() *headers.TransportMode {
			v := headers.TransportModeRecord
			return &v
		}(),
	}

	inTH.InterleavedIDs = &[2]int{0, 1}

	res, err = writeReqReadRes(conn, base.Request{
		Method: base.Setup,
		URL:    mustParseURL("rtsp://localhost:8554/teststream/trackID=0"),
		Header: base.Header{
			"CSeq":      base.HeaderValue{"2"},
			"Transport": inTH.Marshal(),
		},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	var th headers.Transport
	err = th.Unmarshal(res.Header["Transport"])
	require.NoError(t, err)

	var sx headers.Session
	err = sx.Unmarshal(res.Header["Session"])
	require.NoError(t, err)

	res, err = writeReqReadRes(conn, base.Request{
		Method: base.Record,
		URL:    mustParseURL("rtsp://localhost:8554/teststream"),
		Header: base.Header{
			"CSeq":    base.HeaderValue{"3"},
			"Session": base.HeaderValue{sx.Session},
		},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	<-sessionClosed

	<-connClosed
}

func TestServerPublishWithoutTeardown(t *testing.T) {
	connClosed := make(chan struct{})
	sessionClosed := make(chan struct{})

	s := &Server{
		handler: &testServerHandler{
			onConnClose: func(*ServerConn, error) {
				close(connClosed)
			},
			onSessionClose: func(*ServerSession, error) {
				close(sessionClosed)
			},
			onAnnounce: func(*ServerSession, string, Tracks) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
			onSetup: func(*ServerSession, string, int) (*base.Response, *ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil, nil
			},
			onRecord: func(*ServerSession) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
		},
		readTimeout: 20 * time.Millisecond,
		rtspAddress: "localhost:8554",
	}

	err := s.Start()
	require.NoError(t, err)
	defer s.Close()

	nconn, err := net.Dial("tcp", "localhost:8554")
	require.NoError(t, err)
	conn := conn.NewConn(nconn)

	track := &TrackH264{
		PayloadType: 96,
		SPS:         []byte{0x01, 0x02, 0x03, 0x04},
		PPS:         []byte{0x01, 0x02, 0x03, 0x04},
	}

	tracks := Tracks{track}
	tracks.setControls()

	res, err := writeReqReadRes(conn, base.Request{
		Method: base.Announce,
		URL:    mustParseURL("rtsp://localhost:8554/teststream"),
		Header: base.Header{
			"CSeq":         base.HeaderValue{"1"},
			"Content-Type": base.HeaderValue{"application/sdp"},
		},
		Body: tracks.Marshal(),
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	inTH := &headers.Transport{
		Mode: func() *headers.TransportMode {
			v := headers.TransportModeRecord
			return &v
		}(),
	}

	inTH.InterleavedIDs = &[2]int{0, 1}

	res, err = writeReqReadRes(conn, base.Request{
		Method: base.Setup,
		URL:    mustParseURL("rtsp://localhost:8554/teststream/trackID=0"),
		Header: base.Header{
			"CSeq":      base.HeaderValue{"2"},
			"Transport": inTH.Marshal(),
		},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	var th headers.Transport
	err = th.Unmarshal(res.Header["Transport"])
	require.NoError(t, err)

	var sx headers.Session
	err = sx.Unmarshal(res.Header["Session"])
	require.NoError(t, err)

	res, err = writeReqReadRes(conn, base.Request{
		Method: base.Record,
		URL:    mustParseURL("rtsp://localhost:8554/teststream"),
		Header: base.Header{
			"CSeq":    base.HeaderValue{"3"},
			"Session": base.HeaderValue{sx.Session},
		},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	nconn.Close()

	<-sessionClosed
	<-connClosed
}
