package physics

import (
	"math"
	"testing"
	"time"
)

func TestNormalizedPowerFromTime_ConstantPower(t *testing.T) {
	p := make([]float64, 120)
	ts := make([]int, 120)
	for i := range p {
		p[i] = 200
		ts[i] = i
	}
	np, err := NormalizedPowerFromTime(p, ts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if math.Abs(np-200) > 0.5 {
		t.Fatalf("expected np≈200, got %.2f", np)
	}
}

func TestTrainingStressScore_OneHourAtThreshold(t *testing.T) {
	const (
		ftp = 250.0
		np  = 250.0
		ifv = 1.0
		sec = 3600
	)
	tss, err := TrainingStressScore(sec, np, ifv, ftp)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if math.Abs(tss-100.0) > 0.5 {
		t.Fatalf("expected tss≈100, got %.2f", tss)
	}
}

func TestLoadAnalyticsFromPMC_RollingFields(t *testing.T) {
	now := time.Now()
	pmc := make([]PMCPoint, 0, 35)
	for i := 34; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		daily := 60.0
		if i < 7 {
			daily = 85.0
		}
		pmc = append(pmc, PMCPoint{
			Date:     day,
			DailyTSS: daily,
			CTL:      40 + float64(34-i)*0.6,
			ATL:      50 + float64(34-i)*0.8,
		})
	}
	la := LoadAnalyticsFromPMC(pmc)
	if !la.HasData {
		t.Fatalf("expected HasData=true")
	}
	if la.Acute7dAvg <= la.Chronic28dAvg {
		t.Fatalf("expected acute > chronic, got acute %.2f chronic %.2f", la.Acute7dAvg, la.Chronic28dAvg)
	}
	if la.Acwr7Over28 <= 1.0 {
		t.Fatalf("expected acwr > 1, got %.2f", la.Acwr7Over28)
	}
}
