# Roadmap

## Done Recently

- [x] Strava OAuth + local token persistence
- [x] Strava activity cache with refresh indicator
- [x] Garmin FIT import tab with concurrent parsing + progress + dedupe
- [x] Dashboard source selector (`Strava` / `Garmin`)
- [x] Improved run metrics when power is missing (HR-based fallbacks)
- [x] Running Economy panel (cadence, vertical oscillation, vertical ratio)
- [x] Aerobic durability metrics:
  - time-to-decoupling
  - durability score
  - HR stability
- [x] Form breakdown detection (cadence down + HR up + pace down)
- [x] Session classification + confidence labels
- [x] Explain-the-run narrative in activity details

## Near Term

- [ ] Add unit tests for key metric engines (`NP`, `TSS`, `Decoupling`, `Durability`, `Classification`)
- [ ] Surface classification confidence details (why this label won)
- [ ] Add configurable classification thresholds in Settings (run/trail specific)
- [ ] Improve run explanation with clearer training-effect tags (base / tempo / threshold)
- [ ] Add durability trend view across recent runs
- [ ] Add support for Coros (fit files)
- [ ] Add support for Polar (fit files)

## Mid Term

- [ ] Terrain-aware metrics (GAP, uphill efficiency, downhill damage proxy)
- [ ] Better load analytics (monotony, strain, ramp rate)
- [ ] Ground contact time (GCT) + asymmetry support (when FIT data exists)
- [ ] Running Stress Score (RSS) for running power files
- [ ] Recovery index using resting HR/HRV FIT artifacts (if available)

## Longer Term

- [ ] Multi-athlete profiles
- [ ] Plugin system for custom metrics
- [ ] Export reports (Markdown/CSV)
- [ ] Optional web companion dashboard
