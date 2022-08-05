package doods

import (
	"encoding/json"
	"errors"
	"fmt"
	"nvr"
	"nvr/pkg/ffmpeg"
	"nvr/pkg/monitor"
	"strconv"
	"time"
)

type config struct {
	monitorID       string
	hwaccel         string
	ffmpegLogLevel  string
	timestampOffset time.Duration
	thresholds      thresholds
	cropX           float64
	cropY           float64
	cropSize        float64
	mask            mask
	detectorName    string
	grayMode        bool
	feedRate        float64
	recDuration     time.Duration
	useSubStream    bool
}

type rawConfigV1 struct {
	Enable       string `json:"enable"`
	Thresholds   string `json:"thresholds"`
	Crop         string `json:"crop"`
	Mask         string `json:"mask"`
	DetectorName string `json:"detectorName"`
	FeedRate     string `json:"feedRate"`
	Duration     string `json:"duration"`
	UseSubStream string `json:"useSubStream"`
}

type mask struct {
	Enable bool           `json:"enable"`
	Area   ffmpeg.Polygon `json:"area"`
}

func parseConfig(conf monitor.Config) (*config, bool, error) { //nolint:funlen
	rawConf, err := parseRawConfig(conf["doods"])
	if err != nil {
		return nil, false, err
	}
	enable := rawConf.Enable == "true"
	if !enable {
		return nil, false, nil
	}

	timestampOffset, err := parseTimestampOffset(conf["timestampOffset"])
	if err != nil {
		return nil, false, err
	}

	thresholds, err := parseThresholds(rawConf.Thresholds)
	if err != nil {
		return nil, false, err
	}

	var crop [3]float64
	if rawConf.Crop != "" {
		err = json.Unmarshal([]byte(rawConf.Crop), &crop)
		if err != nil {
			return nil, false, fmt.Errorf("unmarshal crop values: %w", err)
		}
	}

	var mask mask
	if rawConf.Mask != "" {
		if err := json.Unmarshal([]byte(rawConf.Mask), &mask); err != nil {
			return nil, false, fmt.Errorf("unmarshal mask: %w", err)
		}
	}

	grayMode := len(rawConf.DetectorName) > 5 &&
		rawConf.DetectorName[0:5] == "gray_"

	var feedRate float64
	if rawConf.FeedRate != "" {
		feedRate, err = strconv.ParseFloat(rawConf.FeedRate, 64)
		if err != nil {
			return nil, false, fmt.Errorf("parse feed rate: %w", err)
		}
	}

	recDuration, err := parseDuration(rawConf.Duration)
	if err != nil {
		return nil, false, err
	}

	useSubStream := conf.SubInputEnabled() && rawConf.UseSubStream == "true"

	return &config{
		monitorID:       conf.ID(),
		hwaccel:         conf.Hwaccel(),
		ffmpegLogLevel:  conf.LogLevel(),
		timestampOffset: timestampOffset,
		thresholds:      thresholds,
		cropX:           crop[0],
		cropY:           crop[1],
		cropSize:        crop[2],
		mask:            mask,
		detectorName:    rawConf.DetectorName,
		grayMode:        grayMode,
		feedRate:        feedRate,
		recDuration:     recDuration,
		useSubStream:    useSubStream,
	}, enable, nil
}

func parseRawConfig(rawDoods string) (rawConfigV1, error) {
	if rawDoods == "" {
		return rawConfigV1{}, nil
	}
	var rawConf rawConfigV1
	err := json.Unmarshal([]byte(rawDoods), &rawConf)
	if err != nil {
		return rawConfigV1{}, fmt.Errorf("unmarshal doods: %w", err)
	}
	return rawConf, nil
}

func parseTimestampOffset(rawTimestampOffset string) (time.Duration, error) {
	if rawTimestampOffset == "" {
		return 0, nil
	}
	timestampOffsetFloat, err := strconv.Atoi(rawTimestampOffset)
	if err != nil {
		return 0, fmt.Errorf("parse timestamp offset %w", err)
	}
	return time.Duration(timestampOffsetFloat) * time.Millisecond, nil
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

func parseDuration(rawDuration string) (time.Duration, error) {
	if rawDuration == "" {
		return 0, nil
	}
	recDurationFloat, err := strconv.ParseFloat(rawDuration, 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	return time.Duration(recDurationFloat * float64(time.Second)), nil
}

const (
	defaultCropSize    = 100
	defaultFeedRate    = 0.2
	defaultRecDuration = 120 * time.Second
)

func (c *config) fillMissing() {
	if c.thresholds == nil {
		c.thresholds = thresholds{}
	}
	if c.cropSize == 0 {
		c.cropSize = defaultCropSize
	}
	if c.feedRate == 0 {
		c.feedRate = defaultFeedRate
	}
	if c.recDuration == 0 {
		c.recDuration = defaultRecDuration
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

func init() {
	nvr.RegisterMigrationMonitorHook(migrate)
}

const currentConfigVersion = 1

func migrate(conf monitor.Config) error {
	configVersion, _ := strconv.Atoi(conf["doodsConfigVersion"])

	if configVersion < 1 {
		if err := migrateV0toV1(conf); err != nil {
			return fmt.Errorf("doods v0 to v1: %w", err)
		}
	}

	conf["doodsConfigVersion"] = strconv.Itoa(currentConfigVersion)
	return nil
}

func migrateV0toV1(conf monitor.Config) error {
	config := rawConfigV1{
		Enable:       conf["doodsEnable"],
		Thresholds:   conf["doodsThresholds"],
		Crop:         conf["doodsCrop"],
		Mask:         conf["doodsMask"],
		DetectorName: conf["doodsDetectorName"],
		FeedRate:     conf["doodsFeedRate"],
		Duration:     conf["doodsDuration"],
		UseSubStream: conf["doodsUseSubStream"],
	}

	delete(conf, "doodsEnable")
	delete(conf, "doodsThresholds")
	delete(conf, "doodsCrop")
	delete(conf, "doodsMask")
	delete(conf, "doodsDetectorName")
	delete(conf, "doodsFeedRate")
	delete(conf, "doodsDuration")
	delete(conf, "doodsUseSubStream")

	rawConfig, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal raw config: %w", err)
	}
	conf["doods"] = string(rawConfig)
	return nil
}
