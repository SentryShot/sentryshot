package video

import (
	"context"
	"errors"
	"fmt"
	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/url"
	"regexp"
	"sync"
	"time"
)

type pathParent interface {
	pathSourceReady(*path)
	pathSourceNotReady(*path)
	pathClose(*path)
}

type pathReaderState int

const (
	pathReaderStatePrePlay pathReaderState = iota
	pathReaderStatePlay
)

type pathReaderRemoveReq struct {
	author reader
	res    chan struct{}
}

type pathPublisherRemoveReq struct {
	author *rtspSession
	res    chan struct{}
}

type pathDescribeRes struct {
	path   *path
	stream *stream
	err    error
}

type pathDescribeReq struct {
	pathName string
	url      *url.URL
	res      chan pathDescribeRes
}

type pathReaderAddReq struct {
	author   reader
	pathName string
	res      chan pathReaderAddRes
}

type pathReaderAddRes struct {
	path   *path
	stream *stream
	err    error
}

type pathPublisherAddReq struct {
	author   *rtspSession
	pathName string
	res      chan pathPublisherAddRes
}

type pathPublisherAddRes struct {
	path *path
	err  error
}

type pathReaderStartReq struct {
	author reader
	res    chan struct{}
}

type pathPublisherStartReq struct {
	author *rtspSession
	tracks gortsplib.Tracks
	res    chan pathPublisherStartRes
}

type pathPublisherStartRes struct {
	stream *stream
	err    error
}

type pathReaderStopReq struct {
	author reader
	res    chan struct{}
}

type pathPublisherStopReq struct {
	author *rtspSession
	res    chan struct{}
}

type path struct {
	confName string
	conf     *PathConf
	name     string
	wg       *sync.WaitGroup
	parent   pathParent
	logger   *log.Logger

	ctx                     context.Context
	ctxCancel               func()
	source                  closer
	sourceReady             bool
	stream                  *stream
	readers                 map[reader]pathReaderState
	describeRequestsOnHold  []pathDescribeReq
	readerAddRequestsOnHold []pathReaderAddReq

	// in
	chDescribe        chan pathDescribeReq
	chPublisherRemove chan pathPublisherRemoveReq
	chPublisherAdd    chan pathPublisherAddReq
	chPublisherStart  chan pathPublisherStartReq
	chPublisherStop   chan pathPublisherStopReq
	chReaderRemove    chan pathReaderRemoveReq
	chReaderAdd       chan pathReaderAddReq
	chReaderStart     chan pathReaderStartReq
	chReaderStop      chan pathReaderStopReq
}

func newPath(
	parentCtx context.Context,
	confName string,
	conf *PathConf,
	name string,
	wg *sync.WaitGroup,
	parent *pathManager,
	logger *log.Logger,
) *path {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	pa := &path{
		confName:          confName,
		conf:              conf,
		name:              name,
		wg:                wg,
		parent:            parent,
		logger:            logger,
		ctx:               ctx,
		ctxCancel:         ctxCancel,
		readers:           make(map[reader]pathReaderState),
		chDescribe:        make(chan pathDescribeReq),
		chPublisherRemove: make(chan pathPublisherRemoveReq),
		chPublisherAdd:    make(chan pathPublisherAddReq),
		chPublisherStart:  make(chan pathPublisherStartReq),
		chPublisherStop:   make(chan pathPublisherStopReq),
		chReaderRemove:    make(chan pathReaderRemoveReq),
		chReaderAdd:       make(chan pathReaderAddReq),
		chReaderStart:     make(chan pathReaderStartReq),
		chReaderStop:      make(chan pathReaderStopReq),
	}

	// pa.logf(log.LevelDebug, "created")

	pa.wg.Add(1)
	go pa.run()

	return pa
}

func (pa *path) close() {
	pa.ctxCancel()
}

// Log is the main logging function.
/*func (pa *path) logf(level log.Level, format string, args ...interface{}) {
	sendLog(pa.logger, *pa.conf, level, "PATH:", fmt.Sprintf(format, args...))
}*/

// ConfName returns the configuration name of this path.
func (pa *path) ConfName() string {
	return pa.confName
}

// Conf returns the configuration of this path.
func (pa *path) Conf() *PathConf {
	return pa.conf
}

// Name returns the name of this path.
func (pa *path) Name() string {
	return pa.name
}

// Errors.
var (
	ErrTerminated = errors.New("terminated")
	ErrNotInUse   = errors.New("not in use")
)

func (pa *path) run() {
	defer pa.wg.Done()

	_ = pa.runLoop()
	pa.ctxCancel()

	for _, req := range pa.describeRequestsOnHold {
		req.res <- pathDescribeRes{err: ErrTerminated}
	}

	for _, req := range pa.readerAddRequestsOnHold {
		req.res <- pathReaderAddRes{err: ErrTerminated}
	}

	for rp := range pa.readers {
		rp.close()
	}

	if pa.stream != nil {
		pa.stream.close()
	}

	if pa.sourceReady {
		pa.sourceSetNotReady()
	}

	if pa.source != nil {
		pa.source.close()
	}

	// pa.logf(log.LevelDebug, "destroyed (%v)", err)

	pa.parent.pathClose(pa)
}

func (pa *path) runLoop() error {
	for {
		select {
		case req := <-pa.chDescribe:
			pa.handleDescribe(req)

			if pa.shouldClose() {
				return ErrNotInUse
			}

		case req := <-pa.chPublisherRemove:
			pa.handlePublisherRemove(req)

			if pa.shouldClose() {
				return ErrNotInUse
			}

		case req := <-pa.chPublisherAdd:
			pa.handlePublisherAnnounce(req)

		case req := <-pa.chPublisherStart:
			pa.handlePublisherRecord(req)

		case req := <-pa.chPublisherStop:
			pa.handlePublisherPause(req)

			if pa.shouldClose() {
				return ErrNotInUse
			}

		case req := <-pa.chReaderRemove:
			pa.handleReaderRemove(req)

		case req := <-pa.chReaderAdd:
			pa.handleReaderSetupPlay(req)

			if pa.shouldClose() {
				return ErrNotInUse
			}

		case req := <-pa.chReaderStart:
			pa.handleReaderPlay(req)

		case req := <-pa.chReaderStop:
			pa.handleReaderPause(req)

		case <-pa.ctx.Done():
			return ErrTerminated
		}
	}
}

func (pa *path) shouldClose() bool {
	return pa.source == nil &&
		len(pa.readers) == 0 &&
		len(pa.describeRequestsOnHold) == 0 &&
		len(pa.readerAddRequestsOnHold) == 0
}

func (pa *path) sourceSetReady(tracks gortsplib.Tracks) {
	pa.sourceReady = true
	pa.stream = newStream(tracks)
	pa.parent.pathSourceReady(pa)
}

func (pa *path) sourceSetNotReady() {
	pa.parent.pathSourceNotReady(pa)

	for r := range pa.readers {
		pa.doReaderRemove(r)
		r.close()
	}

	pa.sourceReady = false
	pa.stream.close()
	pa.stream = nil
}

func (pa *path) doReaderRemove(r reader) {
	state := pa.readers[r]

	if state == pathReaderStatePlay {
		pa.stream.readerRemove(r)
	}

	delete(pa.readers, r)
}

func (pa *path) doPublisherRemove() {
	if pa.sourceReady {
		pa.sourceSetNotReady()
	} else {
		for r := range pa.readers {
			pa.doReaderRemove(r)
			r.close()
		}
	}

	pa.source = nil
}

// ErrPathNoOnePublishing No one is publishing to path.
var ErrPathNoOnePublishing = errors.New("no one is publishing to path")

func (pa *path) handleDescribe(req pathDescribeReq) {
	if pa.sourceReady {
		req.res <- pathDescribeRes{
			stream: pa.stream,
		}
		return
	}

	req.res <- pathDescribeRes{err: fmt.Errorf("%w: (%s)",
		ErrPathNoOnePublishing, pa.name)}
}

func (pa *path) handlePublisherRemove(req pathPublisherRemoveReq) {
	if pa.source == req.author {
		pa.doPublisherRemove()
	}
	close(req.res)
}

// Errors.
var (
	ErrPathAlreadyAssigned = errors.New("path is assigned to a static source")
	ErrPathBusy            = errors.New("another publisher is already publishing to path")
)

func (pa *path) handlePublisherAnnounce(req pathPublisherAddReq) {
	if pa.source != nil {
		if pa.conf.DisablePublisherOverride {
			req.res <- pathPublisherAddRes{err: ErrPathBusy}
			return
		}

		// pa.logf(log.LevelInfo, "closing existing publisher")
		pa.source.close()
		pa.doPublisherRemove()
	}

	pa.source = req.author

	req.res <- pathPublisherAddRes{path: pa}
}

// ErrPublisherNotAssigned .
var ErrPublisherNotAssigned = errors.New("publisher is not assigned to this path anymore")

func (pa *path) handlePublisherRecord(req pathPublisherStartReq) {
	if pa.source != req.author {
		req.res <- pathPublisherStartRes{err: ErrPublisherNotAssigned}
		return
	}

	req.author.publisherAccepted(len(req.tracks))

	pa.sourceSetReady(req.tracks)

	req.res <- pathPublisherStartRes{stream: pa.stream}
}

func (pa *path) handlePublisherPause(req pathPublisherStopReq) {
	if req.author == pa.source && pa.sourceReady {
		pa.sourceSetNotReady()
	}
	close(req.res)
}

func (pa *path) handleReaderRemove(req pathReaderRemoveReq) {
	if _, ok := pa.readers[req.author]; ok {
		pa.doReaderRemove(req.author)
	}
	close(req.res)
}

func (pa *path) handleReaderSetupPlay(req pathReaderAddReq) {
	if pa.sourceReady {
		pa.handleReaderSetupPlayPost(req)
		return
	}

	req.res <- pathReaderAddRes{err: fmt.Errorf("%w: (%s)", ErrPathNoOnePublishing, pa.name)}
}

func (pa *path) handleReaderSetupPlayPost(req pathReaderAddReq) {
	pa.readers[req.author] = pathReaderStatePrePlay

	req.res <- pathReaderAddRes{
		path:   pa,
		stream: pa.stream,
	}
}

func (pa *path) handleReaderPlay(req pathReaderStartReq) {
	pa.readers[req.author] = pathReaderStatePlay

	pa.stream.readerAdd(req.author)

	req.author.readerAccepted()

	close(req.res)
}

func (pa *path) handleReaderPause(req pathReaderStopReq) {
	if state, ok := pa.readers[req.author]; ok && state == pathReaderStatePlay {
		pa.readers[req.author] = pathReaderStatePrePlay
		pa.stream.readerRemove(req.author)
	}
	close(req.res)
}

// describe is called by a reader or publisher through pathManager.
func (pa *path) describe(req pathDescribeReq) pathDescribeRes {
	select {
	case pa.chDescribe <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathDescribeRes{err: ErrTerminated}
	}
}

// publisherRemove is called by a publisher.
func (pa *path) publisherRemove(req pathPublisherRemoveReq) {
	req.res = make(chan struct{})
	select {
	case pa.chPublisherRemove <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// publisherAdd is called by a publisher through pathManager.
func (pa *path) publisherAdd(req pathPublisherAddReq) pathPublisherAddRes {
	select {
	case pa.chPublisherAdd <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathPublisherAddRes{err: ErrTerminated}
	}
}

// publisherStart is called by a publisher.
func (pa *path) publisherStart(req pathPublisherStartReq) pathPublisherStartRes {
	req.res = make(chan pathPublisherStartRes)
	select {
	case pa.chPublisherStart <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathPublisherStartRes{err: ErrTerminated}
	}
}

// publisherStop is called by a publisher.
func (pa *path) publisherStop(req pathPublisherStopReq) {
	req.res = make(chan struct{})
	select {
	case pa.chPublisherStop <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// readerRemove is called by a reader.
func (pa *path) readerRemove(req pathReaderRemoveReq) {
	req.res = make(chan struct{})
	select {
	case pa.chReaderRemove <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// readerAdd is called by a reader through pathManager.
func (pa *path) readerAdd(req pathReaderAddReq) pathReaderAddRes {
	select {
	case pa.chReaderAdd <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathReaderAddRes{err: ErrTerminated}
	}
}

// readerStart is called by a reader.
func (pa *path) readerStart(req pathReaderStartReq) {
	req.res = make(chan struct{})
	select {
	case pa.chReaderStart <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// readerStop is called by a reader.
func (pa *path) readerStop(req pathReaderStopReq) {
	req.res = make(chan struct{})
	select {
	case pa.chReaderStop <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
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

	DisablePublisherOverride bool

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
