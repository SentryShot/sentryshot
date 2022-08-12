package video

import (
	"fmt"
	"nvr/pkg/log"
)

func sendLogf(
	logger *log.Logger,
	conf PathConf,
	level log.Level,
	prefix string,
	format string,
	a ...interface{},
) {
	processName := func() string {
		if conf.IsSub {
			return "sub"
		}
		return "main"
	}()
	logger.Level(level).
		Src("monitor").
		Monitor(conf.MonitorID).
		Msgf("%v %v: %v", prefix, processName, fmt.Sprintf(format, a...))
}
