package ingest

import (
	"testing"
	"time"
)

func TestFromGeoJSONDropsNullIslandAndBad(t *testing.T) {
	locations := []any{
		map[string]any{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "Point",
				"coordinates": []any{139.767125, 35.681236},
			},
			"properties": map[string]any{
				"timestamp": "2025-01-17T21:03:01Z",
			},
		},
		map[string]any{
			"geometry": map[string]any{
				"type":        "Point",
				"coordinates": []any{0.0, 0.0},
			},
			"properties": map[string]any{
				"timestamp": "2025-01-17T21:03:01Z",
			},
		},
		map[string]any{
			"geometry": map[string]any{
				"type":        "Point",
				"coordinates": []any{1.0, 1.0},
			},
			"properties": map[string]any{},
		},
	}
	got := FromGeoJSONLocations(locations, time.UTC)
	if len(got) != 1 {
		t.Fatalf("got %d points", len(got))
	}
	if got[0].Latitude != 35.681236 || got[0].Longitude != 139.767125 {
		t.Fatalf("coord = %v,%v", got[0].Latitude, got[0].Longitude)
	}
	if got[0].Timestamp != time.Date(2025, 1, 17, 21, 3, 1, 0, time.UTC).Unix() {
		t.Fatalf("timestamp = %d", got[0].Timestamp)
	}
}

func TestFromTraccarNestedAndFlat(t *testing.T) {
	nested, ok := FromTraccar(map[string]any{
		"device_id": "iphone-jane",
		"location": map[string]any{
			"timestamp": "2026-04-23T12:34:56Z",
			"latitude":  52.52,
			"longitude": 13.405,
			"accuracy":  5.0,
			"speed":     1.4,
			"heading":   90.0,
			"altitude":  42.0,
		},
		"battery": map[string]any{"level": 0.85, "is_charging": true},
	}, time.UTC)
	if !ok {
		t.Fatal("nested rejected")
	}
	if nested.TrackerID == nil || *nested.TrackerID != "iphone-jane" {
		t.Fatalf("tracker = %v", nested.TrackerID)
	}
	if nested.Battery == nil || *nested.Battery != 85 {
		t.Fatalf("battery = %v", nested.Battery)
	}

	coords, ok := FromTraccar(map[string]any{
		"device_id": "nested-coords",
		"location": map[string]any{
			"timestamp": "1710000000",
			"coords": map[string]any{
				"latitude":  35.0,
				"longitude": 139.0,
			},
		},
	}, time.UTC)
	if !ok || coords.Latitude != 35 || coords.Longitude != 139 {
		t.Fatalf("coords form = %+v ok=%v", coords, ok)
	}

	flat, ok := FromTraccar(map[string]any{
		"id":        "osmand",
		"lat":       35.0,
		"lon":       139.0,
		"timestamp": float64(1_710_000_000),
	}, time.UTC)
	if !ok || flat.Latitude != 35 {
		t.Fatalf("flat = %+v ok=%v", flat, ok)
	}

	if _, ok := FromTraccar(map[string]any{"lat": 1.0}, time.UTC); ok {
		t.Fatal("expected invalid traccar to drop")
	}
}

func TestFromOwnTracks(t *testing.T) {
	p, ok := FromOwnTracks(map[string]any{
		"_type": "location",
		"lat":   35.0,
		"lon":   139.0,
		"tst":   int64(1_710_000_000),
		"tid":   "ab",
		"batt":  80.0,
		"vel":   36.0,
	}, time.UTC)
	if !ok {
		t.Fatal("rejected")
	}
	if p.TrackerID == nil || *p.TrackerID != "ab" {
		t.Fatalf("tid = %v", p.TrackerID)
	}
	if p.Velocity == nil || *p.Velocity != 10 {
		t.Fatalf("vel without topic: got %v want 10 m/s (36 km/h)", p.Velocity)
	}

	withTopic, ok := FromOwnTracks(map[string]any{
		"_type": "location",
		"lat":   35.0,
		"lon":   139.0,
		"tst":   int64(1_710_000_000),
		"vel":   3.6,
		"topic": "owntracks/user/phone",
	}, time.UTC)
	if !ok {
		t.Fatal("rejected with topic")
	}
	if withTopic.Velocity == nil || *withTopic.Velocity != 1 {
		t.Fatalf("vel with topic: got %v want 1 m/s (3.6 km/h)", withTopic.Velocity)
	}

	if _, ok := FromOwnTracks(map[string]any{"_type": "waypoint", "lat": 1.0, "lon": 2.0, "tst": 1.0}, time.UTC); ok {
		t.Fatal("waypoint should drop")
	}
}

func TestParseTimestampMillis(t *testing.T) {
	ts, ok := ParseTimestamp(float64(1_710_000_000_000), time.UTC)
	if !ok || ts != 1_710_000_000 {
		t.Fatalf("got %d ok=%v", ts, ok)
	}
}
