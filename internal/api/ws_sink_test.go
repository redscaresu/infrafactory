package api

import (
	"strings"
	"testing"
	"time"
)

func TestWebSocketSinkWriteBroadcasts(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	ch := make(chan []byte, 1)
	hub.Register(&Client{send: ch})

	sink := NewWebSocketSink(hub)
	n, err := sink.Write([]byte("line"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != 4 {
		t.Fatalf("expected 4 bytes written, got %d", n)
	}

	select {
	case msg := <-ch:
		if !strings.Contains(string(msg), "\"type\":\"log\"") {
			t.Fatalf("unexpected broadcast payload: %q", string(msg))
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestWebSocketSinkNilHubSafe(t *testing.T) {
	t.Parallel()

	sink := NewWebSocketSink(nil)
	if _, err := sink.Write([]byte("line")); err != nil {
		t.Fatalf("expected nil hub write to succeed, got %v", err)
	}
}
