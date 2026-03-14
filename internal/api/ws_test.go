package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/redscaresu/infrafactory/internal/config"
)

func TestWebSocketEndpointReceivesBroadcasts(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	srv := NewServer(ServerConfig{
		Config: config.Default(),
		Hub:    hub,
	})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	payload := readBroadcastPayload(t, ctx, hub, conn)
	if string(payload) != `{"type":"ping"}` {
		t.Fatalf("unexpected payload: %s", string(payload))
	}
}

func TestWebSocketEndpointAllowsLocalDevOrigin(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	srv := NewServer(ServerConfig{
		Config: config.Default(),
		Hub:    hub,
	})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{"http://127.0.0.1:5173"},
		},
	})
	if err != nil {
		t.Fatalf("dial websocket with dev origin: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	payload := readBroadcastPayload(t, ctx, hub, conn)
	if string(payload) != `{"type":"ping"}` {
		t.Fatalf("unexpected payload: %s", string(payload))
	}
}

func TestWebSocketEndpointStaysOpenLongEnoughForDelayedBroadcast(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	srv := NewServer(ServerConfig{
		Config: config.Default(),
		Hub:    hub,
	})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{"http://127.0.0.1:5173"},
		},
	})
	if err != nil {
		t.Fatalf("dial websocket with dev origin: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	time.Sleep(200 * time.Millisecond)
	payload := readBroadcastPayload(t, ctx, hub, conn)
	if string(payload) != `{"type":"ping"}` {
		t.Fatalf("unexpected payload: %s", string(payload))
	}
}

func readBroadcastPayload(t *testing.T, ctx context.Context, hub *Hub, conn *websocket.Conn) []byte {
	t.Helper()

	done := make(chan struct{})
	defer close(done)

	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				hub.Broadcast([]byte(`{"type":"ping"}`))
			}
		}
	}()

	_, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read websocket payload: %v", err)
	}
	return payload
}
