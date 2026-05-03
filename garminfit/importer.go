package garminfit

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"aerobix/domain"

	"github.com/tormoder/fit"
)

type parseResult struct {
	activity domain.Activity
	err      error
}

type LoadResult struct {
	Activities []domain.Activity
	TotalFiles int
	Imported   int
	Failed     int
	Deduped    int
}

type Progress struct {
	Stage     string
	Total     int
	Processed int
	Parsed    int
	Failed    int
	Done      bool
}

func LoadActivitiesFromDir(dir string, onProgress func(Progress)) (LoadResult, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return LoadResult{}, errors.New("garmin fit directory is empty")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return LoadResult{}, err
	}
	if !info.IsDir() {
		return LoadResult{}, fmt.Errorf("not a directory: %s", dir)
	}

	paths, err := collectFITFiles(dir)
	if err != nil {
		return LoadResult{}, err
	}
	if len(paths) == 0 {
		return LoadResult{}, fmt.Errorf("no .fit files found in %s", dir)
	}
	if onProgress != nil {
		onProgress(Progress{Stage: "scanning", Total: len(paths)})
	}

	jobs := make(chan string, len(paths))
	results := make(chan parseResult, len(paths))
	workers := minInt(runtime.NumCPU(), 8)
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				a, err := parseFITFile(p)
				results <- parseResult{activity: a, err: err}
			}
		}()
	}
	for _, p := range paths {
		jobs <- p
	}
	close(jobs)
	wg.Wait()
	close(results)

	activities := make([]domain.Activity, 0, len(paths))
	result := LoadResult{
		TotalFiles: len(paths),
	}
	var firstErr error
	for r := range results {
		result.Imported++
		if r.err != nil {
			result.Failed++
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		activities = append(activities, r.activity)
		if onProgress != nil {
			onProgress(Progress{
				Stage:     "parsing",
				Total:     result.TotalFiles,
				Processed: result.Imported,
				Parsed:    len(activities),
				Failed:    result.Failed,
			})
		}
	}
	if len(activities) == 0 && firstErr != nil {
		return LoadResult{}, firstErr
	}

	activities, deduped := dedupeActivities(activities)
	result.Deduped = deduped
	result.Activities = activities
	if onProgress != nil {
		onProgress(Progress{
			Stage:     "done",
			Total:     result.TotalFiles,
			Processed: result.Imported,
			Parsed:    len(result.Activities),
			Failed:    result.Failed,
			Done:      true,
		})
	}

	sort.Slice(result.Activities, func(i, j int) bool {
		return result.Activities[i].StartTime.After(result.Activities[j].StartTime)
	})
	return result, nil
}

func parseFITFile(path string) (domain.Activity, error) {
	f, err := os.Open(path)
	if err != nil {
		return domain.Activity{}, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to close file %s: %v\n", path, err)
		}
	}()

	file, err := fit.Decode(f)
	if err != nil {
		return domain.Activity{}, err
	}
	activity, err := file.Activity()
	if err != nil {
		return domain.Activity{}, err
	}

	id := filepath.Base(path)
	name := trimExt(id)
	start := time.Now()
	distanceKM := 0.0
	duration := 0 * time.Second
	sport := "Run"

	if len(activity.Sessions) > 0 {
		s := activity.Sessions[0]
		if !s.StartTime.IsZero() {
			start = s.StartTime
		}
		if s.TotalDistance != math.MaxUint32 {
			// FIT total_distance is meters with scale 100.
			distanceKM = (float64(s.TotalDistance) / 100.0) / 1000.0
		}
		if s.TotalElapsedTime > 0 && s.TotalElapsedTime != math.MaxUint32 {
			// FIT total_elapsed_time is seconds with scale 1000.
			seconds := float64(s.TotalElapsedTime) / 1000.0
			duration = time.Duration(seconds * float64(time.Second))
		}
		if s.Sport.String() != "" {
			sport = s.Sport.String()
		}
	}

	power := make([]float64, 0, len(activity.Records))
	hr := make([]float64, 0, len(activity.Records))
	timeSec := make([]int, 0, len(activity.Records))
	speed := make([]float64, 0, len(activity.Records))
	cadence := make([]float64, 0, len(activity.Records))
	altitude := make([]float64, 0, len(activity.Records))
	verticalSpeed := make([]float64, 0, len(activity.Records))
	cadenceSamples := make([]float64, 0, len(activity.Records))
	voSamplesCM := make([]float64, 0, len(activity.Records))
	strideSamplesM := make([]float64, 0, len(activity.Records))
	var stanceVals []float64
	var lateralVals []float64

	var firstTS time.Time
	for i, rr := range activity.Records {
		if rr == nil {
			continue
		}
		r := rr
		if i == 0 {
			firstTS = r.Timestamp
		}
		delta := int(r.Timestamp.Sub(firstTS).Seconds())
		if delta < 0 {
			delta = i
		}
		power = append(power, sanitizeUint16Power(r.Power))
		hr = append(hr, sanitizeUint8HR(r.HeartRate))
		timeSec = append(timeSec, delta)
		sp := sanitizeUint32Speed(r.EnhancedSpeed)
		speed = append(speed, sp)

		altVal := float64(0)
		al := r.GetEnhancedAltitudeScaled()
		if !math.IsNaN(al) {
			altVal = al
		}
		altitude = append(altitude, altVal)

		vsVal := float64(0)
		vv := r.GetVerticalSpeedScaled()
		if !math.IsNaN(vv) {
			vsVal = vv
		}
		verticalSpeed = append(verticalSpeed, vsVal)

		if c, ok := lookupRecordMetric(r, []string{"Cadence", "EnhancedCadence"}); ok {
			c = sanitizeCadence(c)
			if isRunLikeSport(sport) && c > 0 && c < 130 {
				// Many FIT files store running cadence as steps per minute per leg.
				// Convert to total spm for UI/analysis consistency.
				c = c * 2
			}
			cadence = append(cadence, c)
			if c > 0 {
				cadenceSamples = append(cadenceSamples, c)
			}
		} else {
			cadence = append(cadence, 0)
		}
		if vo, ok := lookupRecordMetric(r, []string{"VerticalOscillation", "EnhancedVerticalOscillation"}); ok {
			if voCM := sanitizeVerticalOscillationCM(vo); voCM > 0 {
				voSamplesCM = append(voSamplesCM, voCM)
			}
		}
		if sl, ok := lookupRecordMetric(r, []string{"StepLength"}); ok {
			if slM := sanitizeStrideLengthM(sl); slM > 0 {
				strideSamplesM = append(strideSamplesM, slM)
			}
		}
		st := r.GetStanceTimeScaled()
		if !math.IsNaN(st) && st >= 140 && st <= 520 {
			stanceVals = append(stanceVals, st)
		}
		if ub := uint8(r.LeftRightBalance); ub != 0xFF && isRunLikeSport(sport) {
			pctR := float64(ub&0x7F) / 127.0 * 100.0
			lateralVals = append(lateralVals, math.Abs(pctR-50.0))
		}
	}

	if duration <= 0 && len(timeSec) > 1 {
		duration = time.Duration(timeSec[len(timeSec)-1]) * time.Second
	}
	name = buildActivityName(name, sport, start, distanceKM, duration)
	avgCadence := meanOrZero(cadenceSamples)
	avgVerticalOscillationCM := meanOrZero(voSamplesCM)
	avgStrideLengthM := meanOrZero(strideSamplesM)
	if avgStrideLengthM <= 0 && avgCadence > 0 && distanceKM > 0 && duration > 0 {
		avgStrideLengthM = (distanceKM * 1000.0 / duration.Seconds()) * 60.0 / avgCadence
	}

	avgStance := meanOrZero(stanceVals)
	if avgStance <= 0 && len(activity.Sessions) > 0 && activity.Sessions[0] != nil {
		sm := activity.Sessions[0].GetAvgStanceTimeScaled()
		if !math.IsNaN(sm) && sm >= 140 && sm <= 520 {
			avgStance = sm
		}
	}
	asymAvg := meanOrZero(lateralVals)

	return domain.Activity{
		ID:                       id,
		Name:                     name,
		Source:                   "GarminFIT",
		Sport:                    sport,
		StartTime:                start,
		Duration:                 duration,
		DistanceKM:               distanceKM,
		Power:                    power,
		HeartRate:                hr,
		TimeSec:                  timeSec,
		SpeedMS:                  speed,
		Cadence:                  cadence,
		AltitudeM:                altitude,
		VerticalSpeedMS:          verticalSpeed,
		AvgCadence:               avgCadence,
		AvgVerticalOscillationCM: avgVerticalOscillationCM,
		AvgStrideLengthM:         avgStrideLengthM,
		AvgStanceTimeMs:          avgStance,
		StrideAsymmetryPct:       asymAvg,
	}, nil
}

func trimExt(name string) string {
	ext := filepath.Ext(name)
	return strings.TrimSuffix(name, ext)
}

func buildActivityName(fileName, sport string, start time.Time, distanceKM float64, duration time.Duration) string {
	baseSport := strings.TrimSpace(sport)
	if baseSport == "" || strings.EqualFold(baseSport, "generic") {
		baseSport = "Activity"
	}

	distPart := ""
	if distanceKM > 0 {
		distPart = fmt.Sprintf(" | %.1f km", distanceKM)
	}
	durPart := ""
	if duration > 0 {
		h := int(duration.Hours())
		m := int(duration.Minutes()) % 60
		durPart = " | " + strconv.Itoa(h) + ":" + fmt.Sprintf("%02d", m)
	}

	pretty := fmt.Sprintf("%s%s%s", titleWord(strings.ToLower(baseSport)), distPart, durPart)
	if strings.TrimSpace(pretty) != "" {
		return pretty
	}
	return fileName
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func titleWord(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func sanitizeUint16Power(v uint16) float64 {
	if v == math.MaxUint16 {
		return 0
	}
	return float64(v)
}

func sanitizeUint8HR(v uint8) float64 {
	if v == math.MaxUint8 || v == 0 {
		return 0
	}
	return float64(v)
}

func sanitizeUint32Speed(v uint32) float64 {
	if v == math.MaxUint32 || v == 0 {
		return 0
	}
	// FIT enhanced_speed is m/s with scale 1000.
	return float64(v) / 1000.0
}

func sanitizeCadence(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 40 || v > 260 {
		return 0
	}
	return v
}

func sanitizeVerticalOscillationCM(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
		return 0
	}
	// FIT fields are often scaled by 10 (e.g. 83 => 8.3 cm).
	if v > 25 && v <= 300 {
		v = v / 10.0
	}
	if v < 3 || v > 20 {
		return 0
	}
	return v
}

func sanitizeStrideLengthM(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
		return 0
	}
	// FIT step length is commonly centimeters with scale 100.
	if v > 5 && v <= 400 {
		v = v / 100.0
	}
	if v < 0.3 || v > 3.0 {
		return 0
	}
	return v
}

func lookupRecordMetric(record any, fieldNames []string) (float64, bool) {
	rv := reflect.ValueOf(record)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return 0, false
	}
	for _, name := range fieldNames {
		f := rv.FieldByName(name)
		if !f.IsValid() {
			continue
		}
		switch f.Kind() {
		case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
			return float64(f.Uint()), true
		case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
			return float64(f.Int()), true
		case reflect.Float32, reflect.Float64:
			return f.Float(), true
		}
	}
	return 0, false
}

func meanOrZero(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func isRunLikeSport(sport string) bool {
	s := strings.ToLower(strings.TrimSpace(sport))
	return strings.Contains(s, "run") || strings.Contains(s, "trail") || strings.Contains(s, "walk")
}

func collectFITFiles(dir string) ([]string, error) {
	paths := make([]string, 0, 128)
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(d.Name()), ".fit") {
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}

func dedupeActivities(in []domain.Activity) ([]domain.Activity, int) {
	seen := make(map[string]struct{}, len(in))
	out := make([]domain.Activity, 0, len(in))
	deduped := 0
	for _, a := range in {
		key := fmt.Sprintf("%s|%d|%.3f|%s", a.StartTime.UTC().Format(time.RFC3339), int(a.Duration.Seconds()), a.DistanceKM, strings.ToLower(a.Sport))
		if _, ok := seen[key]; ok {
			deduped++
			continue
		}
		seen[key] = struct{}{}
		out = append(out, a)
	}
	return out, deduped
}
