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
	"github.com/upamune/itayo/internal/logx"
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
	initLogger(cfg.LogFormat)

	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer st.Close()

	handler, err := httpapi.New(cfg, st)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
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
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func initLogger(format string) {
	slog.SetDefault(slog.New(logx.NewHandler(format)))
}
