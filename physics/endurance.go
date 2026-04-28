package physics

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

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

type DurabilityMetrics struct {
	DriftPct               float64
	DecouplingStartMinutes float64
	HRStabilityPct         float64
	Score                  float64
}

type FormBreakdownResult struct {
	Detected bool
	StartMin float64
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

// TimeToDecoupling returns the first minute where pace/HR drift rises above threshold.
// Drift is computed against the baseline ratio from the first quarter of the session.
func TimeToDecoupling(a domain.Activity, thresholdPct float64) (float64, float64, error) {
	if thresholdPct <= 0 {
		return 0, 0, ErrInvalidInput
	}
	ratios, mins, err := paceHRRatios(a)
	if err != nil {
		return 0, 0, err
	}
	if len(ratios) < 20 {
		return 0, 0, ErrInsufficientData
	}
	baseN := len(ratios) / 4
	if baseN < 5 {
		baseN = 5
	}
	if baseN >= len(ratios) {
		baseN = len(ratios) - 1
	}
	base := meanSlice(ratios[:baseN])
	if base <= 0 {
		return 0, 0, ErrInvalidInput
	}
	currentDrift := 0.0
	for i := baseN; i < len(ratios); i++ {
		currentDrift = ((base - ratios[i]) / base) * 100.0
		if currentDrift >= thresholdPct {
			return mins[i], currentDrift, nil
		}
	}
	return 0, currentDrift, nil
}

func AerobicDurability(a domain.Activity) (DurabilityMetrics, error) {
	var out DurabilityMetrics
	driftPct, err := AerobicDecoupling(a)
	if err != nil {
		return out, err
	}
	startMin, _, _ := TimeToDecoupling(a, 3.0)
	stability, _ := heartRateStability(a.HeartRate, a.TimeSec)
	out = DurabilityMetrics{
		DriftPct:               driftPct,
		DecouplingStartMinutes: startMin,
		HRStabilityPct:         stability,
		Score:                  durabilityScore(driftPct, startMin, stability, a.Duration),
	}
	return out, nil
}

func CadenceMetrics(a domain.Activity) (mean, stddev, dropPct float64, err error) {
	cad := cadenceSeries(a)
	if len(cad) < 10 {
		return 0, 0, 0, ErrInsufficientData
	}
	mean = meanSlice(cad)
	if mean <= 0 {
		return 0, 0, 0, ErrInvalidInput
	}
	stddev = stddevSlice(cad, mean)
	seg := len(cad) / 4
	if seg < 3 {
		seg = 3
	}
	if seg >= len(cad) {
		seg = len(cad) / 2
	}
	first := meanSlice(cad[:seg])
	last := meanSlice(cad[len(cad)-seg:])
	if first > 0 {
		dropPct = ((first - last) / first) * 100.0
	}
	return mean, stddev, dropPct, nil
}

func cadenceSeries(a domain.Activity) []float64 {
	if len(a.Cadence) > 0 {
		out := make([]float64, 0, len(a.Cadence))
		for _, c := range a.Cadence {
			if c > 0 {
				out = append(out, c)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if a.AvgCadence <= 0 || len(a.TimeSec) == 0 {
		return nil
	}
	n := len(a.TimeSec)
	out := make([]float64, n)
	for i := range out {
		out[i] = a.AvgCadence
	}
	return out
}

func paceHRRatios(a domain.Activity) ([]float64, []float64, error) {
	if len(a.HeartRate) == 0 || len(a.TimeSec) == 0 {
		return nil, nil, ErrInsufficientData
	}
	series, step := speedSeries(a)
	if len(series) == 0 || len(series) != len(a.HeartRate) {
		return nil, nil, ErrMismatchedSeries
	}
	ratios := make([]float64, 0, len(series))
	mins := make([]float64, 0, len(series))
	for i := range series {
		if a.HeartRate[i] <= 0 || series[i] <= 0 {
			continue
		}
		ratios = append(ratios, series[i]/a.HeartRate[i])
		mins = append(mins, float64(i*step)/60.0)
	}
	if len(ratios) < 10 {
		return nil, nil, ErrInsufficientData
	}
	return ratios, mins, nil
}

func heartRateStability(hr []float64, timeSec []int) (float64, error) {
	if len(hr) < 10 || len(hr) != len(timeSec) {
		return 0, ErrInsufficientData
	}
	mean := 0.0
	n := 0.0
	for _, h := range hr {
		if h > 0 {
			mean += h
			n++
		}
	}
	if n == 0 {
		return 0, ErrInvalidInput
	}
	mean /= n
	if mean <= 0 {
		return 0, ErrInvalidInput
	}
	variance := 0.0
	for _, h := range hr {
		if h <= 0 {
			continue
		}
		d := h - mean
		variance += d * d
	}
	std := math.Sqrt(variance / n)
	// stability score in percent; higher is more stable
	stability := 100.0 - (std/mean)*100.0
	if stability < 0 {
		stability = 0
	}
	if stability > 100 {
		stability = 100
	}
	return stability, nil
}

func durabilityScore(driftPct, startMin, hrStability float64, duration time.Duration) float64 {
	if duration <= 0 {
		return 0
	}
	driftScore := 100.0 - driftPct*8.0
	if driftScore < 0 {
		driftScore = 0
	}
	durationMin := duration.Minutes()
	startScore := 0.0
	if startMin <= 0 {
		startScore = 100.0
	} else {
		startScore = (startMin / durationMin) * 100.0
		if startScore > 100 {
			startScore = 100
		}
	}
	score := 0.45*driftScore + 0.35*startScore + 0.20*hrStability
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func meanSlice(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func stddevSlice(v []float64, mean float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var ss float64
	for _, x := range v {
		d := x - mean
		ss += d * d
	}
	return math.Sqrt(ss / float64(len(v)))
}

func DetectFormBreakdown(a domain.Activity) (FormBreakdownResult, error) {
	var out FormBreakdownResult
	if len(a.TimeSec) < 20 {
		return out, ErrInsufficientData
	}
	speed, step := speedSeries(a)
	if len(speed) < 20 || len(speed) != len(a.HeartRate) {
		return out, ErrMismatchedSeries
	}
	cad := cadenceSeries(a)
	if len(cad) < 20 {
		return out, ErrInsufficientData
	}

	window := len(speed) / 6
	if window < 5 {
		window = 5
	}
	baseSpeed := meanSlice(speed[:window])
	baseHR := meanSlice(nonZeroSlice(a.HeartRate[:window]))
	baseCad := meanSlice(cad[:window])
	if baseSpeed <= 0 || baseHR <= 0 || baseCad <= 0 {
		return out, ErrInvalidInput
	}

	for i := window; i+window <= len(speed); i++ {
		s := meanSlice(speed[i-window+1 : i+1])
		h := meanSlice(nonZeroSlice(a.HeartRate[i-window+1 : i+1]))
		c := meanSlice(cad[i-window+1 : i+1])
		if s <= 0 || h <= 0 || c <= 0 {
			continue
		}
		speedDrop := ((baseSpeed - s) / baseSpeed) * 100.0
		hrRise := ((h - baseHR) / baseHR) * 100.0
		cadDrop := ((baseCad - c) / baseCad) * 100.0
		if speedDrop >= 3.0 && hrRise >= 5.0 && cadDrop >= 2.0 {
			out.Detected = true
			out.StartMin = float64(i*step) / 60.0
			return out, nil
		}
	}
	return out, nil
}

func ClassifySession(a domain.Activity, hrZones [5]float64) string {
	totalMin := 0.0
	for _, z := range hrZones {
		totalMin += z
	}
	if totalMin <= 0 {
		return "Unclassified"
	}
	z1 := hrZones[0] / totalMin * 100.0
	z2 := hrZones[1] / totalMin * 100.0
	z1z2 := (hrZones[0] + hrZones[1]) / totalMin * 100.0
	z3 := hrZones[2] / totalMin * 100.0
	z4z5 := (hrZones[3] + hrZones[4]) / totalMin * 100.0
	durMin := a.Duration.Minutes()

	speed, _ := speedSeries(a)
	varPct := 0.0
	if len(speed) > 10 {
		m := meanSlice(speed)
		if m > 0 {
			varPct = (stddevSlice(speed, m) / m) * 100.0
		}
	}

	switch {
	case durMin >= 90 && z1z2 >= 65:
		return "Long Run"
	case z4z5 >= 22:
		return "Threshold / Interval"
	case z3 >= 36 && z3 >= z2+5 && z1z2 < 75 && z4z5 < 18:
		return "Tempo"
	case z1z2 >= 82 && durMin <= 55:
		return "Recovery Run"
	case z2 >= z3 && z1+z2 >= 68:
		return "Easy Aerobic"
	case z1z2 >= 70 && varPct < 8:
		return "Easy Aerobic"
	default:
		return "Steady Run"
	}
}

func ExplainRun(classification string, durability DurabilityMetrics, breakdown FormBreakdownResult, cadenceDropPct float64) string {
	driftText := "Cardiac drift remained controlled."
	if durability.DecouplingStartMinutes > 0 {
		driftText = "Cardiac drift began around " + formatApproxMinutes(durability.DecouplingStartMinutes) + "."
	}
	formText := "No major form breakdown detected."
	if breakdown.Detected {
		formText = "Form breakdown detected around " + formatApproxMinutes(breakdown.StartMin) + "."
	}
	cadText := "Cadence stayed stable."
	if cadenceDropPct >= 2 {
		cadText = "Cadence dropped by " + formatApproxPercent(cadenceDropPct) + " late in the run."
	}
	return "This was a " + strings.ToLower(classification) + ". " + driftText + " " + cadText + " " + formText
}

func SessionClassificationConfidence(a domain.Activity, hrZones [5]float64, classification string) float64 {
	totalMin := 0.0
	for _, z := range hrZones {
		totalMin += z
	}
	if totalMin <= 0 {
		return 0
	}
	z1 := hrZones[0] / totalMin * 100.0
	z2 := hrZones[1] / totalMin * 100.0
	z3 := hrZones[2] / totalMin * 100.0
	z4z5 := (hrZones[3] + hrZones[4]) / totalMin * 100.0
	z1z2 := z1 + z2
	durMin := a.Duration.Minutes()

	switch classification {
	case "Long Run":
		return clamp01(scoreFromMargins((durMin-90)/60.0, (z1z2-65)/20.0))
	case "Threshold / Interval":
		return clamp01(scoreFromMargins((z4z5-22)/15.0, (z3-15)/30.0))
	case "Tempo":
		return clamp01(scoreFromMargins((z3-36)/20.0, ((z3-z2)-5)/15.0))
	case "Recovery Run":
		return clamp01(scoreFromMargins((z1z2-82)/12.0, (55-durMin)/30.0))
	case "Easy Aerobic":
		return clamp01(scoreFromMargins((z1z2-70)/20.0, (z2-z3)/20.0))
	case "Steady Run":
		return 0.55
	default:
		return 0.3
	}
}

func scoreFromMargins(a, b float64) float64 {
	// Base confidence for satisfying class rules, then increase with margin.
	return 0.6 + 0.25*a + 0.15*b
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func nonZeroSlice(v []float64) []float64 {
	out := make([]float64, 0, len(v))
	for _, x := range v {
		if x > 0 {
			out = append(out, x)
		}
	}
	return out
}

func formatApproxMinutes(v float64) string {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return "n/a"
	}
	return strconv.Itoa(int(math.Round(v))) + " min"
}

func formatApproxPercent(v float64) string {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return "0%"
	}
	return strconv.FormatFloat(v, 'f', 1, 64) + "%"
}
