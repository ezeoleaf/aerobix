// Package paths resolves per-profile storage under the user config directory.
// Layout (see ROADMAP / README):
//
//	~/.config/aerobix/
//	  config.json          # { "active_profile": "default" }
//	  profiles/
//	    <id>/
//	      data.json        # Strava tokens + athlete settings (was strava.json)
//	      cache.json       # activity cache (was activities_cache.json)
//	      garmin/          # default FIT import folder (created empty)
//
// Legacy files strava.json + activities_cache.json at aerobix root are migrated
// into profiles/default/ on first run.
package paths

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	pathpkg "path/filepath"
	"sort"
	"strings"
)

const (
	appConfigFile = "config.json"
	profilesDir   = "profiles"
	dataFile      = "data.json"
	cacheFile     = "cache.json"
	garminDir     = "garmin"
)

type appConfig struct {
	ActiveProfile string `json:"active_profile"`
}

// AerobixDir returns the app config root, e.g.
//   - Linux (default): ~/.config/aerobix
//   - macOS: ~/Library/Application Support/aerobix
//   - If XDG_CONFIG_HOME is set (any OS): $XDG_CONFIG_HOME/aerobix
func AerobixDir() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return pathpkg.Join(xdg, "aerobix"), nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return pathpkg.Join(base, "aerobix"), nil
}

// ListProfiles returns sorted profile folder names under profiles/.
// If the directory is missing, returns ["default"].
func ListProfiles() ([]string, error) {
	root, err := AerobixDir()
	if err != nil {
		return nil, err
	}
	dir := pathpkg.Join(root, profilesDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []string{"default"}, nil
		}
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() && strings.TrimSpace(e.Name()) != "" {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return []string{"default"}, nil
	}
	return ids, nil
}

// ProfileOrExplicit returns sanitized explicit id when non-empty; otherwise ActiveProfile().
func ProfileOrExplicit(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return sanitizeProfileID(explicit)
	}
	return ActiveProfile()
}

// MigrateLegacy moves legacy strava.json and activities_cache.json into profiles/default/
// if the profiles layout does not exist yet.
func MigrateLegacy() error {
	root, err := AerobixDir()
	if err != nil {
		return err
	}
	legacyStrava := pathpkg.Join(root, "strava.json")
	legacyCache := pathpkg.Join(root, "activities_cache.json")
	defaultProfile := pathpkg.Join(root, profilesDir, "default")

	if _, err := os.Stat(defaultProfile); err == nil {
		return nil // already migrated or user created profiles
	}

	if _, err := os.Stat(legacyStrava); errors.Is(err, fs.ErrNotExist) {
		// No legacy config — create empty profile shell + active pointer
		return bootstrapFresh(root)
	}

	if err := os.MkdirAll(pathpkg.Join(defaultProfile, garminDir), 0o755); err != nil {
		return err
	}
	newData := pathpkg.Join(defaultProfile, dataFile)
	newCache := pathpkg.Join(defaultProfile, cacheFile)

	if err := copyFileIfExists(legacyStrava, newData); err != nil {
		return err
	}
	if err := copyFileIfExists(legacyCache, newCache); err != nil {
		return err
	}

	ac := appConfig{ActiveProfile: "default"}
	return saveAppConfig(root, ac)
}

func bootstrapFresh(root string) error {
	defaultProfile := pathpkg.Join(root, profilesDir, "default")
	if err := os.MkdirAll(pathpkg.Join(defaultProfile, garminDir), 0o755); err != nil {
		return err
	}
	ac := appConfig{ActiveProfile: "default"}
	return saveAppConfig(root, ac)
}

func copyFileIfExists(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	return os.WriteFile(dst, b, 0o600)
}

func loadAppConfig(root string) (appConfig, error) {
	var ac appConfig
	p := pathpkg.Join(root, appConfigFile)
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return appConfig{ActiveProfile: "default"}, nil
		}
		return ac, err
	}
	if err := json.Unmarshal(b, &ac); err != nil {
		return ac, err
	}
	if strings.TrimSpace(ac.ActiveProfile) == "" {
		ac.ActiveProfile = "default"
	}
	return ac, nil
}

func saveAppConfig(root string, ac appConfig) error {
	ac.ActiveProfile = strings.TrimSpace(ac.ActiveProfile)
	if ac.ActiveProfile == "" {
		ac.ActiveProfile = "default"
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(ac, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(pathpkg.Join(root, appConfigFile), b, 0o600)
}

// ActiveProfile returns the profile id to use: AEROBIX_PROFILE env, then config.json, then "default".
func ActiveProfile() string {
	if v := strings.TrimSpace(os.Getenv("AEROBIX_PROFILE")); v != "" {
		return sanitizeProfileID(v)
	}
	root, err := AerobixDir()
	if err != nil {
		return "default"
	}
	ac, err := loadAppConfig(root)
	if err != nil {
		return "default"
	}
	return sanitizeProfileID(ac.ActiveProfile)
}

// SetActiveProfile persists the active profile id for the next launch (and current if no env override).
func SetActiveProfile(id string) error {
	root, err := AerobixDir()
	if err != nil {
		return err
	}
	id = sanitizeProfileID(id)
	if id == "" {
		id = "default"
	}
	ac, err := loadAppConfig(root)
	if err != nil {
		ac = appConfig{ActiveProfile: id}
	} else {
		ac.ActiveProfile = id
	}
	return saveAppConfig(root, ac)
}

func sanitizeProfileID(id string) string {
	id = strings.TrimSpace(id)
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" {
		return "default"
	}
	return s
}

// ProfileDir is ~/.config/aerobix/profiles/<id>.
func ProfileDir(profileID string) (string, error) {
	root, err := AerobixDir()
	if err != nil {
		return "", err
	}
	id := sanitizeProfileID(profileID)
	if id == "" {
		id = "default"
	}
	return pathpkg.Join(root, profilesDir, id), nil
}

// DataPath returns the Strava/settings JSON path for a profile.
func DataPath(profileID string) (string, error) {
	dir, err := ProfileDir(profileID)
	if err != nil {
		return "", err
	}
	return pathpkg.Join(dir, dataFile), nil
}

// CachePath returns the activity cache JSON path for a profile.
func CachePath(profileID string) (string, error) {
	dir, err := ProfileDir(profileID)
	if err != nil {
		return "", err
	}
	return pathpkg.Join(dir, cacheFile), nil
}

// GarminDir returns the default FIT directory for a profile (may not exist yet).
func GarminDir(profileID string) (string, error) {
	dir, err := ProfileDir(profileID)
	if err != nil {
		return "", err
	}
	return pathpkg.Join(dir, garminDir), nil
}

// EnsureProfileDirs creates profile folder + garmin subfolder.
func EnsureProfileDirs(profileID string) error {
	dir, err := ProfileDir(profileID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(pathpkg.Join(dir, garminDir), 0o755); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}
