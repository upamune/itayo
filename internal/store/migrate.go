package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const currentSchemaVersion = 2

type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{1, `
CREATE TABLE IF NOT EXISTS points (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  timestamp INTEGER NOT NULL,
  latitude REAL NOT NULL,
  longitude REAL NOT NULL,
  altitude REAL,
  accuracy REAL,
  vertical_accuracy REAL,
  velocity REAL,
  course REAL,
  course_accuracy REAL,
  battery INTEGER,
  battery_status TEXT,
  tracker_id TEXT,
  ssid TEXT,
  raw_data TEXT,
  created_at INTEGER NOT NULL DEFAULT (unixepoch()),
  updated_at INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE UNIQUE INDEX IF NOT EXISTS points_upsert ON points(timestamp, latitude, longitude);
CREATE TABLE IF NOT EXISTS settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  payload TEXT NOT NULL
);
`},
	{2, `
CREATE INDEX IF NOT EXISTS points_timestamp ON points(timestamp);
CREATE INDEX IF NOT EXISTS points_bbox ON points(latitude, longitude);
CREATE TABLE IF NOT EXISTS identity (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  email TEXT NOT NULL,
  theme TEXT NOT NULL DEFAULT 'light',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS mobile_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  payload TEXT NOT NULL,
  updated_at TEXT
);
`},
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return fmt.Errorf("schema_migrations: %w", err)
	}

	current, err := s.schemaVersion(ctx)
	if err != nil {
		return err
	}
	if current == 0 {
		exists, err := s.tableExists(ctx, "points")
		if err != nil {
			return err
		}
		if exists {
			// MVP databases created before versioning are schema v1.
			if err := s.recordVersion(ctx, 1); err != nil {
				return err
			}
			current = 1
		}
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if _, err := s.db.ExecContext(ctx, m.sql); err != nil {
			return fmt.Errorf("migrate v%d: %w", m.version, err)
		}
		if err := s.recordVersion(ctx, m.version); err != nil {
			return err
		}
	}
	return s.ensureWAL(ctx)
}

func (s *Store) schemaVersion(ctx context.Context) (int, error) {
	var v sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v)
	if err != nil {
		return 0, err
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}

func (s *Store) recordVersion(ctx context.Context, v int) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)`, v)
	return err
}

func (s *Store) tableExists(ctx context.Context, name string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	return n > 0, err
}

func (s *Store) ensureWAL(ctx context.Context) error {
	if s.memory {
		return nil
	}
	var mode string
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil {
		return fmt.Errorf("journal_mode: %w", err)
	}
	if !strings.EqualFold(mode, "wal") {
		if _, err := s.db.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
			return fmt.Errorf("enable wal: %w", err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA synchronous=NORMAL`); err != nil {
		return fmt.Errorf("synchronous: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA wal_autocheckpoint=1000`); err != nil {
		return fmt.Errorf("wal_autocheckpoint: %w", err)
	}
	return nil
}

// SchemaVersion returns the applied schema version.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	return s.schemaVersion(ctx)
}

// JournalMode returns the SQLite journal mode.
func (s *Store) JournalMode(ctx context.Context) (string, error) {
	var mode string
	err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode)
	return mode, err
}
