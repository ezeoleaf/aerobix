package provider

import "aerobix/domain"

type Settings struct {
	AthleteName  string
	FTP          float64
	Age          int
	MaxHeartRate float64
	HRZone1Max   float64
	HRZone2Max   float64
	HRZone3Max   float64
	HRZone4Max   float64
	ClientID     string
	ClientSecret string
	Configured   bool
	Connected    bool
}

type DataProvider interface {
	Name() string
	AthleteProfile() domain.AthleteProfile
	RecentActivities(limit int) ([]domain.Activity, error)
	Settings() Settings
	UpdateSettings(Settings) error
	AuthURL() (string, error)
	ExchangeCode(code string) error
}
