package ingest

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/upamune/itayo/internal/store"
)

const courseColumnLimit = 1000

// FromGeoJSONLocations parses official Dawarich iOS GeoJSON location features.
// Invalid and Null Island points are dropped.
func FromGeoJSONLocations(locations []any, loc *time.Location) []store.Point {
	out := make([]store.Point, 0, len(locations))
	for _, raw := range locations {
		m, ok := asMap(raw)
		if !ok {
			continue
		}
		p, ok := geoJSONPoint(m, loc)
		if !ok {
			continue
		}
		out = append(out, p)
	}
	return out
}

func geoJSONPoint(m map[string]any, loc *time.Location) (store.Point, bool) {
	geom, _ := asMap(m["geometry"])
	coords, _ := geom["coordinates"].([]any)
	if len(coords) < 2 {
		return store.Point{}, false
	}
	lon, okLon := asFloat(coords[0])
	lat, okLat := asFloat(coords[1])
	if !okLon || !okLat || !validCoord(lat, lon) || NullIsland(lat, lon) {
		return store.Point{}, false
	}
	props, _ := asMap(m["properties"])
	ts, ok := ParseTimestamp(props["timestamp"], loc)
	if !ok {
		return store.Point{}, false
	}
	raw, _ := json.Marshal(m)
	p := store.Point{
		Timestamp:        ts,
		Latitude:         lat,
		Longitude:        lon,
		Altitude:         optFloat(props["altitude"]),
		Accuracy:         optFloat(props["horizontal_accuracy"]),
		VerticalAccuracy: optFloat(props["vertical_accuracy"]),
		Velocity:         optFloat(props["speed"]),
		Course:           columnSafeDecimal(props["course"]),
		CourseAccuracy:   columnSafeDecimal(props["course_accuracy"]),
		Battery:          batteryPercent(props["battery_level"]),
		BatteryStatus:    optString(props["battery_state"]),
		TrackerID:        firstString(props["device_id"], props["tracker_id"]),
		SSID:             optString(props["wifi"]),
		RawData:          raw,
	}
	return p, true
}

// NullIsland reports (0, 0) coordinates.
func NullIsland(lat, lon float64) bool {
	return lat == 0 && lon == 0
}

func validCoord(lat, lon float64) bool {
	if math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) {
		return false
	}
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

// ParseTimestamp accepts unix seconds/millis or RFC3339 strings.
func ParseTimestamp(v any, loc *time.Location) (int64, bool) {
	if v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return 0, false
		}
		return unixFromFloat(f), true
	case float64:
		return unixFromFloat(t), true
	case int:
		return unixFromFloat(float64(t)), true
	case int64:
		return unixFromFloat(float64(t)), true
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return unixFromFloat(float64(n)), true
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return unixFromFloat(f), true
		}
		if tm, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return tm.Unix(), true
		}
		if tm, err := time.Parse(time.RFC3339, s); err == nil {
			return tm.Unix(), true
		}
		if loc == nil {
			loc = time.UTC
		}
		if tm, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
			return tm.Unix(), true
		}
		return 0, false
	default:
		return 0, false
	}
}

func unixFromFloat(n float64) int64 {
	if n > 1_000_000_000_000 {
		return int64(n / 1000)
	}
	return int64(n)
}

func batteryPercent(v any) *int {
	f, ok := asFloat(v)
	if !ok {
		return nil
	}
	n := int(f * 100)
	if n <= 0 {
		return nil
	}
	return &n
}

func columnSafeDecimal(v any) *float64 {
	f, ok := asFloat(v)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) >= courseColumnLimit {
		return nil
	}
	return &f
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case nil:
		return 0, false
	case float64:
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func optFloat(v any) *float64 {
	f, ok := asFloat(v)
	if !ok {
		return nil
	}
	return &f
}

func optString(v any) *string {
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	return &s
}

func firstString(vals ...any) *string {
	for _, v := range vals {
		if s := optString(v); s != nil {
			return s
		}
	}
	return nil
}
