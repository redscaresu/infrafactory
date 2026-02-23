package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	logLevelInfo  = "info"
	logLevelError = "error"
)

type LogEntry struct {
	Level     string `json:"level"`
	Command   string `json:"command"`
	Event     string `json:"event"`
	Status    string `json:"status,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Iteration int    `json:"iteration,omitempty"`
	Stage     string `json:"stage,omitempty"`
	Check     string `json:"check,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

type AppLogger struct {
	mu    sync.Mutex
	sinks []io.Writer
}

func NewAppLogger(sinks ...io.Writer) *AppLogger {
	nonNil := make([]io.Writer, 0, len(sinks))
	for _, sink := range sinks {
		if sink != nil {
			nonNil = append(nonNil, sink)
		}
	}
	return &AppLogger{sinks: nonNil}
}

func (l *AppLogger) Log(entry LogEntry) {
	if l == nil || entry.Command == "" || entry.Event == "" {
		return
	}
	entry = sanitizeLogEntry(entry)
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	for _, sink := range l.sinks {
		if sink == nil {
			continue
		}
		_, _ = sink.Write(append(line, '\n'))
	}
}

func (l *AppLogger) AddFileSink(path string) (func() error, error) {
	if l == nil {
		return nil, fmt.Errorf("logger is nil")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create log directory for %q: %w", path, err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", path, err)
	}

	l.mu.Lock()
	l.sinks = append(l.sinks, f)
	l.mu.Unlock()

	return func() error {
		l.mu.Lock()
		defer l.mu.Unlock()

		filtered := make([]io.Writer, 0, len(l.sinks))
		for _, sink := range l.sinks {
			if sink != f {
				filtered = append(filtered, sink)
			}
		}
		l.sinks = filtered
		return f.Close()
	}, nil
}

func sanitizeLogEntry(entry LogEntry) LogEntry {
	entry.Level = strings.TrimSpace(strings.ToLower(entry.Level))
	if entry.Level == "" {
		entry.Level = logLevelInfo
	}
	entry.Detail = redactLogDetail(strings.TrimSpace(entry.Detail))
	return entry
}

func redactLogDetail(detail string) string {
	if detail == "" {
		return ""
	}

	// Redact common secret-bearing key/value patterns.
	replacements := []string{"token", "api_key", "apikey", "secret", "password", "prompt"}
	parts := strings.Fields(detail)
	for i, part := range parts {
		clean := strings.ToLower(strings.Trim(part, `"'`))
		for _, marker := range replacements {
			if strings.Contains(clean, marker+"=") || strings.Contains(clean, marker+":") {
				parts[i] = marker + "=[redacted]"
				break
			}
		}
	}

	return strings.Join(parts, " ")
}
