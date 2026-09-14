package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/upamune/itayo/internal/settings"
)

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	current, err := s.loadSettings(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load settings"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": current,
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
	patch, ok := settingsObject(body)
	if !ok {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"message": "Something went wrong",
			"errors":  []string{"settings is required"},
		})
		return
	}
	current, err := s.loadSettings(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load settings"})
		return
	}
	merged := settings.Apply(current, patch)
	raw, err := json.Marshal(merged)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save settings"})
		return
	}
	if err := s.store.SaveSettingsJSON(r.Context(), raw); err != nil {
		slog.Error("save settings", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save settings"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message":                 "Settings updated",
		"settings":                merged,
		"status":                  "success",
		"recalculation_triggered": false,
	})
}

func (s *Server) getMobileSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.mobileResponse(r, ""))
}

func (s *Server) patchMobileSettings(w http.ResponseWriter, r *http.Request) {
	body, err := decodeObject(r)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"message": "Something went wrong",
			"errors":  []string{"invalid json"},
		})
		return
	}
	patch, ok := settingsObject(body)
	if !ok {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"message": "Something went wrong",
			"errors":  []string{"settings is required"},
		})
		return
	}
	stored, err := s.store.LoadMobileSettings(r.Context())
	if err != nil {
		slog.Error("load mobile settings", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load settings"})
		return
	}
	existing := map[string]any{}
	if len(stored.Payload) > 0 {
		if err := json.Unmarshal(stored.Payload, &existing); err != nil {
			existing = map[string]any{}
		}
	}
	merged := settings.MergeMobile(existing, settings.SanitizeMobile(patch))
	stamp := s.now().UTC().Format("2006-01-02T15:04:05Z")
	raw, err := json.Marshal(merged)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save settings"})
		return
	}
	if err := s.store.SaveMobileSettings(r.Context(), raw, stamp); err != nil {
		slog.Error("save mobile settings", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save settings"})
		return
	}
	keys := make([]string, 0, len(patch))
	for k := range settings.SanitizeMobile(patch) {
		keys = append(keys, k)
	}
	slog.Info("mobile settings updated", "keys", keys)
	writeJSON(w, http.StatusOK, s.mobileResponse(r, "Settings updated"))
}

func (s *Server) mobileResponse(r *http.Request, message string) map[string]any {
	stored, err := s.store.LoadMobileSettings(r.Context())
	if err != nil {
		slog.Error("load mobile settings", "err", err)
	}
	blob := map[string]any{}
	if len(stored.Payload) > 0 {
		_ = json.Unmarshal(stored.Payload, &blob)
	}
	var updated any
	if stored.UpdatedAt != "" {
		updated = stored.UpdatedAt
	}
	out := map[string]any{
		"settings":   settings.PublicMobile(blob),
		"updated_at": updated,
		"capabilities": map[string]any{
			"photo_library_import": map[string]any{"version": settings.PhotoLibraryImportVersion},
		},
		"status": "success",
	}
	if message != "" {
		out["message"] = message
	}
	return out
}

func (s *Server) loadSettings(r *http.Request) (map[string]any, error) {
	raw, err := s.store.SettingsJSON(r.Context())
	if err != nil {
		return nil, err
	}
	return settings.Load(raw, s.cfg.TimeZone), nil
}
