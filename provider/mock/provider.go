package mock

import (
	"errors"
	"fmt"
	"time"

	"aerobix/domain"
	"aerobix/provider"
)

type Provider struct{}

func NewProvider() Provider {
	return Provider{}
}

func (p Provider) Name() string {
	return "Mock"
}

func (p Provider) AthleteProfile() domain.AthleteProfile {
	return domain.AthleteProfile{
		Name: "Hacker Athlete",
		FTP:  265,
	}
}

func (p Provider) RecentActivities(limit int) ([]domain.Activity, error) {
	now := time.Now()
	activities := []domain.Activity{
		newRide("strava-001", "Endurance Ride", "Strava", now.Add(-24*time.Hour), 95*time.Minute, 52.4),
		newRide("garmin-002", "Tempo Intervals", "Garmin", now.Add(-48*time.Hour), 78*time.Minute, 41.8),
		newRide("strava-003", "Long Gravel Session", "Strava", now.Add(-72*time.Hour), 130*time.Minute, 67.9),
		newRide("garmin-004", "Recovery Spin", "Garmin", now.Add(-96*time.Hour), 56*time.Minute, 28.1),
	}
	if limit > 0 && limit < len(activities) {
		activities = activities[:limit]
	}
	return activities, nil
}

func (p Provider) Settings() provider.Settings {
	return provider.Settings{
		AthleteName: "Hacker Athlete",
		FTP:         265,
		Age:         30,
		Configured:  true,
		Connected:   true,
	}
}

func (p Provider) UpdateSettings(_ provider.Settings) error {
	return nil
}

func (p Provider) AuthURL() (string, error) {
	return "", errors.New("mock provider does not support auth")
}

func (p Provider) ExchangeCode(_ string) error {
	return errors.New("mock provider does not support auth")
}

func newRide(id, name, source string, start time.Time, duration time.Duration, distanceKM float64) domain.Activity {
	samples := int(duration.Seconds() / 60)
	power := make([]float64, 0, samples)
	hr := make([]float64, 0, samples)
	timeSec := make([]int, 0, samples)

	for i := 0; i < samples; i++ {
		t := i * 60
		basePower := 175.0 + float64(i%35)
		baseHR := 136.0 + float64(i%9)

		// Introduce slight drift to emulate fatigue.
		driftFactor := 1.0 + (float64(i)/float64(samples))*0.06
		power = append(power, basePower)
		hr = append(hr, baseHR*driftFactor)
		timeSec = append(timeSec, t)
	}

	return domain.Activity{
		ID:         id,
		Name:       name,
		Source:     source,
		Sport:      "Ride",
		StartTime:  start,
		Duration:   duration,
		DistanceKM: distanceKM,
		Power:      power,
		HeartRate:  hr,
		TimeSec:    timeSec,
	}
}

func FormatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%02d:%02d", h, m)
}
