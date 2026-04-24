package physics

import (
	"errors"
	"math"

	"aerobix/domain"
)

var (
	ErrInsufficientData = errors.New("insufficient stream data")
	ErrMismatchedSeries = errors.New("power, heart rate, and time slices must align")
	ErrInvalidInput     = errors.New("invalid physiological input")
)

func NormalizedPower(power []float64) (float64, error) {
	if len(power) < 30 {
		return 0, ErrInsufficientData
	}

	rolling := rollingMean(power, 30)
	if len(rolling) == 0 {
		return 0, ErrInsufficientData
	}

	var sum4 float64
	for _, v := range rolling {
		sum4 += math.Pow(v, 4)
	}
	mean4 := sum4 / float64(len(rolling))
	return math.Pow(mean4, 0.25), nil
}

func EfficiencyFactor(np, avgHeartRate float64) (float64, error) {
	if avgHeartRate <= 0 {
		return 0, ErrInvalidInput
	}
	return np / avgHeartRate, nil
}

func IntensityFactor(np, ftp float64) (float64, error) {
	if ftp <= 0 {
		return 0, ErrInvalidInput
	}
	return np / ftp, nil
}

func TrainingStressScore(seconds int, np, ifValue, ftp float64) (float64, error) {
	if seconds <= 0 || ftp <= 0 {
		return 0, ErrInvalidInput
	}
	return ((float64(seconds) * np * ifValue) / (ftp * 3600)) * 100, nil
}

// AerobicDecoupling computes Pw:HR decoupling percentage.
// It first detects the most steady continuous segment (default target: middle-ish long segment),
// then compares (avgPower/avgHR) in first vs second half.
func AerobicDecoupling(a domain.Activity) (float64, error) {
	n := len(a.Power)
	if n < 10 || len(a.HeartRate) != n || len(a.TimeSec) != n {
		return 0, ErrMismatchedSeries
	}

	segment := detectSteadyStateSegment(a.Power)
	if len(segment) < 10 {
		return 0, ErrInsufficientData
	}

	start := segment[0]
	end := segment[len(segment)-1] + 1
	power := a.Power[start:end]
	hr := a.HeartRate[start:end]
	mid := len(power) / 2
	if mid < 1 || len(power)-mid < 1 {
		return 0, ErrInsufficientData
	}

	firstRatio, err := meanRatio(power[:mid], hr[:mid])
	if err != nil {
		return 0, err
	}
	secondRatio, err := meanRatio(power[mid:], hr[mid:])
	if err != nil {
		return 0, err
	}
	if firstRatio == 0 {
		return 0, ErrInvalidInput
	}
	return ((firstRatio - secondRatio) / firstRatio) * 100, nil
}

func AvgHeartRate(hr []float64) (float64, error) {
	if len(hr) == 0 {
		return 0, ErrInsufficientData
	}
	var sum float64
	for _, v := range hr {
		if v <= 0 {
			return 0, ErrInvalidInput
		}
		sum += v
	}
	return sum / float64(len(hr)), nil
}

func TimeInPowerZones(power []float64, ftp float64) ([5]int, error) {
	var z [5]int
	if len(power) == 0 || ftp <= 0 {
		return z, ErrInvalidInput
	}

	for _, p := range power {
		r := p / ftp
		switch {
		case r < 0.55:
			z[0]++
		case r < 0.75:
			z[1]++
		case r < 0.90:
			z[2]++
		case r < 1.05:
			z[3]++
		default:
			z[4]++
		}
	}
	return z, nil
}

// TimeInPowerZonesMinutes returns zone durations in minutes (not sample counts).
func TimeInPowerZonesMinutes(power []float64, timeSec []int, ftp float64) ([5]float64, error) {
	var z [5]float64
	if len(power) == 0 || len(power) != len(timeSec) || ftp <= 0 {
		return z, ErrInvalidInput
	}

	for i, p := range power {
		dtMin := sampleDurationMinutes(timeSec, i)
		r := p / ftp
		switch {
		case r < 0.55:
			z[0] += dtMin
		case r < 0.75:
			z[1] += dtMin
		case r < 0.90:
			z[2] += dtMin
		case r < 1.05:
			z[3] += dtMin
		default:
			z[4] += dtMin
		}
	}
	return z, nil
}

// TimeInHeartRateZonesMinutes bins by percentage of max observed HR.
func TimeInHeartRateZonesMinutes(hr []float64, timeSec []int) ([5]float64, error) {
	var z [5]float64
	if len(hr) == 0 || len(hr) != len(timeSec) {
		return z, ErrInvalidInput
	}
	maxHR := 0.0
	for _, h := range hr {
		if h > maxHR {
			maxHR = h
		}
	}
	if maxHR <= 0 {
		return z, ErrInvalidInput
	}

	for i, h := range hr {
		dtMin := sampleDurationMinutes(timeSec, i)
		r := h / maxHR
		switch {
		case r < 0.68:
			z[0] += dtMin
		case r < 0.78:
			z[1] += dtMin
		case r < 0.88:
			z[2] += dtMin
		case r < 0.94:
			z[3] += dtMin
		default:
			z[4] += dtMin
		}
	}
	return z, nil
}

func rollingMean(values []float64, window int) []float64 {
	if len(values) < window || window <= 0 {
		return nil
	}
	out := make([]float64, 0, len(values)-window+1)
	var sum float64
	for i := 0; i < len(values); i++ {
		sum += values[i]
		if i >= window {
			sum -= values[i-window]
		}
		if i >= window-1 {
			out = append(out, sum/float64(window))
		}
	}
	return out
}

func detectSteadyStateSegment(power []float64) []int {
	if len(power) < 30 {
		idx := make([]int, len(power))
		for i := range power {
			idx[i] = i
		}
		return idx
	}

	window := min(30, len(power)/2)
	bestStart := 0
	bestVar := math.MaxFloat64
	targetCenter := len(power) / 2

	for i := 0; i+window <= len(power); i++ {
		v := variance(power[i : i+window])
		centerPenalty := math.Abs(float64((i+window/2)-targetCenter)) * 0.02
		score := v + centerPenalty
		if score < bestVar {
			bestVar = score
			bestStart = i
		}
	}

	idx := make([]int, window)
	for i := 0; i < window; i++ {
		idx[i] = bestStart + i
	}
	return idx
}

func variance(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var mean float64
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))
	var sum float64
	for _, v := range values {
		d := v - mean
		sum += d * d
	}
	return sum / float64(len(values))
}

func meanRatio(power, hr []float64) (float64, error) {
	if len(power) == 0 || len(power) != len(hr) {
		return 0, ErrInsufficientData
	}
	var sum float64
	for i := range power {
		if hr[i] <= 0 {
			return 0, ErrInvalidInput
		}
		sum += power[i] / hr[i]
	}
	return sum / float64(len(power)), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func sampleDurationMinutes(timeSec []int, i int) float64 {
	if len(timeSec) == 1 {
		return 1
	}
	if i < len(timeSec)-1 {
		dt := timeSec[i+1] - timeSec[i]
		if dt > 0 {
			return float64(dt) / 60.0
		}
	}
	if i > 0 {
		dt := timeSec[i] - timeSec[i-1]
		if dt > 0 {
			return float64(dt) / 60.0
		}
	}
	return 1
}
