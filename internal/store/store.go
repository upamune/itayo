package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// Store is a SQLite-backed point, settings, and identity store.
type Store struct {
	db     *sql.DB
	memory bool
}

// Open opens (and migrates) a SQLite database at path.
func Open(path string) (*Store, error) {
	if path == "" {
		path = "./itayo.sqlite"
	}
	memory := path == ":memory:" || strings.Contains(path, "mode=memory")
	dsn := path
	if !strings.Contains(path, "?") && path != ":memory:" && !strings.HasPrefix(path, "file:") {
		dsn = "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, memory: memory}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
