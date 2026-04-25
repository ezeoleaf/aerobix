package physics

import (
	"errors"
	"math"
	"strings"

	"aerobix/domain"
)

var ErrInsufficientRuns = errors.New("insufficient running data for critical speed")

// TRIMPFromZones computes a simple zone-weighted training impulse.
// Weights: Z1..Z5 => 1,2,3,4,5.
func TRIMPFromZones(zoneMinutes [5]float64) float64 {
	weights := [5]float64{1, 2, 3, 4, 5}
	total := 0.0
	for i := range zoneMinutes {
		total += zoneMinutes[i] * weights[i]
	}
	return total
}

// EstimatedTSSFromHRZones provides an HR-based TSS approximation for
// activities with missing/unreliable power data.
func EstimatedTSSFromHRZones(zoneMinutes [5]float64, durationSec int) float64 {
	if durationSec <= 0 {
		return 0
	}
	totalMin := 0.0
	for _, z := range zoneMinutes {
		totalMin += z
	}
	if totalMin <= 0 {
		return 0
	}

	intensity := [5]float64{0.55, 0.68, 0.80, 0.92, 1.03}
	ifhr := 0.0
	for i := range zoneMinutes {
		ifhr += (zoneMinutes[i] / totalMin) * intensity[i]
	}
	hours := float64(durationSec) / 3600.0
	return hours * ifhr * ifhr * 100.0
}

// SpeedEfficiencyFactor is speed (m/s) over avg HR.
func SpeedEfficiencyFactor(speedMS []float64, avgHR float64) (float64, error) {
	if len(speedMS) == 0 || avgHR <= 0 {
		return 0, ErrInvalidInput
	}
	sum := 0.0
	n := 0
	for _, s := range speedMS {
		if s > 0 {
			sum += s
			n++
		}
	}
	if n == 0 {
		return 0, ErrInvalidInput
	}
	return (sum / float64(n)) / avgHR, nil
}

type CriticalSpeedResult struct {
	CSMS       float64
	DPrimeM    float64
	SourceRuns int
}

// CriticalSpeed estimates CS and D' using best 3 min and 9 min mean speeds.
func CriticalSpeed(activities []domain.Activity) (CriticalSpeedResult, error) {
	best3 := 0.0
	best9 := 0.0
	runCount := 0
	for _, a := range activities {
		if !isRunLike(a.Sport) {
			continue
		}
		runCount++
		v3 := bestMeanSpeed(a, 180)
		v9 := bestMeanSpeed(a, 540)
		if v3 > best3 {
			best3 = v3
		}
		if v9 > best9 {
			best9 = v9
		}
	}
	if runCount == 0 || best3 <= 0 || best9 <= 0 {
		return CriticalSpeedResult{}, ErrInsufficientRuns
	}

	t1 := 180.0
	t2 := 540.0
	d1 := best3 * t1
	d2 := best9 * t2
	cs := (d2 - d1) / (t2 - t1)
	dPrime := d1 - (cs * t1)
	if cs < 0 {
		cs = 0
	}
	if dPrime < 0 {
		dPrime = math.Abs(dPrime)
	}
	return CriticalSpeedResult{CSMS: cs, DPrimeM: dPrime, SourceRuns: runCount}, nil
}

func bestMeanSpeed(a domain.Activity, durationSec int) float64 {
	if durationSec <= 0 {
		return 0
	}
	series, step := speedSeries(a)
	if len(series) == 0 || step <= 0 {
		return 0
	}
	window := durationSec / step
	if window < 1 {
		window = 1
	}
	if len(series) < window {
		return averageSpeed(a)
	}
	best := 0.0
	sum := 0.0
	for i := 0; i < len(series); i++ {
		sum += series[i]
		if i >= window {
			sum -= series[i-window]
		}
		if i >= window-1 {
			mean := sum / float64(window)
			if mean > best {
				best = mean
			}
		}
	}
	return best
}

func speedSeries(a domain.Activity) ([]float64, int) {
	if len(a.SpeedMS) > 1 && len(a.SpeedMS) == len(a.TimeSec) {
		step := sampleStep(a.TimeSec)
		if step <= 0 {
			step = 1
		}
		return a.SpeedMS, step
	}
	avg := averageSpeed(a)
	if avg <= 0 || len(a.TimeSec) == 0 {
		return nil, 0
	}
	s := make([]float64, len(a.TimeSec))
	for i := range s {
		s[i] = avg
	}
	return s, sampleStep(a.TimeSec)
}

func averageSpeed(a domain.Activity) float64 {
	if a.Duration <= 0 || a.DistanceKM <= 0 {
		return 0
	}
	return (a.DistanceKM * 1000.0) / a.Duration.Seconds()
}

func sampleStep(timeSec []int) int {
	if len(timeSec) < 2 {
		return 1
	}
	d := timeSec[1] - timeSec[0]
	if d <= 0 {
		return 1
	}
	return d
}

func isRunLike(sport string) bool {
	s := strings.ToLower(sport)
	return strings.Contains(s, "run") || strings.Contains(s, "trail") || strings.Contains(s, "walk")
}
