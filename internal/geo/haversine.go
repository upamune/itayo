package geo

import "math"

const earthRadiusM = 6_371_000

// DistanceMeters is the great-circle distance between two WGS84 points.
func DistanceMeters(lat1, lon1, lat2, lon2 float64) float64 {
	φ1 := lat1 * math.Pi / 180
	φ2 := lat2 * math.Pi / 180
	Δφ := (lat2 - lat1) * math.Pi / 180
	Δλ := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	return 2 * earthRadiusM * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// ConvertDistance converts meters into km or mi.
func ConvertDistance(meters float64, unit string) float64 {
	if unit == "mi" {
		return meters / 1609.344
	}
	return meters / 1000
}
