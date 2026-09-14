package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Point is a stored location sample.
type Point struct {
	ID               int64
	Timestamp        int64
	Latitude         float64
	Longitude        float64
	Altitude         *float64
	Accuracy         *float64
	VerticalAccuracy *float64
	Velocity         *float64
	Course           *float64
	CourseAccuracy   *float64
	Battery          *int
	BatteryStatus    *string
	TrackerID        *string
	SSID             *string
	RawData          json.RawMessage
}

// ListFilter selects stored points.
type ListFilter struct {
	StartAt *int64
	EndAt   int64
	Order   string
	Page    int
	PerPage int
}

// Store is a SQLite-backed point and settings store.
type Store struct {
	db *sql.DB
}

// Open opens (and migrates) a SQLite database at path.
func Open(path string) (*Store, error) {
	if path == "" {
		path = "./itayo.sqlite"
	}
	dsn := path
	if !strings.Contains(path, "?") && path != ":memory:" && !strings.HasPrefix(path, "file:") {
		dsn = "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
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
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Upsert inserts or updates points keyed by (timestamp, latitude, longitude).
func (s *Store) Upsert(ctx context.Context, points []Point) ([]Point, error) {
	if len(points) == 0 {
		return []Point{}, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO points (
  timestamp, latitude, longitude, altitude, accuracy, vertical_accuracy,
  velocity, course, course_accuracy, battery, battery_status, tracker_id, ssid, raw_data,
  created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, unixepoch(), unixepoch())
ON CONFLICT(timestamp, latitude, longitude) DO UPDATE SET
  altitude=excluded.altitude,
  accuracy=excluded.accuracy,
  vertical_accuracy=excluded.vertical_accuracy,
  velocity=excluded.velocity,
  course=excluded.course,
  course_accuracy=excluded.course_accuracy,
  battery=excluded.battery,
  battery_status=excluded.battery_status,
  tracker_id=excluded.tracker_id,
  ssid=excluded.ssid,
  raw_data=excluded.raw_data,
  updated_at=unixepoch()
RETURNING id, timestamp, latitude, longitude, altitude, accuracy, vertical_accuracy,
  velocity, course, course_accuracy, battery, battery_status, tracker_id, ssid, raw_data`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	out := make([]Point, 0, len(points))
	for _, p := range points {
		var raw any
		if len(p.RawData) > 0 {
			raw = string(p.RawData)
		}
		row := stmt.QueryRowContext(ctx,
			p.Timestamp, p.Latitude, p.Longitude,
			p.Altitude, p.Accuracy, p.VerticalAccuracy,
			p.Velocity, p.Course, p.CourseAccuracy,
			p.Battery, p.BatteryStatus, p.TrackerID, p.SSID, raw,
		)
		got, scanErr := scanPoint(row)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, got)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPoint(row scanner) (Point, error) {
	var (
		p      Point
		alt    sql.NullFloat64
		acc    sql.NullFloat64
		vacc   sql.NullFloat64
		vel    sql.NullFloat64
		course sql.NullFloat64
		cacc   sql.NullFloat64
		batt   sql.NullInt64
		bst    sql.NullString
		tid    sql.NullString
		ssid   sql.NullString
		raw    sql.NullString
	)
	err := row.Scan(
		&p.ID, &p.Timestamp, &p.Latitude, &p.Longitude,
		&alt, &acc, &vacc, &vel, &course, &cacc, &batt, &bst, &tid, &ssid, &raw,
	)
	if err != nil {
		return Point{}, err
	}
	p.Altitude = nullFloat(alt)
	p.Accuracy = nullFloat(acc)
	p.VerticalAccuracy = nullFloat(vacc)
	p.Velocity = nullFloat(vel)
	p.Course = nullFloat(course)
	p.CourseAccuracy = nullFloat(cacc)
	if batt.Valid {
		v := int(batt.Int64)
		p.Battery = &v
	}
	p.BatteryStatus = nullString(bst)
	p.TrackerID = nullString(tid)
	p.SSID = nullString(ssid)
	if raw.Valid && raw.String != "" {
		p.RawData = json.RawMessage(raw.String)
	}
	return p, nil
}

func nullFloat(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

func nullString(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	v := n.String
	return &v
}

// List returns a page of points and the total matching count.
func (s *Store) List(ctx context.Context, f ListFilter) ([]Point, int, error) {
	if f.PerPage <= 0 {
		f.PerPage = 100
	}
	if f.PerPage > 10_000 {
		f.PerPage = 10_000
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	order := "DESC"
	if strings.EqualFold(f.Order, "asc") {
		order = "ASC"
	}
	if f.EndAt == 0 {
		f.EndAt = time.Now().Unix()
	}

	args := []any{}
	where := "WHERE timestamp <= ?"
	args = append(args, f.EndAt)
	if f.StartAt != nil {
		where += " AND timestamp >= ?"
		args = append(args, *f.StartAt)
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM points "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (f.Page - 1) * f.PerPage
	q := fmt.Sprintf(`SELECT id, timestamp, latitude, longitude, altitude, accuracy, vertical_accuracy,
  velocity, course, course_accuracy, battery, battery_status, tracker_id, ssid, raw_data
FROM points %s ORDER BY timestamp %s LIMIT ? OFFSET ?`, where, order)
	args = append(args, f.PerPage, offset)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []Point{}
	for rows.Next() {
		p, scanErr := scanPoint(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

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
