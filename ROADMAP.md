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
- [x] **Load analytics (7d)** on dashboard: monotony, strain, CTL ramp — plus **weekly stress index (WSI)**, **ATL/CTL**, and **7d ΣRSS**
- [x] **Garmin FIT folder watcher** (`fsnotify`): dropping `.fit` files triggers debounced auto-import + best-effort **desktop notification** (macOS / Linux); queues safely if Strava or Garmin load is already in flight
- [x] **Terrain-aware metrics (v1)**: modeled **GAP**, uphill time share, downhill braking proxy from elevation / vertical-speed streams where present in FIT-derived activities
- [x] **Ground contact proxy + stride asymmetry** from FIT stance/balance telemetry when watches record it
- [x] **Running Stress Score-style load (RSS)** from running-power streams versus FTP-ref scale (sessions + weekly rollups where power exists)

## Near Term

- [~] Add unit tests for key metric engines (`NP`, `TSS`, `Decoupling`, `Durability`, `Classification`, load analytics) — baseline tests added for `NP`, `TSS`, and load analytics; expand coverage next
- [x] Surface classification confidence details (“why this label”)
- [ ] Configurable classification thresholds in Settings (run vs trail)
- [x] Hot profile switch (reload provider without restarting the app)
- [x] Durability trend view across recent runs
- [x] Add support for Coros (separate `coros/` FIT folder under profile)
- [x] Add support for Polar (separate `polar/` FIT folder under profile)

## Mid Term

Deeper polish on metrics already seeded above:

- [x] **Terrain (v1.1)**: refined grade model curve for uphill/downhill cost and braking penalty; still room for wind/surface and vendor GAP comparison
- [x] **Load analytics (v1.1)**: richer chronic-vs-acute views added (`Acute7`, `Chronic28`, `ACWR 7/28`, `28d ramp/week`) beyond ATL/CTL
- [x] **GCT / asymmetry (v1.1)**: FIT ingest now surfaces intra-run distributions (`p10/p50/p90`) for stance time and asymmetry
- [x] **RSS (v1.1)**: optional cadence-calibrated RSS scaling + source tags; vendor-alignment remains future refinement when explicit metadata is available
- [ ] Recovery index using resting HR / HRV when ingestible from FIT or wellness files

## Longer Term

- [ ] Optional **SQLite** (or embedded DB) if we need cross-profile analytics, full-text search on activities, or sync — **JSON-on-disk is enough** until then
- [ ] Plugin system for custom metrics
- [ ] Export reports (Markdown/CSV)
- [ ] Optional web companion dashboard

### Note on multi-athlete storage

Per-athlete folders under `~/.config/aerobix/profiles/` with `data.json`, `cache.json`, and vendor subfolders (`garmin/`, later `coros/`, `polar/`) match how a local CLI app should stay portable and git-friendly. A database is **not required** until we need complex queries or multi-device sync; migrating JSON → SQLite later is straightforward if profiles stay one-folder-per-athlete.
