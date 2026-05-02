package physics

import "math"

// LoadAnalytics7d summarizes the last 7 calendar days of daily TSS.
type LoadAnalytics7d struct {
	Monotony   float64 // >1.2 often flagged as "monotonous" training
	Strain     float64 // weekly load * monotony-style factor
	RampPerDay float64 // CTL change per day over the last 7d (using PMC tail)
	HasData    bool
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
	// Strain: week load scaled by monotony (Foster-style strain ≈ week load * monotony)
	strain := sum * monotony
	ramp := 0.0
	if len(pmc) >= 8 {
		now := pmc[len(pmc)-1].CTL
		past := pmc[len(pmc)-8].CTL
		ramp = (now - past) / 7.0
	}
	out.Monotony = monotony
	out.Strain = strain
	out.RampPerDay = ramp
	out.HasData = true
	return out
}
