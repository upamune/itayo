package config

import (
	"fmt"
	"os"
	"time"
)

const (
	DefaultListenAddr   = ":8790"
	DefaultTimeZone     = "Asia/Tokyo"
	DefaultDatabasePath = "./itayo.sqlite"
)

// Config is process configuration loaded from the environment.
type Config struct {
	APIKey       string
	ListenAddr   string
	TimeZone     string
	Location     *time.Location
	DatabasePath string
}

// Load reads configuration from the environment.
func Load() (Config, error) {
	cfg := Config{
		APIKey:       os.Getenv("API_KEY"),
		ListenAddr:   envOr("LISTEN_ADDR", DefaultListenAddr),
		TimeZone:     envOr("TIME_ZONE", DefaultTimeZone),
		DatabasePath: envOr("DATABASE_PATH", DefaultDatabasePath),
	}
	loc, err := time.LoadLocation(cfg.TimeZone)
	if err != nil {
		return Config{}, fmt.Errorf("TIME_ZONE %q: %w", cfg.TimeZone, err)
	}
	cfg.Location = loc
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
