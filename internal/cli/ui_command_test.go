package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redscaresu/infrafactory/internal/api"
	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
)

func TestUIRunStarterPreflightRejectsMissingClaudeCLI(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Agent.Type = generator.AgentTypeClaudeCode
	cfg.Agent.Claude.Command = "infrafactory-missing-claude-test-binary"

	starter := &uiRunStarter{cfg: cfg}
	_, err := starter.StartRun(context.Background(), api.StartRunRequest{ScenarioName: "web-app-paris", ScenarioPath: "training/web-app-paris"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `command "infrafactory-missing-claude-test-binary" not found in PATH`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUIRunStarterPreflightRejectsMissingOpenRouterAPIKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	cfg := config.Default()
	cfg.Agent.Type = generator.AgentTypeOpenRouter

	starter := &uiRunStarter{cfg: cfg}
	_, err := starter.StartRun(context.Background(), api.StartRunRequest{ScenarioName: "web-app-paris", ScenarioPath: "training/web-app-paris"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "OPENROUTER_API_KEY is not set") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUIRunStarterClearsBusyAfterAsyncCompletion(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	var calls atomic.Int32

	starter := &uiRunStarter{
		cfg:           config.Default(),
		baseCtx:       context.Background(),
		preflightFunc: func() error { return nil },
		executeRunFunc: func(context.Context, string, string) error {
			calls.Add(1)
			<-done
			return nil
		},
	}

	req := api.StartRunRequest{ScenarioName: "web-app-paris", ScenarioPath: "training/web-app-paris"}
	if _, err := starter.StartRun(context.Background(), req); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if _, err := starter.StartRun(context.Background(), req); !errors.Is(err, api.ErrRunBusy) {
		t.Fatalf("expected busy error, got %v", err)
	}

	close(done)
	time.Sleep(20 * time.Millisecond)

	if _, err := starter.StartRun(context.Background(), req); err != nil {
		t.Fatalf("expected busy flag to clear, got %v", err)
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() < 2 {
		t.Fatalf("expected execute run to be called twice, got %d", calls.Load())
	}
}

func TestUIRunStarterRunContextPrefersBaseContext(t *testing.T) {
	t.Parallel()

	baseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()

	starter := &uiRunStarter{baseCtx: baseCtx}
	runCtx := starter.runContext(requestCtx)

	select {
	case <-runCtx.Done():
		t.Fatal("expected run context to ignore canceled request context")
	default:
	}
}

func TestUIRunStarterRunContextFallsBackToRequestContext(t *testing.T) {
	t.Parallel()

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	starter := &uiRunStarter{}

	runCtx := starter.runContext(requestCtx)
	cancelRequest()

	select {
	case <-runCtx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected fallback run context to track request context cancellation")
	}
}

func TestUIRunStarterPreflightResolvesClaudeToAbsolutePath(t *testing.T) {
	binDir := t.TempDir()
	claudePath := filepath.Join(binDir, "claude")
	if err := os.WriteFile(claudePath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake claude binary: %v", err)
	}

	t.Setenv("PATH", binDir)

	cfg := config.Default()
	cfg.Agent.Type = generator.AgentTypeClaudeCode
	cfg.Agent.Claude.Command = "claude"

	starter := &uiRunStarter{cfg: cfg}
	if err := starter.preflight(); err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if starter.resolvedClaude != claudePath {
		t.Fatalf("expected resolved claude path %q, got %q", claudePath, starter.resolvedClaude)
	}
}
