package config

import (
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("API_KEY", "your-secret")
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("TIME_ZONE", "")
	t.Setenv("DATABASE_PATH", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != DefaultListenAddr {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.TimeZone != DefaultTimeZone {
		t.Fatalf("TimeZone = %q", cfg.TimeZone)
	}
	if cfg.Location == nil || cfg.Location.String() != DefaultTimeZone {
		t.Fatalf("Location = %v", cfg.Location)
	}
	if cfg.DatabasePath != DefaultDatabasePath {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.APIKey != "your-secret" {
		t.Fatalf("APIKey = %q", cfg.APIKey)
	}
}

func TestLoadInvalidTimeZone(t *testing.T) {
	t.Setenv("TIME_ZONE", "Not/AZone")
	if _, err := Load(); err == nil {
		t.Fatal("expected error")
	}
}
