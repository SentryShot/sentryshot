package liberrors

import (
	"errors"
	"fmt"
	"net"
	"nvr/pkg/video/rtsp/gortsplib/pkg/base"
	"nvr/pkg/video/rtsp/gortsplib/pkg/headers"
)

// ErrServerTerminated terminated.
var ErrServerTerminated = errors.New("terminated")

// ErrServerSessionNotFound session not found.
var ErrServerSessionNotFound = errors.New("session not found")

// ErrServerCSeqMissing CSeq is missing.
var ErrServerCSeqMissing = errors.New("CSeq is missing")

// ServerUnhandledRequestError is an error that can be returned by a server.
type ServerUnhandledRequestError struct {
	Req *base.Request
}

// Error implements the error interface.
func (e ServerUnhandledRequestError) Error() string {
	return fmt.Sprintf("unhandled request: %v %v", e.Req.Method, e.Req.URL)
}

// ServerInvalidStateError is an error that can be returned by a server.
type ServerInvalidStateError struct {
	AllowedList []fmt.Stringer
	State       fmt.Stringer
}

// Error implements the error interface.
func (e ServerInvalidStateError) Error() string {
	return fmt.Sprintf("must be in state %v, while is in state %v",
		e.AllowedList, e.State)
}

// ErrServerInvalidPath invalid path.
var ErrServerInvalidPath = errors.New("invalid path")

// ErrServerContentTypeMissing Content-Type header is missing.
var ErrServerContentTypeMissing = errors.New("Content-Type header is missing")

// ErrServerContentTypeUnsupportedError is an error that can be returned by a server.
type ErrServerContentTypeUnsupportedError struct {
	CT base.HeaderValue
}

// Error implements the error interface.
func (e ErrServerContentTypeUnsupportedError) Error() string {
	return fmt.Sprintf("unsupported Content-Type header '%v'", e.CT)
}

// ServerSDPinvalidError is an error that can be returned by a server.
type ServerSDPinvalidError struct {
	Err error
}

// Error implements the error interface.
func (e ServerSDPinvalidError) Error() string {
	return fmt.Sprintf("invalid SDP: %v", e.Err)
}

// ErrServerSDPnoTracksDefined no tracks defined in the SDP.
var ErrServerSDPnoTracksDefined = errors.New(
	"no tracks defined in the SDP")

// ServerTransportHeaderInvalidError is an error that can be returned by a server.
type ServerTransportHeaderInvalidError struct {
	Err error
}

// Error implements the error interface.
func (e ServerTransportHeaderInvalidError) Error() string {
	return fmt.Sprintf("invalid transport header: %v", e.Err)
}

// ServerTrackAlreadySetupError is an error that can be returned by a server.
type ServerTrackAlreadySetupError struct {
	TrackID int
}

// Error implements the error interface.
func (e ServerTrackAlreadySetupError) Error() string {
	return fmt.Sprintf("track %d has already been setup", e.TrackID)
}

// ServerTransportHeaderInvalidModeError is an error that can be returned by a server.
type ServerTransportHeaderInvalidModeError struct {
	Mode *headers.TransportMode
}

// Error implements the error interface.
func (e ServerTransportHeaderInvalidModeError) Error() string {
	return fmt.Sprintf("transport header contains a invalid mode (%v)", e.Mode)
}

// ErrServerTransportHeaderNoInterleavedIDs is an error that can be returned by a server.
var ErrServerTransportHeaderNoInterleavedIDs = errors.New(
	"transport header does not contain interleaved IDs")

// ErrServerTransportHeaderInvalidInterleavedIDs invalid interleaved IDs.
var ErrServerTransportHeaderInvalidInterleavedIDs = errors.New("invalid interleaved IDs")

// ErrServerTransportHeaderInterleavedIDsAlreadyUsed interleaved IDs already used.
var ErrServerTransportHeaderInterleavedIDsAlreadyUsed = errors.New(
	"interleaved IDs already used")

// ErrServerTracksDifferentProtocols can't setup tracks with different protocols.
var ErrServerTracksDifferentProtocols = errors.New(
	"can't setup tracks with different protocols")

// ErrServerNotAllAnnouncedTracksSetup not all announced tracks have been setup.
var ErrServerNotAllAnnouncedTracksSetup = errors.New(
	"not all announced tracks have been setup")

// ErrServerLinkedToOtherSession connection is linked to another session.
var ErrServerLinkedToOtherSession = errors.New(
	"connection is linked to another session")

// ServerSessionTeardownError is an error that can be returned by a server.
type ServerSessionTeardownError struct {
	Author net.Addr
}

// Error implements the error interface.
func (e ServerSessionTeardownError) Error() string {
	return fmt.Sprintf("teared down by %v", e.Author)
}

// ErrServerSessionLinkedToOtherConn is an error that can be returned by a server.
var ErrServerSessionLinkedToOtherConn = errors.New(
	"session is linked to another connection")

// ErrServerInvalidSession is an error that can be returned by a server.
type ErrServerInvalidSession struct{}

// ServerPathHasChangedError is an error that can be returned by a server.
type ServerPathHasChangedError struct {
	Prev string
	Cur  string
}

// Error implements the error interface.
func (e ServerPathHasChangedError) Error() string {
	return fmt.Sprintf("path has changed, was '%s', now is '%s'", e.Prev, e.Cur)
}

// ErrServerCannotUseSessionCreatedByOtherIP cannot use a session created with a different IP.
var ErrServerCannotUseSessionCreatedByOtherIP = errors.New(
	"cannot use a session created with a different IP")

// ErrServerSessionNotInUse not in use.
var ErrServerSessionNotInUse = errors.New("not in use")
