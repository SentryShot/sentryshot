package video

import (
	"context"
	"errors"
	"fmt"
	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib"
	"regexp"
	"sync"
)

type pathHLSServer interface {
	pathSourceReady(*path, gortsplib.Tracks) (*HLSMuxer, error)
	pathSourceNotReady(pathName string)
}

type path struct {
	name      string
	conf      *PathConf
	wg        *sync.WaitGroup
	hlsServer pathHLSServer
	logger    log.ILogger

	source      *rtspSession
	sourceReady bool
	stream      *stream
	readers     map[*rtspSession]struct{}

	mu       sync.Mutex
	canceled bool
}

func newPath(
	ctx context.Context,
	name string,
	conf *PathConf,
	wg *sync.WaitGroup,
	hlsServer pathHLSServer,
	logger log.ILogger,
) *path {
	pa := &path{
		name:      name,
		conf:      conf,
		wg:        wg,
		hlsServer: hlsServer,
		logger:    logger,
		readers:   make(map[*rtspSession]struct{}),
	}

	pa.wg.Add(1)
	go func() {
		<-ctx.Done()
		pa.close()
	}()

	return pa
}

func (pa *path) logf(level log.Level, format string, a ...interface{}) {
	processName := func() string {
		if pa.conf.IsSub {
			return "sub"
		}
		return "main"
	}()
	msg := fmt.Sprintf("%v: %v", processName, fmt.Sprintf(format, a...))
	pa.logger.Log(log.Entry{
		Level:     level,
		Src:       "monitor",
		MonitorID: pa.conf.MonitorID,
		Msg:       msg,
	})
}

func (pa *path) close() {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	if pa.canceled {
		return
	}

	if pa.sourceReady {
		pa.hlsServer.pathSourceNotReady(pa.name)
		pa.sourceReady = false
	}
	if pa.source != nil {
		pa.source.close()
	}

	// Close source before stream.
	if pa.stream != nil {
		pa.stream.close()
		pa.stream = nil
	}

	for r := range pa.readers {
		r.close()
		delete(pa.readers, r)
	}

	pa.canceled = true
	pa.wg.Done()
}

// ErrPathBusy another publisher is aldreay publishing to path.
var ErrPathBusy = errors.New("another publisher is already publishing to path")

// ErrPathNoOnePublishing No one is publishing to path.
var ErrPathNoOnePublishing = errors.New("no one is publishing to path")

// streamGet is called by a rtsp reader through pathManager.
func (pa *path) streamGet() (*stream, error) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	if pa.canceled {
		return nil, context.Canceled
	}

	if pa.sourceReady {
		return pa.stream, nil
	}
	return nil, fmt.Errorf("%w: (%s)", ErrPathNoOnePublishing, pa.name)
}

// publisherAdd is called by a publisher through pathManager.
func (pa *path) publisherAdd(session *rtspSession) (*path, error) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	if pa.canceled {
		return nil, context.Canceled
	}

	if pa.source != nil {
		return nil, ErrPathBusy
	}
	pa.source = session

	return pa, nil
}

// publisherStart is called by a publisher.
func (pa *path) publisherStart(tracks gortsplib.Tracks) (*stream, error) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	if pa.canceled {
		return nil, context.Canceled
	}

	hlsMuxer, err := pa.hlsServer.pathSourceReady(pa, tracks)
	if err != nil {
		return nil, err
	}

	pa.stream = newStream(tracks, hlsMuxer)
	pa.sourceReady = true

	return pa.stream, err
}

// readerRemove is called by a rtsp session.
func (pa *path) readerRemove(session *rtspSession) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	if pa.canceled {
		return
	}

	delete(pa.readers, session)
}

// readerAdd is called by a reader through pathManager.
func (pa *path) readerAdd(session *rtspSession) (*path, *stream, error) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	if pa.canceled {
		return nil, nil, context.Canceled
	}

	if pa.sourceReady {
		pa.readers[session] = struct{}{}
		return pa, pa.stream, nil
	}

	return nil, nil, fmt.Errorf("%w: (%s)", ErrPathNoOnePublishing, pa.name)
}

// readerStart is called by a rtsp session.
func (pa *path) readerStart(session *rtspSession) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	if pa.canceled {
		return
	}

	pa.readers[session] = struct{}{}
}

// Errors.
var (
	ErrEmptyName    = errors.New("name can not be empty")
	ErrSlashStart   = errors.New("name can't begin with a slash")
	ErrSlashEnd     = errors.New("name can't end with a slash")
	ErrInvalidChars = errors.New("can contain only alphanumeric" +
		" characters, underscore, dot, tilde, minus or slash")
)

var rePathName = regexp.MustCompile(`^[0-9a-zA-Z_\-/\.~]+$`)

func isValidPathName(name string) error {
	if name == "" {
		return ErrEmptyName
	}

	if name[0] == '/' {
		return ErrSlashStart
	}

	if name[len(name)-1] == '/' {
		return ErrSlashEnd
	}

	if !rePathName.MatchString(name) {
		return ErrInvalidChars
	}

	return nil
}

// PathConf is a path configuration.
type PathConf struct {
	MonitorID string
	IsSub     bool
}

// Errors.
var (
	ErrEmptyPathName  = errors.New("path name can not be empty")
	ErrEmptyMonitorID = errors.New("MonitorID can not be empty")
	ErrInvalidURL     = errors.New("invalid URL")
	ErrInvalidSource  = errors.New("invalid source")
)

// CheckAndFillMissing .
func (pconf *PathConf) CheckAndFillMissing(name string) error {
	if name == "" {
		return ErrEmptyPathName
	}
	if pconf.MonitorID == "" {
		return ErrEmptyMonitorID
	}

	err := isValidPathName(name)
	if err != nil {
		return fmt.Errorf("invalid path name: %s (%w)", name, err)
	}

	return nil
}
