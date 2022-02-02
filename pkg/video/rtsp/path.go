package rtsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"nvr/pkg/log"
	"nvr/pkg/video/rtsp/gortsplib"
	"nvr/pkg/video/rtsp/gortsplib/pkg/base"
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
	Author reader
	Res    chan struct{}
}

type pathPublisherRemoveReq struct {
	Author *rtspSession
	Res    chan struct{}
}

type pathDescribeRes struct {
	Path   *path
	Stream *stream
	Err    error
}

type pathDescribeReq struct {
	PathName string
	URL      *base.URL
	Res      chan pathDescribeRes
}

type pathReaderSetupPlayRes struct {
	Path   *path
	Stream *stream
	Err    error
}

type pathReaderSetupPlayReq struct {
	Author   reader
	PathName string
	Res      chan pathReaderSetupPlayRes
}

type pathPublisherAnnounceRes struct {
	Path *path
	Err  error
}

type pathPublisherAnnounceReq struct {
	Author   *rtspSession
	PathName string
	Res      chan pathPublisherAnnounceRes
}

type pathReaderPlayReq struct {
	Author reader
	Res    chan struct{}
}

type pathPublisherRecordRes struct {
	Stream *stream
	Err    error
}

type pathPublisherRecordReq struct {
	Author *rtspSession
	Tracks gortsplib.Tracks
	Res    chan pathPublisherRecordRes
}

type pathReaderPauseReq struct {
	Author reader
	Res    chan struct{}
}

type pathPublisherPauseReq struct {
	Author *rtspSession
	Res    chan struct{}
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
		req.Res <- pathDescribeRes{Err: ErrTerminated}
	}

	for _, req := range pa.setupPlayRequests {
		req.Res <- pathReaderSetupPlayRes{Err: ErrTerminated}
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
		req.Res <- pathDescribeRes{
			Stream: pa.stream,
		}
		return
	}

	req.Res <- pathDescribeRes{Err: fmt.Errorf("%w: (%s)", ErrPathNoOnePublishing, pa.name)}
}

func (pa *path) handlePublisherRemove(req pathPublisherRemoveReq) {
	if pa.source == req.Author {
		pa.doPublisherRemove()
	}
	close(req.Res)
}

// Errors.
var (
	ErrPathAlreadyAssigned = errors.New("path is assigned to a static source")
	ErrPathBusy            = errors.New("another publisher is already publishing to path")
)

func (pa *path) handlePublisherAnnounce(req pathPublisherAnnounceReq) {
	if pa.source != nil {
		if pa.conf.DisablePublisherOverride {
			req.Res <- pathPublisherAnnounceRes{Err: ErrPathBusy}
			return
		}

		pa.logf(log.LevelInfo, "closing existing publisher")
		pa.source.close()
		pa.doPublisherRemove()
	}

	pa.source = req.Author

	req.Res <- pathPublisherAnnounceRes{Path: pa}
}

// ErrPublisherNotAssigned .
var ErrPublisherNotAssigned = errors.New("publisher is not assigned to this path anymore")

func (pa *path) handlePublisherRecord(req pathPublisherRecordReq) {
	if pa.source != req.Author {
		req.Res <- pathPublisherRecordRes{Err: ErrPublisherNotAssigned}
		return
	}

	req.Author.onPublisherAccepted(len(req.Tracks))

	pa.sourceSetReady(req.Tracks)

	req.Res <- pathPublisherRecordRes{Stream: pa.stream}
}

func (pa *path) handlePublisherPause(req pathPublisherPauseReq) {
	if req.Author == pa.source && pa.sourceReady {
		pa.sourceSetNotReady()
	}
	close(req.Res)
}

func (pa *path) handleReaderRemove(req pathReaderRemoveReq) {
	if _, ok := pa.readers[req.Author]; ok {
		pa.doReaderRemove(req.Author)
	}
	close(req.Res)
}

func (pa *path) handleReaderSetupPlay(req pathReaderSetupPlayReq) {
	if pa.sourceReady {
		pa.handleReaderSetupPlayPost(req)
		return
	}

	req.Res <- pathReaderSetupPlayRes{Err: fmt.Errorf("%w: (%s)", ErrPathNoOnePublishing, pa.name)}
}

func (pa *path) handleReaderSetupPlayPost(req pathReaderSetupPlayReq) {
	pa.readers[req.Author] = pathReaderStatePrePlay

	req.Res <- pathReaderSetupPlayRes{
		Path:   pa,
		Stream: pa.stream,
	}
}

func (pa *path) handleReaderPlay(req pathReaderPlayReq) {
	pa.readers[req.Author] = pathReaderStatePlay

	pa.stream.readerAdd(req.Author)

	req.Author.onReaderAccepted()

	close(req.Res)
}

func (pa *path) handleReaderPause(req pathReaderPauseReq) {
	if state, ok := pa.readers[req.Author]; ok && state == pathReaderStatePlay {
		pa.readers[req.Author] = pathReaderStatePrePlay
		pa.stream.readerRemove(req.Author)
	}
	close(req.Res)
}

// onDescribe is called by a reader or publisher through pathManager.
func (pa *path) onDescribe(req pathDescribeReq) pathDescribeRes {
	select {
	case pa.describe <- req:
		return <-req.Res
	case <-pa.ctx.Done():
		return pathDescribeRes{Err: ErrTerminated}
	}
}

// onPublisherRemove is called by a publisher.
func (pa *path) onPublisherRemove(req pathPublisherRemoveReq) {
	req.Res = make(chan struct{})
	select {
	case pa.publisherRemove <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// onPublisherAnnounce is called by a publisher through pathManager.
func (pa *path) onPublisherAnnounce(req pathPublisherAnnounceReq) pathPublisherAnnounceRes {
	select {
	case pa.publisherAnnounce <- req:
		return <-req.Res
	case <-pa.ctx.Done():
		return pathPublisherAnnounceRes{Err: ErrTerminated}
	}
}

// onPublisherRecord is called by a publisher.
func (pa *path) onPublisherRecord(req pathPublisherRecordReq) pathPublisherRecordRes {
	req.Res = make(chan pathPublisherRecordRes)
	select {
	case pa.publisherRecord <- req:
		return <-req.Res
	case <-pa.ctx.Done():
		return pathPublisherRecordRes{Err: ErrTerminated}
	}
}

// onPublisherPause is called by a publisher.
func (pa *path) onPublisherPause(req pathPublisherPauseReq) {
	req.Res = make(chan struct{})
	select {
	case pa.publisherPause <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// onReaderRemove is called by a reader.
func (pa *path) onReaderRemove(req pathReaderRemoveReq) {
	req.Res = make(chan struct{})
	select {
	case pa.readerRemove <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// onReaderSetupPlay is called by a reader through pathManager.
func (pa *path) onReaderSetupPlay(req pathReaderSetupPlayReq) pathReaderSetupPlayRes {
	select {
	case pa.readerSetupPlay <- req:
		return <-req.Res
	case <-pa.ctx.Done():
		return pathReaderSetupPlayRes{Err: ErrTerminated}
	}
}

// onReaderPlay is called by a reader.
func (pa *path) onReaderPlay(req pathReaderPlayReq) {
	req.Res = make(chan struct{})
	select {
	case pa.readerPlay <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// onReaderPause is called by a reader.
func (pa *path) onReaderPause(req pathReaderPauseReq) {
	req.Res = make(chan struct{})
	select {
	case pa.readerPause <- req:
		<-req.Res
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
