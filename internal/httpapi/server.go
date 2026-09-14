package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/upamune/itayo/internal/config"
	"github.com/upamune/itayo/internal/ingest"
	"github.com/upamune/itayo/internal/store"
	"github.com/upamune/itayo/internal/version"
)

// Server is the Dawarich-compatible HTTP API.
type Server struct {
	cfg   config.Config
	store *store.Store
	now   func() time.Time
}

// New returns an http.Handler for itayo.
func New(cfg config.Config, st *store.Store) http.Handler {
	s := &Server{
		cfg:   cfg,
		store: st,
		now:   time.Now,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("POST /api/v1/points", s.authed(s.createPoints))
	mux.HandleFunc("GET /api/v1/points", s.authed(s.listPoints))
	mux.HandleFunc("GET /api/v1/users/me", s.authed(s.usersMe))
	mux.HandleFunc("GET /api/v1/settings", s.authed(s.getSettings))
	mux.HandleFunc("PATCH /api/v1/settings", s.authed(s.patchSettings))
	return s.withCompatHeaders(mux)
}

func (s *Server) withCompatHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&compatWriter{ResponseWriter: w, s: s, r: r}, r)
	})
}

type compatWriter struct {
	http.ResponseWriter
	s     *Server
	r     *http.Request
	wrote bool
}

func (w *compatWriter) WriteHeader(code int) {
	w.setHeaders()
	w.ResponseWriter.WriteHeader(code)
}

func (w *compatWriter) Write(b []byte) (int, error) {
	w.setHeaders()
	return w.ResponseWriter.Write(b)
}

func (w *compatWriter) setHeaders() {
	if w.wrote {
		return
	}
	w.wrote = true
	msg := "Hey, I'm alive!"
	if w.s.authorized(w.r) {
		msg = "Hey, I'm alive and authenticated!"
	}
	w.Header().Set("X-Dawarich-Response", msg)
	w.Header().Set("X-Dawarich-Version", version.DawarichCompat)
}

func (s *Server) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) authorized(r *http.Request) bool {
	if s.cfg.APIKey == "" {
		return false
	}
	got := apiKeyFrom(r)
	if len(got) != len(s.cfg.APIKey) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.APIKey)) == 1
}

func apiKeyFrom(r *http.Request) string {
	if k := r.URL.Query().Get("api_key"); k != "" {
		return k
	}
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) >= len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) createPoints(w http.ResponseWriter, r *http.Request) {
	body, err := decodeObject(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	locations, _ := body["locations"].([]any)
	points := ingest.FromGeoJSONLocations(locations, s.cfg.Location)
	stored, err := s.store.Upsert(r.Context(), points)
	if err != nil {
		slog.Error("upsert points", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "point creation failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": createPayload(stored)})
}

func (s *Server) listPoints(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.ListFilter{
		Order:   q.Get("order"),
		Page:    atoiDefault(q.Get("page"), 1),
		PerPage: atoiDefault(q.Get("per_page"), 100),
		EndAt:   s.now().Unix(),
	}
	if raw := q.Get("end_at"); raw != "" {
		if ts, ok := ingest.ParseTimestamp(raw, s.cfg.Location); ok {
			f.EndAt = ts
		}
	}
	if raw := q.Get("start_at"); raw != "" {
		if ts, ok := ingest.ParseTimestamp(raw, s.cfg.Location); ok {
			f.StartAt = &ts
		}
	}
	points, total, err := s.store.List(r.Context(), f)
	if err != nil {
		slog.Error("list points", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list points"})
		return
	}
	pages := 0
	if f.PerPage > 0 {
		pages = (total + f.PerPage - 1) / f.PerPage
	}
	w.Header().Set("X-Current-Page", strconv.Itoa(f.Page))
	w.Header().Set("X-Total-Pages", strconv.Itoa(pages))
	if q.Get("slim") == "true" {
		writeJSON(w, http.StatusOK, slimPayload(points))
		return
	}
	writeJSON(w, http.StatusOK, fullPayload(points))
}

func (s *Server) usersMe(w http.ResponseWriter, r *http.Request) {
	settings, err := s.loadSettings(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load user"})
		return
	}
	now := s.now().UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"email":      "itayo@example.com",
			"theme":      "light",
			"created_at": now,
			"updated_at": now,
			"settings":   userSettingsView(settings),
		},
		"features": map[string]any{
			"self_hosted": true,
		},
	})
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.loadSettings(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load settings"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": settings,
		"status":   "success",
	})
}

func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) {
	body, err := decodeObject(r)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"message": "Something went wrong",
			"errors":  []string{"invalid json"},
		})
		return
	}
	patch, _ := body["settings"].(map[string]any)
	if patch == nil {
		patch = body
	}
	current, err := s.loadSettings(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load settings"})
		return
	}
	maps.Copy(current, patch)
	raw, err := json.Marshal(current)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save settings"})
		return
	}
	if err := s.store.SaveSettingsJSON(r.Context(), raw); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save settings"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message":                 "Settings updated",
		"settings":                current,
		"status":                  "success",
		"recalculation_triggered": false,
	})
}

func (s *Server) loadSettings(r *http.Request) (map[string]any, error) {
	raw, err := s.store.SettingsJSON(r.Context())
	if err != nil {
		return nil, err
	}
	settings := defaultSettings(s.cfg.TimeZone)
	if len(raw) == 0 {
		return settings, nil
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		return settings, nil
	}
	maps.Copy(settings, stored)
	return settings, nil
}

func decodeObject(r *http.Request) (map[string]any, error) {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 32<<20))
	dec.UseNumber()
	var body map[string]any
	if err := dec.Decode(&body); err != nil {
		return nil, err
	}
	if body == nil {
		body = map[string]any{}
	}
	return body, nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func atoiDefault(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

func createPayload(points []store.Point) []map[string]any {
	out := make([]map[string]any, 0, len(points))
	for _, p := range points {
		out = append(out, map[string]any{
			"id":        p.ID,
			"latitude":  p.Latitude,
			"longitude": p.Longitude,
			"timestamp": p.Timestamp,
		})
	}
	return out
}

func slimPayload(points []store.Point) []map[string]any {
	out := make([]map[string]any, 0, len(points))
	for _, p := range points {
		out = append(out, map[string]any{
			"id":           p.ID,
			"latitude":     formatCoord(p.Latitude),
			"longitude":    formatCoord(p.Longitude),
			"timestamp":    p.Timestamp,
			"velocity":     p.Velocity,
			"country_name": nil,
			"tracker_id":   p.TrackerID,
		})
	}
	return out
}

func fullPayload(points []store.Point) []map[string]any {
	out := make([]map[string]any, 0, len(points))
	for _, p := range points {
		out = append(out, map[string]any{
			"id":                p.ID,
			"latitude":          formatCoord(p.Latitude),
			"longitude":         formatCoord(p.Longitude),
			"timestamp":         p.Timestamp,
			"altitude":          p.Altitude,
			"accuracy":          p.Accuracy,
			"vertical_accuracy": p.VerticalAccuracy,
			"velocity":          p.Velocity,
			"course":            p.Course,
			"course_accuracy":   p.CourseAccuracy,
			"battery":           p.Battery,
			"battery_status":    p.BatteryStatus,
			"tracker_id":        p.TrackerID,
			"ssid":              p.SSID,
			"country_name":      nil,
		})
	}
	return out
}

func formatCoord(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func userSettingsView(settings map[string]any) map[string]any {
	keys := []string{
		"timezone", "maps", "fog_of_war_meters", "meters_between_routes",
		"preferred_map_layer", "speed_colored_routes", "points_rendering_mode",
		"minutes_between_routes", "time_threshold_minutes", "merge_threshold_minutes",
		"live_map_enabled", "route_opacity", "immich_url", "photoprism_url",
		"visits_suggestions_enabled", "speed_color_scale", "fog_of_war_threshold",
		"globe_projection",
	}
	out := map[string]any{}
	for _, k := range keys {
		out[k] = settings[k]
	}
	return out
}

func defaultSettings(tz string) map[string]any {
	if tz == "" {
		tz = "Asia/Tokyo"
	}
	return map[string]any{
		"fog_of_war_meters":          50,
		"meters_between_routes":      500,
		"preferred_map_layer":        "OpenStreetMap",
		"speed_colored_routes":       false,
		"points_rendering_mode":      "raw",
		"minutes_between_routes":     30,
		"time_threshold_minutes":     30,
		"merge_threshold_minutes":    15,
		"live_map_enabled":           true,
		"route_opacity":              0.6,
		"route_color":                "#0000ff",
		"track_color":                "#6366F1",
		"immich_url":                 nil,
		"immich_api_key":             nil,
		"photoprism_url":             nil,
		"photoprism_api_key":         nil,
		"maps":                       map[string]any{"distance_unit": "km"},
		"visits_suggestions_enabled": true,
		"speed_color_scale":          nil,
		"fog_of_war_threshold":       50,
		"fog_of_war_mode":            "points",
		"enabled_map_layers":         []string{"Tracks", "Heatmap"},
		"maps_maplibre_style":        "light",
		"globe_projection":           true,
		"gps_filtering_enabled":      true,
		"timezone":                   tz,
		"point_dragging_enabled":     false,
		"min_minutes_spent_in_city":  60,
	}
}
