package strava

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"aerobix/domain"
	"aerobix/provider"
)

const (
	baseURL     = "https://www.strava.com/api/v3"
	oauthURL    = "https://www.strava.com/oauth/authorize"
	tokenURL    = "https://www.strava.com/oauth/token"
	redirectURI = "http://localhost/exchange_token"
)

type config struct {
	AthleteName  string  `json:"athlete_name"`
	FTP          float64 `json:"ftp"`
	Age          int     `json:"age"`
	MaxHeartRate float64 `json:"max_heart_rate"`
	HRZone1Max   float64 `json:"hr_zone_1_max"`
	HRZone2Max   float64 `json:"hr_zone_2_max"`
	HRZone3Max   float64 `json:"hr_zone_3_max"`
	HRZone4Max   float64 `json:"hr_zone_4_max"`
	ClientID     string  `json:"client_id"`
	ClientSecret string  `json:"client_secret"`
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	ExpiresAt    int64   `json:"expires_at"`
}

type Provider struct {
	cfgPath string
	http    *http.Client
	cfg     config
}

func NewProvider() (Provider, error) {
	cfgPath, err := configPath()
	if err != nil {
		return Provider{}, err
	}
	p := Provider{
		cfgPath: cfgPath,
		http:    &http.Client{Timeout: 20 * time.Second},
		cfg: config{
			AthleteName: "Hacker Athlete",
			FTP:         265,
			Age:         30,
		},
	}
	_ = p.load()
	return p, nil
}

func (p Provider) Name() string { return "Strava" }

func (p Provider) AthleteProfile() domain.AthleteProfile {
	return domain.AthleteProfile{Name: p.cfg.AthleteName, FTP: p.cfg.FTP}
}

func (p Provider) Settings() provider.Settings {
	return provider.Settings{
		AthleteName:  p.cfg.AthleteName,
		FTP:          p.cfg.FTP,
		Age:          p.cfg.Age,
		MaxHeartRate: p.cfg.MaxHeartRate,
		HRZone1Max:   p.cfg.HRZone1Max,
		HRZone2Max:   p.cfg.HRZone2Max,
		HRZone3Max:   p.cfg.HRZone3Max,
		HRZone4Max:   p.cfg.HRZone4Max,
		ClientID:     p.cfg.ClientID,
		ClientSecret: p.cfg.ClientSecret,
		Configured:   p.cfg.ClientID != "" && p.cfg.ClientSecret != "",
		Connected:    p.cfg.AccessToken != "",
	}
}

func (p *Provider) UpdateSettings(s provider.Settings) error {
	p.cfg.AthleteName = s.AthleteName
	p.cfg.FTP = s.FTP
	p.cfg.Age = s.Age
	p.cfg.MaxHeartRate = s.MaxHeartRate
	p.cfg.HRZone1Max = s.HRZone1Max
	p.cfg.HRZone2Max = s.HRZone2Max
	p.cfg.HRZone3Max = s.HRZone3Max
	p.cfg.HRZone4Max = s.HRZone4Max
	p.cfg.ClientID = strings.TrimSpace(s.ClientID)
	p.cfg.ClientSecret = strings.TrimSpace(s.ClientSecret)
	return p.save()
}

func (p Provider) AuthURL() (string, error) {
	if p.cfg.ClientID == "" {
		return "", errors.New("missing client id")
	}
	v := url.Values{}
	v.Set("client_id", p.cfg.ClientID)
	v.Set("response_type", "code")
	v.Set("redirect_uri", redirectURI)
	v.Set("approval_prompt", "auto")
	v.Set("scope", "read,activity:read_all")
	return oauthURL + "?" + v.Encode(), nil
}

func (p *Provider) ExchangeCode(code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return errors.New("empty auth code")
	}
	if p.cfg.ClientID == "" || p.cfg.ClientSecret == "" {
		return errors.New("configure client id and secret first")
	}

	body := url.Values{}
	body.Set("client_id", p.cfg.ClientID)
	body.Set("client_secret", p.cfg.ClientSecret)
	body.Set("code", code)
	body.Set("grant_type", "authorization_code")

	req, _ := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("token exchange failed: %s", resp.Status)
	}

	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    int64  `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	p.cfg.AccessToken = out.AccessToken
	p.cfg.RefreshToken = out.RefreshToken
	p.cfg.ExpiresAt = out.ExpiresAt
	return p.save()
}

func (p *Provider) RecentActivities(limit int) ([]domain.Activity, error) {
	if p.cfg.AccessToken == "" {
		return nil, errors.New("not connected: get auth code in Settings and press x")
	}
	if limit <= 0 {
		limit = 20
	}
	if err := p.ensureFreshToken(); err != nil {
		return nil, err
	}

	u := fmt.Sprintf("%s/athlete/activities?per_page=%d&page=1", baseURL, limit)
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Authorization", "Bearer "+p.cfg.AccessToken)
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("activities request failed: %s", resp.Status)
	}

	var raw []struct {
		ID           int64   `json:"id"`
		Name         string  `json:"name"`
		SportType    string  `json:"sport_type"`
		StartDate    string  `json:"start_date"`
		ElapsedTime  int     `json:"elapsed_time"`
		Distance     float64 `json:"distance"`
		AverageWatts float64 `json:"average_watts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	activities := make([]domain.Activity, 0, len(raw))
	for _, a := range raw {
		start, _ := time.Parse(time.RFC3339, a.StartDate)
		stream, _ := p.fetchStreams(a.ID)
		if len(stream.TimeSec) == 0 {
			stream = syntheticStreams(a.ElapsedTime, a.AverageWatts)
		}
		activities = append(activities, domain.Activity{
			ID:         strconv.FormatInt(a.ID, 10),
			Name:       a.Name,
			Source:     "Strava",
			Sport:      a.SportType,
			StartTime:  start,
			Duration:   time.Duration(a.ElapsedTime) * time.Second,
			DistanceKM: a.Distance / 1000.0,
			Power:      stream.Power,
			HeartRate:  stream.HeartRate,
			TimeSec:    stream.TimeSec,
		})
	}
	return activities, nil
}

type streamData struct {
	Power     []float64
	HeartRate []float64
	TimeSec   []int
}

func (p *Provider) fetchStreams(activityID int64) (streamData, error) {
	var out streamData
	u := fmt.Sprintf("%s/activities/%d/streams?keys=time,watts,heartrate&key_by_type=true", baseURL, activityID)
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Authorization", "Bearer "+p.cfg.AccessToken)
	resp, err := p.http.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return out, fmt.Errorf("stream request failed: %s", resp.Status)
	}

	var raw map[string]struct {
		Data []float64 `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return out, err
	}
	timeF := raw["time"].Data
	watts := raw["watts"].Data
	hr := raw["heartrate"].Data

	n := len(timeF)
	if n == 0 {
		return out, nil
	}
	out.Power = make([]float64, n)
	out.HeartRate = make([]float64, n)
	out.TimeSec = make([]int, n)
	for i := 0; i < n; i++ {
		out.TimeSec[i] = int(timeF[i])
		if i < len(watts) {
			out.Power[i] = watts[i]
		}
		if i < len(hr) {
			out.HeartRate[i] = hr[i]
		}
	}
	return out, nil
}

func syntheticStreams(elapsed int, avgWatts float64) streamData {
	if elapsed < 1800 {
		elapsed = 1800
	}
	if avgWatts <= 0 {
		avgWatts = 180
	}
	samples := elapsed / 60
	p := make([]float64, 0, samples)
	hr := make([]float64, 0, samples)
	t := make([]int, 0, samples)
	for i := 0; i < samples; i++ {
		pwr := avgWatts + float64((i%10)-5)
		heart := 132.0 + float64(i%8) + float64(i)*0.02
		p = append(p, pwr)
		hr = append(hr, heart)
		t = append(t, i*60)
	}
	return streamData{Power: p, HeartRate: hr, TimeSec: t}
}

func (p *Provider) ensureFreshToken() error {
	if p.cfg.RefreshToken == "" || p.cfg.ExpiresAt > time.Now().Add(2*time.Minute).Unix() {
		return nil
	}
	body := url.Values{}
	body.Set("client_id", p.cfg.ClientID)
	body.Set("client_secret", p.cfg.ClientSecret)
	body.Set("grant_type", "refresh_token")
	body.Set("refresh_token", p.cfg.RefreshToken)

	req, _ := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("token refresh failed: %s", resp.Status)
	}

	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    int64  `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	p.cfg.AccessToken = out.AccessToken
	p.cfg.RefreshToken = out.RefreshToken
	p.cfg.ExpiresAt = out.ExpiresAt
	return p.save()
}

func (p *Provider) load() error {
	b, err := os.ReadFile(p.cfgPath)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &p.cfg)
}

func (p *Provider) save() error {
	if err := os.MkdirAll(filepath.Dir(p.cfgPath), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p.cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.cfgPath, b, 0o600)
}

func configPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "aerobix", "strava.json"), nil
}
