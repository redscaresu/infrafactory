package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/api"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// The WIRING, end to end, with nothing faked between the harness and the
// websocket.
//
// Every piece of this path was unit-tested and the path itself was not,
// which is how the original slice shipped a feature that did not
// function. A review demonstrated two one-line mutations that broke it
// completely while the whole Go suite stayed green:
//
//   - `cmd.SetErr(&progressCopy)` — drop the tee, and the UI log is
//     permanently empty;
//   - `Run(ctx, dir, env, nil)` — drop the writer at the call site, and
//     every stage line vanishes.
//
// Both are invisible to tests that check `deployStderr` in isolation and
// tests that call `Deploy` with a runtime that fails before it is used.
// This asserts the whole chain: real harness → real progress plumbing →
// real ProgressSink → real hub → what a browser would receive.
func TestStageProgressReachesTheWebsocketThroughTheRealChain(t *testing.T) {
	hub := api.NewHub()
	client := api.NewTestClient(256)
	hub.Register(client)

	// The REAL harness, with only the subprocess faked -- that is the
	// only seam a test may cut here, because everything above it is
	// what shipped broken. It records what a websocket client could see
	// while the apply was still running.
	var duringApply []string
	runner := harness.CommandRunnerFunc(func(_ context.Context, cmd harness.Command) (harness.CommandResult, error) {
		if len(cmd.Args) > 0 && cmd.Args[0] == "apply" {
			duringApply = drainProgress(client)
		}
		return harness.CommandResult{Stdout: []byte("ok")}, nil
	})

	rt, _, scenarioPath := deployTestRuntime(t, liveServiceScenarioYAML, harness.NewSandboxDeployHarness(runner))
	writeDeployableHCL(t, rt.OutputDir())
	t.Setenv("SCW_ACCESS_KEY", "test-access")
	t.Setenv("SCW_SECRET_KEY", "test-secret")
	t.Setenv("SCW_DEFAULT_ORGANIZATION_ID", "org-1")

	// Through LiveDeployer, so the tee inside it is on the path too.
	deployer := NewLiveDeployer(filepath.Dir(scenarioPath), func() (*CommandRuntime, error) {
		return rt, nil
	})

	sink := api.NewProgressSink(hub, "deploy_progress", "web-live-paris")
	_, err := deployer.Deploy(context.Background(), "web-live-paris", "", sink)
	require.NoError(t, err)
	require.NoError(t, sink.Close())

	seen := strings.Join(duringApply, "\n")
	assert.Contains(t, seen, "init: running",
		"a browser must learn init happened before the apply finishes")
	assert.Contains(t, seen, "apply: running",
		"and that the apply is what is taking the time")
}

// drainProgress reads the deploy_progress lines a websocket client would
// have received so far.
func drainProgress(client *api.Client) []string {
	var out []string
	for {
		raw, ok := client.TryReceive()
		if !ok {
			return out
		}
		var event struct {
			Type string            `json:"type"`
			Data map[string]string `json:"data"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			continue
		}
		if event.Type == "deploy_progress" {
			out = append(out, strings.TrimSpace(event.Data["line"]))
		}
	}
}
