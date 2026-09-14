package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

func decodeObject(r *http.Request) (map[string]any, error) {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 32<<20))
	dec.UseNumber()
	var body map[string]any
	if err := dec.Decode(&body); err != nil {
		return nil, err
	}
	if body == nil {
		body = map[string]any{}
	}
	return body, nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func atoiDefault(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

func formatCoord(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func settingsObject(body map[string]any) (map[string]any, bool) {
	patch, ok := body["settings"].(map[string]any)
	return patch, ok
}
