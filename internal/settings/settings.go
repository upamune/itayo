package settings

import (
	"encoding/json"
	"maps"
	"strconv"
	"strings"
)

const (
	PhotoLibraryImportVersion = 1
)

var scalarKeys = []string{
	"timezone",
	"meters_between_routes",
	"minutes_between_routes",
	"fog_of_war_meters",
	"time_threshold_minutes",
	"merge_threshold_minutes",
	"route_opacity",
	"route_color",
	"track_color",
	"preferred_map_layer",
	"points_rendering_mode",
	"live_map_enabled",
	"immich_url",
	"immich_api_key",
	"photoprism_url",
	"photoprism_api_key",
	"speed_colored_routes",
	"speed_color_scale",
	"fog_of_war_threshold",
	"fog_of_war_mode",
	"maps_v2_style",
	"maps_maplibre_style",
	"maps_maplibre_tiles_url",
	"globe_projection",
	"transportation_expert_mode",
	"min_minutes_spent_in_city",
	"max_gap_minutes_in_city",
	"stay_max_gap_minutes",
	"gps_filtering_enabled",
	"gps_accuracy_threshold",
	"point_dragging_enabled",
	"visits_suggestions_enabled",
}

var arrayKeys = []string{
	"enabled_map_layers",
	"enabled_transportation_modes",
}

var objectKeys = []string{
	"transportation_thresholds",
	"transportation_expert_thresholds",
	"maps_maplibre_custom_theme",
}

var userViewKeys = []string{
	"timezone", "maps", "fog_of_war_meters", "meters_between_routes",
	"preferred_map_layer", "speed_colored_routes", "points_rendering_mode",
	"minutes_between_routes", "time_threshold_minutes", "merge_threshold_minutes",
	"live_map_enabled", "route_opacity", "immich_url", "photoprism_url",
	"visits_suggestions_enabled", "speed_color_scale", "fog_of_war_threshold",
	"globe_projection",
}

var mobileKeys = []string{
	"tracking_mode",
	"tracking_visits",
	"track_visits_independently",
	"auto_start",
	"distance_filter",
	"time_filter",
	"track_break",
	"accuracy",
	"show_background_location_indicator",
	"upload_automatically",
	"upload_all_on_tracking_stop",
	"batch_size",
}

var mobileBooleans = map[string]struct{}{
	"tracking_visits":                    {},
	"track_visits_independently":         {},
	"auto_start":                         {},
	"show_background_location_indicator": {},
	"upload_automatically":               {},
	"upload_all_on_tracking_stop":        {},
}

var mobileClamps = map[string][2]int{
	"distance_filter": {1, 10_000},
	"time_filter":     {1, 3600},
	"track_break":     {1, 1440},
	"accuracy":        {1, 6},
	"batch_size":      {1, 1000},
}

// Default returns Dawarich-compatible settings for a self-hosted single user.
func Default(tz string) map[string]any {
	if tz == "" {
		tz = "Asia/Tokyo"
	}
	return map[string]any{
		"fog_of_war_meters":          50,
		"meters_between_routes":      500,
		"preferred_map_layer":        "OpenStreetMap",
		"speed_colored_routes":       false,
		"points_rendering_mode":      "raw",
		"minutes_between_routes":     30,
		"time_threshold_minutes":     30,
		"merge_threshold_minutes":    15,
		"live_map_enabled":           true,
		"route_opacity":              0.6,
		"route_color":                "#0000ff",
		"track_color":                "#6366F1",
		"immich_url":                 nil,
		"immich_api_key":             nil,
		"photoprism_url":             nil,
		"photoprism_api_key":         nil,
		"maps":                       map[string]any{"distance_unit": "km"},
		"visits_suggestions_enabled": true,
		"speed_color_scale":          nil,
		"fog_of_war_threshold":       50,
		"fog_of_war_mode":            "points",
		"enabled_map_layers":         []any{"Tracks", "Heatmap"},
		"maps_maplibre_style":        "light",
		"globe_projection":           true,
		"gps_filtering_enabled":      true,
		"timezone":                   tz,
		"point_dragging_enabled":     false,
		"min_minutes_spent_in_city":  60,
	}
}

// Load merges stored JSON onto defaults. Corrupt payloads fall back to defaults.
func Load(raw json.RawMessage, tz string) map[string]any {
	out := Default(tz)
	if len(raw) == 0 {
		return out
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		return out
	}
	maps.Copy(out, stored)
	return out
}

// Apply merges a client patch into current using Dawarich's permit list.
// Unknown keys are dropped. maps is deep-merged; a non-object maps is ignored.
func Apply(current map[string]any, patch map[string]any) map[string]any {
	out := cloneMap(current)
	if patch == nil {
		return out
	}
	allowed := make(map[string]struct{}, len(scalarKeys)+len(arrayKeys)+len(objectKeys)+1)
	for _, k := range scalarKeys {
		allowed[k] = struct{}{}
	}
	for _, k := range arrayKeys {
		allowed[k] = struct{}{}
	}
	for _, k := range objectKeys {
		allowed[k] = struct{}{}
	}
	allowed["maps"] = struct{}{}

	for k, v := range patch {
		if _, ok := allowed[k]; !ok {
			continue
		}
		if k == "maps" {
			src, ok := v.(map[string]any)
			if !ok {
				continue
			}
			dst, _ := out["maps"].(map[string]any)
			if dst == nil {
				dst = map[string]any{}
			} else {
				dst = cloneMap(dst)
			}
			mergeMapsObject(dst, src)
			out["maps"] = dst
			continue
		}
		out[k] = v
	}
	return out
}

func mergeMapsObject(dst, src map[string]any) {
	if unit, ok := src["distance_unit"].(string); ok {
		unit = strings.ToLower(strings.TrimSpace(unit))
		if unit == "km" || unit == "mi" {
			dst["distance_unit"] = unit
		}
	}
	for k, v := range src {
		if k == "distance_unit" {
			continue
		}
		dst[k] = v
	}
}

// UserView is the /users/me settings subset from Api::UserSerializer.
func UserView(settings map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range userViewKeys {
		out[k] = settings[k]
	}
	return out
}

// SanitizeMobile keeps the 12 mobile sync keys, clamps numbers, and casts booleans.
func SanitizeMobile(patch map[string]any) map[string]any {
	out := map[string]any{}
	if patch == nil {
		return out
	}
	allowed := make(map[string]struct{}, len(mobileKeys))
	for _, k := range mobileKeys {
		allowed[k] = struct{}{}
	}
	for k, v := range patch {
		if _, ok := allowed[k]; !ok {
			continue
		}
		if k == "tracking_mode" {
			s, _ := v.(string)
			if s == "precise" || s == "significant" {
				out[k] = s
			}
			continue
		}
		if _, ok := mobileBooleans[k]; ok {
			if b, ok := asBool(v); ok {
				out[k] = b
			}
			continue
		}
		if bounds, ok := mobileClamps[k]; ok {
			if n, ok := asInt(v); ok {
				if n < bounds[0] {
					n = bounds[0]
				}
				if n > bounds[1] {
					n = bounds[1]
				}
				out[k] = n
			}
		}
	}
	return out
}

// MergeMobile last-write-wins merges sanitized keys into the stored mobile blob.
func MergeMobile(existing, sanitized map[string]any) map[string]any {
	out := cloneMap(existing)
	maps.Copy(out, sanitized)
	return out
}

// PublicMobile drops updated_at so the settings object matches swagger.
func PublicMobile(stored map[string]any) map[string]any {
	out := cloneMap(stored)
	delete(out, "updated_at")
	return out
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if m, ok := v.(map[string]any); ok {
			out[k] = cloneMap(m)
			continue
		}
		out[k] = v
	}
	return out
}

func asBool(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return false, false
		}
		b, err := strconv.ParseBool(s)
		return b, err == nil
	default:
		return false, false
	}
}

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	case json.Number:
		n, err := t.Int64()
		return int(n), err == nil
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		n, err := strconv.Atoi(s)
		return n, err == nil
	default:
		return 0, false
	}
}
