package logx

import (
	"log/slog"
	"testing"
)

func TestRedactAttr(t *testing.T) {
	t.Parallel()
	got := RedactAttr(nil, slog.String("api_key", "your-secret"))
	if got.Value.String() != "[redacted]" {
		t.Fatalf("api_key = %q", got.Value.String())
	}
	got = RedactAttr(nil, slog.String("Authorization", "Bearer your-secret"))
	if got.Value.String() != "[redacted]" {
		t.Fatalf("authorization = %q", got.Value.String())
	}
	got = RedactAttr(nil, slog.String("path", "/api/v1/health"))
	if got.Value.String() != "/api/v1/health" {
		t.Fatalf("path redacted: %q", got.Value.String())
	}
	got = RedactAttr(nil, slog.String("version", "0.2.0"))
	if got.Value.String() != "0.2.0" {
		t.Fatalf("version redacted: %q", got.Value.String())
	}
}
