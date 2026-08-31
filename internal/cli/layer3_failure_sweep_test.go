package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// liveStateWithProject is a Layer 3 state the sweep can actually derive a
// blast radius from -- CaptureSweepTarget needs the scaleway_account_project
// id, which is exactly what tofu destroy erases.
const liveStateWithProject = `{
  "version": 4,
  "resources": [
    {
      "type": "scaleway_account_project",
      "name": "main",
      "instances": [{"attributes": {"id": "11111111-1111-1111-1111-111111111111"}}]
    }
  ]
}`

// failingRunDeps builds a run that fails at validate with Layer 3 state on
// disk, so the auto-destroy-on-failure path is reached.
func failingRunDeps(sweep *fakeOrphanSweep, destroy *fakeSandboxDestroyHarness) RuntimeDependencies {
	return RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(_ context.Context, _ generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{
				"main.tf":                 []byte("terraform {}\n"),
				"project.tf":              []byte("resource \"scaleway_account_project\" \"main\" { name = \"test\" }\n"),
				harness.LiveStateFilename: []byte(liveStateWithProject),
			}}, nil
		}),
		Static: &fakeStaticHarness{err: &harness.StageError{
			StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}},
			Err:         errors.New("validate failed"),
		}},
		MockDeploy:     &fakeMockDeployHarness{},
		Destroy:        &fakeDestroyHarness{},
		SandboxDeploy:  &fakeSandboxDeployHarness{},
		SandboxDestroy: destroy,
		OrphanSweep:    sweep,
		RealProbe:      &fakeRealProbeHarness{result: &harness.RealProbeResult{}},
	}
}

func runFailingLayer3(t *testing.T, sweep *fakeOrphanSweep, destroy *fakeSandboxDestroyHarness) string {
	t.Helper()
	h := newCommandTestHarness(t)
	sandboxCredsForTest(t)

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Paths.Output = filepath.Join(h.WorkspaceDir, "output")
		cfg.Validation.Layers.SandboxDeploy.Enabled = true
		cfg.Agent.RepairIterationsMax = 1
		return cfg
	})
	opts.deps = failingRunDeps(sweep, destroy)

	out := &bytes.Buffer{}
	cmd := newRunCommandForTest(opts)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected the run to fail")
	}
	return out.String()
}

// The cleanup most likely to leave orphans was the one path that never
// verified itself: a failed run destroyed real resources and then simply
// trusted that destroy worked.
func TestFailedRunSweepsAfterAutoDestroy(t *testing.T) {
	destroy := &fakeSandboxDestroyHarness{
		result: &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}},
	}
	sweep := &fakeOrphanSweep{}

	runFailingLayer3(t, sweep, destroy)

	if destroy.calls != 1 {
		t.Fatalf("got %d auto-destroy calls, want 1", destroy.calls)
	}
	if sweep.calls != 1 {
		t.Fatalf("got %d orphan sweep calls after auto-destroy, want 1", sweep.calls)
	}
}

// "We could not check" and "nothing leaked" must never look alike. The run
// has already failed, so the exit code cannot change -- what must change is
// that the operator is told the cleanup went unverified, and what to run.
func TestFailedRunReportsUnverifiableSweepWithRecoveryCommand(t *testing.T) {
	destroy := &fakeSandboxDestroyHarness{
		result: &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}},
	}
	sweep := &fakeOrphanSweep{err: errors.New("dial tcp: lookup api.scaleway.com: no such host")}

	out := runFailingLayer3(t, sweep, destroy)

	if sweep.calls != 1 {
		t.Fatalf("got %d sweep calls, want 1", sweep.calls)
	}
	if !strings.Contains(out, "orphan_sweep") {
		t.Errorf("an unreachable sweep was not reported at all:\n%s", out)
	}
	if !strings.Contains(out, "no such host") {
		t.Errorf("run did not say why the sweep could not verify:\n%s", out)
	}
	assertNamesReapCommand(t, out)
}

// A failed destroy is the orphaned-billing case. Don't sweep after it (there
// is nothing meaningful to conclude), but do name the recovery command.
func TestFailedRunSkipsSweepButHintsWhenAutoDestroyFails(t *testing.T) {
	destroy := &fakeSandboxDestroyHarness{
		err: &harness.SandboxDestroyError{
			Stage:   "destroy",
			Destroy: harness.StageResult{Stage: "destroy", Stderr: "Error: project not empty"},
			Err:     errors.New("exit status 1"),
		},
	}
	sweep := &fakeOrphanSweep{}

	out := runFailingLayer3(t, sweep, destroy)

	if sweep.calls != 0 {
		t.Fatalf("got %d sweep calls after a failed destroy, want 0", sweep.calls)
	}
	assertNamesReapCommand(t, out)
}

// assertNamesReapCommand checks the run printed a cleanup command the
// operator can actually paste. These runs use a non-default --config, so
// the hint must carry it: reap rebuilds its runtime from that flag, and
// without it the operator is sent to the default output directory where
// reap finds no live state and reports nothing to do.
func assertNamesReapCommand(t *testing.T, out string) {
	t.Helper()
	if !strings.Contains(out, " reap ") {
		t.Errorf("run did not name the recovery command:\n%s", out)
		return
	}
	if !strings.Contains(out, "--config") {
		t.Errorf("recovery command dropped the run's --config, so reap would look in the wrong output dir:\n%s", out)
	}
}
