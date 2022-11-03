package motion

import (
	"image"
	"math"
	"nvr/pkg/ffmpeg"
)

type zones []*zone

func (z zones) analyze(frame1, frame2, diff []uint8, onActive func(int, float64)) {
	diffFrames(frame1, frame2, diff)
	for i, zone := range z {
		if zone == nil {
			continue
		}
		score, isActive := zone.checkDiff(diff)
		// score, active := zone.compareFrames(frame1, frame2)
		if isActive {
			onActive(i, score)
		}
	}
}

func diffFrames(frame1, frame2, diff []byte) {
	for i := 0; i < len(frame1); i++ {
		diff[i] = abs(frame1[i], frame2[i])
	}
}

type zone struct {
	// Index of masked pixels.
	// Even indexes are n masked pixels in row.
	// Odd indexes are n unmasked pixels in a row.
	maskIndex []int

	zoneSize  int
	frameSize int

	sensitivity  uint8
	thresholdMin float64
	thresholdMax float64
}

func newZone(width int, height int, config zoneConfig) *zone {
	polygon := convertPolygon(width, height, config.Area)
	maskImage := ffmpeg.CreateInvertedMask(width, height, polygon)
	mask, zoneSize := parseMaskImage(maskImage)

	var check bool
	var index []int

	if !mask[0] {
		check = true
		index = []int{0}
	}

	for _, maskPixel := range mask {
		if check == maskPixel {
			// Last item++.
			index[len(index)-1]++
		} else {
			index = append(index, 1)
			check = maskPixel
		}
	}

	return &zone{
		maskIndex:    index,
		zoneSize:     zoneSize,
		frameSize:    width * height,
		sensitivity:  uint8(math.Round(config.Sensitivity * 2.56)),
		thresholdMin: config.ThresholdMin,
		thresholdMax: config.ThresholdMax,
	}
}

func (z zone) checkDiff(diff []uint8) (float64, bool) {
	var nChangedPixels int
	var pos int
	var check bool
	for _, index := range z.maskIndex {
		if check {
			for i := 0; i < index; i++ {
				if diff[pos] >= z.sensitivity {
					nChangedPixels++
				}
				pos++
			}
		} else {
			pos += index
		}
		check = !check
	}

	percentChanged := (float64(nChangedPixels) / float64(z.zoneSize)) * 100
	isActive := percentChanged > z.thresholdMin && percentChanged < z.thresholdMax

	return percentChanged, isActive
}

func abs(x, y uint8) uint8 {
	if x < y {
		return y - x
	}
	return x - y
}

// convertPolygon percent to absolute values.
func convertPolygon(w int, h int, area area) ffmpeg.Polygon {
	polygon := make(ffmpeg.Polygon, len(area))
	for i, point := range area {
		px := point[0]
		py := point[1]
		polygon[i] = [2]int{
			int(float64(w) * (float64(px) / 100)),
			int(float64(h) * (float64(py) / 100)),
		}
	}
	return polygon
}

func parseMaskImage(img image.Image) ([]bool, int) {
	var mask []bool
	var zoneSize int

	max := img.Bounds().Max
	for y := 0; y < max.Y; y++ {
		for x := 0; x < max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a == 0 {
				zoneSize++
				mask = append(mask, false)
			} else {
				mask = append(mask, true)
			}
		}
	}
	return mask, zoneSize
}
