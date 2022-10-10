package gortsplib

import (
	"fmt"
	"net"
	"testing"

	"nvr/pkg/video/gortsplib/pkg/base"
	"nvr/pkg/video/gortsplib/pkg/conn"
	"nvr/pkg/video/gortsplib/pkg/headers"

	"github.com/stretchr/testify/require"
)

func writeReqReadRes(conn *conn.Conn, req base.Request) (*base.Response, error) {
	if err := conn.WriteRequest(&req); err != nil {
		return nil, err
	}

	return conn.ReadResponse()
}

type testServerHandler struct {
	onConnClose    func(*ServerConn, error)
	onSessionOpen  func(*ServerSession, *ServerConn)
	onSessionClose func(*ServerSession, error)
	onAnnounce     func(*ServerSession, string, Tracks) (*base.Response, error)
	onSetup        func(*ServerSession, string, int) (*base.Response, *ServerStream, error)
	onPlay         func(*ServerSession) (*base.Response, error)
	onRecord       func(*ServerSession) (*base.Response, error)
	onPacketRTP    func(*PacketRTPCtx)
}

func (sh *testServerHandler) OnConnClose(sc *ServerConn, err error) {
	if sh.onConnClose != nil {
		sh.onConnClose(sc, err)
	}
}

func (sh *testServerHandler) OnSessionOpen(
	session *ServerSession,
	conn *ServerConn,
) {
	if sh.onSessionOpen != nil {
		sh.onSessionOpen(session, conn)
	}
}

func (sh *testServerHandler) OnSessionClose(session *ServerSession, err error) {
	if sh.onSessionClose != nil {
		sh.onSessionClose(session, err)
	}
}

func (sh *testServerHandler) OnDescribe(
	pathName string,
) (*base.Response, *ServerStream, error) {
	return nil, nil, fmt.Errorf("unimplemented")
}

func (sh *testServerHandler) OnAnnounce(
	session *ServerSession,
	path string,
	tracks Tracks,
) (*base.Response, error) {
	if sh.onAnnounce != nil {
		return sh.onAnnounce(session, path, tracks)
	}
	return nil, fmt.Errorf("unimplemented")
}

func (sh *testServerHandler) OnSetup(
	session *ServerSession,
	path string,
	trackID int,
) (*base.Response, *ServerStream, error) {
	if sh.onSetup != nil {
		return sh.onSetup(session, path, trackID)
	}
	return nil, nil, fmt.Errorf("unimplemented")
}

func (sh *testServerHandler) OnPlay(
	session *ServerSession,
) (*base.Response, error) {
	if sh.onPlay != nil {
		return sh.onPlay(session)
	}
	return nil, fmt.Errorf("unimplemented")
}

func (sh *testServerHandler) OnRecord(
	session *ServerSession,
) (*base.Response, error) {
	if sh.onRecord != nil {
		return sh.onRecord(session)
	}
	return nil, fmt.Errorf("unimplemented")
}

func (sh *testServerHandler) OnPacketRTP(ctx *PacketRTPCtx) {
	if sh.onPacketRTP != nil {
		sh.onPacketRTP(ctx)
	}
}

func TestServerClose(t *testing.T) {
	s := &Server{
		Handler:     &testServerHandler{},
		RTSPAddress: "localhost:8554",
	}

	err := s.Start()
	require.NoError(t, err)
	s.Close()
	s.Close()
}

func TestServerCSeq(t *testing.T) {
	s := &Server{
		RTSPAddress: "localhost:8554",
		Handler:     &testServerHandler{},
	}
	err := s.Start()
	require.NoError(t, err)
	defer s.Close()

	nconn, err := net.Dial("tcp", "localhost:8554")
	require.NoError(t, err)
	defer nconn.Close()
	conn := conn.NewConn(nconn)

	res, err := writeReqReadRes(conn, base.Request{
		Method: base.Options,
		URL:    mustParseURL("rtsp://localhost:8554/"),
		Header: base.Header{
			"CSeq": base.HeaderValue{"5"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	require.Equal(t, base.HeaderValue{"5"}, res.Header["CSeq"])
}

func TestServerErrorCSeqMissing(t *testing.T) {
	connClosed := make(chan struct{})

	s := &Server{
		Handler: &testServerHandler{
			onConnClose: func(_ *ServerConn, err error) {
				require.EqualError(t, err, "CSeq is missing")
				close(connClosed)
			},
		},
		RTSPAddress: "localhost:8554",
	}
	err := s.Start()
	require.NoError(t, err)
	defer s.Close()

	nconn, err := net.Dial("tcp", "localhost:8554")
	require.NoError(t, err)
	defer nconn.Close()
	conn := conn.NewConn(nconn)

	res, err := writeReqReadRes(conn, base.Request{
		Method: base.Options,
		URL:    mustParseURL("rtsp://localhost:8554/"),
		Header: base.Header{},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusBadRequest, res.StatusCode)

	<-connClosed
}

func TestServerErrorMethodNotImplemented(t *testing.T) {
	for _, ca := range []string{"outside session", "inside session"} {
		t.Run(ca, func(t *testing.T) {
			track := &TrackH264{
				PayloadType: 96,
				SPS:         []byte{0x01, 0x02, 0x03, 0x04},
				PPS:         []byte{0x01, 0x02, 0x03, 0x04},
			}

			stream := NewServerStream(Tracks{track})
			defer stream.Close()
			s := &Server{
				Handler: &testServerHandler{
					onSetup: func(*ServerSession, string, int) (*base.Response, *ServerStream, error) {
						return &base.Response{
							StatusCode: base.StatusOK,
						}, stream, nil
					},
				},
				RTSPAddress: "localhost:8554",
			}

			err := s.Start()
			require.NoError(t, err)
			defer s.Close()

			nconn, err := net.Dial("tcp", "localhost:8554")
			require.NoError(t, err)
			defer nconn.Close()
			conn := conn.NewConn(nconn)

			var sx headers.Session

			if ca == "inside session" {
				res, err := writeReqReadRes(conn, base.Request{
					Method: base.Setup,
					URL:    mustParseURL("rtsp://localhost:8554/teststream/trackID=0"),
					Header: base.Header{
						"CSeq": base.HeaderValue{"1"},
						"Transport": headers.Transport{
							Mode: func() *headers.TransportMode {
								v := headers.TransportModePlay
								return &v
							}(),
							InterleavedIDs: &[2]int{0, 1},
						}.Marshal(),
					},
				})
				require.NoError(t, err)

				err = sx.Unmarshal(res.Header["Session"])
				require.NoError(t, err)
			}

			headers := base.Header{
				"CSeq": base.HeaderValue{"2"},
			}
			if ca == "inside session" {
				headers["Session"] = base.HeaderValue{sx.Session}
			}

			res, err := writeReqReadRes(conn, base.Request{
				Method: base.SetParameter,
				URL:    mustParseURL("rtsp://localhost:8554/teststream/trackID=0"),
				Header: headers,
			})
			require.NoError(t, err)
			require.Equal(t, base.StatusNotImplemented, res.StatusCode)

			headers = base.Header{
				"CSeq": base.HeaderValue{"3"},
			}
			if ca == "inside session" {
				headers["Session"] = base.HeaderValue{sx.Session}
			}

			res, err = writeReqReadRes(conn, base.Request{
				Method: base.Options,
				URL:    mustParseURL("rtsp://localhost:8554/teststream/trackID=0"),
				Header: headers,
			})
			require.NoError(t, err)
			require.Equal(t, base.StatusOK, res.StatusCode)
		})
	}
}

func TestServerErrorTCPTwoConnOneSession(t *testing.T) {
	track := &TrackH264{
		PayloadType: 96,
		SPS:         []byte{0x01, 0x02, 0x03, 0x04},
		PPS:         []byte{0x01, 0x02, 0x03, 0x04},
	}

	stream := NewServerStream(Tracks{track})
	defer stream.Close()

	s := &Server{
		Handler: &testServerHandler{
			onSetup: func(*ServerSession, string, int) (*base.Response, *ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, stream, nil
			},
			onPlay: func(*ServerSession) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
		},
		RTSPAddress: "localhost:8554",
	}

	err := s.Start()
	require.NoError(t, err)
	defer s.Close()

	nconn1, err := net.Dial("tcp", "localhost:8554")
	require.NoError(t, err)
	defer nconn1.Close()
	conn1 := conn.NewConn(nconn1)

	res, err := writeReqReadRes(conn1, base.Request{
		Method: base.Setup,
		URL:    mustParseURL("rtsp://localhost:8554/teststream/trackID=0"),
		Header: base.Header{
			"CSeq": base.HeaderValue{"1"},
			"Transport": headers.Transport{
				Mode: func() *headers.TransportMode {
					v := headers.TransportModePlay
					return &v
				}(),
				InterleavedIDs: &[2]int{0, 1},
			}.Marshal(),
		},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	var sx headers.Session
	err = sx.Unmarshal(res.Header["Session"])
	require.NoError(t, err)

	res, err = writeReqReadRes(conn1, base.Request{
		Method: base.Play,
		URL:    mustParseURL("rtsp://localhost:8554/teststream"),
		Header: base.Header{
			"CSeq":    base.HeaderValue{"2"},
			"Session": base.HeaderValue{sx.Session},
		},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	nconn2, err := net.Dial("tcp", "localhost:8554")
	require.NoError(t, err)
	defer nconn2.Close()
	conn2 := conn.NewConn(nconn2)

	res, err = writeReqReadRes(conn2, base.Request{
		Method: base.Setup,
		URL:    mustParseURL("rtsp://localhost:8554/teststream/trackID=0"),
		Header: base.Header{
			"CSeq": base.HeaderValue{"1"},
			"Transport": headers.Transport{
				Mode: func() *headers.TransportMode {
					v := headers.TransportModePlay
					return &v
				}(),
				InterleavedIDs: &[2]int{0, 1},
			}.Marshal(),
			"Session": base.HeaderValue{sx.Session},
		},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusBadRequest, res.StatusCode)
}

func TestServerErrorTCPOneConnTwoSessions(t *testing.T) {
	track := &TrackH264{
		PayloadType: 96,
		SPS:         []byte{0x01, 0x02, 0x03, 0x04},
		PPS:         []byte{0x01, 0x02, 0x03, 0x04},
	}

	stream := NewServerStream(Tracks{track})
	defer stream.Close()

	s := &Server{
		Handler: &testServerHandler{
			onSetup: func(*ServerSession, string, int) (*base.Response, *ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, stream, nil
			},
			onPlay: func(*ServerSession) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
		},
		RTSPAddress: "localhost:8554",
	}

	err := s.Start()
	require.NoError(t, err)
	defer s.Close()

	nconn, err := net.Dial("tcp", "localhost:8554")
	require.NoError(t, err)
	defer nconn.Close()
	conn := conn.NewConn(nconn)

	res, err := writeReqReadRes(conn, base.Request{
		Method: base.Setup,
		URL:    mustParseURL("rtsp://localhost:8554/teststream/trackID=0"),
		Header: base.Header{
			"CSeq": base.HeaderValue{"1"},
			"Transport": headers.Transport{
				Mode: func() *headers.TransportMode {
					v := headers.TransportModePlay
					return &v
				}(),
				InterleavedIDs: &[2]int{0, 1},
			}.Marshal(),
		},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	var sx headers.Session
	err = sx.Unmarshal(res.Header["Session"])
	require.NoError(t, err)

	res, err = writeReqReadRes(conn, base.Request{
		Method: base.Play,
		URL:    mustParseURL("rtsp://localhost:8554/teststream"),
		Header: base.Header{
			"CSeq":    base.HeaderValue{"2"},
			"Session": base.HeaderValue{sx.Session},
		},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusOK, res.StatusCode)

	res, err = writeReqReadRes(conn, base.Request{
		Method: base.Setup,
		URL:    mustParseURL("rtsp://localhost:8554/teststream/trackID=0"),
		Header: base.Header{
			"CSeq": base.HeaderValue{"3"},
			"Transport": headers.Transport{
				Mode: func() *headers.TransportMode {
					v := headers.TransportModePlay
					return &v
				}(),
				InterleavedIDs: &[2]int{0, 1},
			}.Marshal(),
		},
	})
	require.NoError(t, err)
	require.Equal(t, base.StatusBadRequest, res.StatusCode)
}

func TestServerErrorInvalidSession(t *testing.T) {
	for _, method := range []base.Method{
		base.Play,
		base.Record,
		base.Teardown,
	} {
		t.Run(string(method), func(t *testing.T) {
			s := &Server{
				Handler: &testServerHandler{
					onPlay: func(*ServerSession) (*base.Response, error) {
						return &base.Response{
							StatusCode: base.StatusOK,
						}, nil
					},
					onRecord: func(*ServerSession) (*base.Response, error) {
						return &base.Response{
							StatusCode: base.StatusOK,
						}, nil
					},
				},
				RTSPAddress: "localhost:8554",
			}

			err := s.Start()
			require.NoError(t, err)
			defer s.Close()

			nconn, err := net.Dial("tcp", "localhost:8554")
			require.NoError(t, err)
			defer nconn.Close()
			conn := conn.NewConn(nconn)

			res, err := writeReqReadRes(conn, base.Request{
				Method: method,
				URL:    mustParseURL("rtsp://localhost:8554/teststream"),
				Header: base.Header{
					"CSeq":    base.HeaderValue{"1"},
					"Session": base.HeaderValue{"ABC"},
				},
			})
			require.NoError(t, err)
			require.Equal(t, base.StatusSessionNotFound, res.StatusCode)
		})
	}
}

func TestServerSessionClose(t *testing.T) {
	sessionClosed := make(chan struct{})

	s := &Server{
		Handler: &testServerHandler{
			onSessionOpen: func(session *ServerSession, _ *ServerConn) {
				session.Close()
			},
			onSessionClose: func(*ServerSession, error) {
				close(sessionClosed)
			},
			onSetup: func(*ServerSession, string, int) (*base.Response, *ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil, nil
			},
		},
		RTSPAddress: "localhost:8554",
	}

	err := s.Start()
	require.NoError(t, err)
	defer s.Close()

	conn, err := net.Dial("tcp", "localhost:8554")
	require.NoError(t, err)
	defer conn.Close()

	byts, _ := base.Request{
		Method: base.Setup,
		URL:    mustParseURL("rtsp://localhost:8554/teststream/trackID=0"),
		Header: base.Header{
			"CSeq": base.HeaderValue{"1"},
			"Transport": headers.Transport{
				Mode: func() *headers.TransportMode {
					v := headers.TransportModePlay
					return &v
				}(),
				InterleavedIDs: &[2]int{0, 1},
			}.Marshal(),
		},
	}.Marshal()
	_, err = conn.Write(byts)
	require.NoError(t, err)

	<-sessionClosed
}

func TestServerSessionAutoClose(t *testing.T) {
	for _, ca := range []string{
		"200", "400",
	} {
		t.Run(ca, func(t *testing.T) {
			sessionClosed := make(chan struct{})

			track := &TrackH264{
				PayloadType: 96,
				SPS:         []byte{0x01, 0x02, 0x03, 0x04},
				PPS:         []byte{0x01, 0x02, 0x03, 0x04},
			}

			stream := NewServerStream(Tracks{track})
			defer stream.Close()

			s := &Server{
				Handler: &testServerHandler{
					onSessionClose: func(*ServerSession, error) {
						close(sessionClosed)
					},
					onSetup: func(*ServerSession, string, int) (*base.Response, *ServerStream, error) {
						if ca == "200" {
							return &base.Response{
								StatusCode: base.StatusOK,
							}, stream, nil
						}

						return &base.Response{
							StatusCode: base.StatusBadRequest,
						}, nil, fmt.Errorf("error")
					},
				},
				RTSPAddress: "localhost:8554",
			}

			err := s.Start()
			require.NoError(t, err)
			defer s.Close()

			nconn, err := net.Dial("tcp", "localhost:8554")
			require.NoError(t, err)
			conn := conn.NewConn(nconn)

			_, err = writeReqReadRes(conn, base.Request{
				Method: base.Setup,
				URL:    mustParseURL("rtsp://localhost:8554/teststream/trackID=0"),
				Header: base.Header{
					"CSeq": base.HeaderValue{"1"},
					"Transport": headers.Transport{
						Mode: func() *headers.TransportMode {
							v := headers.TransportModePlay
							return &v
						}(),
						InterleavedIDs: &[2]int{0, 1},
					}.Marshal(),
				},
			})
			require.NoError(t, err)

			nconn.Close()

			<-sessionClosed
		})
	}
}
