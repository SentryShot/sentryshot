package rtsp

import (
	"context"
	"errors"
	"fmt"
	"nvr/pkg/log"
	"sync"
	"time"
)

type pathManager struct {
	rtspAddress     string
	readTimeout     time.Duration
	writeTimeout    time.Duration
	readBufferCount int
	readBufferSize  int
	pathConfs       map[string]*PathConf
	log             *log.Logger

	ctx   context.Context
	paths map[string]*path
	wg    *sync.WaitGroup

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
	readBufferSize int,
	log *log.Logger) *pathManager {
	pm := &pathManager{
		wg:                wg,
		log:               log,
		rtspAddress:       rtspAddress,
		readTimeout:       readTimeout,
		writeTimeout:      writeTimeout,
		readBufferCount:   readBufferCount,
		readBufferSize:    readBufferSize,
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
			newPathConfs[req.name] = &req.config

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
			if _, exist := pm.pathConfs[name]; !exist {
				continue
			}

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

		case req := <-pm.describe:
			pathConfName, pathConf, err := pm.findPathConf(req.PathName)
			if err != nil {
				req.Res <- pathDescribeRes{Err: err}
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.PathName]; !ok {
				pm.createPath(pathConfName, pathConf, req.PathName)
			}

			req.Res <- pathDescribeRes{Path: pm.paths[req.PathName]}

		case req := <-pm.readerSetupPlay:
			pathConfName, pathConf, err := pm.findPathConf(req.PathName)
			if err != nil {
				req.Res <- pathReaderSetupPlayRes{Err: err}
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.PathName]; !ok {
				pm.createPath(pathConfName, pathConf, req.PathName)
			}

			req.Res <- pathReaderSetupPlayRes{Path: pm.paths[req.PathName]}

		case req := <-pm.publisherAnnounce:
			pathConfName, pathConf, err := pm.findPathConf(req.PathName)
			if err != nil {
				req.Res <- pathPublisherAnnounceRes{Err: err}
				continue
			}
			// create path if it doesn't exist
			if _, ok := pm.paths[req.PathName]; !ok {
				pm.createPath(pathConfName, pathConf, req.PathName)
			}

			req.Res <- pathPublisherAnnounceRes{Path: pm.paths[req.PathName]}

		case <-pm.ctx.Done():
			return
		}
	}
}

func (pm *pathManager) createPath(
	pathConfName string,
	pathConf *PathConf,
	name string) {
	pm.paths[name] = newPath(
		pm.ctx,
		pm.rtspAddress,
		pm.readTimeout,
		pm.writeTimeout,
		pm.readBufferCount,
		pm.readBufferSize,
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
	config PathConf
	ret    chan error
}

// AddPath add path to pathManager.
func (pm *pathManager) AddPath(name string, newConf PathConf) error {
	err := newConf.CheckAndFillMissing(name)
	if err != nil {
		return err
	}

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

// onPathClose is called by path.
func (pm *pathManager) onPathClose(pa *path) {
	select {
	case pm.pathClose <- pa:
	case <-pm.ctx.Done():
	}
}

// onDescribe is called by a reader or publisher.
func (pm *pathManager) onDescribe(req pathDescribeReq) pathDescribeRes {
	req.Res = make(chan pathDescribeRes)
	select {
	case pm.describe <- req:
		res := <-req.Res
		if res.Err != nil {
			return res
		}

		return res.Path.onDescribe(req)

	case <-pm.ctx.Done():
		return pathDescribeRes{Err: ErrTerminated}
	}
}

// onPublisherAnnounce is called by a publisher.
func (pm *pathManager) onPublisherAnnounce(req pathPublisherAnnounceReq) pathPublisherAnnounceRes {
	req.Res = make(chan pathPublisherAnnounceRes)
	select {
	case pm.publisherAnnounce <- req:
		res := <-req.Res
		if res.Err != nil {
			return res
		}

		return res.Path.onPublisherAnnounce(req)

	case <-pm.ctx.Done():
		return pathPublisherAnnounceRes{Err: ErrTerminated}
	}
}

// onReaderSetupPlay is called by a reader.
func (pm *pathManager) onReaderSetupPlay(req pathReaderSetupPlayReq) pathReaderSetupPlayRes {
	req.Res = make(chan pathReaderSetupPlayRes)
	select {
	case pm.readerSetupPlay <- req:
		res := <-req.Res
		if res.Err != nil {
			return res
		}

		return res.Path.onReaderSetupPlay(req)

	case <-pm.ctx.Done():
		return pathReaderSetupPlayRes{Err: ErrTerminated}
	}
}
