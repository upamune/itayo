package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
	rec = doJSON(t, h, http.MethodPost, "/api/v1/points?api_key=wrong-key", map[string]any{"locations": []any{}})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key status %d", rec.Code)
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

func TestInvalidJSONDoesNotPanic(t *testing.T) {
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/points?api_key="+testKey, bytes.NewReader([]byte(`[`)))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
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

	rec = doJSON(t, h, http.MethodGet, "/api/v1/points?api_key="+testKey+"&min_latitude=35.05&max_latitude=35.15&min_longitude=139.05&max_longitude=139.15", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("bbox %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &points); err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || points[0]["latitude"] != "35.1" {
		t.Fatalf("bbox points %+v", points)
	}

	rec = doJSON(t, h, http.MethodGet, "/api/v1/points?api_key="+testKey+"&min_latitude=1&max_latitude=0&min_longitude=0&max_longitude=1", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid bbox status %d", rec.Code)
	}

	rec = doJSON(t, h, http.MethodGet, "/api/v1/points?api_key="+testKey+"&min_latitude=35", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("partial bbox status %d", rec.Code)
	}

	rec = doJSON(t, h, http.MethodGet, "/api/v1/points?api_key="+testKey, nil)
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing etag")
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/points?api_key="+testKey, nil)
	req.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("etag 304 got %d", rec.Code)
	}
}

func TestRemovedThirdPartyIngestPaths(t *testing.T) {
	h := newTestHandler(t)
	for _, path := range []string{
		"/api/v1/overland/batches",
		"/api/v1/owntracks/points",
		"/api/v1/traccar/points",
	} {
		rec := doJSON(t, h, http.MethodPost, path+"?api_key="+testKey, map[string]any{})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status %d, want 404", path, rec.Code)
		}
	}
}

func TestUsersMeIsHonestAndStable(t *testing.T) {
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
	created := user["created_at"]
	features, _ := me["features"].(map[string]any)
	if _, ok := features["self_hosted"]; ok {
		t.Fatal("self_hosted must not appear in features")
	}
	if features["reverse_geocoding"] != false || features["family"] != false {
		t.Fatalf("features %+v", features)
	}

	rec = doJSON(t, h, http.MethodGet, "/api/v1/users/me?api_key="+testKey, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	user, _ = me["user"].(map[string]any)
	if user["created_at"] != created {
		t.Fatalf("created_at changed %v -> %v", created, user["created_at"])
	}
}

func TestSettingsPersistPermitAndMapsMerge(t *testing.T) {
	h := newTestHandler(t)
	rec := doJSON(t, h, http.MethodGet, "/api/v1/settings?api_key="+testKey, nil)
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
		"settings": map[string]any{
			"live_map_enabled": false,
			"unknown_key":      "nope",
			"maps":             map[string]any{"distance_unit": "mi"},
		},
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
	if _, ok := settings["unknown_key"]; ok {
		t.Fatal("unknown settings key persisted")
	}
	maps, _ := settings["maps"].(map[string]any)
	if maps["distance_unit"] != "mi" {
		t.Fatalf("maps %+v", maps)
	}

	rec = doJSON(t, h, http.MethodPatch, "/api/v1/settings?api_key="+testKey, map[string]any{"live_map_enabled": true})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing settings wrapper status %d", rec.Code)
	}
}

func TestMobileSettingsLastWriteWins(t *testing.T) {
	h := newTestHandler(t)
	rec := doJSON(t, h, http.MethodGet, "/api/v1/settings/mobile?api_key="+testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get mobile %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["updated_at"] != nil {
		t.Fatalf("empty updated_at = %v", body["updated_at"])
	}
	caps, _ := body["capabilities"].(map[string]any)
	photo, _ := caps["photo_library_import"].(map[string]any)
	if photo["version"] != float64(1) {
		t.Fatalf("capabilities %+v", caps)
	}

	rec = doJSON(t, h, http.MethodPatch, "/api/v1/settings/mobile?api_key="+testKey, map[string]any{
		"settings": map[string]any{
			"tracking_mode":        "precise",
			"batch_size":           250,
			"upload_automatically": true,
			"unknown":              1,
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch mobile %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	settings, _ := body["settings"].(map[string]any)
	if settings["tracking_mode"] != "precise" || settings["batch_size"] != float64(250) {
		t.Fatalf("mobile settings %+v", settings)
	}
	if _, ok := settings["unknown"]; ok {
		t.Fatal("unknown mobile key")
	}
	if body["updated_at"] == nil || body["message"] != "Settings updated" {
		t.Fatalf("stamp %+v", body)
	}

	rec = doJSON(t, h, http.MethodPatch, "/api/v1/settings/mobile?api_key="+testKey, map[string]any{
		"settings": map[string]any{"batch_size": 10},
	})
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	settings, _ = body["settings"].(map[string]any)
	if settings["tracking_mode"] != "precise" || settings["batch_size"] != float64(10) {
		t.Fatalf("merge %+v", settings)
	}
}

func TestPlanAndTrackedMonthsAndInsights(t *testing.T) {
	h := newTestHandler(t)
	rec := doJSON(t, h, http.MethodGet, "/api/v1/plan?api_key="+testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("plan %d", rec.Code)
	}
	var plan map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan["plan"] != "pro" {
		t.Fatalf("plan %+v", plan)
	}
	features, _ := plan["features"].(map[string]any)
	if features["write_api"] != true || features["data_window"] != nil {
		t.Fatalf("features %+v", features)
	}
	if features["full_digest"] != false {
		t.Fatalf("full_digest must be false (digests are out of scope): %+v", features)
	}

	_ = doJSON(t, h, http.MethodPost, "/api/v1/points?api_key="+testKey, map[string]any{
		"locations": []any{
			feature(139.0, 35.0, "2025-01-01T00:00:00Z"),
			feature(139.01, 35.01, "2025-01-01T00:10:00Z"),
			feature(139.02, 35.02, "2025-02-01T00:00:00Z"),
		},
	})

	rec = doJSON(t, h, http.MethodGet, "/api/v1/points/tracked_months?api_key="+testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("months %d %s", rec.Code, rec.Body.String())
	}
	var months []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &months); err != nil {
		t.Fatal(err)
	}
	if len(months) != 1 || months[0]["year"] != float64(2025) {
		t.Fatalf("months %+v", months)
	}

	rec = doJSON(t, h, http.MethodGet, "/api/v1/insights?api_key="+testKey+"&year=2025", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("insights %d %s", rec.Code, rec.Body.String())
	}
	var overview map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	if overview["year"] != float64(2025) {
		t.Fatalf("overview %+v", overview)
	}
	totals, _ := overview["totals"].(map[string]any)
	if totals["countriesCount"] != float64(0) {
		t.Fatal("geocoding fields must stay honest zeros")
	}

	rec = doJSON(t, h, http.MethodGet, "/api/v1/insights/details?api_key="+testKey+"&year=2025", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("details %d %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodGet, "/api/v1/stats?api_key="+testKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats %d %s", rec.Code, rec.Body.String())
	}
	var stats map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats["totalPointsTracked"] != float64(3) {
		t.Fatalf("stats %+v", stats)
	}
}

func TestRequestLogOmitsAPIKey(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := newTestHandler(t)
	_ = doJSON(t, h, http.MethodGet, "/api/v1/health?api_key="+testKey, nil)
	logged := buf.String()
	if strings.Contains(logged, testKey) {
		t.Fatalf("log leaked api key: %s", logged)
	}
	if !strings.Contains(logged, "/api/v1/health") {
		t.Fatalf("log missing path: %s", logged)
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
	h, err := New(config.Config{
		APIKey:       testKey,
		ListenAddr:   ":8790",
		TimeZone:     "Asia/Tokyo",
		Location:     loc,
		DatabasePath: "unused",
		UserEmail:    "itayo@example.com",
		UserTheme:    "light",
		LogFormat:    "text",
	}, st)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestNewFailsWhenIdentityCannotBeSeeded(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "itayo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	_, err = New(config.Config{
		APIKey:    testKey,
		TimeZone:  "Asia/Tokyo",
		UserEmail: "itayo@example.com",
		UserTheme: "light",
	}, st)
	if err == nil {
		t.Fatal("expected identity seed failure")
	}
}
