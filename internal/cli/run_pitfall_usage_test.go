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
// generate_hcl phase reports carrying two scaleway pitfalls, as an adapter
// does, with the generated HCL setting enable_ipv6 on a server and
// validate failing with staticErr (nil passes). It returns the run's
// metadata, its first iteration's failures and app.log, and the two
// carried entries.
func runWithTwoPitfalls(t *testing.T, staticErr, pitfallsErr error) (runstore.RunMetadata, []FailureSummary, string, []generator.PitfallEntry) {
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
	carried, err := generator.LoadRenderedPitfalls(h.PitfallsDir(), "scaleway")
	require.NoError(t, err)
	require.Len(t, carried, 2)

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
				Files: map[string][]byte{"main.tf": []byte("resource \"scaleway_instance_server\" \"web\" {\n  enable_ipv6 = true\n}\n")},
				Metadata: generator.GenerationMetadata{
					Phases:      []generator.PhaseResult{{Name: generator.PhasePlanArchitecture}, {Name: generator.PhaseGenerateHCL, Pitfalls: carried}},
					PitfallsErr: pitfallsErr,
				},
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
	runDir := filepath.Join(h.RunstoreRoot(), "example-scenario", runs[0].RunID)
	payload, err := os.ReadFile(filepath.Join(runDir, "iterations", "1", "iteration.json"))
	require.NoError(t, err)
	var iteration struct {
		Failures []FailureSummary `json:"failures"`
	}
	require.NoError(t, json.Unmarshal(payload, &iteration))
	appLog, err := os.ReadFile(filepath.Join(runDir, "app.log"))
	require.NoError(t, err)
	return runs[0], iteration.Failures, string(appLog), carried
}

func TestRunRecordsRenderedPitfallIDs(t *testing.T) {
	meta, _, _, carried := runWithTwoPitfalls(t, nil, nil)

	assert.Equal(t, "success", meta.Status)
	assert.Equal(t, []runstore.RenderedPitfall{
		{ID: generator.PitfallID(carried[0]), File: "scaleway.yaml"},
		{ID: generator.PitfallID(carried[1]), File: "scaleway.yaml"},
	}, meta.PitfallsRendered)
}

func TestRunRecordsFailureMatchingRenderedPitfall(t *testing.T) {
	_, failures, _, carried := runWithTwoPitfalls(t, errors.New("Error: Unsupported argument\n  with scaleway_instance_server.web,\n  on main.tf line 2"), nil)

	require.NotEmpty(t, failures)
	assert.Equal(t, generator.PitfallID(carried[0]), failures[0].MatchesPitfall)
}

func TestRunLogsAPitfallsLoadError(t *testing.T) {
	_, _, appLog, _ := runWithTwoPitfalls(t, nil, errors.New(`load pitfalls "pitfalls/scaleway.yaml": parse error`))

	assert.Contains(t, appLog, `"event":"pitfall_load_error"`)
	assert.Contains(t, appLog, "pitfalls/scaleway.yaml")
}

// A failure raised after the generator returned but before generated/ is
// written still matches: the generated files are checked in memory.
func TestRecordPitfallUsageMatchesFilesNeverWritten(t *testing.T) {
	carried := []generator.PitfallEntry{{Resource: "scaleway_instance_server", Rule: usageIPv6Rule, File: "scaleway.yaml"}}
	generated := &generator.GeneratedCode{
		Files:    map[string][]byte{"main.tf": []byte(`resource "scaleway_instance_server" "web" { enable_ipv6 = true }`)},
		Metadata: generator.GenerationMetadata{Phases: []generator.PhaseResult{{Name: generator.PhaseGenerateHCL, Pitfalls: carried}}},
	}
	failures := []FailureSummary{{Layer: "run", Stage: "iteration_1_generate", Check: "generate", Detail: "validate generated files: scaleway_instance_server.web is refused"}}
	runtime := &CommandRuntime{Logger: NewAppLogger(&bytes.Buffer{})}

	rendered := recordPitfallUsage(runtime, "run-1", generated, failures, nil)

	assert.Equal(t, []runstore.RenderedPitfall{{ID: generator.PitfallID(carried[0]), File: "scaleway.yaml"}}, rendered)
	assert.Equal(t, generator.PitfallID(carried[0]), failures[0].MatchesPitfall)
}
