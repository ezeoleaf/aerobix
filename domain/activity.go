package domain

import "time"

type Activity struct {
	ID         string
	Name       string
	Source     string
	Sport      string
	StartTime  time.Time
	Duration   time.Duration
	DistanceKM float64

	Power     []float64
	HeartRate []float64
	TimeSec   []int
	SpeedMS   []float64
}

type AthleteProfile struct {
	Name string
	FTP  float64
}
