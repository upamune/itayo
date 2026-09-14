package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestUpsertAndList(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	lat, lon := 35.681236, 139.767125
	ts := int64(1_710_000_000)
	batt := 85
	tid := "device-1"
	got, err := s.Upsert(ctx, []Point{{
		Timestamp: ts,
		Latitude:  lat,
		Longitude: lon,
		Battery:   &batt,
		TrackerID: &tid,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID == 0 {
		t.Fatalf("upsert = %+v", got)
	}

	batt2 := 40
	again, err := s.Upsert(ctx, []Point{{
		Timestamp: ts,
		Latitude:  lat,
		Longitude: lon,
		Battery:   &batt2,
		TrackerID: &tid,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if again[0].ID != got[0].ID {
		t.Fatalf("expected same id, got %d vs %d", again[0].ID, got[0].ID)
	}
	if again[0].Battery == nil || *again[0].Battery != 40 {
		t.Fatalf("battery = %v", again[0].Battery)
	}

	list, total, err := s.List(ctx, ListFilter{EndAt: ts + 10, Order: "asc", Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("list total=%d n=%d", total, len(list))
	}
}

func TestListFiltersAndBBox(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	points := []Point{
		{Timestamp: 100, Latitude: 1, Longitude: 2},
		{Timestamp: 200, Latitude: 3, Longitude: 4},
		{Timestamp: 300, Latitude: 5, Longitude: 6},
	}
	if _, err := s.Upsert(ctx, points); err != nil {
		t.Fatal(err)
	}
	start := int64(150)
	list, total, err := s.List(ctx, ListFilter{StartAt: &start, EndAt: 250, Order: "desc", Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || list[0].Timestamp != 200 {
		t.Fatalf("filtered = total=%d %+v", total, list)
	}

	page, total, err := s.List(ctx, ListFilter{EndAt: 400, Order: "asc", Page: 2, PerPage: 2})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(page) != 1 || page[0].Timestamp != 300 {
		t.Fatalf("page = total=%d %+v", total, page)
	}

	bbox, total, err := s.List(ctx, ListFilter{
		EndAt: 400, Order: "asc", Page: 1, PerPage: 10,
		BBox: &BBox{MinLatitude: 2.5, MaxLatitude: 3.5, MinLongitude: 3.5, MaxLongitude: 4.5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || bbox[0].Latitude != 3 {
		t.Fatalf("bbox = total=%d %+v", total, bbox)
	}
}

func TestWALAndSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "itayo.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	mode, err := s.JournalMode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(mode, "wal") {
		t.Fatalf("journal_mode = %q", mode)
	}
	v, err := s.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != currentSchemaVersion {
		t.Fatalf("schema = %d", v)
	}
	_ = s.Close()

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, err = s.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != currentSchemaVersion {
		t.Fatalf("reopen schema = %d", v)
	}
}

func TestMigrateFromMVP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE points (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  timestamp INTEGER NOT NULL,
  latitude REAL NOT NULL,
  longitude REAL NOT NULL,
  altitude REAL, accuracy REAL, vertical_accuracy REAL, velocity REAL,
  course REAL, course_accuracy REAL, battery INTEGER, battery_status TEXT,
  tracker_id TEXT, ssid TEXT, raw_data TEXT,
  created_at INTEGER NOT NULL DEFAULT (unixepoch()),
  updated_at INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE UNIQUE INDEX points_upsert ON points(timestamp, latitude, longitude);
CREATE TABLE settings (id INTEGER PRIMARY KEY CHECK (id = 1), payload TEXT NOT NULL);
INSERT INTO points (timestamp, latitude, longitude) VALUES (100, 1, 2);
`)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	v, err := s.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != currentSchemaVersion {
		t.Fatalf("migrated schema = %d", v)
	}
	n, err := s.PointCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("points lost in migration: %d", n)
	}
	if _, err := s.EnsureIdentity(ctx, "itayo@example.com", "light", time.Unix(1, 0).UTC()); err != nil {
		t.Fatal(err)
	}
}

func TestIdentityStableCreatedAt(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	first, err := s.EnsureIdentity(ctx, "itayo@example.com", "light", time.Unix(1_700_000_000, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	second, err := s.EnsureIdentity(ctx, "itayo@example.com", "light", time.Unix(1_800_000_000, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if first.CreatedAt != second.CreatedAt {
		t.Fatalf("created_at changed %q -> %q", first.CreatedAt, second.CreatedAt)
	}
	renamed, err := s.EnsureIdentity(ctx, "owner@example.com", "dark", time.Unix(1_900_000_000, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Email != "owner@example.com" || renamed.CreatedAt != first.CreatedAt {
		t.Fatalf("rename = %+v", renamed)
	}
}

func TestTrackedMonths(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, err := s.Upsert(ctx, []Point{
		{Timestamp: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC).Unix(), Latitude: 1, Longitude: 1},
		{Timestamp: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC).Unix(), Latitude: 2, Longitude: 2},
		{Timestamp: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC).Unix(), Latitude: 3, Longitude: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.TrackedMonths(ctx, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Year != 2025 || got[1].Year != 2024 {
		t.Fatalf("years = %+v", got)
	}
	if strings.Join(got[1].Months, ",") != "Jan,Mar" {
		t.Fatalf("2024 months = %v", got[1].Months)
	}
}

func TestTrackedMonthsUsesLocalTimezone(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	// Same UTC calendar day; 10:00 UTC is still 31 Jan in Tokyo, 20:00 UTC is 1 Feb.
	_, err = s.Upsert(ctx, []Point{
		{Timestamp: time.Date(2025, 1, 31, 10, 0, 0, 0, time.UTC).Unix(), Latitude: 35, Longitude: 139},
		{Timestamp: time.Date(2025, 1, 31, 20, 0, 0, 0, time.UTC).Unix(), Latitude: 35.1, Longitude: 139.1},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.TrackedMonths(ctx, tokyo)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Year != 2025 {
		t.Fatalf("years = %+v", got)
	}
	if strings.Join(got[0].Months, ",") != "Jan,Feb" {
		t.Fatalf("local months = %v, want Jan,Feb", got[0].Months)
	}
}

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "itayo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
