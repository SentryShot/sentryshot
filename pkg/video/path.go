package video

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/base"
	"regexp"
	"sync"
	"time"
)

// ErrPathNoOnePublishing No one is publishing to path.
var ErrPathNoOnePublishing = errors.New("no one is publishing to path")

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
	url      *base.URL
	res      chan pathDescribeRes
}

type pathReaderSetupPlayRes struct {
	path   *path
	stream *stream
	err    error
}

type pathReaderSetupPlayReq struct {
	author   reader
	pathName string
	res      chan pathReaderSetupPlayRes
}

type pathPublisherAnnounceRes struct {
	path *path
	err  error
}

type pathPublisherAnnounceReq struct {
	author   *rtspSession
	pathName string
	res      chan pathPublisherAnnounceRes
}

type pathReaderPlayReq struct {
	author reader
	res    chan struct{}
}

type pathPublisherRecordRes struct {
	stream *stream
	err    error
}

type pathPublisherRecordReq struct {
	author *rtspSession
	tracks gortsplib.Tracks
	res    chan pathPublisherRecordRes
}

type pathReaderPauseReq struct {
	author reader
	res    chan struct{}
}

type pathPublisherPauseReq struct {
	author *rtspSession
	res    chan struct{}
}

type path struct {
	rtspAddress     string
	readTimeout     time.Duration
	writeTimeout    time.Duration
	readBufferCount int
	readBufferSize  int
	confName        string
	conf            *PathConf
	name            string
	wg              *sync.WaitGroup
	parent          *pathManager
	logger          *log.Logger

	ctx               context.Context
	ctxCancel         func()
	source            closer
	sourceReady       bool
	readers           map[reader]pathReaderState
	describeRequests  []pathDescribeReq
	setupPlayRequests []pathReaderSetupPlayReq
	stream            *stream

	// in
	describe          chan pathDescribeReq
	publisherRemove   chan pathPublisherRemoveReq
	publisherAnnounce chan pathPublisherAnnounceReq
	publisherRecord   chan pathPublisherRecordReq
	publisherPause    chan pathPublisherPauseReq
	readerRemove      chan pathReaderRemoveReq
	readerSetupPlay   chan pathReaderSetupPlayReq
	readerPlay        chan pathReaderPlayReq
	readerPause       chan pathReaderPauseReq
}

func newPath(
	parentCtx context.Context,
	rtspAddress string,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	readBufferSize int,
	confName string,
	conf *PathConf,
	name string,
	wg *sync.WaitGroup,
	parent *pathManager,
	logger *log.Logger) *path {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	pa := &path{
		rtspAddress:       rtspAddress,
		readTimeout:       readTimeout,
		writeTimeout:      writeTimeout,
		readBufferCount:   readBufferCount,
		readBufferSize:    readBufferSize,
		confName:          confName,
		conf:              conf,
		name:              name,
		wg:                wg,
		parent:            parent,
		logger:            logger,
		ctx:               ctx,
		ctxCancel:         ctxCancel,
		readers:           make(map[reader]pathReaderState),
		describe:          make(chan pathDescribeReq),
		publisherRemove:   make(chan pathPublisherRemoveReq),
		publisherAnnounce: make(chan pathPublisherAnnounceReq),
		publisherRecord:   make(chan pathPublisherRecordReq),
		publisherPause:    make(chan pathPublisherPauseReq),
		readerRemove:      make(chan pathReaderRemoveReq),
		readerSetupPlay:   make(chan pathReaderSetupPlayReq),
		readerPlay:        make(chan pathReaderPlayReq),
		readerPause:       make(chan pathReaderPauseReq),
	}

	pa.logf(log.LevelDebug, "opened")

	pa.wg.Add(1)
	go pa.run()

	return pa
}

func (pa *path) close() {
	pa.ctxCancel()
}

// Log is the main logging function.
func (pa *path) logf(level log.Level, format string, args ...interface{}) {
	sendLog(pa.logger, *pa.conf, level, fmt.Sprintf(format, args...))
}

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

	err := pa.runLoop()
	pa.ctxCancel()

	for _, req := range pa.describeRequests {
		req.res <- pathDescribeRes{err: ErrTerminated}
	}

	for _, req := range pa.setupPlayRequests {
		req.res <- pathReaderSetupPlayRes{err: ErrTerminated}
	}

	for rp := range pa.readers {
		rp.close()
	}

	if pa.stream != nil {
		pa.stream.close()
	}

	if pa.source != nil {
		pa.source.close()
	}

	pa.logf(log.LevelDebug, "closed (%v)", err)

	pa.parent.onPathClose(pa)
}

func (pa *path) runLoop() error {
	for {
		select {
		case req := <-pa.describe:
			pa.handleDescribe(req)

			if pa.shouldClose() {
				return ErrNotInUse
			}

		case req := <-pa.publisherRemove:
			pa.handlePublisherRemove(req)

			if pa.shouldClose() {
				return ErrNotInUse
			}

		case req := <-pa.publisherAnnounce:
			pa.handlePublisherAnnounce(req)

		case req := <-pa.publisherRecord:
			pa.handlePublisherRecord(req)

		case req := <-pa.publisherPause:
			pa.handlePublisherPause(req)

			if pa.shouldClose() {
				return ErrNotInUse
			}

		case req := <-pa.readerRemove:
			pa.handleReaderRemove(req)

		case req := <-pa.readerSetupPlay:
			pa.handleReaderSetupPlay(req)

			if pa.shouldClose() {
				return ErrNotInUse
			}

		case req := <-pa.readerPlay:
			pa.handleReaderPlay(req)

		case req := <-pa.readerPause:
			pa.handleReaderPause(req)

		case <-pa.ctx.Done():
			return ErrTerminated
		}
	}
}

func (pa *path) shouldClose() bool {
	return pa.source == nil &&
		len(pa.readers) == 0 &&
		len(pa.describeRequests) == 0 &&
		len(pa.setupPlayRequests) == 0
}

func (pa *path) sourceSetReady(tracks gortsplib.Tracks) {
	pa.sourceReady = true
	pa.stream = newStream(tracks)
}

func (pa *path) sourceSetNotReady() {
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

func (pa *path) handleDescribe(req pathDescribeReq) {
	if pa.sourceReady {
		req.res <- pathDescribeRes{
			stream: pa.stream,
		}
		return
	}

	req.res <- pathDescribeRes{err: fmt.Errorf("%w: (%s)", ErrPathNoOnePublishing, pa.name)}
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

func (pa *path) handlePublisherAnnounce(req pathPublisherAnnounceReq) {
	if pa.source != nil {
		if pa.conf.DisablePublisherOverride {
			req.res <- pathPublisherAnnounceRes{err: ErrPathBusy}
			return
		}

		pa.logf(log.LevelInfo, "closing existing publisher")
		pa.source.close()
		pa.doPublisherRemove()
	}

	pa.source = req.author

	req.res <- pathPublisherAnnounceRes{path: pa}
}

// ErrPublisherNotAssigned .
var ErrPublisherNotAssigned = errors.New("publisher is not assigned to this path anymore")

func (pa *path) handlePublisherRecord(req pathPublisherRecordReq) {
	if pa.source != req.author {
		req.res <- pathPublisherRecordRes{err: ErrPublisherNotAssigned}
		return
	}

	req.author.onPublisherAccepted(len(req.tracks))

	pa.sourceSetReady(req.tracks)

	req.res <- pathPublisherRecordRes{stream: pa.stream}
}

func (pa *path) handlePublisherPause(req pathPublisherPauseReq) {
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

func (pa *path) handleReaderSetupPlay(req pathReaderSetupPlayReq) {
	if pa.sourceReady {
		pa.handleReaderSetupPlayPost(req)
		return
	}

	req.res <- pathReaderSetupPlayRes{err: fmt.Errorf("%w: (%s)", ErrPathNoOnePublishing, pa.name)}
}

func (pa *path) handleReaderSetupPlayPost(req pathReaderSetupPlayReq) {
	pa.readers[req.author] = pathReaderStatePrePlay

	req.res <- pathReaderSetupPlayRes{
		path:   pa,
		stream: pa.stream,
	}
}

func (pa *path) handleReaderPlay(req pathReaderPlayReq) {
	pa.readers[req.author] = pathReaderStatePlay

	pa.stream.readerAdd(req.author)

	req.author.onReaderAccepted()

	close(req.res)
}

func (pa *path) handleReaderPause(req pathReaderPauseReq) {
	if state, ok := pa.readers[req.author]; ok && state == pathReaderStatePlay {
		pa.readers[req.author] = pathReaderStatePrePlay
		pa.stream.readerRemove(req.author)
	}
	close(req.res)
}

// onDescribe is called by a reader or publisher through pathManager.
func (pa *path) onDescribe(req pathDescribeReq) pathDescribeRes {
	select {
	case pa.describe <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathDescribeRes{err: ErrTerminated}
	}
}

// onPublisherRemove is called by a publisher.
func (pa *path) onPublisherRemove(req pathPublisherRemoveReq) {
	req.res = make(chan struct{})
	select {
	case pa.publisherRemove <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// onPublisherAnnounce is called by a publisher through pathManager.
func (pa *path) onPublisherAnnounce(req pathPublisherAnnounceReq) pathPublisherAnnounceRes {
	select {
	case pa.publisherAnnounce <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathPublisherAnnounceRes{err: ErrTerminated}
	}
}

// onPublisherRecord is called by a publisher.
func (pa *path) onPublisherRecord(req pathPublisherRecordReq) pathPublisherRecordRes {
	req.res = make(chan pathPublisherRecordRes)
	select {
	case pa.publisherRecord <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathPublisherRecordRes{err: ErrTerminated}
	}
}

// onPublisherPause is called by a publisher.
func (pa *path) onPublisherPause(req pathPublisherPauseReq) {
	req.res = make(chan struct{})
	select {
	case pa.publisherPause <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// onReaderRemove is called by a reader.
func (pa *path) onReaderRemove(req pathReaderRemoveReq) {
	req.res = make(chan struct{})
	select {
	case pa.readerRemove <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// onReaderSetupPlay is called by a reader through pathManager.
func (pa *path) onReaderSetupPlay(req pathReaderSetupPlayReq) pathReaderSetupPlayRes {
	select {
	case pa.readerSetupPlay <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathReaderSetupPlayRes{err: ErrTerminated}
	}
}

// onReaderPlay is called by a reader.
func (pa *path) onReaderPlay(req pathReaderPlayReq) {
	req.res = make(chan struct{})
	select {
	case pa.readerPlay <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// onReaderPause is called by a reader.
func (pa *path) onReaderPause(req pathReaderPauseReq) {
	req.res = make(chan struct{})
	select {
	case pa.readerPause <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
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
		return fmt.Errorf("%w: %s (%v)", ErrPathInvalidName, name, err)
	}

	return nil
}

// Equal checks whether two PathConfs are equal.
func (pconf *PathConf) Equal(other *PathConf) bool {
	a, _ := json.Marshal(pconf)
	b, _ := json.Marshal(other)
	return string(a) == string(b)
}
