package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

// SettingsJSON returns the persisted settings payload, or empty if unset.
func (s *Store) SettingsJSON(ctx context.Context) (json.RawMessage, error) {
	var payload string
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM settings WHERE id = 1`).Scan(&payload)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

// SaveSettingsJSON upserts the settings payload.
func (s *Store) SaveSettingsJSON(ctx context.Context, payload json.RawMessage) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO settings (id, payload) VALUES (1, ?)
ON CONFLICT(id) DO UPDATE SET payload=excluded.payload`, string(payload))
	return err
}

// MobileSettings is the persisted mobile sync blob plus server-stamped updated_at.
type MobileSettings struct {
	Payload   json.RawMessage
	UpdatedAt string
}

// LoadMobileSettings returns the stored mobile settings, or empty if unset.
func (s *Store) LoadMobileSettings(ctx context.Context) (MobileSettings, error) {
	var payload sql.NullString
	var updated sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT payload, updated_at FROM mobile_settings WHERE id = 1`).Scan(&payload, &updated)
	if err == sql.ErrNoRows {
		return MobileSettings{}, nil
	}
	if err != nil {
		return MobileSettings{}, err
	}
	out := MobileSettings{UpdatedAt: updated.String}
	if payload.Valid && payload.String != "" {
		out.Payload = json.RawMessage(payload.String)
	}
	return out, nil
}

// SaveMobileSettings upserts the mobile settings blob.
func (s *Store) SaveMobileSettings(ctx context.Context, payload json.RawMessage, updatedAt string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO mobile_settings (id, payload, updated_at) VALUES (1, ?, ?)
ON CONFLICT(id) DO UPDATE SET payload=excluded.payload, updated_at=excluded.updated_at`,
		string(payload), updatedAt)
	return err
}
