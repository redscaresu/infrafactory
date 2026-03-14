package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
)

func TestDiagnosticsHandlerClaudeMissingCommand(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Agent.Type = generator.AgentTypeClaudeCode
	cfg.Agent.Claude.Command = "infrafactory-missing-claude-test-binary"

	req := httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil)
	rec := httptest.NewRecorder()
	diagnosticsHandler(&serverState{cfg: cfg, sessionID: "session-1", startedAt: time.Date(2026, 2, 28, 13, 0, 0, 0, time.UTC)}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload diagnosticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if payload.Ready {
		t.Fatal("expected diagnostics to report not ready")
	}
	if payload.Summary != "Claude CLI is unavailable." {
		t.Fatalf("unexpected summary: %q", payload.Summary)
	}
	if len(payload.Checks) != 1 || payload.Checks[0].Status != "fail" {
		t.Fatalf("unexpected checks: %+v", payload.Checks)
	}
}

func TestDiagnosticsHandlerOpenRouterMissingAPIKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	cfg := config.Default()
	cfg.Agent.Type = generator.AgentTypeOpenRouter

	req := httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil)
	rec := httptest.NewRecorder()
	diagnosticsHandler(&serverState{cfg: cfg, sessionID: "session-2", startedAt: time.Date(2026, 2, 28, 13, 0, 0, 0, time.UTC)}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload diagnosticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if payload.Ready {
		t.Fatal("expected diagnostics to report not ready")
	}
	if payload.Summary != "OpenRouter API key is unavailable." {
		t.Fatalf("unexpected summary: %q", payload.Summary)
	}
}

func TestDiagnosticsHandlerClaudeReady(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Agent.Type = generator.AgentTypeClaudeCode
	cfg.Agent.Claude.Command = "sh"

	req := httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil)
	rec := httptest.NewRecorder()
	diagnosticsHandler(&serverState{cfg: cfg, sessionID: "session-3", startedAt: time.Date(2026, 2, 28, 13, 0, 0, 0, time.UTC)}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload diagnosticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if !payload.Ready {
		t.Fatalf("expected ready diagnostics, got %+v", payload)
	}
	if payload.Summary != "Generator runtime looks available." {
		t.Fatalf("unexpected summary: %q", payload.Summary)
	}
	if payload.SessionID == "" || payload.StartedAt == "" {
		t.Fatalf("expected session metadata, got %+v", payload)
	}
}

func TestDiagnosticsHandlerOpenRouterReady(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	cfg := config.Default()
	cfg.Agent.Type = generator.AgentTypeOpenRouter

	req := httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil)
	rec := httptest.NewRecorder()
	diagnosticsHandler(&serverState{cfg: cfg, sessionID: "session-4", startedAt: time.Date(2026, 2, 28, 13, 0, 0, 0, time.UTC)}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload diagnosticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if !payload.Ready {
		t.Fatalf("expected ready diagnostics, got %+v", payload)
	}
	if payload.SessionID == "" || payload.StartedAt == "" {
		t.Fatalf("expected session metadata, got %+v", payload)
	}
}

func TestDiagnosticsHandlerRejectsWrongMethod(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/api/diagnostics", nil)
	rec := httptest.NewRecorder()
	diagnosticsHandler(&serverState{cfg: config.Default()}).ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
