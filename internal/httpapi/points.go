package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/upamune/itayo/internal/ingest"
	"github.com/upamune/itayo/internal/store"
)

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
	bbox, err := parseBBox(q.Get("min_latitude"), q.Get("max_latitude"), q.Get("min_longitude"), q.Get("max_longitude"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid bounding box"})
		return
	}
	f.BBox = bbox

	meta, err := s.store.ListFingerprint(r.Context(), f)
	if err != nil {
		slog.Error("list fingerprint", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list points"})
		return
	}
	etag := pointsETag(f, meta, q.Get("slim") == "true")
	w.Header().Set("ETag", etag)
	if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
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

func (s *Server) trackedMonths(w http.ResponseWriter, r *http.Request) {
	months, err := s.store.TrackedMonths(r.Context(), s.cfg.Location)
	if err != nil {
		slog.Error("tracked months", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list tracked months"})
		return
	}
	out := make([]map[string]any, 0, len(months))
	for _, ym := range months {
		out = append(out, map[string]any{"year": ym.Year, "months": ym.Months})
	}
	writeJSON(w, http.StatusOK, out)
}

func parseBBox(minLat, maxLat, minLon, maxLon string) (*store.BBox, error) {
	if minLat == "" && maxLat == "" && minLon == "" && maxLon == "" {
		return nil, nil
	}
	if minLat == "" || maxLat == "" || minLon == "" || maxLon == "" {
		return nil, errInvalidBBox
	}
	a, err1 := strconv.ParseFloat(minLat, 64)
	b, err2 := strconv.ParseFloat(maxLat, 64)
	c, err3 := strconv.ParseFloat(minLon, 64)
	d, err4 := strconv.ParseFloat(maxLon, 64)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return nil, errInvalidBBox
	}
	if !finite(a) || !finite(b) || !finite(c) || !finite(d) {
		return nil, errInvalidBBox
	}
	if a > b || c > d {
		return nil, errInvalidBBox
	}
	if a < -90 || b > 90 || c < -180 || d > 180 {
		return nil, errInvalidBBox
	}
	return &store.BBox{MinLatitude: a, MaxLatitude: b, MinLongitude: c, MaxLongitude: d}, nil
}

var errInvalidBBox = strconv.ErrSyntax

func finite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

func pointsETag(f store.ListFilter, meta store.ListMeta, slim bool) string {
	raw := fmt.Sprintf("%d|%d|%d|%s|%d|%d|%t", meta.Count, meta.MaxTS, meta.MaxUpdated, f.Order, f.Page, f.PerPage, slim)
	sum := sha256.Sum256([]byte(raw))
	return `"` + hex.EncodeToString(sum[:8]) + `"`
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
