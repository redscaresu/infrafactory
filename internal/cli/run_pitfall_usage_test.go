package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/runstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const usageIPv6Rule = "exit status 1 | stderr: ╷ Do NOT use attribute `enable_ipv6` on `scaleway_instance_server` — observed in scenario \"web\" to cause the failure above."

// runWithTwoPitfalls runs one stubbed iteration of example-scenario whose
// generate_hcl prompt renders two scaleway pitfalls, the generated HCL setting enable_ipv6
// on a server, and validate failing with staticErr (nil passes). It
// returns the run's metadata, its first iteration's failures, and the
// two rendered entries.
func runWithTwoPitfalls(t *testing.T, staticErr error) (runstore.RunMetadata, []FailureSummary, []generator.PitfallEntry) {
	t.Helper()
	h := newCommandTestHarness(t)
	mustWriteFile(t, filepath.Join(h.PitfallsDir(), "scaleway.yaml"), `provider: scaleway
pitfalls:
  - resource: scaleway_instance_server
    rule: '`+usageIPv6Rule+`'
    source: avoid
  - resource: scaleway_block_volume
    rule: Always set a 'size_in_gb'.
    source: docs
`)
	rendered, err := generator.LoadRenderedPitfalls(h.PitfallsDir(), "scaleway")
	require.NoError(t, err)
	require.Len(t, rendered, 2)

	prompt, err := generator.LoadPitfalls(h.PitfallsDir(), "scaleway")
	require.NoError(t, err)

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Agent.RepairIterationsMax = 1
		return cfg
	})
	static := &fakeStaticHarness{result: &harness.StaticResult{Stages: []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}}, PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`)}}
	if staticErr != nil {
		static = &fakeStaticHarness{err: &harness.StageError{StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}}, Err: staticErr}}
	}
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{
				Files:    map[string][]byte{"main.tf": []byte("resource \"scaleway_instance_server\" \"web\" {\n  enable_ipv6 = true\n}\n")},
				Metadata: generator.GenerationMetadata{Phases: []generator.PhaseResult{{Name: generator.PhaseGenerateHCL, Prompt: []byte(prompt)}}},
			}, nil
		}),
		Static:     static,
		MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{}`)}},
		Destroy:    &fakeDestroyHarness{result: &harness.DestroyResult{Destroy: harness.StageResult{Stage: "destroy"}}},
	}
	cmd := newRunCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})
	_ = cmd.Execute()

	runs, err := runstore.NewFilesystemStore(h.RunstoreRoot()).ListRuns("example-scenario")
	require.NoError(t, err)
	require.Len(t, runs, 1)
	payload, err := os.ReadFile(filepath.Join(h.RunstoreRoot(), "example-scenario", runs[0].RunID, "iterations", "1", "iteration.json"))
	require.NoError(t, err)
	var iteration struct {
		Failures []FailureSummary `json:"failures"`
	}
	require.NoError(t, json.Unmarshal(payload, &iteration))
	return runs[0], iteration.Failures, rendered
}

func TestRunRecordsRenderedPitfallIDs(t *testing.T) {
	meta, _, rendered := runWithTwoPitfalls(t, nil)

	assert.Equal(t, "success", meta.Status)
	assert.Equal(t, []string{
		generator.PitfallID("scaleway", rendered[0]),
		generator.PitfallID("scaleway", rendered[1]),
	}, meta.PitfallsRendered)
}

func TestRunRecordsFailureMatchingRenderedPitfall(t *testing.T) {
	_, failures, rendered := runWithTwoPitfalls(t, errors.New("Error: Unsupported argument\n  with scaleway_instance_server.web,\n  on main.tf line 2"))

	require.NotEmpty(t, failures)
	assert.Equal(t, generator.PitfallID("scaleway", rendered[0]), failures[0].MatchesPitfall)
}
