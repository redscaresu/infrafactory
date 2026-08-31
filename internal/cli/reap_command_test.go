package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/feedback"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const reapProjectID = "2397e80e-ec12-4a7e-819f-a2caba3867b6"
const reapOrgID = "22222222-2222-2222-2222-222222222222"

// reapRuntime builds a Layer-3-enabled runtime whose output dir is
// workspace-scoped, so parallel tests cannot share live state.
func reapRuntime(t *testing.T, destroy *fakeSandboxDestroyHarness, sweep *fakeOrphanSweep) (*CommandRuntime, string) {
	t.Helper()
	h := newCommandTestHarness(t)
	scenarioPath := writeUnsupportedCriteriaScenario(t, h.WorkspaceDir)

	cfg, err := config.Load(h.ConfigPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Paths.Output = h.OutputDir()
	cfg.Validation.Layers.SandboxDeploy.Enabled = true

	rt := &CommandRuntime{
		Config:         cfg,
		scenarioLoader: defaultScenarioLoader,
		Deps: RuntimeDependencies{
			SandboxDestroy: destroy,
			OrphanSweep:    sweep,
			RunProject:     &fakeRunProject{},
		},
	}
	if _, err := rt.LoadScenario(scenarioPath); err != nil {
		t.Fatalf("load scenario: %v", err)
	}
	if err := os.MkdirAll(rt.OutputDir(), 0o755); err != nil {
		t.Fatalf("mkdir output: %v", err)
	}
	return rt, scenarioPath
}

func writeReapLiveState(t *testing.T, dir, projectID string) {
	t.Helper()
	// ADR-0025: reap reads the marker, not the state, to learn what it
	// may destroy. Written alongside so fixtures cover both.
	if err := harness.WriteRunProjectMarker(dir, harness.RunProject{
		ID: projectID, Name: harness.RunProjectNamePrefix + "reap",
	}); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	body := `{"resources":[
	  {"type":"scaleway_account_project","instances":[{"attributes":{"id":"` + projectID + `"}}]},
	  {"type":"scaleway_block_volume","instances":[{"attributes":{"id":"vol-1","project_id":"` + projectID + `"}}]}
	]}`
	if err := os.WriteFile(filepath.Join(dir, harness.LiveStateFilename), []byte(body), 0o600); err != nil {
		t.Fatalf("write live state: %v", err)
	}
}

func runReap(t *testing.T, rt *CommandRuntime, scenarioPath string, out *strings.Builder, args ...string) error {
	t.Helper()
	cmd := &cobra.Command{Use: "reap"}
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetContext(context.Background())
	return runReapCommand(cmd, []string{scenarioPath}, rt)
}

// --dry-run must be inert. An operator reaching for reap is already in a
// bad situation; pointing it at something just to see what it would do
// has to be safe.
func TestReapDryRunDestroysNothing(t *testing.T) {
	sandboxCredsForTest(t)
	destroy, sweep := &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{}
	rt, scenarioPath := reapRuntime(t, destroy, sweep)
	writeReapLiveState(t, rt.OutputDir(), reapProjectID)

	out := &strings.Builder{}
	if err := runReap(t, rt, scenarioPath, out, "--dry-run"); err != nil {
		t.Fatalf("dry-run should succeed: %v", err)
	}
	if destroy.calls != 0 || sweep.calls != 0 {
		t.Fatalf("--dry-run must not act; destroy=%d sweep=%d", destroy.calls, sweep.calls)
	}
	if !strings.Contains(out.String(), reapProjectID) {
		t.Fatalf("dry-run should report what it would destroy, got:\n%s", out.String())
	}
}

func TestReapDestroysThenVerifies(t *testing.T) {
	sandboxCredsForTest(t)
	destroy := &fakeSandboxDestroyHarness{result: &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}}}
	sweep := &fakeOrphanSweep{}
	rt, scenarioPath := reapRuntime(t, destroy, sweep)
	writeReapLiveState(t, rt.OutputDir(), reapProjectID)

	if err := runReap(t, rt, scenarioPath, &strings.Builder{}); err != nil {
		t.Fatalf("reap: %v", err)
	}
	if destroy.calls != 1 || sweep.calls != 1 {
		t.Fatalf("reap must destroy then verify; destroy=%d sweep=%d", destroy.calls, sweep.calls)
	}
	// The sweep verifies the project is GONE, and since ADR-0025 tofu
	// cannot delete it. Without this, every clean reap reported a leak.
	projects := rt.Deps.RunProject.(*fakeRunProject)
	assert.Equal(t, 1, projects.deletes, "reap must delete the run project itself")
	assert.Equal(t, reapProjectID, projects.deletedID)
}

// Destroying is not the same as proving the account is clean.
func TestReapFailsWhenSweepStillReportsALeak(t *testing.T) {
	sandboxCredsForTest(t)
	destroy := &fakeSandboxDestroyHarness{result: &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}}}
	sweep := &fakeOrphanSweep{result: &harness.OrphanSweepResult{
		ProjectID: reapProjectID,
		Failures: []feedback.Failure{{
			Layer: "sandbox_deploy", Stage: "orphan_sweep", Check: "project_deleted",
			Detail: "project still exists after destroy",
		}},
	}}
	rt, scenarioPath := reapRuntime(t, destroy, sweep)
	writeReapLiveState(t, rt.OutputDir(), reapProjectID)

	if err := runReap(t, rt, scenarioPath, &strings.Builder{}); err == nil {
		t.Fatal("a reap that cannot prove the account is clean must fail")
	}
}

func TestReapWithoutLiveStateIsANoop(t *testing.T) {
	sandboxCredsForTest(t)
	destroy := &fakeSandboxDestroyHarness{}
	rt, scenarioPath := reapRuntime(t, destroy, &fakeOrphanSweep{})

	out := &strings.Builder{}
	if err := runReap(t, rt, scenarioPath, out); err != nil {
		t.Fatalf("no live state should be a clean no-op: %v", err)
	}
	if destroy.calls != 0 {
		t.Fatal("nothing to reap must mean nothing destroyed")
	}
}

// The guard between reap and someone's real infrastructure.
func TestReapRefusesProjectTheRunDidNotCreate(t *testing.T) {
	sandboxCredsForTest(t)
	destroy := &fakeSandboxDestroyHarness{}
	rt, scenarioPath := reapRuntime(t, destroy, &fakeOrphanSweep{})
	// The org id IS the default project's id; deleting it would take the
	// account's contents with it.
	writeReapLiveState(t, rt.OutputDir(), reapOrgID)

	if err := runReap(t, rt, scenarioPath, &strings.Builder{}); err == nil {
		t.Fatal("reap must refuse the organization default project")
	}
	if destroy.calls != 0 {
		t.Fatal("refusal must happen before any destroy is issued")
	}
}

func cancelledNotify() func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
		return ctx, func() {}
	}
}

func guardCmd(out *strings.Builder) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetErr(out)
	cmd.SetContext(context.Background())
	return cmd
}

// Interrupted before anything was created: not a cleanup failure.
func TestInterruptGuardNoopsWithoutLiveState(t *testing.T) {
	sandboxCredsForTest(t)
	destroy := &fakeSandboxDestroyHarness{}
	rt, _ := reapRuntime(t, destroy, &fakeOrphanSweep{})

	out := &strings.Builder{}
	_ = withSandboxInterruptGuard(guardCmd(out), rt, cancelledNotify(), func(context.Context) error { return nil })

	if destroy.calls != 0 {
		t.Fatal("nothing was created, so nothing should be destroyed")
	}
	if !strings.Contains(out.String(), "nothing to clean up") {
		t.Fatalf("operator should be told there is nothing live, got: %q", out.String())
	}
}

// The case the guard exists for: interrupted AFTER real resources were
// created. Destroy must still run, on a context that is not cancelled.
func TestInterruptGuardDestroysLiveResources(t *testing.T) {
	sandboxCredsForTest(t)
	destroy := &fakeSandboxDestroyHarness{result: &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}}}
	rt, _ := reapRuntime(t, destroy, &fakeOrphanSweep{})
	writeReapLiveState(t, rt.OutputDir(), reapProjectID)

	out := &strings.Builder{}
	_ = withSandboxInterruptGuard(guardCmd(out), rt, cancelledNotify(), func(context.Context) error { return nil })

	if destroy.calls != 1 {
		t.Fatalf("interrupt with live resources must trigger destroy, got %d", destroy.calls)
	}
	if destroy.lastCtx != nil && destroy.lastCtx.Err() != nil {
		t.Fatal("cleanup destroy must run on a fresh context — doing work after cancellation is the whole point")
	}
	// tofu cannot delete the project any more, and an interrupt is the
	// one exit with no summary to report a kept project in.
	projects := rt.Deps.RunProject.(*fakeRunProject)
	assert.Equal(t, 1, projects.deletes, "the interrupt must delete the run project too")
	assert.Contains(t, out.String(), "Run project "+reapProjectID+" deleted")
}

// Interrupted between creating the project and writing any state: there
// is nothing for tofu to destroy, but a real project to delete. Before
// ADR-0025 this shape did not exist -- the project came from the apply.
func TestInterruptGuardDeletesTheProjectWhenNothingWasApplied(t *testing.T) {
	sandboxCredsForTest(t)
	destroy := &fakeSandboxDestroyHarness{}
	rt, _ := reapRuntime(t, destroy, &fakeOrphanSweep{})
	require.NoError(t, harness.WriteRunProjectMarker(rt.OutputDir(),
		harness.RunProject{ID: reapProjectID, Name: harness.RunProjectNamePrefix + "interrupt"}))

	out := &strings.Builder{}
	_ = withSandboxInterruptGuard(guardCmd(out), rt, cancelledNotify(), func(context.Context) error { return nil })

	assert.Zero(t, destroy.calls, "no state means nothing for tofu to do")
	assert.Equal(t, 1, rt.Deps.RunProject.(*fakeRunProject).deletes)
	assert.Contains(t, out.String(), "deleted")
}

// State on disk and no marker: the destroy cannot be scoped to the
// project the apply used, and running it against the shared fallback
// would not be the inverse of that apply. Refuse and hand over the
// recovery command rather than destroy against a guess.
func TestInterruptGuardRefusesToDestroyWithoutAMarker(t *testing.T) {
	sandboxCredsForTest(t)
	destroy := &fakeSandboxDestroyHarness{result: &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}}}
	rt, _ := reapRuntime(t, destroy, &fakeOrphanSweep{})
	writeReapLiveState(t, rt.OutputDir(), reapProjectID)
	require.NoError(t, os.Remove(filepath.Join(rt.OutputDir(), harness.RunProjectMarkerFilename)))

	out := &strings.Builder{}
	_ = withSandboxInterruptGuard(guardCmd(out), rt, cancelledNotify(), func(context.Context) error { return nil })

	assert.Zero(t, destroy.calls, "a destroy scoped to the wrong project is not the inverse of the apply")
	assert.Zero(t, rt.Deps.RunProject.(*fakeRunProject).deletes)
	assert.Contains(t, out.String(), "CLEANUP FAILED")
	assert.Contains(t, out.String(), "cannot tell which project this run owns")
}

// If cleanup itself fails the operator must be left knowing resources
// are live and exactly how to finish the job.
func TestInterruptGuardReportsAbandonedResourcesOnCleanupFailure(t *testing.T) {
	sandboxCredsForTest(t)
	destroy := &fakeSandboxDestroyHarness{err: harness.ErrSandboxDestroyFailed}
	rt, _ := reapRuntime(t, destroy, &fakeOrphanSweep{})
	writeReapLiveState(t, rt.OutputDir(), reapProjectID)

	out := &strings.Builder{}
	_ = withSandboxInterruptGuard(guardCmd(out), rt, cancelledNotify(), func(context.Context) error { return nil })

	for _, want := range []string{"CLEANUP FAILED", "infrafactory reap", harness.LiveStateFilename} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("cleanup-failure message must contain %q, got:\n%s", want, out.String())
		}
	}
}

// Layer 3 off: no signal handler at all. Interrupting a mock-only run
// costs nothing.
func TestInterruptGuardInertWhenLayer3Disabled(t *testing.T) {
	rt, _ := reapRuntime(t, &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{})
	rt.Config.Validation.Layers.SandboxDeploy.Enabled = false

	notified := false
	notify := func(ctx context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
		notified = true
		return ctx, func() {}
	}
	if err := withSandboxInterruptGuard(guardCmd(&strings.Builder{}), rt, notify, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("guard: %v", err)
	}
	if notified {
		t.Fatal("no signal handler should be installed when Layer 3 is off")
	}
}
