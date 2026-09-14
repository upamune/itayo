package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	DefaultListenAddr   = ":8790"
	DefaultTimeZone     = "Asia/Tokyo"
	DefaultDatabasePath = "./itayo.sqlite"
	DefaultUserEmail    = "itayo@example.com"
	DefaultUserTheme    = "light"
	DefaultLogFormat    = "text"
)

// Config is process configuration loaded from the environment.
type Config struct {
	APIKey       string
	ListenAddr   string
	TimeZone     string
	Location     *time.Location
	DatabasePath string
	UserEmail    string
	UserTheme    string
	LogFormat    string
}

// Load reads configuration from the environment.
func Load() (Config, error) {
	cfg := Config{
		APIKey:       os.Getenv("API_KEY"),
		ListenAddr:   envOr("LISTEN_ADDR", DefaultListenAddr),
		TimeZone:     envOr("TIME_ZONE", DefaultTimeZone),
		DatabasePath: envOr("DATABASE_PATH", DefaultDatabasePath),
		UserEmail:    envOr("USER_EMAIL", DefaultUserEmail),
		UserTheme:    envOr("USER_THEME", DefaultUserTheme),
		LogFormat:    strings.ToLower(envOr("LOG_FORMAT", DefaultLogFormat)),
	}
	loc, err := time.LoadLocation(cfg.TimeZone)
	if err != nil {
		return Config{}, fmt.Errorf("TIME_ZONE %q: %w", cfg.TimeZone, err)
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return Config{}, fmt.Errorf("API_KEY is required")
	}
	if cfg.LogFormat != "text" && cfg.LogFormat != "json" {
		return Config{}, fmt.Errorf("LOG_FORMAT must be text or json")
	}
	if strings.TrimSpace(cfg.UserEmail) == "" {
		cfg.UserEmail = DefaultUserEmail
	}
	if strings.TrimSpace(cfg.UserTheme) == "" {
		cfg.UserTheme = DefaultUserTheme
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
