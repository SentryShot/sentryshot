package rtsp

import (
	"errors"
	"nvr/pkg/log"
	"nvr/pkg/video/rtsp/gortsplib"
	"nvr/pkg/video/rtsp/gortsplib/pkg/base"
	"time"
)

type rtspConn struct {
	rtspAddress string
	readTimeout time.Duration
	pathManager *pathManager
	conn        *gortsplib.ServerConn
	logger      *log.Logger
}

func newRTSPConn(
	rtspAddress string,
	readTimeout time.Duration,
	pathManager *pathManager,
	conn *gortsplib.ServerConn,
	logger *log.Logger) *rtspConn {
	c := &rtspConn{
		rtspAddress: rtspAddress,
		readTimeout: readTimeout,
		pathManager: pathManager,
		conn:        conn,
		logger:      logger,
	}

	// c.log(Info, "opened")
	return c
}

/*func (c *rtspConn) log(level log.Level, conf PathConf, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	sendLog(c.logger, conf, level, fmt.Sprintf("C:%s %s", c.conn.NetConn().RemoteAddr(), msg))
}*/

/*func (c *rtspConn) log(level Level, format string, args ...interface{}) {
	c.parent.log(level, "[conn %v] "+format, append([]interface{}{c.conn.NetConn().RemoteAddr()}, args...)...)
}*/

// Conn returns the RTSP connection.
func (c *rtspConn) Conn() *gortsplib.ServerConn {
	return c.conn
}

// onClose is called by rtspServer.
func (c *rtspConn) onClose(err error) {
	// c.log(Info, "closed (%v)", err)
}

// onRequest is called by rtspServer.
func (c *rtspConn) onRequest(req *base.Request) {
	// c.log(Debug, "[c->s] %v", req)
}

// OnResponse is called by rtspServer.
func (c *rtspConn) OnResponse(res *base.Response) {
	// c.log(Debug, "[s->c] %v", res)
}

// onDescribe is called by rtspServer.
func (c *rtspConn) onDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	res := c.pathManager.onDescribe(pathDescribeReq{
		PathName: ctx.Path,
		URL:      ctx.Req.URL,
	})

	if res.Err != nil {
		if errors.Is(res.Err, ErrPathNoOnePublishing) {
			return &base.Response{
				StatusCode: base.StatusNotFound,
			}, nil, res.Err
		}
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, nil, res.Err
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, res.Stream.rtspStream, nil
}
