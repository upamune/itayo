package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
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
	UpdatedAt        int64
}

// Coord is a lightweight point used for distance aggregates.
type Coord struct {
	Timestamp int64
	Latitude  float64
	Longitude float64
}

// BBox is an inclusive geographic window. All four fields must be valid.
type BBox struct {
	MinLatitude  float64
	MaxLatitude  float64
	MinLongitude float64
	MaxLongitude float64
}

// ListFilter selects stored points.
type ListFilter struct {
	StartAt *int64
	EndAt   int64
	Order   string
	Page    int
	PerPage int
	BBox    *BBox
}

// YearMonths is one year of tracked month abbreviations (Jan, Feb, ...).
type YearMonths struct {
	Year   int
	Months []string
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

func normalizeListFilter(f ListFilter) ListFilter {
	if f.PerPage <= 0 {
		f.PerPage = 100
	}
	if f.PerPage > 10_000 {
		f.PerPage = 10_000
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.EndAt == 0 {
		f.EndAt = time.Now().Unix()
	}
	return f
}

func listWhere(f ListFilter) (string, []any) {
	args := []any{f.EndAt}
	where := "WHERE timestamp <= ?"
	if f.StartAt != nil {
		where += " AND timestamp >= ?"
		args = append(args, *f.StartAt)
	}
	if f.BBox != nil {
		where += " AND latitude BETWEEN ? AND ? AND longitude BETWEEN ? AND ?"
		args = append(args, f.BBox.MinLatitude, f.BBox.MaxLatitude, f.BBox.MinLongitude, f.BBox.MaxLongitude)
	}
	return where, args
}

// List returns a page of points and the total matching count.
func (s *Store) List(ctx context.Context, f ListFilter) ([]Point, int, error) {
	f = normalizeListFilter(f)
	order := "DESC"
	if strings.EqualFold(f.Order, "asc") {
		order = "ASC"
	}
	where, args := listWhere(f)

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

// ListMeta is the cache fingerprint for GET /points.
type ListMeta struct {
	Count      int
	MaxTS      int64
	MaxUpdated int64
}

// ListFingerprint returns count and newest timestamps for ETag generation.
func (s *Store) ListFingerprint(ctx context.Context, f ListFilter) (ListMeta, error) {
	f = normalizeListFilter(f)
	where, args := listWhere(f)
	var meta ListMeta
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*), COALESCE(MAX(timestamp),0), COALESCE(MAX(updated_at),0) FROM points "+where,
		args...,
	).Scan(&meta.Count, &meta.MaxTS, &meta.MaxUpdated)
	return meta, err
}

// CoordsBetween returns timestamped coordinates in ascending time order.
func (s *Store) CoordsBetween(ctx context.Context, start, end int64) ([]Coord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT timestamp, latitude, longitude FROM points
WHERE timestamp >= ? AND timestamp <= ?
ORDER BY timestamp ASC`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Coord{}
	for rows.Next() {
		var c Coord
		if err := rows.Scan(&c.Timestamp, &c.Latitude, &c.Longitude); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PointCount returns the number of stored points.
func (s *Store) PointCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM points`).Scan(&n)
	return n, err
}

// TrackedMonths groups points into year → English month abbreviations in loc.
func (s *Store) TrackedMonths(ctx context.Context, loc *time.Location) ([]YearMonths, error) {
	if loc == nil {
		loc = time.UTC
	}
	rows, err := s.db.QueryContext(ctx, `SELECT MIN(timestamp) FROM points GROUP BY timestamp / 86400`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type key struct {
		year  int
		month time.Month
	}
	seen := map[key]struct{}{}
	order := []int{}
	monthsByYear := map[int]map[time.Month]struct{}{}
	for rows.Next() {
		var ts int64
		if err := rows.Scan(&ts); err != nil {
			return nil, err
		}
		tm := time.Unix(ts, 0).In(loc)
		k := key{year: tm.Year(), month: tm.Month()}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		if _, ok := monthsByYear[k.year]; !ok {
			monthsByYear[k.year] = map[time.Month]struct{}{}
			order = append(order, k.year)
		}
		monthsByYear[k.year][k.month] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Years descending, months calendar-ascending.
	for i := 0; i < len(order); i++ {
		for j := i + 1; j < len(order); j++ {
			if order[j] > order[i] {
				order[i], order[j] = order[j], order[i]
			}
		}
	}
	out := make([]YearMonths, 0, len(order))
	for _, year := range order {
		ym := YearMonths{Year: year, Months: make([]string, 0, 12)}
		for m := time.January; m <= time.December; m++ {
			if _, ok := monthsByYear[year][m]; ok {
				ym.Months = append(ym.Months, m.String()[:3])
			}
		}
		out = append(out, ym)
	}
	return out, nil
}

// YearsWithPoints returns descending calendar years that contain at least one point.
func (s *Store) YearsWithPoints(ctx context.Context, loc *time.Location) ([]int, error) {
	months, err := s.TrackedMonths(ctx, loc)
	if err != nil {
		return nil, err
	}
	years := make([]int, 0, len(months))
	for _, ym := range months {
		years = append(years, ym.Year)
	}
	return years, nil
}
