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

// FromGeoJSONLocations parses Dawarich iOS / Overland GeoJSON location features.
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

// FromOwnTracks parses an OwnTracks HTTP location payload.
func FromOwnTracks(body map[string]any, loc *time.Location) (store.Point, bool) {
	if strings.EqualFold(asString(body["_type"]), "waypoint") {
		return store.Point{}, false
	}
	lat, okLat := asFloat(body["lat"])
	lon, okLon := asFloat(body["lon"])
	if !okLat || !okLon || !validCoord(lat, lon) || NullIsland(lat, lon) {
		return store.Point{}, false
	}
	ts, ok := ParseTimestamp(body["tst"], loc)
	if !ok {
		return store.Point{}, false
	}
	vel := optFloat(body["vel"])
	if vel != nil {
		// OwnTracks vel is km/h; store m/s like Dawarich.
		ms := *vel * 1000 / 3600
		vel = &ms
	}
	raw, _ := json.Marshal(body)
	p := store.Point{
		Timestamp:        ts,
		Latitude:         lat,
		Longitude:        lon,
		Altitude:         optFloat(body["alt"]),
		Accuracy:         optFloat(body["acc"]),
		VerticalAccuracy: optFloat(body["vac"]),
		Velocity:         vel,
		Course:           columnSafeDecimal(body["cog"]),
		Battery:          intPtr(asInt(body["batt"])),
		BatteryStatus:    ownTracksBatteryStatus(body["bs"]),
		TrackerID:        optString(body["tid"]),
		SSID:             optString(body["SSID"]),
		RawData:          raw,
	}
	if p.Battery != nil && *p.Battery <= 0 {
		p.Battery = nil
	}
	return p, true
}

func ownTracksBatteryStatus(v any) *string {
	n, ok := asFloat(v)
	if !ok {
		return new("unknown")
	}
	s := "unknown"
	switch int(n) {
	case 1:
		s = "unplugged"
	case 2:
		s = "charging"
	case 3:
		s = "full"
	}
	return &s
}

// FromTraccar parses nested (official client) or flat OsmAnd-style payloads.
func FromTraccar(body map[string]any, loc *time.Location) (store.Point, bool) {
	if isFlatTraccar(body) {
		body = flattenTraccar(body)
	}
	location, _ := asMap(body["location"])
	coords, _ := asMap(location["coords"])
	if len(coords) == 0 {
		coords = location
	}
	lat, okLat := asFloat(coords["latitude"])
	lon, okLon := asFloat(coords["longitude"])
	if !okLat || !okLon || !validCoord(lat, lon) || NullIsland(lat, lon) {
		return store.Point{}, false
	}
	ts, ok := ParseTimestamp(location["timestamp"], loc)
	if !ok {
		return store.Point{}, false
	}
	battery, _ := asMap(location["battery"])
	if len(battery) == 0 {
		battery, _ = asMap(body["battery"])
	}
	raw, _ := json.Marshal(body)
	p := store.Point{
		Timestamp: ts,
		Latitude:  lat,
		Longitude: lon,
		Altitude:  optFloat(coords["altitude"]),
		Accuracy:  optFloat(coords["accuracy"]),
		Velocity:  optFloat(coords["speed"]),
		Course:    columnSafeDecimal(first(coords["heading"], location["heading"])),
		Battery:   batteryPercent(battery["level"]),
		TrackerID: firstString(body["device_id"], body["id"]),
		RawData:   raw,
	}
	if charging, ok := battery["is_charging"].(bool); ok {
		st := "unplugged"
		if charging {
			st = "charging"
		}
		p.BatteryStatus = &st
	}
	return p, true
}

func isFlatTraccar(body map[string]any) bool {
	if _, ok := body["location"]; ok {
		return false
	}
	_, hasLat := asFloat(body["lat"])
	_, hasLon := asFloat(body["lon"])
	return hasLat && hasLon
}

func flattenTraccar(in map[string]any) map[string]any {
	speed := optFloat(in["speed"])
	if speed != nil {
		ms := *speed / 1.94384 // knots -> m/s, matching Dawarich
		speed = &ms
	}
	loc := map[string]any{
		"timestamp": in["timestamp"],
		"latitude":  in["lat"],
		"longitude": in["lon"],
		"accuracy":  in["accuracy"],
		"altitude":  in["altitude"],
		"heading":   in["bearing"],
	}
	if speed != nil {
		loc["speed"] = *speed
	}
	out := map[string]any{
		"device_id": first(in["id"], in["device_id"]),
		"location":  loc,
	}
	if _, ok := in["charge"]; ok {
		out["battery"] = map[string]any{"is_charging": asBool(in["charge"])}
	}
	if batt, ok := asFloat(in["batt"]); ok {
		b, _ := asMap(out["battery"])
		if b == nil {
			b = map[string]any{}
		}
		b["level"] = batt / 100
		out["battery"] = b
	}
	return out
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

func asInt(v any) int {
	f, ok := asFloat(v)
	if !ok {
		return 0
	}
	return int(f)
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "1" || strings.EqualFold(t, "true") || strings.EqualFold(t, "yes")
	case float64:
		return t != 0
	case json.Number:
		f, err := t.Float64()
		return err == nil && f != 0
	default:
		return false
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

func first(vals ...any) any {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

//go:fix inline
func strPtr(s string) *string { return new(s) }

func intPtr(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}
