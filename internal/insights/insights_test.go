package insights

import (
	"testing"
	"time"

	"github.com/upamune/itayo/internal/store"
)

func TestComputeOverviewDistanceAndDays(t *testing.T) {
	t.Parallel()
	loc := time.UTC
	day := time.Date(2025, 6, 15, 10, 0, 0, 0, loc)
	coords := []store.Coord{
		{Timestamp: day.Unix(), Latitude: 35.0, Longitude: 139.0},
		{Timestamp: day.Add(10 * time.Minute).Unix(), Latitude: 35.01, Longitude: 139.01},
		{Timestamp: day.Add(24 * time.Hour).Unix(), Latitude: 35.02, Longitude: 139.02},
		{Timestamp: day.Add(24*time.Hour + 10*time.Minute).Unix(), Latitude: 35.03, Longitude: 139.03},
	}
	got := ComputeOverview(coords, []int{2025}, 2025, loc, "km")
	if got.Totals.DaysTraveling != 2 {
		t.Fatalf("days = %d", got.Totals.DaysTraveling)
	}
	if got.Totals.TotalDistance <= 0 {
		t.Fatalf("distance = %d", got.Totals.TotalDistance)
	}
	if got.Totals.CountriesCount != 0 || got.ActivityHeatmap.ActiveDays != 2 {
		t.Fatalf("overview = %+v", got)
	}
	if got.Totals.BiggestMonth == nil {
		t.Fatal("expected biggest month")
	}
}

func TestTeleportAndGapDropped(t *testing.T) {
	t.Parallel()
	loc := time.UTC
	a := time.Date(2025, 1, 1, 0, 0, 0, 0, loc)
	coords := []store.Coord{
		{Timestamp: a.Unix(), Latitude: 35.0, Longitude: 139.0},
		{Timestamp: a.Add(time.Second).Unix(), Latitude: -35.0, Longitude: -139.0},
		{Timestamp: a.Add(7 * time.Hour).Unix(), Latitude: 35.001, Longitude: 139.001},
	}
	got := ComputeOverview(coords, []int{2025}, 2025, loc, "km")
	if got.Totals.TotalDistance != 0 {
		t.Fatalf("teleport/gap should be dropped, got %d", got.Totals.TotalDistance)
	}
}

func TestComputeStatsMonthlyKeys(t *testing.T) {
	t.Parallel()
	loc := time.UTC
	day := time.Date(2024, 1, 2, 8, 0, 0, 0, loc)
	coords := []store.Coord{
		{Timestamp: day.Unix(), Latitude: 35.0, Longitude: 139.0},
		{Timestamp: day.Add(15 * time.Minute).Unix(), Latitude: 35.02, Longitude: 139.02},
	}
	got := ComputeStats(map[int][]store.Coord{2024: coords}, 2, loc)
	if got.TotalPointsTracked != 2 {
		t.Fatalf("points = %d", got.TotalPointsTracked)
	}
	if len(got.YearlyStats) != 1 || got.YearlyStats[0].MonthlyDistanceKm["january"] <= 0 {
		t.Fatalf("stats = %+v", got)
	}
}
