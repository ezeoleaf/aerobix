package physics

import "math"

// LoadAnalytics7d summarizes the last 7 calendar days of daily TSS.
type LoadAnalytics7d struct {
	Monotony   float64 // >1.2 often flagged as "monotonous" training
	Strain     float64 // Foster-style fatigue index: Σ daily TSS * monotony
	RampPerDay float64 // CTL change per day over the last 7d (using PMC tail)
	// WeeklyStressIndex folds monotony into a single week stress scalar (ΔCTL scaled).
	WeeklyStressIndex float64
	ATLCTL            float64 // acute/chronic workload ratio from latest PMC snapshot
	Acute7dAvg        float64 // rolling acute load proxy from daily TSS
	Chronic28dAvg     float64 // rolling chronic load proxy from daily TSS
	Acwr7Over28       float64 // acute/chronic ratio over 7d vs 28d windows
	Ramp28dPerWeek    float64 // CTL change rate over the last 28 days
	RSS7dSum          float64 // optional running squared-power load summed over 7d (see UI)
	HasData           bool
}

// LoadAnalyticsFromPMC computes monotony/strain from the last 7 days of daily TSS
// embedded in PMC points, and ramp as (CTL_today - CTL_7d_ago) / 7.
func LoadAnalyticsFromPMC(pmc []PMCPoint) LoadAnalytics7d {
	var out LoadAnalytics7d
	if len(pmc) == 0 {
		return out
	}
	today := pmc[len(pmc)-1].Date
	cutoff := today.AddDate(0, 0, -6)
	var daily []float64
	for _, p := range pmc {
		if !p.Date.Before(cutoff) {
			daily = append(daily, p.DailyTSS)
		}
	}
	if len(daily) == 0 {
		return out
	}
	sum := 0.0
	for _, v := range daily {
		sum += v
	}
	mean := sum / float64(len(daily))
	var varSum float64
	for _, v := range daily {
		d := v - mean
		varSum += d * d
	}
	std := math.Sqrt(varSum / float64(len(daily)))
	monotony := 0.0
	if std > 1e-6 {
		monotony = mean / std
	}
	strain := sum * monotony
	wsi := 0.0
	if len(daily) > 1 {
		// Lightweight stress composite: variability of daily dose + average load.
		dmax := daily[0]
		dmin := daily[0]
		for _, v := range daily {
			if v > dmax {
				dmax = v
			}
			if v < dmin {
				dmin = v
			}
		}
		wsi = sum * (1 + (dmax-dmin)/(mean+5))
	} else {
		wsi = sum
	}
	ramp := 0.0
	if len(pmc) >= 8 {
		now := pmc[len(pmc)-1].CTL
		past := pmc[len(pmc)-8].CTL
		ramp = (now - past) / 7.0
	}
	acwr := 0.0
	if len(pmc) > 0 {
		last := pmc[len(pmc)-1]
		if last.CTL > 1e-3 {
			acwr = last.ATL / last.CTL
		}
	}
	acute7, chronic28, acwr7over28 := rollingAcuteChronic(pmc)
	ramp28 := 0.0
	if len(pmc) >= 29 {
		now := pmc[len(pmc)-1].CTL
		past := pmc[len(pmc)-29].CTL
		ramp28 = (now - past) / 4.0
	}
	out.Monotony = monotony
	out.Strain = strain
	out.RampPerDay = ramp
	out.WeeklyStressIndex = wsi
	out.ATLCTL = acwr
	out.Acute7dAvg = acute7
	out.Chronic28dAvg = chronic28
	out.Acwr7Over28 = acwr7over28
	out.Ramp28dPerWeek = ramp28
	out.HasData = true
	return out
}

func rollingAcuteChronic(pmc []PMCPoint) (acute7d, chronic28d, ratio float64) {
	if len(pmc) == 0 {
		return 0, 0, 0
	}
	n7 := minInt(7, len(pmc))
	n28 := minInt(28, len(pmc))
	var s7, s28 float64
	for i := len(pmc) - n7; i < len(pmc); i++ {
		s7 += pmc[i].DailyTSS
	}
	for i := len(pmc) - n28; i < len(pmc); i++ {
		s28 += pmc[i].DailyTSS
	}
	acute7d = s7 / float64(n7)
	chronic28d = s28 / float64(n28)
	if chronic28d > 1e-3 {
		ratio = acute7d / chronic28d
	}
	return acute7d, chronic28d, ratio
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
