# Roadmap

## Done Recently

- [x] Strava OAuth + local token persistence
- [x] Strava activity cache with refresh indicator
- [x] Garmin FIT import tab with concurrent parsing + progress + dedupe
- [x] Dashboard source selector (`Strava` / `Garmin`)
- [x] Improved run metrics when power is missing (HR-based fallbacks)
- [x] Running Economy panel (cadence, vertical oscillation, vertical ratio)
- [x] Aerobic durability metrics (time-to-decoupling, durability score, HR stability)
- [x] Form breakdown detection
- [x] Session classification + confidence + explain-the-run
- [x] Run-only filter + dashboard deltas after reload
- [x] **Multi-profile filesystem layout** (`profiles/<id>/data.json`, `cache.json`, `garmin/`) + legacy migration + `AEROBIX_PROFILE` + Settings profile id (restart to apply)
- [x] **Load analytics (7d)** on dashboard: monotony, strain, CTL ramp

## Near Term

- [ ] Add unit tests for key metric engines (`NP`, `TSS`, `Decoupling`, `Durability`, `Classification`, load analytics)
- [ ] Surface classification confidence details (“why this label”)
- [ ] Configurable classification thresholds in Settings (run vs trail)
- [ ] Hot profile switch (reload provider without restarting the app)
- [ ] Durability trend view across recent runs
- [ ] Add support for Coros (reuse FIT folder under profile)
- [ ] Add support for Polar (reuse FIT folder under profile)

## Mid Term

Detailed tracking for previously listed capabilities:

- [ ] **Terrain-aware metrics**: grade-adjusted pace (GAP), uphill efficiency, downhill eccentric-load proxy from elevation streams
- [ ] **Better load analytics**: refine monotony/strain formulas, weekly training stress index, chronic workload comparisons
- [ ] **Ground contact time (GCT) + asymmetry** from FIT records when present (`stance_time`, balance fields / vendor-specific)
- [ ] **Running Stress Score (RSS)** when running power is available (Stryd / Garmin Running Power): duration × intensity² style load
- [ ] Recovery index using resting HR / HRV when ingestible from FIT or wellness files

## Longer Term

- [ ] Optional **SQLite** (or embedded DB) if we need cross-profile analytics, full-text search on activities, or sync — **JSON-on-disk is enough** until then
- [ ] Plugin system for custom metrics
- [ ] Export reports (Markdown/CSV)
- [ ] Optional web companion dashboard

### Note on multi-athlete storage

Per-athlete folders under `~/.config/aerobix/profiles/` with `data.json`, `cache.json`, and vendor subfolders (`garmin/`, later `coros/`, `polar/`) match how a local CLI app should stay portable and git-friendly. A database is **not required** until we need complex queries or multi-device sync; migrating JSON → SQLite later is straightforward if profiles stay one-folder-per-athlete.
