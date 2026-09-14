package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/upamune/itayo/internal/settings"
)

func (s *Server) usersMe(w http.ResponseWriter, r *http.Request) {
	id, err := s.store.EnsureIdentity(r.Context(), s.cfg.UserEmail, s.cfg.UserTheme, s.now())
	if err != nil {
		slog.Error("load identity", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load user"})
		return
	}
	current, err := s.loadSettings(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load user"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"email":      id.Email,
			"theme":      id.Theme,
			"created_at": id.CreatedAt,
			"updated_at": id.UpdatedAt,
			"settings":   settings.UserView(current),
		},
		"features": map[string]any{
			"reverse_geocoding": false,
			"family":            false,
		},
	})
}

func (s *Server) plan(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"plan":                "pro",
		"effective_plan":      "pro",
		"status":              "active",
		"subscription_source": nil,
		"active_until":        "3000-01-01T00:00:00Z",
		"features": map[string]any{
			"heatmap":      true,
			"fog_of_war":   true,
			"scratch_map":  true,
			"globe_view":   true,
			"integrations": true,
			"write_api":    true,
			"sharing":      true,
			"full_digest":  true,
			"data_window":  nil,
		},
	})
}
