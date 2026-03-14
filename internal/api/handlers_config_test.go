package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
)

func TestConfigHandlerReturnsAllowlistedFieldsOnly(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Version = "1.0"
	cfg.Agent.Type = "claude-code"
	cfg.Paths.Scenarios = "./scenarios"
	cfg.Paths.Output = "./output"
	cfg.Mockway.URL = "http://localhost:8080"

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	newConfigHandler(&serverState{cfg: cfg}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}

	if _, ok := payload["mockway"]; ok {
		t.Fatalf("unexpected mockway field in allowlisted config response")
	}

	paths, ok := payload["paths"].(map[string]any)
	if !ok {
		t.Fatalf("expected paths object in response")
	}
	if _, ok := paths["mappings"]; ok {
		t.Fatalf("unexpected paths.mappings in allowlisted response")
	}
	if _, ok := paths["scenarios"]; !ok {
		t.Fatalf("expected paths.scenarios in allowlisted response")
	}
	if _, ok := paths["output"]; !ok {
		t.Fatalf("expected paths.output in allowlisted response")
	}
}
