package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKeyFromRequest(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/points?api_key=from-query", nil)
	req.Header.Set("Authorization", "Bearer from-header")
	if got := KeyFromRequest(req); got != "from-query" {
		t.Fatalf("query should win for official-client compat, got %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/points", nil)
	req.Header.Set("Authorization", "bearer  spaced ")
	if got := KeyFromRequest(req); got != "spaced" {
		t.Fatalf("bearer = %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/points", nil)
	if got := KeyFromRequest(req); got != "" {
		t.Fatalf("empty = %q", got)
	}
}

func TestEqual(t *testing.T) {
	t.Parallel()
	if !Equal("your-secret", "your-secret") {
		t.Fatal("same key")
	}
	if Equal("your-secret", "other-secret") {
		t.Fatal("different key")
	}
	if Equal("short", "much-longer-secret") {
		t.Fatal("different lengths must not match")
	}
	if Equal("", "your-secret") {
		t.Fatal("empty got")
	}
	if Equal("your-secret", "") {
		t.Fatal("empty want must refuse")
	}
}
