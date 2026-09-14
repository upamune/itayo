package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/upamune/itayo/internal/config"
	"github.com/upamune/itayo/internal/store"
	"github.com/upamune/itayo/internal/version"
)

const testKey = "your-secret"

func TestHealth(t *testing.T) {
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Header().Get("X-Dawarich-Version") != version.DawarichCompat {
		t.Fatalf("version header %q", rec.Header().Get("X-Dawarich-Version"))
	}
	if rec.Header().Get("X-Dawarich-Response") != "Hey, I'm alive!" {
		t.Fatalf("response header %q", rec.Header().Get("X-Dawarich-Response"))
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body %+v", body)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/health?api_key="+testKey, nil)
	h.ServeHTTP(rec, req)
	if rec.Header().Get("X-Dawarich-Response") != "Hey, I'm alive and authenticated!" {
		t.Fatalf("auth header %q", rec.Header().Get("X-Dawarich-Response"))
	}
}

func TestUnauthorized(t *testing.T) {
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/points", bytes.NewReader([]byte(`{"locations":[]}`)))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestCreatePointsIOSGeoJSON(t *testing.T) {
	h := newTestHandler(t)
	payload := map[string]any{
		"locations": []any{
			map[string]any{
				"type": "Feature",
				"geometry": map[string]any{
					"type":        "Point",
					"coordinates": []any{-0.85, 51.11},
				},
				"properties": map[string]any{
					"timestamp":           "2026-02-23T08:50:21Z",
					"horizontal_accuracy": 11.5,
					"battery_level":       0.87,
					"battery_state":       "unplugged",
					"device_id":           "example-device",
					"track_id":            "1",
					"altitude":            121.3,
					"speed":               0,
					"course":              0,
					"vertical_accuracy":   0.7,
					"course_accuracy":     0,
					"speed_accuracy":      0,
				},
			},
			map[string]any{
				"type": "Feature",
				"geometry": map[string]any{
					"type":        "Point",
					"coordinates": []any{0.0, 0.0},
				},
				"properties": map[string]any{
					"timestamp": "2026-02-23T08:50:21Z",
				},
			},
			map[string]any{
				"geometry": map[string]any{
					"type":        "Point",
					"coordinates": []any{1.0, 1.0},
				},
				"properties": map[string]any{},
			},
		},
	}
	rec := doJSON(t, h, http.MethodPost, "/api/v1/points?api_key="+testKey, payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("stored %d: %s", len(body.Data), rec.Body.String())
	}
	if body.Data[0]["latitude"] != 51.11 {
		t.Fatalf("lat %+v", body.Data[0]["latitude"])
	}
}

func TestBearerAuthAndListFilters(t *testing.T) {
	h := newTestHandler(t)
	payload := map[string]any{
		"locations": []any{
			feature(139.0, 35.0, "2025-01-01T00:00:00Z"),
			feature(139.1, 35.1, "2025-06-01T00:00:00Z"),
			feature(139.2, 35.2, "2025-12-01T00:00:00Z"),
		},
	}
	rec := httptest.NewRecorder()
	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/points", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+testKey)
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodGet, "/api/v1/points?api_key="+testKey+"&start_at=2025-03-01T00:00:00Z&end_at=2025-09-01T00:00:00Z&order=asc&page=1&per_page=10", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %d %s", rec.Code, rec.Body.String())
	}
	var points []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &points); err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || points[0]["latitude"] != "35.1" {
		t.Fatalf("filtered %+v", points)
	}
	if rec.Header().Get("X-Current-Page") != "1" || rec.Header().Get("X-Total-Pages") != "1" {
		t.Fatalf("pages %q/%q", rec.Header().Get("X-Current-Page"), rec.Header().Get("X-Total-Pages"))
	}

	rec = doJSON(t, h, http.MethodGet, "/api/v1/points?api_key="+testKey+"&slim=true&order=desc&per_page=2", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("slim %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &points); err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 {
		t.Fatalf("slim n=%d", len(points))
	}
	for _, p := range points {
		for _, k := range []string{"id", "latitude", "longitude", "timestamp", "velocity", "country_name", "tracker_id"} {
			if _, ok := p[k]; !ok {
				t.Fatalf("missing slim key %s in %+v", k, p)
			}
		}
	}
}

func TestOverlandOwnTracksTraccar(t *testing.T) {
	h := newTestHandler(t)

	rec := doJSON(t, h, http.MethodPost, "/api/v1/overland/batches?api_key="+testKey, map[string]any{
		"locations": []any{feature(139.7, 35.6, "2025-01-17T21:03:01Z")},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("overland %d %s", rec.Code, rec.Body.String())
	}
	var overland map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &overland); err != nil || overland["result"] != "ok" {
		t.Fatalf("overland body %s", rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodPost, "/api/v1/owntracks/points?api_key="+testKey, map[string]any{
		"_type": "location",
		"lat":   35.0,
		"lon":   139.0,
		"tst":   1_710_000_000,
		"tid":   "ab",
	})
	if rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Fatalf("owntracks %d %q", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodPost, "/api/v1/traccar/points?api_key="+testKey, map[string]any{
		"device_id": "iphone-jane",
		"location": map[string]any{
			"timestamp": "2026-04-23T12:34:56Z",
			"latitude":  52.52,
			"longitude": 13.405,
		},
		"battery": map[string]any{"level": 0.85, "is_charging": true},
	})
	if rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Fatalf("traccar nested %d %q", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodPost, "/api/v1/traccar/points?api_key="+testKey, map[string]any{
		"id":        "osmand",
		"lat":       35.0,
		"lon":       139.1,
		"timestamp": 1_710_000_100,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("traccar flat %d %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodPost, "/api/v1/traccar/points?api_key="+testKey, map[string]any{
		"device_id": "bad",
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("traccar invalid %d", rec.Code)
	}
}

func TestUsersAndSettingsStubs(t *testing.T) {
	h := newTestHandler(t)
	rec := doJSON(t, h, http.MethodGet, "/api/v1/users/me?api_key="+testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("me %d %s", rec.Code, rec.Body.String())
	}
	var me map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	user, _ := me["user"].(map[string]any)
	if user["email"] != "itayo@example.com" {
		t.Fatalf("user %+v", user)
	}

	rec = doJSON(t, h, http.MethodGet, "/api/v1/settings?api_key="+testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("settings %d", rec.Code)
	}
	var settingsBody map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &settingsBody); err != nil {
		t.Fatal(err)
	}
	settings, _ := settingsBody["settings"].(map[string]any)
	if settings["timezone"] != "Asia/Tokyo" {
		t.Fatalf("tz %+v", settings["timezone"])
	}

	rec = doJSON(t, h, http.MethodPatch, "/api/v1/settings?api_key="+testKey, map[string]any{
		"settings": map[string]any{"live_map_enabled": false},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &settingsBody); err != nil {
		t.Fatal(err)
	}
	settings, _ = settingsBody["settings"].(map[string]any)
	if settings["live_map_enabled"] != false {
		t.Fatalf("patched %+v", settings["live_map_enabled"])
	}
}

func feature(lon, lat float64, ts string) map[string]any {
	return map[string]any{
		"type": "Feature",
		"geometry": map[string]any{
			"type":        "Point",
			"coordinates": []any{lon, lat},
		},
		"properties": map[string]any{"timestamp": ts},
	}
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(raw)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "itayo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	return New(config.Config{
		APIKey:       testKey,
		ListenAddr:   ":8790",
		TimeZone:     "Asia/Tokyo",
		Location:     loc,
		DatabasePath: "unused",
	}, st)
}
