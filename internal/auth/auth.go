package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// KeyFromRequest accepts Dawarich ?api_key= and Authorization: Bearer.
// Query support is required for official-client compatibility; do not remove it.
func KeyFromRequest(r *http.Request) string {
	if k := r.URL.Query().Get("api_key"); k != "" {
		return k
	}
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) >= len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// Equal reports whether got matches want using a length-independent compare.
// Both values are hashed first so differing lengths do not short-circuit.
func Equal(got, want string) bool {
	if want == "" {
		return false
	}
	gotSum := sha256.Sum256([]byte(got))
	wantSum := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotSum[:], wantSum[:]) == 1
}
