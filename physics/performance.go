package physics

import (
	"math"
	"time"
)

type DailyLoad struct {
	Date time.Time
	TSS  float64
}

type PMCPoint struct {
	Date     time.Time
	DailyTSS float64
	CTL      float64
	ATL      float64
	TSB      float64
}

func PerformanceManagement(loads []DailyLoad) []PMCPoint {
	if len(loads) == 0 {
		return nil
	}

	loc := time.Local
	tssByDate := getTssForDate(loads, loc)
	firstDate := firstActivityDate(loads, loc)
	today := normalizeDate(time.Now().In(loc))

	ctlDecay := math.Exp(-1.0 / 42.0)
	atlDecay := math.Exp(-1.0 / 7.0)

	points := make([]PMCPoint, 0, int(today.Sub(firstDate).Hours()/24)+1)
	var ctlYesterday float64
	var atlYesterday float64

	for d := firstDate; !d.After(today); d = d.AddDate(0, 0, 1) {
		tss := tssByDate[d.Format("2006-01-02")]
		tsbToday := ctlYesterday - atlYesterday
		ctlToday := tss*(1.0-ctlDecay) + ctlYesterday*ctlDecay
		atlToday := tss*(1.0-atlDecay) + atlYesterday*atlDecay

		points = append(points, PMCPoint{
			Date:     d,
			DailyTSS: tss,
			CTL:      ctlToday,
			ATL:      atlToday,
			TSB:      tsbToday,
		})
		ctlYesterday = ctlToday
		atlYesterday = atlToday
	}
	return points
}

func getTssForDate(loads []DailyLoad, loc *time.Location) map[string]float64 {
	tss := make(map[string]float64, len(loads))
	for _, l := range loads {
		d := normalizeDate(l.Date.In(loc))
		key := d.Format("2006-01-02")
		tss[key] += l.TSS
	}
	return tss
}

func firstActivityDate(loads []DailyLoad, loc *time.Location) time.Time {
	first := normalizeDate(loads[0].Date.In(loc))
	for _, l := range loads[1:] {
		d := normalizeDate(l.Date.In(loc))
		if d.Before(first) {
			first = d
		}
	}
	return first
}

func normalizeDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
