package video

import (
	"context"
	"errors"
	"fmt"
	"nvr/pkg/log"
	"sync"
	"time"
)

type pathManagerHLSServer interface {
	onPathSourceReady(pa *path)
}

type pathManager struct {
	rtspAddress     string
	readTimeout     time.Duration
	writeTimeout    time.Duration
	readBufferCount int
	pathConfs       map[string]*PathConf
	log             *log.Logger

	ctx       context.Context
	paths     map[string]*path
	wg        *sync.WaitGroup
	hlsServer pathManagerHLSServer

	// in
	onAddPath         chan addPathReq
	onRemovePath      chan string
	onPathExist       chan pathExistReq
	pathClose         chan *path
	pathSourceReady   chan *path
	describe          chan pathDescribeReq
	readerSetupPlay   chan pathReaderSetupPlayReq
	publisherAnnounce chan pathPublisherAnnounceReq
}

func newPathManager(
	wg *sync.WaitGroup,
	rtspAddress string,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	log *log.Logger,
) *pathManager {
	pm := &pathManager{
		wg:                wg,
		log:               log,
		rtspAddress:       rtspAddress,
		readTimeout:       readTimeout,
		writeTimeout:      writeTimeout,
		readBufferCount:   readBufferCount,
		pathConfs:         make(map[string]*PathConf),
		paths:             make(map[string]*path),
		onAddPath:         make(chan addPathReq),
		onRemovePath:      make(chan string),
		onPathExist:       make(chan pathExistReq),
		pathClose:         make(chan *path),
		pathSourceReady:   make(chan *path),
		describe:          make(chan pathDescribeReq),
		readerSetupPlay:   make(chan pathReaderSetupPlayReq),
		publisherAnnounce: make(chan pathPublisherAnnounceReq),
	}

	return pm
}

func (pm *pathManager) start(ctx context.Context) {
	pm.ctx = ctx

	go pm.run()
}

// ErrPathExist Path exist.
var ErrPathExist = errors.New("path exist")

func (pm *pathManager) run() { //nolint:funlen,gocognit
	for {
		select {
		case req := <-pm.onAddPath:
			newPathConfs := pm.pathConfs
			if _, exist := newPathConfs[req.name]; exist {
				req.ret <- ErrPathExist
				continue
			}
			newPathConfs[req.name] = req.config

			// add confs
			for pathConfName, pathConf := range newPathConfs {
				if _, ok := pm.pathConfs[pathConfName]; !ok {
					pm.pathConfs[pathConfName] = pathConf
				}
			}

			// add new paths
			for pathConfName, pathConf := range pm.pathConfs {
				if _, ok := pm.paths[pathConfName]; !ok {
					pm.createPath(pathConfName, pathConf, pathConfName)
				}
			}
			req.ret <- nil

		case name := <-pm.onRemovePath:
			pconf, exist := pm.pathConfs[name]
			if !exist {
				continue
			}
			pconf.cancel()

			// remove confs
			delete(pm.pathConfs, name)

			// remove paths associated with a conf which doesn't exist anymore
			for _, path := range pm.paths {
				if _, ok := pm.pathConfs[path.ConfName()]; !ok {
					delete(pm.paths, path.Name())
					path.close()
				}
			}

		case req := <-pm.onPathExist:
			_, exist := pm.pathConfs[req.name]
			req.ret <- exist

		case pa := <-pm.pathClose:
			if pmpa, ok := pm.paths[pa.Name()]; !ok || pmpa != pa {
				continue
			}
			delete(pm.paths, pa.Name())
			pa.close()

		case pa := <-pm.pathSourceReady:
			if pm.hlsServer != nil {
				pm.hlsServer.onPathSourceReady(pa)
			}

		case req := <-pm.describe:
			pathConfName, pathConf, err := pm.findPathConf(req.pathName)
			if err != nil {
				req.res <- pathDescribeRes{err: err}
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.pathName]; !ok {
				pm.createPath(pathConfName, pathConf, req.pathName)
			}

			req.res <- pathDescribeRes{path: pm.paths[req.pathName]}

		case req := <-pm.readerSetupPlay:
			pathConfName, pathConf, err := pm.findPathConf(req.pathName)
			if err != nil {
				req.res <- pathReaderSetupPlayRes{err: err}
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.pathName]; !ok {
				pm.createPath(pathConfName, pathConf, req.pathName)
			}

			req.res <- pathReaderSetupPlayRes{path: pm.paths[req.pathName]}

		case req := <-pm.publisherAnnounce:
			pathConfName, pathConf, err := pm.findPathConf(req.pathName)
			if err != nil {
				req.res <- pathPublisherAnnounceRes{err: err}
				continue
			}
			// create path if it doesn't exist
			if _, ok := pm.paths[req.pathName]; !ok {
				pm.createPath(pathConfName, pathConf, req.pathName)
			}

			req.res <- pathPublisherAnnounceRes{path: pm.paths[req.pathName]}

		case <-pm.ctx.Done():
			return
		}
	}
}

func (pm *pathManager) createPath(
	pathConfName string,
	pathConf *PathConf,
	name string,
) {
	pm.paths[name] = newPath(
		pm.ctx,
		pm.rtspAddress,
		pm.readTimeout,
		pm.writeTimeout,
		pm.readBufferCount,
		pathConfName,
		pathConf,
		name,
		pm.wg,
		pm,
		pm.log,
	)
}

// Errors.
var (
	ErrPathInvalidName   = errors.New("invalid path name")
	ErrPathNotConfigured = errors.New("path is not configured")
)

func (pm *pathManager) findPathConf(name string) (string, *PathConf, error) {
	err := isValidPathName(name)
	if err != nil {
		return "", nil, fmt.Errorf("%w: (%s) %v", ErrPathInvalidName, name, err)
	}

	if pathConf, exist := pm.pathConfs[name]; exist {
		return name, pathConf, nil
	}

	return "", nil, fmt.Errorf("%w: (%s)", ErrPathNotConfigured, name)
}

type addPathReq struct {
	name   string
	config *PathConf
	ret    chan error
}

// AddPath add path to pathManager.
func (pm *pathManager) AddPath(name string, newConf *PathConf) error {
	err := newConf.CheckAndFillMissing(name)
	if err != nil {
		return err
	}

	newConf.start(pm.ctx)
	go func() {
		<-pm.ctx.Done()
		close(newConf.onNewHLSsegment)
	}()

	ret := make(chan error)
	defer close(ret)

	req := addPathReq{
		name:   name,
		config: newConf,
		ret:    ret,
	}
	select {
	case <-pm.ctx.Done():
		return context.Canceled
	case pm.onAddPath <- req:
		return <-ret
	}
}

// RemovePath remove path from pathManager.
func (pm *pathManager) RemovePath(name string) {
	select {
	case <-pm.ctx.Done():
	case pm.onRemovePath <- name:
	}
}

type pathExistReq struct {
	name string
	ret  chan bool
}

func (pm *pathManager) pathExist(name string) bool {
	ret := make(chan bool)
	defer close(ret)

	req := pathExistReq{
		name: name,
		ret:  ret,
	}

	select {
	case <-pm.ctx.Done():
		return false
	case pm.onPathExist <- req:
		return <-ret
	}
}

// onPathSourceReady is called by path.
func (pm *pathManager) onPathSourceReady(pa *path) {
	select {
	case pm.pathSourceReady <- pa:
	case <-pm.ctx.Done():
	}
}

// onPathClose is called by path.
func (pm *pathManager) onPathClose(pa *path) {
	select {
	case pm.pathClose <- pa:
	case <-pm.ctx.Done():
	}
}

// onDescribe is called by a reader or publisher.
func (pm *pathManager) onDescribe(req pathDescribeReq) pathDescribeRes {
	req.res = make(chan pathDescribeRes)
	select {
	case pm.describe <- req:
		res := <-req.res
		if res.err != nil {
			return res
		}

		return res.path.onDescribe(req)

	case <-pm.ctx.Done():
		return pathDescribeRes{err: ErrTerminated}
	}
}

// onPublisherAnnounce is called by a publisher.
func (pm *pathManager) onPublisherAnnounce(req pathPublisherAnnounceReq) pathPublisherAnnounceRes {
	req.res = make(chan pathPublisherAnnounceRes)
	select {
	case pm.publisherAnnounce <- req:
		res := <-req.res
		if res.err != nil {
			return res
		}

		return res.path.onPublisherAnnounce(req)

	case <-pm.ctx.Done():
		return pathPublisherAnnounceRes{err: ErrTerminated}
	}
}

// onReaderSetupPlay is called by a reader.
func (pm *pathManager) onReaderSetupPlay(req pathReaderSetupPlayReq) pathReaderSetupPlayRes {
	req.res = make(chan pathReaderSetupPlayRes)
	select {
	case pm.readerSetupPlay <- req:
		res := <-req.res
		if res.err != nil {
			return res
		}

		return res.path.onReaderSetupPlay(req)

	case <-pm.ctx.Done():
		return pathReaderSetupPlayRes{err: ErrTerminated}
	}
}

// onHLSServerSet is called by hlsServer before pathManager is started.
func (pm *pathManager) onHLSServerSet(s pathManagerHLSServer) {
	pm.hlsServer = s
}
