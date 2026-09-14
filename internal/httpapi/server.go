package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/upamune/itayo/internal/auth"
	"github.com/upamune/itayo/internal/config"
	"github.com/upamune/itayo/internal/store"
	"github.com/upamune/itayo/internal/version"
)

// Server is the Dawarich-compatible HTTP API.
type Server struct {
	cfg   config.Config
	store *store.Store
	now   func() time.Time
}

// New returns an http.Handler for itayo. Identity seed failure is fatal.
func New(cfg config.Config, st *store.Store) (http.Handler, error) {
	s := &Server{
		cfg:   cfg,
		store: st,
		now:   time.Now,
	}
	if _, err := st.EnsureIdentity(context.Background(), cfg.UserEmail, cfg.UserTheme, s.now()); err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("POST /api/v1/points", s.authed(s.createPoints))
	mux.HandleFunc("GET /api/v1/points", s.authed(s.listPoints))
	mux.HandleFunc("GET /api/v1/points/tracked_months", s.authed(s.trackedMonths))
	mux.HandleFunc("GET /api/v1/users/me", s.authed(s.usersMe))
	mux.HandleFunc("GET /api/v1/settings", s.authed(s.getSettings))
	mux.HandleFunc("PATCH /api/v1/settings", s.authed(s.patchSettings))
	mux.HandleFunc("GET /api/v1/settings/mobile", s.authed(s.getMobileSettings))
	mux.HandleFunc("PATCH /api/v1/settings/mobile", s.authed(s.patchMobileSettings))
	mux.HandleFunc("GET /api/v1/plan", s.authed(s.plan))
	mux.HandleFunc("GET /api/v1/insights", s.authed(s.insightsOverview))
	mux.HandleFunc("GET /api/v1/insights/details", s.authed(s.insightsDetails))
	mux.HandleFunc("GET /api/v1/stats", s.authed(s.stats))
	return s.withRequestLog(s.withCompatHeaders(s.withRecover(mux))), nil
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

func (s *Server) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.code,
			"ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func (s *Server) withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic", "err", rec, "stack", string(debug.Stack()))
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
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
	return auth.Equal(auth.KeyFromRequest(r), s.cfg.APIKey)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
