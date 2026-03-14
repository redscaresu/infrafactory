package api

import (
	"context"
	"errors"
	"time"

	"github.com/coder/websocket"
)

type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

func newClient(hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),
	}
}

func (c *Client) ReadPump(ctx context.Context) {
	defer c.hub.Unregister(c)
	for {
		_, _, err := c.conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure || errors.Is(err, context.Canceled) {
				return
			}
			return
		}
	}
}

func (c *Client) WritePump(ctx context.Context) {
	defer c.hub.Unregister(c)
	for {
		select {
		case <-ctx.Done():
			_ = c.conn.Close(websocket.StatusNormalClosure, "context canceled")
			return
		case msg, ok := <-c.send:
			if !ok {
				_ = c.conn.Close(websocket.StatusNormalClosure, "hub closed")
				return
			}
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := c.conn.Write(writeCtx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
