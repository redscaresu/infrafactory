package generator

import (
	"strings"
	"testing"
)

func TestRedactTransportDetail(t *testing.T) {
	t.Parallel()

	detail := "Authorization: Bearer abc123 prompt=scenario: smoke key=sk-or-v1-super-secret env=my-secret"
	redacted := redactTransportDetail(detail, "scenario: smoke", map[string]string{"X": "my-secret"}, "sk-or-v1-super-secret")

	if strings.Contains(redacted, "abc123") {
		t.Fatalf("expected bearer token to be redacted, got %q", redacted)
	}
	if strings.Contains(redacted, "scenario: smoke") {
		t.Fatalf("expected prompt content to be redacted, got %q", redacted)
	}
	if strings.Contains(redacted, "sk-or-v1-super-secret") {
		t.Fatalf("expected openrouter token to be redacted, got %q", redacted)
	}
	if strings.Contains(redacted, "my-secret") {
		t.Fatalf("expected env secret to be redacted, got %q", redacted)
	}
}
