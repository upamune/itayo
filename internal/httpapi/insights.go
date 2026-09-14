package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/upamune/itayo/internal/insights"
	"github.com/upamune/itayo/internal/store"
)

func (s *Server) insightsOverview(w http.ResponseWriter, r *http.Request) {
	years, year, unit, err := s.insightsScope(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load insights"})
		return
	}
	start, end := insights.YearBounds(year, s.cfg.Location)
	coords, err := s.store.CoordsBetween(r.Context(), start, end)
	if err != nil {
		slog.Error("insights coords", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load insights"})
		return
	}
	writeJSON(w, http.StatusOK, insights.ComputeOverview(coords, years, year, s.cfg.Location, unit))
}

func (s *Server) insightsDetails(w http.ResponseWriter, r *http.Request) {
	_, year, unit, err := s.insightsScope(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load insights"})
		return
	}
	start, end := insights.YearBounds(year, s.cfg.Location)
	thisYear, err := s.store.CoordsBetween(r.Context(), start, end)
	if err != nil {
		slog.Error("insights details", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load insights"})
		return
	}
	prevStart, prevEnd := insights.YearBounds(year-1, s.cfg.Location)
	prevYear, err := s.store.CoordsBetween(r.Context(), prevStart, prevEnd)
	if err != nil {
		slog.Error("insights details prev", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load insights"})
		return
	}
	writeJSON(w, http.StatusOK, insights.ComputeDetails(thisYear, prevYear, year, s.cfg.Location, unit))
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	years, err := s.store.YearsWithPoints(r.Context(), s.cfg.Location)
	if err != nil {
		slog.Error("stats years", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load stats"})
		return
	}
	byYear := map[int][]store.Coord{}
	for _, year := range years {
		start, end := insights.YearBounds(year, s.cfg.Location)
		coords, err := s.store.CoordsBetween(r.Context(), start, end)
		if err != nil {
			slog.Error("stats coords", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load stats"})
			return
		}
		byYear[year] = coords
	}
	n, err := s.store.PointCount(r.Context())
	if err != nil {
		slog.Error("stats count", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load stats"})
		return
	}
	writeJSON(w, http.StatusOK, insights.ComputeStats(byYear, n, s.cfg.Location))
}

func (s *Server) insightsScope(r *http.Request) (years []int, year int, unit string, err error) {
	years, err = s.store.YearsWithPoints(r.Context(), s.cfg.Location)
	if err != nil {
		slog.Error("insights years", "err", err)
		return nil, 0, "", err
	}
	unit = r.URL.Query().Get("distance_unit")
	if unit == "" {
		settings, loadErr := s.loadSettings(r)
		if loadErr == nil {
			if maps, ok := settings["maps"].(map[string]any); ok {
				if u, ok := maps["distance_unit"].(string); ok {
					unit = u
				}
			}
		}
	}
	if raw := r.URL.Query().Get("year"); raw != "" {
		if n, convErr := strconv.Atoi(raw); convErr == nil {
			year = n
		}
	}
	if year == 0 {
		if len(years) > 0 {
			year = years[0]
		} else {
			year = time.Now().In(s.cfg.Location).Year()
		}
	}
	return years, year, unit, nil
}
