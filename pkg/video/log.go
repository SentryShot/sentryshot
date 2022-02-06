package video

import (
	"nvr/pkg/log"
)

func sendLog(logger *log.Logger, conf PathConf, level log.Level, message string) {
	var msg string

	msg += "RTSP:"

	if conf.IsSub {
		msg += " sub:"
	} else {
		msg += " main:"
	}
	msg += " " + message

	id := conf.MonitorID
	switch level {
	case log.LevelDebug:
		logger.Debug().Src("monitor").Monitor(id).Msg(msg)
	case log.LevelInfo:
		logger.Info().Src("monitor").Monitor(id).Msg(msg)
	case log.LevelWarning:
		logger.Warn().Src("monitor").Monitor(id).Msg(msg)
	case log.LevelError:
		logger.Error().Src("monitor").Monitor(id).Msg(msg)
	}
}
