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
	Cadence   []float64
	// AltitudeM aligns with records when sourced from barometric altitude (Garmin FIT, meters).
	AltitudeM []float64
	// VerticalSpeedMS is vertical velocity from FIT when present (m/s, descending negative).
	VerticalSpeedMS []float64

	AvgCadence               float64
	AvgVerticalOscillationCM float64
	AvgStrideLengthM         float64

	// Ground contact mechanics when present on running watches (milliseconds, FIT stance_time).
	AvgStanceTimeMs float64
	// StrideAsymmetryPct is |right% - 50| from stride balance telemetry (0 = even split).
	StrideAsymmetryPct float64
}

type AthleteProfile struct {
	Name string
	FTP  float64
}
