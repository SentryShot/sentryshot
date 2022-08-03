package doods

import (
	"encoding/json"
	"errors"
	"fmt"
	"nvr/pkg/ffmpeg"
	"nvr/pkg/monitor"
	"strconv"
	"time"
)

type config struct {
	useSubStream    bool
	monitorID       string
	feedRate        float64
	recDuration     time.Duration
	thresholds      thresholds
	timestampOffset time.Duration
	detectorName    string
	cropX           float64
	cropY           float64
	cropSize        float64
	ffmpegLogLevel  string
	grayMode        bool
	mask            mask
	hwaccel         string
}

type mask struct {
	Enable bool           `json:"enable"`
	Area   ffmpeg.Polygon `json:"area"`
}

func parseConfig(conf monitor.Config) (*config, bool, error) { //nolint:funlen
	enable := conf["doodsEnable"] == "true"
	if !enable {
		return nil, false, nil
	}

	useSubStream := conf.SubInputEnabled() && conf["doodsUseSubStream"] == "true"

	thresholds, err := parseThresholds(conf["doodsThresholds"])
	if err != nil {
		return nil, false, fmt.Errorf("unmarshal thresholds: %w", err)
	}

	var feedRate float64
	rawFeedRate := conf["doodsFeedRate"]
	if rawFeedRate != "" {
		feedRate, err = strconv.ParseFloat(rawFeedRate, 64)
		if err != nil {
			return nil, false, fmt.Errorf("parse feed rate: %w", err)
		}
	}

	var recDuration time.Duration
	rawDuration := conf["doodsDuration"]
	if rawDuration != "" {
		recDurationFloat, err := strconv.ParseFloat(rawDuration, 64)
		if err != nil {
			return nil, false, fmt.Errorf("parse duration: %w", err)
		}
		recDuration = time.Duration(recDurationFloat * float64(time.Second))
	}

	var timestampOffset time.Duration
	rawTimestampOffset := conf["timestampOffset"]
	if rawTimestampOffset != "" {
		timestampOffsetFloat, err := strconv.Atoi(rawTimestampOffset)
		if err != nil {
			return nil, false, fmt.Errorf("parse timestamp offset %w", err)
		}
		timestampOffset = time.Duration(timestampOffsetFloat) * time.Millisecond
	}

	var crop [3]float64
	rawCrop := conf["doodsCrop"]
	if rawCrop != "" {
		err = json.Unmarshal([]byte(rawCrop), &crop)
		if err != nil {
			return nil, false, fmt.Errorf("unmarshal crop values: %w", err)
		}
	}

	detectorName := conf["doodsDetectorName"]
	grayMode := len(detectorName) > 5 && detectorName[0:5] == "gray_"

	var mask mask
	rawMask := conf["doodsMask"]
	if rawMask != "" {
		if err := json.Unmarshal([]byte(rawMask), &mask); err != nil {
			return nil, false, fmt.Errorf("unmarshal mask: %w", err)
		}
	}

	return &config{
		useSubStream:    useSubStream,
		monitorID:       conf.ID(),
		feedRate:        feedRate,
		recDuration:     recDuration,
		thresholds:      thresholds,
		timestampOffset: timestampOffset,
		detectorName:    detectorName,
		grayMode:        grayMode,
		cropX:           crop[0],
		cropY:           crop[1],
		cropSize:        crop[2],
		ffmpegLogLevel:  conf.LogLevel(),
		mask:            mask,
		hwaccel:         conf.Hwaccel(),
	}, enable, nil
}

func parseThresholds(rawThresholds string) (thresholds, error) {
	if rawThresholds == "" {
		return nil, nil
	}

	var t thresholds
	err := json.Unmarshal([]byte(rawThresholds), &t)
	if err != nil {
		return nil, err
	}
	for key, thresh := range t {
		if thresh == -1 {
			delete(t, key)
		}
	}
	return t, nil
}

const (
	defaultCropSize    = 100
	defaultFeedRate    = 0.5
	defaultRecDuration = 120 * time.Second
)

func (c *config) fillMissing() {
	if c.cropSize == 0 {
		c.cropSize = defaultCropSize
	}
	if c.feedRate == 0 {
		c.feedRate = defaultFeedRate
	}
	if c.recDuration == 0 {
		c.recDuration = defaultRecDuration
	}
	if c.thresholds == nil {
		c.thresholds = thresholds{}
	}
}

// Validate errors.
var (
	ErrInvalidCropSize = errors.New("invalid crop size")
	ErrInvalidCropX    = errors.New("invalid cropX")
	ErrInvalidCropY    = errors.New("invalid cropY")
	ErrInvalidFeedRate = errors.New("invalid feed rate")
	ErrInvalidDuration = errors.New("invalid duration")
)

// The WebUI shouldn't allow the user to save invalid values, this is more of
// a sanity check in case of failed migration or manual config file edits.
func (c *config) validate() error {
	if c.cropSize < 0 || c.cropSize > 100 {
		return fmt.Errorf("%w: %v", ErrInvalidCropSize, c.cropSize)
	}
	if c.cropX < 0 || c.cropX > 100 {
		return fmt.Errorf("%w: %v", ErrInvalidCropX, c.cropX)
	}
	if c.cropY < 0 || c.cropY > 100 {
		return fmt.Errorf("%w: %v", ErrInvalidCropY, c.cropY)
	}
	if c.feedRate <= 0 {
		return fmt.Errorf("%w: %v", ErrInvalidFeedRate, c.feedRate)
	}
	if c.recDuration < 0 {
		return fmt.Errorf("%w: %v", ErrInvalidDuration, c.recDuration)
	}
	return nil
}
