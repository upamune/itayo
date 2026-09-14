package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/upamune/itayo/internal/config"
	"github.com/upamune/itayo/internal/httpapi"
	"github.com/upamune/itayo/internal/store"
	"github.com/upamune/itayo/internal/version"
)

func main() {
	if err := run(); err != nil {
		slog.Error("itayo", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.APIKey == "" {
		slog.Warn("API_KEY is empty; authenticated endpoints will return 401")
	}
	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer st.Close()

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           httpapi.New(cfg, st),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening",
			"addr", cfg.ListenAddr,
			"compat", version.DawarichCompat,
			"version", version.Version,
			"tz", cfg.TimeZone,
		)
		errCh <- srv.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
