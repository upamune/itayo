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

func TestParseTimestampMillis(t *testing.T) {
	ts, ok := ParseTimestamp(float64(1_710_000_000_000), time.UTC)
	if !ok || ts != 1_710_000_000 {
		t.Fatalf("got %d ok=%v", ts, ok)
	}
}
