package store

import (
	"context"
	"database/sql"
	"time"
)

// Identity is the single-user record returned by /users/me.
type Identity struct {
	Email     string
	Theme     string
	CreatedAt string
	UpdatedAt string
}

// EnsureIdentity creates the singleton identity on first boot and
// updates email when the configured value changes. created_at is stable.
func (s *Store) EnsureIdentity(ctx context.Context, email, theme string, now time.Time) (Identity, error) {
	if email == "" {
		email = "itayo@example.com"
	}
	if theme == "" {
		theme = "light"
	}
	stamp := now.UTC().Format(time.RFC3339)

	var got Identity
	err := s.db.QueryRowContext(ctx, `SELECT email, theme, created_at, updated_at FROM identity WHERE id = 1`).Scan(
		&got.Email, &got.Theme, &got.CreatedAt, &got.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		_, err = s.db.ExecContext(ctx, `
INSERT INTO identity (id, email, theme, created_at, updated_at) VALUES (1, ?, ?, ?, ?)`,
			email, theme, stamp, stamp)
		if err != nil {
			return Identity{}, err
		}
		return Identity{Email: email, Theme: theme, CreatedAt: stamp, UpdatedAt: stamp}, nil
	}
	if err != nil {
		return Identity{}, err
	}

	if got.Email != email || got.Theme != theme {
		_, err = s.db.ExecContext(ctx, `
UPDATE identity SET email=?, theme=?, updated_at=? WHERE id=1`, email, theme, stamp)
		if err != nil {
			return Identity{}, err
		}
		got.Email = email
		got.Theme = theme
		got.UpdatedAt = stamp
	}
	return got, nil
}
