package api

import "encoding/json"

type WebSocketSink struct {
	hub *Hub
}

func NewWebSocketSink(hub *Hub) *WebSocketSink {
	return &WebSocketSink{hub: hub}
}

func (s *WebSocketSink) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if s != nil && s.hub != nil {
		data := json.RawMessage(p)
		if !json.Valid(data) {
			quoted, _ := json.Marshal(string(p))
			data = quoted
		}
		payload, _ := json.Marshal(map[string]json.RawMessage{
			"type": json.RawMessage(`"log"`),
			"data": data,
		})
		s.hub.Broadcast(payload)
	}
	return len(p), nil
}
