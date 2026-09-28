package e2e

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStartMockwayInfrastructure verifies the e2e helper can start mockway
// from source, fetch state, reset state, and clean up. It is the
// foundational smoke test for the cross-repo e2e infrastructure (S33-T1).
//
// Gated behind INFRAFACTORY_ENABLE_E2E=1 because it shells out to
// `go run ./cmd/mockway` against the sibling repo.
func TestStartMockwayInfrastructure(t *testing.T) {
	SkipUnlessEnabled(t)

	mock := StartMockway(t)
	if mock.URL == "" {
		t.Fatal("expected non-empty mockway URL")
	}

	resp, err := http.Get(mock.URL + "/mock/state")
	if err != nil {
		t.Fatalf("get mockway state: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /mock/state, got %d", resp.StatusCode)
	}

	state := mock.FetchState(t)
	if state == nil {
		t.Fatal("expected non-nil state map")
	}

	mock.Reset(t)

	state = mock.FetchState(t)
	if state == nil {
		t.Fatal("expected non-nil state after reset")
	}
}

// TestRunInfrafactoryDrivesValidate exercises the in-process CLI driver by
// running the `validate` command against a hermetic scenario+config that
// only requires the static layer (no mockway, no tofu network calls). It
// verifies that WithRuntimeDependencies is wired correctly so external
// callers can inject stub generators without going through subprocess
// boundaries.
func TestRunInfrafactoryDrivesValidate(t *testing.T) {
	workspace := t.TempDir()
	scenarioPath := workspace + "/scenarios/training/e2e-smoke.yaml"
	outputRoot := workspace + "/output"
	outputDir := outputRoot + "/e2e-smoke"
	configPath := workspace + "/infrafactory.yaml"

	WriteFile(t, scenarioPath, []byte(`scenario: e2e-smoke
version: "1.0"
cloud: scaleway
description: e2e smoke
resources:
  compute:
    purpose: smoke
    size: small
acceptance_criteria:
  - type: policy
    check: region_restriction
    params:
      region: fr-par
    expect: pass
`))
	WriteFile(t, outputDir+"/main.tf", []byte("terraform {}\n"))
	WriteFile(t, configPath, []byte(`version: "1.0"
agent:
  type: claude-code
mockway:
  url: http://127.0.0.1:0
paths:
  output: `+outputRoot+`
validation:
  layers:
    static:
      enabled: false
      policy_paths: []
    mock_deploy:
      enabled: false
    sandbox_deploy:
      enabled: false
    destruction:
      enabled: false
`))

	result := RunInfrafactory(t, InfrafactoryRunOptions{
		Args: []string{"validate", scenarioPath, "--config", configPath},
	})

	if result.Err != nil {
		t.Fatalf("validate failed: %v\nstdout:\n%s\nstderr:\n%s",
			result.Err, result.Stdout, result.Stderr)
	}
}

// TestRunInfrafactoryPassesRunIsolationGuard fails if the harness would
// hand `run` a relative output dir or run-store root, which the M81 guard
// in run_command.go refuses under go test. That refusal broke every gated
// suite in this package at once, silently, because CI does not run them;
// this test is ungated. With no mock listening, the run stops at run-mode
// detection, the first step after the guard.
func TestRunInfrafactoryPassesRunIsolationGuard(t *testing.T) {
	workspace := t.TempDir()
	scenarioPath := filepath.Join(workspace, "scenarios", "training", "e2e-isolation.yaml")
	configPath := filepath.Join(workspace, "infrafactory.yaml")

	WriteFile(t, scenarioPath, []byte(`scenario: e2e-isolation
version: "1.0"
cloud: scaleway
description: e2e isolation guard
resources:
  compute:
    purpose: smoke
    size: small
acceptance_criteria:
  - type: policy
    check: region_restriction
    params:
      region: fr-par
    expect: pass
`))
	WriteConfig(t, configPath, "http://127.0.0.1:1", filepath.Join(workspace, "output"))

	result := RunInfrafactory(t, InfrafactoryRunOptions{
		Args: []string{"run", scenarioPath, "--config", configPath},
	})

	require.Error(t, result.Err)
	assert.NotContains(t, result.Err.Error(), "test isolation")
	// Proves the run got past the guard, not that it stopped before it.
	assert.Contains(t, result.Err.Error(), "detect run mode")
}
