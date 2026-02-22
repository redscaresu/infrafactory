package cli

import (
	"testing"
	"time"
)

func TestNewMockwayStateClientSetsHTTPTimeout(t *testing.T) {
	t.Parallel()

	client := newMockwayStateClient("http://localhost:8080")
	if client.client.Timeout != 30*time.Second {
		t.Fatalf("expected timeout 30s, got %s", client.client.Timeout)
	}
}
