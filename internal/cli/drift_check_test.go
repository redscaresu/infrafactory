package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// driftingMockDeploy reports a non-empty second plan, which is what a
// mock that does not echo back what the provider sent produces.
type driftingMockDeploy struct {
	calls int
}

func (d *driftingMockDeploy) Run(_ context.Context, _ string, _ map[string]string, _ harness.MockDeployMode) (*harness.MockDeployResult, error) {
	d.calls++
	return &harness.MockDeployResult{
		Apply:         harness.StageResult{Stage: "apply"},
		StateSnapshot: []byte(`{"instance":{"servers":[]}}`),
		Converge: harness.StageResult{
			Stage:  "converge",
			Stdout: "# scaleway_lb.main will be updated in-place\n  ~ ip_ids = [...]",
		},
		Drifted: true,
	}, nil
}

func driftRuntime(t *testing.T, h *CommandTestHarness, deploy *driftingMockDeploy) (*CommandRuntime, string) {
	t.Helper()
	scenarioPath := writeUnsupportedCriteriaScenario(t, h.WorkspaceDir)
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Paths.Output = h.OutputDir()
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		livestoreRoot:  h.LivestoreRoot(),
		deps: RuntimeDependencies{
			MockDeploy: deploy,
			Destroy: &fakeDestroyHarness{result: &harness.DestroyResult{
				Destroy:       harness.StageResult{Stage: "destroy"},
				StateSnapshot: []byte(`{}`),
			}},
		},
	}
	cmd := &cobra.Command{Use: "test <scenario>"}
	cmd.Flags().String("config", h.ConfigPath, "")
	rt, err := buildRuntime(cmd, opts)
	require.NoError(t, err)
	_, err = rt.LoadScenario(scenarioPath)
	require.NoError(t, err)
	return rt, scenarioPath
}

// Stopping is the default because continuing feeds the drift to the
// repair loop, and if the cause is the mock the model rewrites HCL that
// was never wrong.
func TestDriftStopsTheTestByDefault(t *testing.T) {
	h := newCommandTestHarness(t)
	deploy := &driftingMockDeploy{}
	rt, scenarioPath := driftRuntime(t, h, deploy)

	result, err := executeTest(t.Context(), rt, scenarioPath, testExecutionOptions{})

	require.Error(t, err, "a stack that does not converge is a defect")
	assert.Equal(t, CommandStatusFailed, result.Status)

	detail := failureDetail(result.Failures, "mock_deploy", "converge")
	assert.Contains(t, detail, "does not converge")
	assert.Contains(t, detail, "cannot tell them apart",
		"naming one cause would be a confident diagnosis this run cannot support")
	assert.Contains(t, detail, "Layer 3", "the operator needs the command that disambiguates")
	assert.Contains(t, detail, "scaleway_lb.main", "the plan says WHICH attribute drifts")

	// Nothing downstream ran. Checked by LAYER rather than by detail:
	// a passing stage carries an empty detail too, so a detail check
	// cannot tell "did not run" from "ran fine".
	for _, st := range result.Stages {
		assert.NotEqual(t, "sandbox_deploy", st.Layer,
			"a stack that does not converge must not reach real infrastructure")
	}

	// The mock IS torn down, though. Leaving it up satisfies two of
	// detectRunMode's three incremental conditions, so the next run of
	// a scenario with any earlier success would silently build on a
	// state already known to disagree with its config.
	assert.Equal(t, StageStatusPass, stageStatus(result.Stages, "destruction", "destroy"),
		"Layer 2 teardown is free; skipping it buys nothing and poisons the next run mode")
}

func TestDriftContinuesWhenAsked(t *testing.T) {
	h := newCommandTestHarness(t)
	deploy := &driftingMockDeploy{}
	rt, scenarioPath := driftRuntime(t, h, deploy)

	result, _ := executeTest(t.Context(), rt, scenarioPath, testExecutionOptions{ContinueOnDrift: true})

	detail := failureDetail(result.Failures, "mock_deploy", "converge")
	require.NotEmpty(t, detail, "continuing does not make it stop being a failure")
	assert.Contains(t, detail, "Continuing by request",
		"the mode taken has to be in the record, not only in the operator's memory")
	assert.Contains(t, detail, "may never have been wrong")

	// The proof it carried on: stages exist past the converge check.
	var sawLater bool
	for _, s := range result.Stages {
		if s.Layer == "destruction" || s.Layer == "criteria" {
			sawLater = true
		}
	}
	assert.True(t, sawLater, "continue mode must actually reach the later layers")
}

// "Drift found" and "drift found, and here is what I did" are different
// messages. Only the second lets somebody watching decide whether to
// interrupt -- and `run` persists no StageSummary anywhere they can see.
func TestDriftIsLoggedWithTheModeItTook(t *testing.T) {
	for _, tc := range []struct {
		name     string
		opts     testExecutionOptions
		expected string
	}{
		{name: "stop", opts: testExecutionOptions{}, expected: "Stopping the run"},
		{name: "continue", opts: testExecutionOptions{ContinueOnDrift: true}, expected: "Continuing by request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newCommandTestHarness(t)
			rt, scenarioPath := driftRuntime(t, h, &driftingMockDeploy{})
			logs := &bytes.Buffer{}
			rt.Logger = NewAppLogger(logs)

			_, _ = executeTest(t.Context(), rt, scenarioPath, tc.opts)

			assert.Contains(t, logs.String(), "mock_deploy_drift")
			assert.Contains(t, logs.String(), "drift detected")
			assert.Contains(t, logs.String(), tc.expected)
		})
	}
}

// A plan that could not run is not a plan that found changes.
func TestABrokenConvergePlanIsNotReportedAsDrift(t *testing.T) {
	h := newCommandTestHarness(t)
	rt, scenarioPath := driftRuntime(t, h, nil)
	rt.Deps.MockDeploy = &fakeMockDeployHarness{
		err: &harness.MockDeployError{
			Stage:    "converge",
			Converge: harness.StageResult{Stderr: "Error: Get \"http://127.0.0.1:8080\": connection refused"},
			Err:      errors.New("exit status 1"),
		},
	}

	result, err := executeTest(t.Context(), rt, scenarioPath, testExecutionOptions{})

	require.Error(t, err)
	assert.Equal(t, StageStatusFail, stageStatus(result.Stages, "mock_deploy", "converge"))

	detail := failureDetail(result.Failures, "mock_deploy", "converge")
	assert.NotContains(t, detail, "does not converge",
		"the plan never ran, so it observed no drift to report")
	// A guard that stops without saying why is half a guard: bare
	// "exit status 1" discards the only text that says what broke.
	assert.Contains(t, detail, "connection refused")
}

// The run loop must not hand an ambiguous signal to the repair loop:
// it can only ever change the HCL, and the cause may be the mock.
func TestRunStopsOnDriftWithoutSpendingAnotherIteration(t *testing.T) {
	h := newCommandTestHarness(t)
	scenarioPath := writeUnsupportedCriteriaScenario(t, h.WorkspaceDir)

	generated := 0
	deploy := &driftingMockDeploy{}
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Agent.RepairIterationsMax = 5
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			generated++
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		Static: &fakeStaticHarness{result: &harness.StaticResult{
			Stages:   []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}},
			PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`),
		}},
		MockDeploy: deploy,
		Destroy:    &fakeDestroyHarness{result: &harness.DestroyResult{StateSnapshot: []byte(`{}`)}},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath})

	require.Error(t, cmd.Execute())
	assert.Equal(t, 1, generated, "the repair loop must not be given a signal it cannot act on")
	assert.Equal(t, 1, deploy.calls)
	assert.Contains(t, stdout.String(), "drift")
}

func failureDetail(failures []FailureSummary, layer, stage string) string {
	for _, f := range failures {
		if f.Layer == layer && f.Stage == stage {
			return f.Detail
		}
	}
	return ""
}

func stageStatus(stages []StageSummary, layer, stage string) StageStatus {
	for _, s := range stages {
		if s.Layer == layer && s.Stage == stage {
			return s.Status
		}
	}
	return ""
}

var _ = os.Getenv
var _ = filepath.Join

// The drift message tells the operator to pass --continue-on-drift.
// Advice naming a flag the command does not have is worse than no
// advice: it sends them to an "unknown flag" error.
func TestBothCommandsThatCanHitDriftAcceptTheFlag(t *testing.T) {
	cfg := &rootConfig{}
	for _, tc := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "run", cmd: newRunCmd(cfg)},
		{name: "test", cmd: newTestCmd(cfg)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NotNil(t, tc.cmd.Flags().Lookup("continue-on-drift"),
				"the drift failure tells the operator to pass this to %s", tc.name)
		})
	}
}
