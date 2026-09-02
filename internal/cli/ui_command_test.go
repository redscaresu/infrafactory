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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// The flag is the ONLY way a UI server does real-cloud apply, and it is
// read once when the server starts (ADR-0026, S160b).
func TestUICommandRegistersAllowLayer3OffByDefault(t *testing.T) {
	cmd := newUICmd(nil)

	flag := cmd.Flags().Lookup("allow-layer3")
	require.NotNil(t, flag, "without this flag a UI server has no way to be told it may spend money")
	assert.Equal(t, "false", flag.DefValue, "spending money is never the default")
}

// The stricter half of the rule, and the one with a live failure mode.
//
// The per-run config is RE-READ from disk on every run, so editing
// infrafactory.yaml takes effect without a restart. That means a file
// saying `sandbox_deploy.enabled: true` would walk real-cloud apply back
// in on a server the operator started without --allow-layer3 -- silently,
// and on every run after it.
func TestUIRunConfigCannotReEnableRealCloudFromTheConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "infrafactory.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`version: "1.0"
agent:
  type: claude-code
mockway:
  url: http://localhost:8080
validation:
  layers:
    sandbox_deploy:
      enabled: true
`), 0o600))

	serverCfg := config.Default()
	serverCfg.Validation.Layers.SandboxDeploy.Enabled = false
	starter := &uiRunStarter{cfg: serverCfg}

	loaded, err := starter.configLoader()(path)
	require.NoError(t, err)

	assert.False(t, loaded.Validation.Layers.SandboxDeploy.Enabled,
		"a checked-in config file must not be able to start spending money on a server nobody authorised")
}

// And the same seam carries the decision the other way, so --allow-layer3
// is not quietly ignored.
func TestUIRunConfigCarriesTheOperatorsPermissionThrough(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "infrafactory.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`version: "1.0"
agent:
  type: claude-code
mockway:
  url: http://localhost:8080
`), 0o600))

	serverCfg := config.Default()
	serverCfg.Validation.Layers.SandboxDeploy.Enabled = true
	starter := &uiRunStarter{cfg: serverCfg}

	loaded, err := starter.configLoader()(path)
	require.NoError(t, err)

	assert.True(t, loaded.Validation.Layers.SandboxDeploy.Enabled)
}

// Destroying infrastructure is not a capability a request may confer.
// Without the flag the actor is nil, which means the endpoints do not
// exist rather than existing and refusing (S159b).
func TestUICommandBuildsNoTeardownActorWithoutTheFlag(t *testing.T) {
	cmd := newUICmd(nil)

	flag := cmd.Flags().Lookup("allow-teardown")
	require.NotNil(t, flag, "without this flag there is no way to ask for the capability")
	assert.Equal(t, "false", flag.DefValue, "destroying infrastructure is never the default")

	actor, err := teardownActor(cmd, false)
	require.NoError(t, err)
	assert.Nil(t, actor, "nothing to bypass if it does not exist")
}

// A guard that stops without saying why is half a guard. If the operator
// ASKED for teardown and it cannot be built, starting anyway would hand
// them a UI silently missing the capability they requested.
func TestUICommandRefusesToStartWhenRequestedTeardownCannotBeBuilt(t *testing.T) {
	cmd := newUICmd(nil)
	cmd.Flags().String("config", filepath.Join(t.TempDir(), "does-not-exist.yaml"), "")

	_, err := teardownActor(cmd, true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--allow-teardown was requested")
}

// Requiring LLM credentials in order to DESTROY infrastructure would
// make the recovery capability unavailable in exactly the situation that
// needs it: real resources running on a machine where the generator is
// not configured. Same reasoning as `pitfalls retire`.
func TestUITeardownActorDoesNotNeedTheGenerator(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "infrafactory.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`version: "1.0"
agent:
  type: claude-code
  claude:
    command: /nonexistent/claude-binary
mockway:
  url: http://localhost:8080
`), 0o600))

	cmd := newUICmd(nil)
	cmd.Flags().String("config", path, "")

	actor, err := teardownActor(cmd, true)

	require.NoError(t, err, "a missing claude binary must not stop an operator tearing down real infrastructure")
	assert.NotNil(t, actor)
}

// ADR-0027: three flags, three kinds of harm, and none implies another.
// An operator who accepted an ephemeral apply, or accepted cleanup, has
// not accepted infrastructure that persists and bills hourly.
func TestUICapabilityFlagsDoNotImplyEachOther(t *testing.T) {
	cmd := newUICmd(nil)

	for _, name := range []string{"allow-layer3", "allow-teardown", "allow-deploy"} {
		flag := cmd.Flags().Lookup(name)
		require.NotNil(t, flag, "--%s must exist", name)
		assert.Equal(t, "false", flag.DefValue, "--%s must never default on", name)
	}

	// Teardown on, deploy off: the destructive capability confers no
	// creative one.
	deployer, err := deployActor(cmd, false)
	require.NoError(t, err)
	assert.Nil(t, deployer)

	actor, err := teardownActor(cmd, false)
	require.NoError(t, err)
	assert.Nil(t, actor)
}

// A guard that stops without saying why is half a guard. If the operator
// asked for deploy and it cannot be built, starting anyway would hand
// them a UI silently missing the capability they requested.
func TestUIRefusesToStartWhenRequestedDeployCannotBeBuilt(t *testing.T) {
	cmd := newUICmd(nil)
	cmd.Flags().String("config", filepath.Join(t.TempDir(), "missing.yaml"), "")

	_, err := deployActor(cmd, true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--allow-deploy was requested")
}
