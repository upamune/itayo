package store

import (
	"context"
	"path/filepath"
	"testing"
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

func TestListFilters(t *testing.T) {
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
