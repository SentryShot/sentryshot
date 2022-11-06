package video

import (
	"context"
	"errors"
	"fmt"
	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib"
	"regexp"
	"sync"
	"time"
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

	source      closer
	sourceReady bool
	stream      *stream
	readers     map[closer]struct{}

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
		readers:   make(map[closer]struct{}),
	}

	pa.wg.Add(1)
	go func() {
		<-ctx.Done()
		pa.close()
	}()

	return pa
}

func (pa *path) close() {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	if pa.canceled {
		return
	}

	for r := range pa.readers {
		r.close()
		delete(pa.readers, r)
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

	pa.readers[hlsMuxer] = struct{}{}
	pa.stream = newStream(tracks, hlsMuxer)
	pa.sourceReady = true

	return pa.stream, err
}

// readerRemove is called by a reader.
func (pa *path) readerRemove(author closer) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	if pa.canceled {
		return
	}

	delete(pa.readers, author)
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

// readerStart is called by a reader.
func (pa *path) readerStart(author closer) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	if pa.canceled {
		return
	}

	pa.readers[author] = struct{}{}
}

func (pa *path) hlsSegmentCount() int {
	return pa.conf.HLSSegmentCount
}

func (pa *path) hlsSegmentDuration() time.Duration {
	return pa.conf.HLSSegmentDuration
}

func (pa *path) hlsPartDuration() time.Duration {
	return pa.conf.HLSPartDuration
}

func (pa *path) hlsSegmentMaxSize() uint64 {
	return pa.conf.HLSSegmentMaxSize
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

	HLSSegmentCount    int
	HLSSegmentDuration time.Duration
	HLSPartDuration    time.Duration
	HLSSegmentMaxSize  uint64
}

// Errors.
var (
	ErrEmptyPathName  = errors.New("path name can not be empty")
	ErrEmptyMonitorID = errors.New("MonitorID can not be empty")
	ErrInvalidURL     = errors.New("invalid URL")
	ErrInvalidSource  = errors.New("invalid source")
)

const (
	defaultHLSSegmentCount    = 3
	defaultHLSSegmentDuration = 900 * time.Millisecond
	defaultHLSPartDuration    = 300 * time.Millisecond
)

var mb = uint64(1000000)

var defaultHLSsegmentMaxSize = 50 * mb

// ErrPathInvalidName invalid path name.
var ErrPathInvalidName = errors.New("invalid path name")

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
		return fmt.Errorf("%w: %s (%v)", ErrPathInvalidName, name, err)
	}

	if pconf.HLSSegmentCount == 0 {
		pconf.HLSSegmentCount = defaultHLSSegmentCount
	}
	if pconf.HLSSegmentDuration == 0 {
		pconf.HLSSegmentDuration = defaultHLSSegmentDuration
	}
	if pconf.HLSPartDuration == 0 {
		pconf.HLSPartDuration = defaultHLSPartDuration
	}
	if pconf.HLSSegmentMaxSize == 0 {
		pconf.HLSSegmentMaxSize = defaultHLSsegmentMaxSize
	}

	return nil
}
