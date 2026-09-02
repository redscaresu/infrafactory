package api

import (
	"bytes"
	"encoding/json"
	"sync"
)

// ProgressSink turns a command's progress output into websocket events,
// one per line (S163).
//
// # Why lines, and not writes
//
// `WebSocketSink` broadcasts each Write as it arrives. A command writing
// with several Fprintf calls, or a subprocess flushing on a buffer
// boundary, produces fragments -- and a page appending fragments shows
// half a word, then the rest of it on the next row. Buffering to newline
// costs nothing and makes the output mean what it says.
//
// # Why every event names its subject
//
// A deploy takes minutes and the reader can navigate during it, so these
// events arrive on whatever page is open. An unattributed progress line
// is a statement about something the reader may not be looking at --
// exactly what cost S162c seven review findings. The subject travels
// with every line so a client can filter, label, or ignore it.
type ProgressSink struct {
	hub     *Hub
	kind    string
	subject string

	mu      sync.Mutex
	partial bytes.Buffer
}

// NewProgressSink builds a sink. `kind` is the event type a client
// switches on ("deploy_progress"); `subject` is what the events are
// about.
func NewProgressSink(hub *Hub, kind, subject string) *ProgressSink {
	return &ProgressSink{hub: hub, kind: kind, subject: subject}
}

func (s *ProgressSink) Write(p []byte) (int, error) {
	if s == nil {
		return len(p), nil
	}

	s.mu.Lock()
	s.partial.Write(p)

	var lines []string
	for {
		buffered := s.partial.Bytes()
		idx := bytes.IndexByte(buffered, '\n')
		if idx < 0 {
			break
		}
		lines = append(lines, string(bytes.TrimRight(buffered[:idx], "\r")))
		s.partial.Next(idx + 1)
	}
	s.mu.Unlock()

	for _, line := range lines {
		s.emit(line)
	}
	return len(p), nil
}

// Close flushes whatever did not end in a newline.
//
// A command's last line often has no trailing newline, and it is
// frequently the one that matters -- "Deployed as dep-…". Dropping it
// would end the stream one line short of the conclusion.
func (s *ProgressSink) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	tail := s.partial.String()
	s.partial.Reset()
	s.mu.Unlock()

	if tail != "" {
		s.emit(tail)
	}
	return nil
}

func (s *ProgressSink) emit(line string) {
	if s.hub == nil || line == "" {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"type": s.kind,
		"data": map[string]string{"subject": s.subject, "line": line},
	})
	if err != nil {
		return
	}
	s.hub.Broadcast(payload)
}
