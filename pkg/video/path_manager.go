package video

import (
	"context"
	"errors"
	"nvr/pkg/log"
	"nvr/pkg/video/gortsplib"
	"nvr/pkg/video/gortsplib/pkg/base"
	"nvr/pkg/video/hls"
	"sync"
)

type pathManagerHLSServer interface {
	pathSourceReady(*path, gortsplib.Tracks) (*HLSMuxer, error)
	pathSourceNotReady(pathName string)
	MuxerByPathName(pathName string) (*hls.Muxer, error)
}

type pathManager struct {
	wg  *sync.WaitGroup
	log log.ILogger
	mu  sync.Mutex

	hlsServer pathManagerHLSServer
	pathConfs map[string]*PathConf
	paths     map[string]*path
}

func newPathManager(
	wg *sync.WaitGroup,
	log log.ILogger,
	hlsServer pathManagerHLSServer,
) *pathManager {
	return &pathManager{
		wg:  wg,
		log: log,

		hlsServer: hlsServer,
		pathConfs: make(map[string]*PathConf),
		paths:     make(map[string]*path),
	}
}

// Errors.
var (
	ErrPathAlreadyExist = errors.New("path already exist")
	ErrPathNotExist     = errors.New("path not exist")
)

// AddPath add path to pathManager.
func (pm *pathManager) AddPath(
	ctx context.Context,
	name string,
	newConf PathConf,
) (HlsMuxerFunc, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	err := newConf.CheckAndFillMissing(name)
	if err != nil {
		return nil, err
	}

	if _, exist := pm.pathConfs[name]; exist {
		return nil, ErrPathAlreadyExist
	}

	config := &newConf

	// Add config.
	pm.pathConfs[name] = config

	// Add path.
	pm.paths[name] = newPath(
		ctx,
		name,
		config,
		pm.wg,
		pm.hlsServer,
		pm.log,
	)

	hlsMuxer := func() (IHLSMuxer, error) {
		return pm.hlsServer.MuxerByPathName(name)
	}

	go func() {
		// Remove path.
		<-ctx.Done()

		pm.mu.Lock()
		defer pm.mu.Unlock()

		// Remove config.
		delete(pm.pathConfs, name)

		// Close and remove path.
		pm.paths[name].close()
		delete(pm.paths, name)
	}()

	return hlsMuxer, nil
}

// Testing.
func (pm *pathManager) pathExist(name string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	_, exist := pm.pathConfs[name]
	return exist
}

// describe is called by a rtsp reader.
func (pm *pathManager) onDescribe(
	pathName string,
) (*base.Response, *gortsplib.ServerStream, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	path, exist := pm.paths[pathName]
	if !exist {
		return &base.Response{
			StatusCode: base.StatusNotFound,
		}, nil, ErrPathNotExist
	}

	stream, err := path.streamGet()
	if err != nil {
		if errors.Is(err, ErrPathNoOnePublishing) {
			return &base.Response{
				StatusCode: base.StatusNotFound,
			}, nil, err
		}
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, nil, err
	}

	return &base.Response{StatusCode: base.StatusOK}, stream.rtspStream, nil
}

// publisherAdd is called by a rtsp publisher.
func (pm *pathManager) publisherAdd(
	name string,
	session *rtspSession,
) (*path, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	path, exist := pm.paths[name]
	if !exist {
		return nil, ErrPathNotExist
	}
	return path.publisherAdd(session)
}

// readerAdd is called by a rtsp reader.
func (pm *pathManager) readerAdd(
	name string,
	session *rtspSession,
) (*path, *stream, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	path, exist := pm.paths[name]
	if !exist {
		return nil, nil, ErrPathNotExist
	}
	return path.readerAdd(session)
}
