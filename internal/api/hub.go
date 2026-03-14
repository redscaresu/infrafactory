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
