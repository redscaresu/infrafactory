package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/spf13/cobra"
)

type fakeMockDeployHarness struct {
	result *harness.MockDeployResult
	err    error
	calls  int
}

func (f *fakeMockDeployHarness) Run(context.Context, string, map[string]string) (*harness.MockDeployResult, error) {
	f.calls++
	return f.result, f.err
}

type fakeDestroyHarness struct {
	result *harness.DestroyResult
	err    error
	calls  int
}

func (f *fakeDestroyHarness) Run(context.Context, string, map[string]string) (*harness.DestroyResult, error) {
	f.calls++
	return f.result, f.err
}

func TestTestCommandSuccess(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	mockDeploy := &fakeMockDeployHarness{
		result: &harness.MockDeployResult{
			Apply:         harness.StageResult{Stage: "apply"},
			StateSnapshot: []byte(`{"mock":true}`),
		},
	}
	destroy := &fakeDestroyHarness{
		result: &harness.DestroyResult{
			Destroy:       harness.StageResult{Stage: "destroy"},
			StateSnapshot: []byte(`{"mock":true}`),
			OrphanCount:   0,
		},
	}

	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockDeploy: mockDeploy,
			Destroy:    destroy,
		},
	}

	cmd := newTestCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute test command: %v", err)
	}
	if mockDeploy.calls != 1 || destroy.calls != 1 {
		t.Fatalf("expected one deploy and one destroy call, got deploy=%d destroy=%d", mockDeploy.calls, destroy.calls)
	}
	if !strings.Contains(stdout.String(), "Status: success") {
		t.Fatalf("expected success output, got:\n%s", stdout.String())
	}
}

func TestTestCommandMockDeployFailure(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	mockDeploy := &fakeMockDeployHarness{
		err: &harness.MockDeployError{
			Stage: "apply",
			Apply: harness.StageResult{Stage: "apply", Cmd: []string{"tofu", "apply"}},
			Err:   errors.New("apply failed"),
		},
	}
	destroy := &fakeDestroyHarness{}

	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockDeploy: mockDeploy,
			Destroy:    destroy,
		},
	}

	cmd := newTestCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected failure")
	}
	if destroy.calls != 0 {
		t.Fatalf("expected destroy not to run after deploy failure, got %d", destroy.calls)
	}
	if !strings.Contains(stdout.String(), "- mock_deploy/apply: fail") {
		t.Fatalf("expected apply failure stage in output, got:\n%s", stdout.String())
	}
	if ExitCodeForError(err) != ExitCodeRuntime {
		t.Fatalf("expected runtime exit code, got %d", ExitCodeForError(err))
	}
}

func TestTestCommandDestroyFailure(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	mockDeploy := &fakeMockDeployHarness{
		result: &harness.MockDeployResult{
			Apply:         harness.StageResult{Stage: "apply"},
			StateSnapshot: []byte(`{"mock":true}`),
		},
	}
	destroy := &fakeDestroyHarness{
		err: &harness.DestroyError{
			Stage:   "orphan_check",
			Destroy: harness.StageResult{Stage: "destroy", Cmd: []string{"tofu", "destroy"}},
			Err:     errors.New("detected 1 orphaned resources"),
		},
	}

	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockDeploy: mockDeploy,
			Destroy:    destroy,
		},
	}

	cmd := newTestCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected failure")
	}
	if !strings.Contains(stdout.String(), "- destruction/orphan_check: fail") {
		t.Fatalf("expected orphan check failure stage in output, got:\n%s", stdout.String())
	}
}

func newTestCommandForTest(opts runtimeOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "test <scenario>",
		Args: requireScenarioArg,
		RunE: withRuntimeWithOptions("test", opts, runTestCommand),
	}
	cmd.Flags().String("config", config.DefaultPath, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	return cmd
}
