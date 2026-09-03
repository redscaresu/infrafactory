package api

import (
	"context"
	"sync"
)

type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*Client]struct{})}
}

func (h *Hub) Register(c *Client) {
	if h == nil || c == nil {
		return
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) Unregister(c *Client) {
	if h == nil || c == nil {
		return
	}
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

func (h *Hub) Broadcast(msg []byte) {
	if h == nil || len(msg) == 0 {
		return
	}

	slow := make([]*Client, 0)
	h.mu.RLock()
	for c := range h.clients {
		select {
		case c.send <- append([]byte(nil), msg...):
		default:
			slow = append(slow, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range slow {
		h.Unregister(c)
	}
}

func (h *Hub) Run(ctx context.Context) {
	if h == nil {
		return
	}
	<-ctx.Done()
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		delete(h.clients, c)
		close(c.send)
	}
}

// NewTestClient builds a hub client with no connection.
//
// Broadcast never touches the socket, so a test can observe exactly what
// a browser would receive without one. Exported because the wiring this
// supports crosses package boundaries: `internal/cli` needs to assert
// that stage progress reaches a websocket subscriber, and that path was
// broken twice while every unit test passed.
func NewTestClient(buffer int) *Client {
	return &Client{send: make(chan []byte, buffer)}
}

// TryReceive takes the next queued message, if any.
func (c *Client) TryReceive() ([]byte, bool) {
	select {
	case msg := <-c.send:
		return msg, true
	default:
		return nil, false
	}
}
