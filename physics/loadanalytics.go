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
	out.Monotony = monotony
	out.Strain = strain
	out.RampPerDay = ramp
	out.WeeklyStressIndex = wsi
	out.ATLCTL = acwr
	out.HasData = true
	return out
}
