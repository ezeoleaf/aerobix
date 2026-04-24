package physics

import "time"

type DailyLoad struct {
	Date time.Time
	TSS  float64
}

type PMCPoint struct {
	Date time.Time
	CTL  float64
	ATL  float64
	TSB  float64
}

func PerformanceManagement(loads []DailyLoad) []PMCPoint {
	if len(loads) == 0 {
		return nil
	}

	points := make([]PMCPoint, 0, len(loads))
	var ctl float64
	var atl float64
	ctlAlpha := 2.0 / (42.0 + 1.0)
	atlAlpha := 2.0 / (7.0 + 1.0)

	for _, l := range loads {
		ctl = ctl + ctlAlpha*(l.TSS-ctl)
		atl = atl + atlAlpha*(l.TSS-atl)
		points = append(points, PMCPoint{
			Date: l.Date,
			CTL:  ctl,
			ATL:  atl,
			TSB:  ctl - atl,
		})
	}
	return points
}
