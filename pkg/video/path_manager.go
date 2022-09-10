package video

import (
	"context"
	"errors"
	"fmt"
	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/base"
	"nvr/pkg/video/hls"
	"sync"
)

type pathManagerHLSServer interface {
	pathSourceReady(pa *path)
	pathSourceNotReady(pa *path)
	MuxerByPathName(string) (*hls.Muxer, error)
}

type pathManager struct {
	pathConfs map[string]*PathConf
	log       *log.Logger

	ctx       context.Context
	paths     map[string]*path
	wg        *sync.WaitGroup
	hlsServer pathManagerHLSServer

	// in
	chAddPath            chan addPathReq
	chRemovePath         chan string
	chPathExist          chan pathExistReq
	chPathClose          chan *path
	chPathSourceReady    chan *path
	chPathSourceNotReady chan *path
	chDescribe           chan pathDescribeReq
	chReaderAdd          chan pathReaderAddReq
	chPublisherAdd       chan pathPublisherAddReq
}

func newPathManager(wg *sync.WaitGroup, log *log.Logger) *pathManager {
	pm := &pathManager{
		wg:                   wg,
		log:                  log,
		pathConfs:            make(map[string]*PathConf),
		paths:                make(map[string]*path),
		chAddPath:            make(chan addPathReq),
		chRemovePath:         make(chan string),
		chPathExist:          make(chan pathExistReq),
		chPathClose:          make(chan *path),
		chPathSourceReady:    make(chan *path),
		chPathSourceNotReady: make(chan *path),
		chDescribe:           make(chan pathDescribeReq),
		chReaderAdd:          make(chan pathReaderAddReq),
		chPublisherAdd:       make(chan pathPublisherAddReq),
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
		case req := <-pm.chAddPath:
			newPathConfs := pm.pathConfs
			if _, exist := newPathConfs[req.name]; exist {
				req.ret <- addPathRes{err: ErrPathExist}
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

			hlsMuxer := func() (IHLSMuxer, error) {
				return pm.hlsServer.MuxerByPathName(req.name)
			}

			req.ret <- addPathRes{hlsMuxer: hlsMuxer}

		case name := <-pm.chRemovePath:
			// remove confs
			delete(pm.pathConfs, name)

			// remove paths associated with a conf which doesn't exist anymore
			for _, path := range pm.paths {
				if _, ok := pm.pathConfs[path.ConfName()]; !ok {
					delete(pm.paths, path.Name())
					path.close()
				}
			}

		case req := <-pm.chPathExist:
			_, exist := pm.pathConfs[req.name]
			req.ret <- exist

		case pa := <-pm.chPathClose:
			if pmpa, ok := pm.paths[pa.Name()]; !ok || pmpa != pa {
				continue
			}
			delete(pm.paths, pa.Name())
			pa.close()

		case pa := <-pm.chPathSourceReady:
			if pm.hlsServer != nil {
				pm.hlsServer.pathSourceReady(pa)
			}

		case pa := <-pm.chPathSourceNotReady:
			pm.hlsServer.pathSourceNotReady(pa)

		case req := <-pm.chDescribe:
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

		case req := <-pm.chReaderAdd:
			pathConfName, pathConf, err := pm.findPathConf(req.pathName)
			if err != nil {
				req.res <- pathReaderAddRes{err: err}
				continue
			}

			// create path if it doesn't exist
			if _, ok := pm.paths[req.pathName]; !ok {
				pm.createPath(pathConfName, pathConf, req.pathName)
			}

			req.res <- pathReaderAddRes{path: pm.paths[req.pathName]}

		case req := <-pm.chPublisherAdd:
			pathConfName, pathConf, err := pm.findPathConf(req.pathName)
			if err != nil {
				req.res <- pathPublisherAddRes{err: err}
				continue
			}
			// create path if it doesn't exist
			if _, ok := pm.paths[req.pathName]; !ok {
				pm.createPath(pathConfName, pathConf, req.pathName)
			}

			req.res <- pathPublisherAddRes{path: pm.paths[req.pathName]}

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
	ret    chan addPathRes
}

type addPathRes struct {
	hlsMuxer HlsMuxerFunc
	err      error
}

// AddPath add path to pathManager.
func (pm *pathManager) AddPath(name string, newConf PathConf) (HlsMuxerFunc, error) {
	err := newConf.CheckAndFillMissing(name)
	if err != nil {
		return nil, err
	}

	ret := make(chan addPathRes)
	defer close(ret)

	req := addPathReq{
		name:   name,
		config: newConf,
		ret:    ret,
	}
	select {
	case <-pm.ctx.Done():
		return nil, context.Canceled
	case pm.chAddPath <- req:
		res := <-ret
		return res.hlsMuxer, res.err
	}
}

// RemovePath remove path from pathManager.
func (pm *pathManager) RemovePath(name string) {
	select {
	case <-pm.ctx.Done():
	case pm.chRemovePath <- name:
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
	case pm.chPathExist <- req:
		return <-ret
	}
}

// pathSourceReady is called by path.
func (pm *pathManager) pathSourceReady(pa *path) {
	select {
	case pm.chPathSourceReady <- pa:
	case <-pm.ctx.Done():
	}
}

// pathSourceNotReady is called by path.
func (pm *pathManager) pathSourceNotReady(pa *path) {
	select {
	case pm.chPathSourceNotReady <- pa:
	case <-pm.ctx.Done():
	}
}

// pathClose is called by path.
func (pm *pathManager) pathClose(pa *path) {
	select {
	case pm.chPathClose <- pa:
	case <-pm.ctx.Done():
	}
}

// describe is called by a reader or publisher.
func (pm *pathManager) onDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	res := func() pathDescribeRes {
		req := pathDescribeReq{
			pathName: ctx.Path,
			url:      ctx.Request.URL,
			res:      make(chan pathDescribeRes),
		}
		select {
		case pm.chDescribe <- req:
			res := <-req.res
			if res.err != nil {
				return res
			}

			return res.path.describe(req)

		case <-pm.ctx.Done():
			return pathDescribeRes{err: ErrTerminated}
		}
	}()

	if res.err != nil {
		if errors.Is(res.err, ErrPathNoOnePublishing) {
			return &base.Response{
				StatusCode: base.StatusNotFound,
			}, nil, res.err
		}
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, nil, res.err
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, res.stream.rtspStream, nil
}

// publisherAdd is called by a publisher.
func (pm *pathManager) publisherAdd(req pathPublisherAddReq) pathPublisherAddRes {
	req.res = make(chan pathPublisherAddRes)
	select {
	case pm.chPublisherAdd <- req:
		res := <-req.res
		if res.err != nil {
			return res
		}

		return res.path.publisherAdd(req)

	case <-pm.ctx.Done():
		return pathPublisherAddRes{err: ErrTerminated}
	}
}

// readerAdd is called by a reader.
func (pm *pathManager) readerAdd(req pathReaderAddReq) pathReaderAddRes {
	req.res = make(chan pathReaderAddRes)
	select {
	case pm.chReaderAdd <- req:
		res := <-req.res
		if res.err != nil {
			return res
		}

		return res.path.readerAdd(req)

	case <-pm.ctx.Done():
		return pathReaderAddRes{err: ErrTerminated}
	}
}

// hlsServerSet is called by hlsServer before pathManager is started.
func (pm *pathManager) hlsServerSet(s pathManagerHLSServer) {
	pm.hlsServer = s
}
