package api

import (
	"context"
	"testing"
	"time"
)

func TestHubRegisterBroadcastUnregister(t *testing.T) {
	t.Parallel()

	h := NewHub()
	ch := make(chan []byte, 1)
	client := &Client{send: ch}
	h.Register(client)

	h.Broadcast([]byte("hello"))
	select {
	case msg := <-ch:
		if string(msg) != "hello" {
			t.Fatalf("unexpected message: %s", string(msg))
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for message")
	}

	h.Unregister(client)
}

func TestHubDropSlowClient(t *testing.T) {
	t.Parallel()

	h := NewHub()
	ch := make(chan []byte, 1)
	h.Register(&Client{send: ch})

	h.Broadcast([]byte("a"))
	h.Broadcast([]byte("b"))

	select {
	case <-ch:
	default:
		t.Fatal("expected at least one message")
	}
}

func TestHubRunClosesChannelsOnCancel(t *testing.T) {
	t.Parallel()

	h := NewHub()
	ch := make(chan []byte, 1)
	h.Register(&Client{send: ch})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		h.Run(ctx)
		close(done)
	}()
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("hub run did not stop")
	}
}
