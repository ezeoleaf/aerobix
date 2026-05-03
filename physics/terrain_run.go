package physics

import (
	"math"

	"aerobix/domain"
)

// GradeAdjustedAvgPaceMinKm returns modeled flat-equivalent avg pace (min/km) for runners when
// barometric altitude tracks are usable. Rough grade correction (similar spirit to classic GAP heuristics).
// Returns nan if unavailable.
func GradeAdjustedAvgPaceMinKm(a domain.Activity) float64 {
	if len(a.TimeSec) < 2 || len(a.SpeedMS) != len(a.TimeSec) || len(a.AltitudeM) != len(a.TimeSec) {
		return math.NaN()
	}
	sec := float64(a.Duration.Seconds())
	if sec < 90 {
		return math.NaN()
	}
	validAlt := false
	prevAlt := 0.0
	for _, h := range a.AltitudeM {
		if h > 1e-3 {
			validAlt = true
			prevAlt = h
			break
		}
	}
	if !validAlt {
		return math.NaN()
	}
	var sumGap float64
	var wSum float64
	for i := 1; i < len(a.TimeSec); i++ {
		dt := float64(a.TimeSec[i] - a.TimeSec[i-1])
		if dt <= 0 {
			continue
		}
		s1 := a.SpeedMS[i-1]
		s2 := a.SpeedMS[i]
		if s1 <= 0 && s2 <= 0 {
			continue
		}
		avgSp := (s1 + s2) / 2
		h1 := prevAlt
		h2 := a.AltitudeM[i]
		if math.IsNaN(h2) || h2 <= 1e-3 {
			h2 = prevAlt
		}
		dElev := h2 - h1
		prevAlt = h2
		hDist := avgSp * dt
		if hDist <= 1e-6 || avgSp <= 0 {
			continue
		}
		gradePct := 100 * dElev / hDist // small-angle pct grade
		// Clamp correction factor — crude but stable outdoors.
		corr := 1 + 0.055*gradePct
		corr = math.Max(0.55, math.Min(3.0, corr))
		secPerKm := 1000 / avgSp
		gap := secPerKm / corr
		sumGap += gap * hDist
		wSum += hDist
	}
	if wSum <= 10 {
		return math.NaN()
	}
	minPerKm := (sumGap / wSum) / 60
	if math.IsNaN(minPerKm) || minPerKm <= 0 || minPerKm > 45 {
		return math.NaN()
	}
	return minPerKm
}

// UphillTimeFraction estimates fraction of moving time spent with modeled grade ≥ threshold (%).
func UphillTimeFraction(a domain.Activity, gradeThresholdPct float64) float64 {
	if len(a.TimeSec) < 2 || len(a.SpeedMS) != len(a.TimeSec) || len(a.AltitudeM) != len(a.TimeSec) {
		return 0
	}
	var moveDur, uphillDur float64
	prevAlt := 0.0
	altOk := false
	for _, h := range a.AltitudeM {
		if h > 1e-3 && !math.IsNaN(h) {
			prevAlt = h
			altOk = true
			break
		}
	}
	if !altOk {
		return 0
	}
	for i := 1; i < len(a.TimeSec); i++ {
		dt := float64(a.TimeSec[i] - a.TimeSec[i-1])
		if dt <= 0 {
			continue
		}
		s1 := a.SpeedMS[i-1]
		s2 := a.SpeedMS[i]
		avgSp := (s1 + s2) / 2
		if avgSp < 0.5 {
			continue
		}
		moveDur += dt
		h2 := a.AltitudeM[i]
		if math.IsNaN(h2) || h2 <= 1e-3 {
			h2 = prevAlt
		}
		dElev := h2 - prevAlt
		prevAlt = h2
		hDist := avgSp * dt
		if hDist <= 1e-6 {
			continue
		}
		gradePct := 100 * dElev / hDist
		if gradePct >= gradeThresholdPct {
			uphillDur += dt
		}
	}
	if moveDur <= 1 {
		return 0
	}
	return math.Min(1.0, uphillDur/moveDur)
}

// DownhillBrakingLoad approximates eccentric braking stress from descending (arbitrary units, trend only).
func DownhillBrakingLoad(a domain.Activity) float64 {
	if len(a.TimeSec) < 2 {
		return 0
	}
	var score float64
	if len(a.VerticalSpeedMS) == len(a.TimeSec) {
		for i := 1; i < len(a.TimeSec); i++ {
			dt := float64(a.TimeSec[i] - a.TimeSec[i-1])
			if dt <= 0 {
				continue
			}
			vv := math.Min(a.VerticalSpeedMS[i], a.VerticalSpeedMS[i-1])
			if vv >= 0 {
				continue
			}
			horiz := math.Max(0.0, a.SpeedMS[i]) * dt
			if horiz <= 0 {
				horiz = 0.1
			}
			score += (-vv) * horiz // m/s descending * approximate horizontal displacement
		}
		return score
	}
	// Fallback: differentiate altitude vs speed-derived horizontal advance.
	if len(a.AltitudeM) != len(a.TimeSec) || len(a.SpeedMS) != len(a.TimeSec) {
		return 0
	}
	prevAlt := 0.0
	seen := false
	for i := 1; i < len(a.TimeSec); i++ {
		dt := float64(a.TimeSec[i] - a.TimeSec[i-1])
		if dt <= 0 {
			continue
		}
		h := a.AltitudeM[i]
		if !seen && !math.IsNaN(h) && h > 1e-3 {
			prevAlt = h
			seen = true
		}
		if !seen {
			continue
		}
		if math.IsNaN(h) {
			continue
		}
		dElev := h - prevAlt
		prevAlt = h
		avgSp := (a.SpeedMS[i] + a.SpeedMS[i-1]) / 2
		hDist := math.Max(avgSp*dt, 1e-3)
		if dElev >= 0 {
			continue
		}
		grade := clamp(-100*dElev/hDist, 0, 50)
		score += grade * hDist / 50
	}
	return score
}

// RunningSquaredPowerLoad aggregates ∫(P/ref)² dt in seconds-equivalent impulse (like RSS spirit).
func RunningSquaredPowerLoad(power []float64, timeSec []int, refW float64) float64 {
	if refW <= 1 || len(power) != len(timeSec) || len(power) < 2 {
		return 0
	}
	var sum float64
	for i := 1; i < len(power); i++ {
		dt := timeSec[i] - timeSec[i-1]
		if dt <= 0 {
			continue
		}
		r1 := math.Max(0.0, power[i-1]) / refW
		r2 := math.Max(0.0, power[i]) / refW
		avgSq := ((r1 * r1) + (r2 * r2)) / 2
		sum += float64(dt) * avgSq
	}
	return sum
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}
