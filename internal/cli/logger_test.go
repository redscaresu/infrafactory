package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppLoggerWritesJSONLine(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	logger := NewAppLogger(buf)
	logger.Log(LogEntry{
		Level:   "INFO",
		Command: "run",
		Event:   "iteration_end",
		Status:  "failed",
		RunID:   "run-1",
		Detail:  "validation failed",
	})

	var entry LogEntry
	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected log line")
	}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("unmarshal log line: %v", err)
	}
	if entry.Level != logLevelInfo {
		t.Fatalf("expected normalized info level, got %q", entry.Level)
	}
	if entry.Command != "run" || entry.Event != "iteration_end" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
}

func TestAppLoggerRedactsSecretLikeDetailTokens(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	logger := NewAppLogger(buf)
	logger.Log(LogEntry{
		Level:   logLevelError,
		Command: "generate",
		Event:   "command_end",
		Status:  "failed",
		Detail:  "token=abc123 api_key:xyz prompt=long",
	})

	line := buf.String()
	if strings.Contains(line, "abc123") || strings.Contains(line, "xyz") || strings.Contains(line, "long") {
		t.Fatalf("expected redaction, got: %s", line)
	}
	if !strings.Contains(line, "[redacted]") {
		t.Fatalf("expected redaction marker, got: %s", line)
	}
}

func TestAppLoggerAddFileSinkWritesToFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "runs", "example", "run-1", "app.log")
	buf := &bytes.Buffer{}
	logger := NewAppLogger(buf)

	closeSink, err := logger.AddFileSink(path)
	if err != nil {
		t.Fatalf("add file sink: %v", err)
	}
	defer func() { _ = closeSink() }()

	logger.Log(LogEntry{
		Level:   logLevelInfo,
		Command: "run",
		Event:   "terminal_reason",
		Status:  "target_reached",
	})

	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(payload), "\"event\":\"terminal_reason\"") {
		t.Fatalf("expected terminal_reason event in file, got: %s", string(payload))
	}
}
