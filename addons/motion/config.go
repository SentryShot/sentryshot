package motion

import (
	"encoding/json"
	"fmt"
	"image"
	"nvr/pkg/ffmpeg"
	"nvr/pkg/monitor"
	"strconv"
	"strings"
	"time"
)

type config struct {
	monitorID       string
	logLevel        string
	hwaccel         string
	timestampOffset time.Duration
	feedRate        string
	duration        time.Duration
	scale           int
	recDuration     time.Duration
	zones           []zoneConfig
}

type rawConfigV0 struct {
	Enable     string `json:"enable"`
	FeedRate   string `json:"feedRate"`
	FrameScale string `json:"frameScale"`
	Duration   string `json:"duration"`
	Zones      []zoneConfig
}

func parseConfig(c monitor.Config) (*config, bool, error) {
	var rawConf rawConfigV0
	err := json.Unmarshal([]byte(c["motion"]), &rawConf)
	if err != nil {
		return nil, false, fmt.Errorf("unmarshal config: %w", err)
	}

	enable := rawConf.Enable == "true"
	if !enable {
		return nil, false, nil
	}

	timestampOffset, err := parseTimestampOffset(c["timestampOffset"])
	if err != nil {
		return nil, false, err
	}

	feedRateFloat, err := strconv.ParseFloat(rawConf.FeedRate, 64)
	if err != nil {
		return nil, false, fmt.Errorf("parse feed rate: %w", err)
	}
	duration := ffmpeg.FeedRateToDuration(feedRateFloat)

	scale := parseScale(rawConf.FrameScale)

	durationInt, err := strconv.Atoi(rawConf.Duration)
	if err != nil {
		return nil, false, fmt.Errorf("parse duration: %w", err)
	}
	recDuration := time.Duration(durationInt) * time.Second

	return &config{
		monitorID:       c.ID(),
		logLevel:        c.LogLevel(),
		hwaccel:         c.Hwaccel(),
		timestampOffset: timestampOffset,
		feedRate:        rawConf.FeedRate,
		duration:        duration,
		scale:           scale,
		recDuration:     recDuration,
		zones:           rawConf.Zones,
	}, enable, nil
}

func parseTimestampOffset(rawOffset string) (time.Duration, error) {
	if rawOffset == "" {
		return 0, nil
	}
	timestampOffsetFloat, err := strconv.Atoi(rawOffset)
	if err != nil {
		return 0, fmt.Errorf("parse timestamp offset %w", err)
	}
	return time.Duration(timestampOffsetFloat) * time.Millisecond, nil
}

func parseScale(scale string) int {
	switch strings.ToLower(scale) {
	case "full":
		return 1
	case "half":
		return 2
	case "third":
		return 3
	case "quarter":
		return 4
	case "sixth":
		return 6
	case "eighth":
		return 8
	default:
		return 1
	}
}

type zoneConfig struct {
	Enable    bool           `json:"enable"`
	Preview   bool           `json:"preview"`
	Threshold float64        `json:"threshold"`
	Area      []ffmpeg.Point `json:"area"`
}

func (z zoneConfig) generateMask(w int, h int) image.Image {
	polygon := z.calculatePolygon(w, h)

	return ffmpeg.CreateInvertedMask(w, h, polygon)
}

func (z zoneConfig) calculatePolygon(w int, h int) ffmpeg.Polygon {
	polygon := make(ffmpeg.Polygon, len(z.Area))
	for i, point := range z.Area {
		px := point[0]
		py := point[1]
		polygon[i] = [2]int{
			int(float32(w) * (float32(px) / 100)),
			int(float32(h) * (float32(py) / 100)),
		}
	}
	return polygon
}
