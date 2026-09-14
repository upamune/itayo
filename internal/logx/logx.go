package logx

import (
	"log/slog"
	"os"
	"strings"
)

// NewHandler returns a slog handler that never emits secret attribute values.
func NewHandler(format string) slog.Handler {
	opts := &slog.HandlerOptions{ReplaceAttr: RedactAttr}
	if format == "json" {
		return slog.NewJSONHandler(os.Stderr, opts)
	}
	return slog.NewTextHandler(os.Stderr, opts)
}

// RedactAttr replaces secret-looking slog attributes with "[redacted]".
func RedactAttr(_ []string, a slog.Attr) slog.Attr {
	k := strings.ToLower(a.Key)
	switch k {
	case "api_key", "authorization", "immich_api_key", "photoprism_api_key",
		"airtrail_api_key", "teslamate_password", "teslamate_api_token":
		return slog.String(a.Key, "[redacted]")
	}
	if strings.Contains(k, "secret") || strings.Contains(k, "password") || strings.Contains(k, "token") {
		return slog.String(a.Key, "[redacted]")
	}
	return a
}
