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
	onConnClose    func(*ServerHandlerOnConnCloseCtx)
	onSessionOpen  func(*ServerHandlerOnSessionOpenCtx)
	onSessionClose func(*ServerHandlerOnSessionCloseCtx)
	onDescribe     func(*ServerHandlerOnDescribeCtx) (*base.Response, *ServerStream, error)
	onAnnounce     func(*ServerHandlerOnAnnounceCtx) (*base.Response, error)
	onSetup        func(*ServerHandlerOnSetupCtx) (*base.Response, *ServerStream, error)
	onPlay         func(*ServerHandlerOnPlayCtx) (*base.Response, error)
	onRecord       func(*ServerHandlerOnRecordCtx) (*base.Response, error)
	onPause        func(*ServerHandlerOnPauseCtx) (*base.Response, error)
	onPacketRTP    func(*ServerHandlerOnPacketRTPCtx)
}

func (sh *testServerHandler) OnConnClose(ctx *ServerHandlerOnConnCloseCtx) {
	if sh.onConnClose != nil {
		sh.onConnClose(ctx)
	}
}

func (sh *testServerHandler) OnSessionOpen(ctx *ServerHandlerOnSessionOpenCtx) {
	if sh.onSessionOpen != nil {
		sh.onSessionOpen(ctx)
	}
}

func (sh *testServerHandler) OnSessionClose(ctx *ServerHandlerOnSessionCloseCtx) {
	if sh.onSessionClose != nil {
		sh.onSessionClose(ctx)
	}
}

func (sh *testServerHandler) OnDescribe(ctx *ServerHandlerOnDescribeCtx) (*base.Response, *ServerStream, error) {
	if sh.onDescribe != nil {
		return sh.onDescribe(ctx)
	}
	return nil, nil, fmt.Errorf("unimplemented")
}

func (sh *testServerHandler) OnAnnounce(ctx *ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	if sh.onAnnounce != nil {
		return sh.onAnnounce(ctx)
	}
	return nil, fmt.Errorf("unimplemented")
}

func (sh *testServerHandler) OnSetup(ctx *ServerHandlerOnSetupCtx) (*base.Response, *ServerStream, error) {
	if sh.onSetup != nil {
		return sh.onSetup(ctx)
	}
	return nil, nil, fmt.Errorf("unimplemented")
}

func (sh *testServerHandler) OnPlay(ctx *ServerHandlerOnPlayCtx) (*base.Response, error) {
	if sh.onPlay != nil {
		return sh.onPlay(ctx)
	}
	return nil, fmt.Errorf("unimplemented")
}

func (sh *testServerHandler) OnRecord(ctx *ServerHandlerOnRecordCtx) (*base.Response, error) {
	if sh.onRecord != nil {
		return sh.onRecord(ctx)
	}
	return nil, fmt.Errorf("unimplemented")
}

func (sh *testServerHandler) OnPause(ctx *ServerHandlerOnPauseCtx) (*base.Response, error) {
	if sh.onPause != nil {
		return sh.onPause(ctx)
	}
	return nil, fmt.Errorf("unimplemented")
}

func (sh *testServerHandler) OnPacketRTP(ctx *ServerHandlerOnPacketRTPCtx) {
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
			onConnClose: func(ctx *ServerHandlerOnConnCloseCtx) {
				require.EqualError(t, ctx.Error, "CSeq is missing")
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
					onSetup: func(ctx *ServerHandlerOnSetupCtx) (*base.Response, *ServerStream, error) {
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
			onSetup: func(ctx *ServerHandlerOnSetupCtx) (*base.Response, *ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, stream, nil
			},
			onPlay: func(ctx *ServerHandlerOnPlayCtx) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
			onPause: func(ctx *ServerHandlerOnPauseCtx) (*base.Response, error) {
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
			onSetup: func(ctx *ServerHandlerOnSetupCtx) (*base.Response, *ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, stream, nil
			},
			onPlay: func(ctx *ServerHandlerOnPlayCtx) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
			onPause: func(ctx *ServerHandlerOnPauseCtx) (*base.Response, error) {
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
		base.Pause,
		base.Teardown,
	} {
		t.Run(string(method), func(t *testing.T) {
			s := &Server{
				Handler: &testServerHandler{
					onPlay: func(ctx *ServerHandlerOnPlayCtx) (*base.Response, error) {
						return &base.Response{
							StatusCode: base.StatusOK,
						}, nil
					},
					onRecord: func(ctx *ServerHandlerOnRecordCtx) (*base.Response, error) {
						return &base.Response{
							StatusCode: base.StatusOK,
						}, nil
					},
					onPause: func(ctx *ServerHandlerOnPauseCtx) (*base.Response, error) {
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
			onSessionOpen: func(ctx *ServerHandlerOnSessionOpenCtx) {
				ctx.Session.Close()
				ctx.Session.Close()
			},
			onSessionClose: func(ctx *ServerHandlerOnSessionCloseCtx) {
				close(sessionClosed)
			},
			onSetup: func(ctx *ServerHandlerOnSetupCtx) (*base.Response, *ServerStream, error) {
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
					onSessionClose: func(ctx *ServerHandlerOnSessionCloseCtx) {
						close(sessionClosed)
					},
					onSetup: func(ctx *ServerHandlerOnSetupCtx) (*base.Response, *ServerStream, error) {
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
