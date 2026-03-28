package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestMockwayStateClientStateReadsWithinBound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mock/state" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"mock":true}`))
	}))
	defer server.Close()

	client := newMockwayStateClient(server.URL)
	state, err := client.State(context.Background())
	if err != nil {
		t.Fatalf("state read: %v", err)
	}
	if string(state) != `{"mock":true}` {
		t.Fatalf("unexpected state payload: %q", string(state))
	}
}

func TestMockwayStateClientStateFailsWhenPayloadExceedsBound(t *testing.T) {
	t.Parallel()

	oversized := strings.Repeat("a", maxMockwayStateResponseBytes+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(oversized))
	}))
	defer server.Close()

	client := newMockwayStateClient(server.URL)
	_, err := client.State(context.Background())
	if err == nil {
		t.Fatal("expected payload limit error")
	}
	expected := "read state response: payload exceeds"
	if !strings.Contains(err.Error(), expected) {
		t.Fatalf("expected %q error, got %v", expected, err)
	}
}

func TestMockwayStateClientStateTruncatesErrorPayload(t *testing.T) {
	t.Parallel()

	oversized := strings.Repeat("x", maxMockwayErrorPayloadBytes+100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(oversized))
	}))
	defer server.Close()

	client := newMockwayStateClient(server.URL)
	_, err := client.State(context.Background())
	if err == nil {
		t.Fatal("expected status error")
	}
	if !strings.Contains(err.Error(), "...") {
		t.Fatalf("expected truncated payload marker, got %v", err)
	}
	if strings.Contains(err.Error(), oversized) {
		t.Fatalf("expected payload truncation, got %v", err)
	}
}
