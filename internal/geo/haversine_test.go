package geo

import (
	"math"
	"testing"
)

func TestDistanceMetersTokyoStationToTokyoTower(t *testing.T) {
	t.Parallel()
	// Roughly 3.2 km.
	d := DistanceMeters(35.681236, 139.767125, 35.658581, 139.745433)
	if d < 3000 || d > 3500 {
		t.Fatalf("distance = %v", d)
	}
}

func TestConvertDistance(t *testing.T) {
	t.Parallel()
	if got := ConvertDistance(1000, "km"); math.Abs(got-1) > 1e-9 {
		t.Fatalf("km = %v", got)
	}
	if got := ConvertDistance(1609.344, "mi"); math.Abs(got-1) > 1e-9 {
		t.Fatalf("mi = %v", got)
	}
}
