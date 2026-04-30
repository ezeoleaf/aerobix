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
	GarminFITDir string
	RunOnly      bool
	ClientID     string
	ClientSecret string
	Configured   bool
	Connected    bool
}

type FetchInfo struct {
	Source    string
	FetchedAt string
}

type DataProvider interface {
	Name() string
	AthleteProfile() domain.AthleteProfile
	RecentActivities(limit int, forceRefresh bool) ([]domain.Activity, error)
	Settings() Settings
	FetchInfo() FetchInfo
	UpdateSettings(Settings) error
	AuthURL() (string, error)
	ExchangeCode(code string) error
}
