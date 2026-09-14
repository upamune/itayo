package insights

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/upamune/itayo/internal/geo"
	"github.com/upamune/itayo/internal/store"
)

const (
	maxGap     = 6 * time.Hour
	maxSpeedMS = 300 // ~1080 km/h; drops teleport jumps
)

// Overview is GET /api/v1/insights.
type Overview struct {
	Year            int     `json:"year"`
	AvailableYears  []int   `json:"availableYears"`
	Totals          Totals  `json:"totals"`
	ActivityHeatmap Heatmap `json:"activityHeatmap"`
	PlanRestricted  bool    `json:"planRestricted"`
	UpgradeURL      *string `json:"upgradeUrl"`
}

// Totals is the year summary. countries/cities stay 0 without geocoding.
type Totals struct {
	TotalDistance  int      `json:"totalDistance"`
	DistanceUnit   string   `json:"distanceUnit"`
	CountriesCount int      `json:"countriesCount"`
	CitiesCount    int      `json:"citiesCount"`
	CountriesList  []string `json:"countriesList"`
	DaysTraveling  int      `json:"daysTraveling"`
	BiggestMonth   any      `json:"biggestMonth"`
}

// BiggestMonth is the highest-distance calendar month.
type BiggestMonth struct {
	Month    string `json:"month"`
	Distance int    `json:"distance"`
}

// Heatmap is the Insights activity grid.
type Heatmap struct {
	DailyData          map[string]int `json:"dailyData"`
	ActivityLevels     Levels         `json:"activityLevels"`
	MaxDistance        int            `json:"maxDistance"`
	ActiveDays         int            `json:"activeDays"`
	CurrentStreak      int            `json:"currentStreak"`
	LongestStreak      int            `json:"longestStreak"`
	LongestStreakStart *string        `json:"longestStreakStart"`
	LongestStreakEnd   *string        `json:"longestStreakEnd"`
}

// Levels are meter percentiles used to color the heatmap.
type Levels struct {
	P25 int `json:"p25"`
	P50 int `json:"p50"`
	P75 int `json:"p75"`
	P90 int `json:"p90"`
}

// Details is GET /api/v1/insights/details.
type Details struct {
	Year           int            `json:"year"`
	Comparison     any            `json:"comparison"`
	TravelPatterns TravelPatterns `json:"travelPatterns"`
	PlanRestricted bool           `json:"planRestricted"`
	UpgradeURL     *string        `json:"upgradeUrl"`
}

// Comparison is year-over-year deltas.
type Comparison struct {
	PreviousYear          int `json:"previousYear"`
	DistanceChangePercent int `json:"distanceChangePercent"`
	CountriesChange       int `json:"countriesChange"`
	CitiesChange          int `json:"citiesChange"`
	DaysChange            int `json:"daysChange"`
}

// TravelPatterns is computed from point timestamps. Locations stay empty.
type TravelPatterns struct {
	TimeOfDay           map[string]int `json:"timeOfDay"`
	DayOfWeek           []int          `json:"dayOfWeek"`
	Seasonality         map[string]int `json:"seasonality"`
	ActivityBreakdown   map[string]int `json:"activityBreakdown"`
	TopVisitedLocations []any          `json:"topVisitedLocations"`
}

// YearStats is one year in GET /api/v1/stats.
type YearStats struct {
	Year                  int            `json:"year"`
	TotalDistanceKm       int            `json:"totalDistanceKm"`
	TotalCountriesVisited int            `json:"totalCountriesVisited"`
	TotalCitiesVisited    int            `json:"totalCitiesVisited"`
	MonthlyDistanceKm     map[string]int `json:"monthlyDistanceKm"`
}

// Stats is GET /api/v1/stats.
type Stats struct {
	TotalDistanceKm            int         `json:"totalDistanceKm"`
	TotalPointsTracked         int         `json:"totalPointsTracked"`
	TotalReverseGeocodedPoints int         `json:"totalReverseGeocodedPoints"`
	TotalCountriesVisited      int         `json:"totalCountriesVisited"`
	TotalCitiesVisited         int         `json:"totalCitiesVisited"`
	YearlyStats                []YearStats `json:"yearlyStats"`
}

type dailyMeters map[string]int

// ComputeOverview builds Insights from stored coordinates. Geocoding fields are zero.
func ComputeOverview(coords []store.Coord, years []int, year int, loc *time.Location, unit string) Overview {
	if unit != "mi" {
		unit = "km"
	}
	if loc == nil {
		loc = time.UTC
	}
	daily := distancesByDay(coords, loc)
	totals := totalsFromDaily(daily, year, unit)
	return Overview{
		Year:            year,
		AvailableYears:  years,
		Totals:          totals,
		ActivityHeatmap: heatmapFromDaily(daily, year, loc),
		PlanRestricted:  false,
		UpgradeURL:      nil,
	}
}

// ComputeDetails builds year-over-year comparison and timestamp patterns.
func ComputeDetails(thisYear, prevYear []store.Coord, year int, loc *time.Location, unit string) Details {
	if loc == nil {
		loc = time.UTC
	}
	if unit != "mi" {
		unit = "km"
	}
	thisDaily := distancesByDay(thisYear, loc)
	thisTotals := totalsFromDaily(thisDaily, year, unit)
	var cmp any
	if len(prevYear) > 0 {
		prevDaily := distancesByDay(prevYear, loc)
		prevTotals := totalsFromDaily(prevDaily, year-1, unit)
		cmp = Comparison{
			PreviousYear:          year - 1,
			DistanceChangePercent: percentChange(thisTotals.TotalDistance, prevTotals.TotalDistance),
			CountriesChange:       0,
			CitiesChange:          0,
			DaysChange:            thisTotals.DaysTraveling - prevTotals.DaysTraveling,
		}
	}
	return Details{
		Year:           year,
		Comparison:     cmp,
		TravelPatterns: patternsFromCoords(thisYear, loc),
		PlanRestricted: false,
		UpgradeURL:     nil,
	}
}

// ComputeStats aggregates all years for GET /api/v1/stats.
func ComputeStats(byYear map[int][]store.Coord, totalPoints int, loc *time.Location) Stats {
	if loc == nil {
		loc = time.UTC
	}
	years := make([]int, 0, len(byYear))
	for y := range byYear {
		years = append(years, y)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(years)))

	yearly := make([]YearStats, 0, len(years))
	totalKm := 0
	for _, y := range years {
		daily := distancesByDay(byYear[y], loc)
		meters := 0
		monthly := emptyMonths()
		for day, m := range daily {
			meters += m
			if tm, err := time.ParseInLocation("2006-01-02", day, loc); err == nil {
				monthly[strings.ToLower(tm.Month().String())] += int(math.Round(geo.ConvertDistance(float64(m), "km")))
			}
		}
		km := int(math.Round(geo.ConvertDistance(float64(meters), "km")))
		totalKm += km
		yearly = append(yearly, YearStats{
			Year:                  y,
			TotalDistanceKm:       km,
			TotalCountriesVisited: 0,
			TotalCitiesVisited:    0,
			MonthlyDistanceKm:     monthly,
		})
	}
	return Stats{
		TotalDistanceKm:            totalKm,
		TotalPointsTracked:         totalPoints,
		TotalReverseGeocodedPoints: 0,
		TotalCountriesVisited:      0,
		TotalCitiesVisited:         0,
		YearlyStats:                yearly,
	}
}

func distancesByDay(coords []store.Coord, loc *time.Location) dailyMeters {
	out := dailyMeters{}
	for i := 1; i < len(coords); i++ {
		prev, cur := coords[i-1], coords[i]
		dt := time.Duration(cur.Timestamp-prev.Timestamp) * time.Second
		if dt <= 0 || dt > maxGap {
			continue
		}
		d := geo.DistanceMeters(prev.Latitude, prev.Longitude, cur.Latitude, cur.Longitude)
		if d <= 0 {
			continue
		}
		if d/dt.Seconds() > maxSpeedMS {
			continue
		}
		day := time.Unix(cur.Timestamp, 0).In(loc).Format("2006-01-02")
		out[day] += int(math.Round(d))
	}
	return out
}

func totalsFromDaily(daily dailyMeters, year int, unit string) Totals {
	monthMeters := map[time.Month]int{}
	total := 0
	days := 0
	for day, meters := range daily {
		if meters <= 0 {
			continue
		}
		tm, err := time.Parse("2006-01-02", day)
		if err != nil || tm.Year() != year {
			continue
		}
		total += meters
		days++
		monthMeters[tm.Month()] += meters
	}
	var biggest any
	bestM, bestD := time.Month(0), 0
	for m, d := range monthMeters {
		if d > bestD {
			bestM, bestD = m, d
		}
	}
	if bestD > 0 {
		biggest = BiggestMonth{
			Month:    bestM.String(),
			Distance: int(math.Round(geo.ConvertDistance(float64(bestD), unit))),
		}
	}
	return Totals{
		TotalDistance:  int(math.Round(geo.ConvertDistance(float64(total), unit))),
		DistanceUnit:   unit,
		CountriesCount: 0,
		CitiesCount:    0,
		CountriesList:  []string{},
		DaysTraveling:  days,
		BiggestMonth:   biggest,
	}
}

func heatmapFromDaily(daily dailyMeters, year int, loc *time.Location) Heatmap {
	filtered := map[string]int{}
	distances := []int{}
	for day, meters := range daily {
		tm, err := time.Parse("2006-01-02", day)
		if err != nil || tm.Year() != year || meters <= 0 {
			continue
		}
		filtered[day] = meters
		distances = append(distances, meters)
	}
	if len(filtered) == 0 {
		return Heatmap{
			DailyData:      map[string]int{},
			ActivityLevels: Levels{P25: 1000, P50: 5000, P75: 10_000, P90: 20_000},
		}
	}
	sort.Ints(distances)
	streak := streaks(filtered, year, loc)
	return Heatmap{
		DailyData:          filtered,
		ActivityLevels:     percentileLevels(distances),
		MaxDistance:        distances[len(distances)-1],
		ActiveDays:         len(distances),
		CurrentStreak:      streak.current,
		LongestStreak:      streak.longest,
		LongestStreakStart: streak.start,
		LongestStreakEnd:   streak.end,
	}
}

type streakResult struct {
	current int
	longest int
	start   *string
	end     *string
}

func streaks(daily map[string]int, year int, loc *time.Location) streakResult {
	dates := make([]time.Time, 0, len(daily))
	for day := range daily {
		tm, err := time.ParseInLocation("2006-01-02", day, loc)
		if err == nil {
			dates = append(dates, tm)
		}
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	if len(dates) == 0 {
		return streakResult{}
	}

	longest, cur := 1, 1
	longStart, longEnd := dates[0], dates[0]
	runStart := dates[0]
	for i := 1; i < len(dates); i++ {
		if dates[i].Equal(dates[i-1].AddDate(0, 0, 1)) {
			cur++
		} else {
			cur = 1
			runStart = dates[i]
		}
		if cur > longest {
			longest = cur
			longStart = runStart
			longEnd = dates[i]
		}
	}
	start := longStart.Format("2006-01-02")
	end := longEnd.Format("2006-01-02")

	today := time.Now().In(loc)
	yearEnd := time.Date(year, 12, 31, 0, 0, 0, 0, loc)
	ref := today
	if ref.After(yearEnd) {
		ref = yearEnd
	}
	set := map[string]struct{}{}
	for _, d := range dates {
		set[d.Format("2006-01-02")] = struct{}{}
	}
	current := 0
	check := time.Date(ref.Year(), ref.Month(), ref.Day(), 0, 0, 0, 0, loc)
	for {
		if _, ok := set[check.Format("2006-01-02")]; !ok {
			break
		}
		current++
		check = check.AddDate(0, 0, -1)
	}
	if current == 0 {
		check = time.Date(ref.Year(), ref.Month(), ref.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -1)
		for {
			if _, ok := set[check.Format("2006-01-02")]; !ok {
				break
			}
			current++
			check = check.AddDate(0, 0, -1)
		}
	}
	return streakResult{current: current, longest: longest, start: &start, end: &end}
}

func percentileLevels(sorted []int) Levels {
	return Levels{
		P25: percentile(sorted, 25),
		P50: percentile(sorted, 50),
		P75: percentile(sorted, 75),
		P90: percentile(sorted, 90),
	}
}

func percentile(sorted []int, pct int) int {
	if len(sorted) == 0 {
		return 0
	}
	k := max(int(math.Round(float64(pct)/100.0*float64(len(sorted)-1))), 0)
	if k >= len(sorted) {
		k = len(sorted) - 1
	}
	return sorted[k]
}

func patternsFromCoords(coords []store.Coord, loc *time.Location) TravelPatterns {
	tod := map[string]int{}
	dow := make([]int, 7)
	season := map[string]int{"winter": 0, "spring": 0, "summer": 0, "autumn": 0}
	for _, c := range coords {
		tm := time.Unix(c.Timestamp, 0).In(loc)
		tod[tm.Format("15")]++
		// Sunday=0 to match Dawarich's weekly_pattern.
		dow[int(tm.Weekday())]++
		switch tm.Month() {
		case time.December, time.January, time.February:
			season["winter"]++
		case time.March, time.April, time.May:
			season["spring"]++
		case time.June, time.July, time.August:
			season["summer"]++
		default:
			season["autumn"]++
		}
	}
	return TravelPatterns{
		TimeOfDay:           tod,
		DayOfWeek:           dow,
		Seasonality:         season,
		ActivityBreakdown:   map[string]int{},
		TopVisitedLocations: []any{},
	}
}

func percentChange(now, prev int) int {
	if prev == 0 {
		if now == 0 {
			return 0
		}
		return 100
	}
	return int(math.Round(float64(now-prev) / float64(prev) * 100))
}

func emptyMonths() map[string]int {
	return map[string]int{
		"january": 0, "february": 0, "march": 0, "april": 0,
		"may": 0, "june": 0, "july": 0, "august": 0,
		"september": 0, "october": 0, "november": 0, "december": 0,
	}
}

// YearBounds returns [start, end] unix seconds for a calendar year in loc.
func YearBounds(year int, loc *time.Location) (int64, int64) {
	if loc == nil {
		loc = time.UTC
	}
	start := time.Date(year, 1, 1, 0, 0, 0, 0, loc)
	end := time.Date(year+1, 1, 1, 0, 0, 0, 0, loc).Add(-time.Second)
	return start.Unix(), end.Unix()
}
