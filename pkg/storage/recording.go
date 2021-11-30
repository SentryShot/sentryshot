package storage

import (
	"errors"
	"fmt"
	"nvr/pkg/ffmpeg"
	"time"
)

// Recording contains identifier and path.
// `.mp4`, `.jpeg` or `.json` can be appended to the
// path to get the video, thumbnail or data file.
type Recording struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// RecordingData recording data marshaled to json and saved next to video and thumbnail.
type RecordingData struct {
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	Events []Event   `json:"events"`
}

// Events .
type Events []Event

// Event is a recording trigger event.
type Event struct {
	Time        time.Time     `json:"time,omitempty"`
	Detections  []Detection   `json:"detections,omitempty"`
	Duration    time.Duration `json:"duration,omitempty"`
	RecDuration time.Duration `json:"-"`
}

func (e Event) String() string {
	return fmt.Sprintf("\n Time: %v\n Detections: %v\n Duration: %v\n RecDuration: %v",
		e.Time, e.Detections, e.Duration, e.RecDuration)
}

// ErrValueMissing value missing.
var ErrValueMissing = errors.New("value missing")

// Validate events.
func (e Event) Validate() error {
	if e.Time == (time.Time{}) {
		return fmt.Errorf("{%v\n}\n'Time': %w", e, ErrValueMissing)
	}
	if e.RecDuration == 0 {
		return fmt.Errorf("{%v\n}\n'RecDuration': %w", e, ErrValueMissing)
	}
	return nil
}

// Detection .
type Detection struct {
	Label  string  `json:"label,omitempty"`
	Score  float64 `json:"score,omitempty"`
	Region *Region `json:"region,omitempty"`
}

// Region where detection occurred.
type Region struct {
	Rect    *ffmpeg.Rect    `json:"rect,omitempty"`
	Polygon *ffmpeg.Polygon `json:"polygon,omitempty"`
}

func (r *Region) String() string {
	return fmt.Sprintf("%v, %v", r.Rect, r.Polygon)
}
