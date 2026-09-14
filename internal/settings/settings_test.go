package settings

import (
	"encoding/json"
	"testing"
)

func TestApplyStripsUnknownAndMergesMaps(t *testing.T) {
	t.Parallel()
	current := Default("Asia/Tokyo")
	got := Apply(current, map[string]any{
		"live_map_enabled": false,
		"unknown_key":      "drop-me",
		"maps": map[string]any{
			"distance_unit":          "mi",
			"hidden_tile_categories": []any{"poi"},
		},
		"immich_api_key": "secret-value",
	})
	if got["live_map_enabled"] != false {
		t.Fatalf("live_map_enabled = %v", got["live_map_enabled"])
	}
	if _, ok := got["unknown_key"]; ok {
		t.Fatal("unknown key leaked")
	}
	maps, _ := got["maps"].(map[string]any)
	if maps["distance_unit"] != "mi" {
		t.Fatalf("distance_unit = %v", maps["distance_unit"])
	}
	if maps["hidden_tile_categories"] == nil {
		t.Fatal("maps subkeys should survive merge")
	}
	if got["timezone"] != "Asia/Tokyo" {
		t.Fatalf("timezone clobbered: %v", got["timezone"])
	}
	if got["immich_api_key"] != "secret-value" {
		t.Fatal("permitted secret key should persist")
	}
}

func TestApplyIgnoresNonObjectMaps(t *testing.T) {
	t.Parallel()
	current := Default("UTC")
	got := Apply(current, map[string]any{"maps": []any{"not", "an", "object"}})
	maps, _ := got["maps"].(map[string]any)
	if maps["distance_unit"] != "km" {
		t.Fatalf("maps should be unchanged, got %+v", maps)
	}
}

func TestApplyRejectsInvalidDistanceUnit(t *testing.T) {
	t.Parallel()
	current := Default("UTC")
	got := Apply(current, map[string]any{"maps": map[string]any{"distance_unit": "furlong"}})
	maps, _ := got["maps"].(map[string]any)
	if maps["distance_unit"] != "km" {
		t.Fatalf("invalid unit accepted: %v", maps["distance_unit"])
	}
}

func TestLoadCorruptJSONUsesDefaults(t *testing.T) {
	t.Parallel()
	got := Load(json.RawMessage(`not-json`), "Asia/Tokyo")
	if got["timezone"] != "Asia/Tokyo" {
		t.Fatalf("tz = %v", got["timezone"])
	}
}

func TestSanitizeMobile(t *testing.T) {
	t.Parallel()
	got := SanitizeMobile(map[string]any{
		"tracking_mode":     "precise",
		"tracking_visits":   true,
		"distance_filter":   0,
		"batch_size":        5000,
		"time_filter":       "",
		"unknown":           1,
		"auto_start":        "true",
		"tracking_mode_bad": "warp",
	})
	if got["tracking_mode"] != "precise" {
		t.Fatalf("mode = %v", got["tracking_mode"])
	}
	if got["distance_filter"] != 1 {
		t.Fatalf("clamp low = %v", got["distance_filter"])
	}
	if got["batch_size"] != 1000 {
		t.Fatalf("clamp high = %v", got["batch_size"])
	}
	if _, ok := got["time_filter"]; ok {
		t.Fatal("blank numeric should be dropped")
	}
	if _, ok := got["unknown"]; ok {
		t.Fatal("unknown mobile key")
	}
	if got["auto_start"] != true {
		t.Fatalf("bool cast = %v", got["auto_start"])
	}

	bad := SanitizeMobile(map[string]any{"tracking_mode": "warp"})
	if _, ok := bad["tracking_mode"]; ok {
		t.Fatal("invalid tracking_mode should be dropped")
	}
}

func TestUserViewOmitsSecrets(t *testing.T) {
	t.Parallel()
	s := Default("UTC")
	s["immich_api_key"] = "secret-value"
	view := UserView(s)
	if _, ok := view["immich_api_key"]; ok {
		t.Fatal("users/me must not include integration secrets")
	}
	if view["timezone"] != "UTC" {
		t.Fatalf("timezone = %v", view["timezone"])
	}
}
