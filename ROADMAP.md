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

- [ ] Add unit tests for key metric engines (`NP`, `TSS`, `Decoupling`, `Durability`, `Classification`, load analytics)
- [ ] Surface classification confidence details (“why this label”)
- [ ] Configurable classification thresholds in Settings (run vs trail)
- [ ] Hot profile switch (reload provider without restarting the app)
- [ ] Durability trend view across recent runs
- [ ] Add support for Coros (reuse FIT folder under profile)
- [ ] Add support for Polar (reuse FIT folder under profile)

## Mid Term

Deeper polish on metrics already seeded above:

- [ ] **Terrain**: refine grade model (wind, surface), compare to vendor GAP where available
- [ ] **Load analytics**: revisit monotony/strain formulations; richer chronic-vs-acute views (rolling windows beyond ATL/CTL)
- [ ] **GCT / asymmetry**: parse more vendor-specific FIT messages; intra-run distributions, not just session averages
- [ ] **RSS**: optional device-calibrated cadence/power scaling; align with vendor RSS when identifiable in FIT metadata
- [ ] Recovery index using resting HR / HRV when ingestible from FIT or wellness files

## Longer Term

- [ ] Optional **SQLite** (or embedded DB) if we need cross-profile analytics, full-text search on activities, or sync — **JSON-on-disk is enough** until then
- [ ] Plugin system for custom metrics
- [ ] Export reports (Markdown/CSV)
- [ ] Optional web companion dashboard

### Note on multi-athlete storage

Per-athlete folders under `~/.config/aerobix/profiles/` with `data.json`, `cache.json`, and vendor subfolders (`garmin/`, later `coros/`, `polar/`) match how a local CLI app should stay portable and git-friendly. A database is **not required** until we need complex queries or multi-device sync; migrating JSON → SQLite later is straightforward if profiles stay one-folder-per-athlete.
