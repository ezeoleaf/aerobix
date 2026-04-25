# Aerobix

A terminal-first training analytics app for endurance athletes.

<img width="1704" height="954" alt="image" src="https://github.com/user-attachments/assets/c4840348-2e10-45f1-9ca6-1b530b3fcd62" />

Aerobix is a Go TUI built with Bubble Tea + Lip Gloss that fetches activity data (currently Strava), computes training metrics, and visualizes your fitness trends directly in the terminal.

## Features

- Top-tab navigation (`Dashboard`, `Activities`, `Garmin (Beta)`, `Settings`)
- Strava OAuth integration with local token persistence
- Local activity caching (instant startup if cache exists)
- Mock provider fallback for offline/dev usage
- Core performance metrics:
  - Normalized Power (NP)
  - Efficiency Factor (EF speed/HR)
  - Intensity Factor (IF)
  - Training Stress Score (TSS)
  - TRIMP (HR-zone weighted training impulse)
  - Critical Speed (CS) + D' for running
  - Aerobic Decoupling (Pw:HR)
  - CTL / ATL / TSB (Performance Management Chart model)
- Activity details:
  - Sparkline (power or HR depending on available data)
  - Time in zones (power-based) + HR zones for runs
- Garmin (Beta):
  - Import local `.fit` files from a folder
  - Parse activities concurrently and display in dedicated tab
- Keyboard-driven settings form (including paste support)

## Metrics Notes

- **NP (Normalized Power)**: captures metabolic cost better than plain average power by weighting harder efforts more heavily.  
  Formula: 30s rolling average -> 4th power average -> 4th root.
- **IF (Intensity Factor)**: how hard a session is relative to your FTP.  
  `IF = NP / FTP` (example: 0.80 = moderate aerobic, 1.00 = FTP effort).
- **TSS (Training Stress Score)**: total session load combining duration and intensity.  
  `TSS = ((sec * NP * IF) / (FTP * 3600)) * 100`  
  Rule of thumb: ~100 TSS is close to 1 hour at FTP.
- **EF (Efficiency Factor)**: aerobic efficiency proxy, usually `NP / Avg HR`.
- **EF (speed/HR)**: average speed divided by average HR.  
  Track trend over time: rising EF at similar conditions often means improved aerobic efficiency.
- **Heart Rate Zones**:
  - If custom HR zone bounds are set in Settings, Aerobix uses those.
  - Otherwise, it estimates zones from `220-age` (or Max HR override) with 60/70/80/90% splits.
- **Decoupling**:
  - Detects a steady-state segment first
  - Compares `(avg power / avg HR)` first half vs second half  
  Rule of thumb: `<5%` strong aerobic durability, `5-10%` moderate drift, `>10%` significant drift.
- **CTL/ATL/TSB**:
  - EWMA over 42d (CTL) and 7d (ATL), `TSB = CTL - ATL`
  - CTL = long-term fitness, ATL = short-term fatigue, TSB = readiness/form
- **TRIMP**:
  - HR-zone weighted internal load (not just distance/time).
  - Useful when power data is absent (many run sessions).
- **Critical Speed (CS) / D'**:
  - Running analog to FTP derived from best sustained speeds.
  - Aerobix estimates from best 3min and 9min efforts across your run set.
 
## Look
<img width="865" height="670" alt="image" src="https://github.com/user-attachments/assets/d0157b24-a915-4166-841c-048467260e7a" />  \
<img width="841" height="695" alt="image" src="https://github.com/user-attachments/assets/58c306b8-98f2-42a3-a007-2ef4871e15ca" />  \
<img width="1060" height="679" alt="image" src="https://github.com/user-attachments/assets/4b042b8d-b0c6-4632-8b01-2e7d556c7790" />


## Quick Start

### Prerequisites

- Go 1.26+
- Strava API app credentials (Client ID + Client Secret) if using live data

### Run

```bash
go mod tidy
go run .
```

If Strava provider cannot initialize, Aerobix falls back to mock data.

## Activity Caching

- On startup, Aerobix uses cached activities when available.
- Fresh Strava fetch happens when:
  - cache is missing, or
  - you press `r` to reload.
- Cache file:
  - macOS/Linux: `~/.config/aerobix/activities_cache.json`

## Strava Setup

1. Create a Strava API application.
2. Run Aerobix and open `Settings`.
3. Fill:
   - `Client ID`
   - `Client Secret`
   - Optional HR profile fields:
     - `Age`
     - `Max HR override`
     - `HR Z1-Z4 max bpm` (for fully custom personal zones)
4. Press `s` to save.
5. Press `a` to open Strava auth page.
6. Approve access and copy the `code` query parameter from the redirect URL.
7. Paste code into `Auth Code` field.
8. Press `x` to exchange code and load activities.

Strava config/tokens are stored at:

- macOS/Linux: `~/.config/aerobix/strava.json`

## Keybindings

- Global:
  - `q` quit
  - `r` reload activities
  - `h`/`l` or left/right arrows: switch sidebar section
- Activities:
  - `j`/`k` or down/up arrows: move selection
- Settings:
  - `e` edit selected field
  - `s` save settings
  - `a` open auth URL
  - `x` exchange auth code
  - in edit mode: `Enter` save field, `Esc` cancel
- Garmin (Beta):
  - `g` import `.fit` files from `Garmin FIT dir`

## Current Project Layout

```text
.
├── domain/
├── physics/
├── provider/
│   ├── mock/
│   └── strava/
├── ui/
├── main.go
├── go.mod
└── go.sum
```

## Roadmap

See `ROADMAP.md`.

## Contributing

See `CONTRIBUTING.md`.
