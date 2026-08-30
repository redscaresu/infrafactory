package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
)

// liveTeardownRuntime builds a Layer-3-enabled runtime with fake destroy
// and sweep harnesses and a workspace-scoped live store, so no test can
// reach the operator's real record or the real API.
func liveTeardownRuntime(t *testing.T, destroy *fakeSandboxDestroyHarness, sweep *fakeOrphanSweep) (*CommandRuntime, *livestore.FilesystemStore, string) {
	t.Helper()
	h := newCommandTestHarness(t)

	cfg, err := config.Load(h.ConfigPath)
	require.NoError(t, err)
	cfg.Paths.Output = h.OutputDir()
	cfg.Validation.Layers.SandboxDeploy.Enabled = true

	rt := &CommandRuntime{
		Config:        cfg,
		livestoreRoot: h.LivestoreRoot(),
		Deps: RuntimeDependencies{
			SandboxDestroy: destroy,
			OrphanSweep:    sweep,
		},
	}

	return rt, livestore.NewFilesystemStore(h.LivestoreRoot()), h.WorkspaceDir
}

// liveDeploymentWithState writes a deployment record plus the live state
// file its teardown reads, both scoped to the same project.
func liveDeploymentWithState(t *testing.T, store *livestore.FilesystemStore, workspace, id string, expiresIn time.Duration) livestore.Deployment {
	t.Helper()

	workDir := filepath.Join(workspace, "live", id)
	require.NoError(t, os.MkdirAll(workDir, 0o755))
	writeReapLiveState(t, workDir, reapProjectID)

	now := time.Now()
	d := livestore.Deployment{
		ID:        id,
		Scenario:  "web-live-paris",
		ProjectID: reapProjectID,
		Image:     "nginx",
		Tag:       "1.27",
		WorkDir:   workDir,
		CreatedAt: now.Add(-time.Hour),
		ExpiresAt: now.Add(expiresIn),
	}
	require.NoError(t, store.Put(d))
	return d
}

func runLiveReap(t *testing.T, rt *CommandRuntime, out *strings.Builder, args ...string) error {
	t.Helper()
	cmd := &cobra.Command{Use: "reap"}
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	require.NoError(t, cmd.ParseFlags(args))
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetContext(context.Background())
	return runLiveReapCommand(cmd, nil, rt)
}

func TestTeardownDestroysSweepsAndReleases(t *testing.T) {
	sandboxCredsForTest(t)
	destroy, sweep := &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{}
	rt, store, workspace := liveTeardownRuntime(t, destroy, sweep)
	d := liveDeploymentWithState(t, store, workspace, "dep-1", -time.Minute)

	_, failures := tearDownDeployment(context.Background(), rt, store, d)

	assert.Empty(t, failures)
	assert.Equal(t, 1, destroy.calls, "the destroy harness ran")
	assert.Equal(t, 1, sweep.calls, "and the account was verified afterwards")

	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.Equal(t, livestore.StateReleased, got.State)
	assert.False(t, got.Reapable(time.Now()), "a released deployment is not reaped again")
}

// The registry says WHICH deployment; the state file says which project.
// A record pointed at a project its own state does not name must not be
// able to aim the destroyer at it.
func TestTeardownRefusesWhenTheRecordDisagreesWithTheState(t *testing.T) {
	sandboxCredsForTest(t)
	destroy, sweep := &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{}
	rt, store, workspace := liveTeardownRuntime(t, destroy, sweep)

	d := liveDeploymentWithState(t, store, workspace, "dep-tampered", -time.Minute)
	d.ProjectID = "99999999-9999-9999-9999-999999999999"

	_, failures := tearDownDeployment(context.Background(), rt, store, d)

	require.NotEmpty(t, failures)
	assert.Contains(t, failures[0].Detail, "refusing to destroy")
	assert.Zero(t, destroy.calls, "nothing was destroyed")
	assert.Zero(t, sweep.calls)
}

// Refusing to destroy the organization default is the guard that matters
// most, because that project holds real infrastructure.
func TestTeardownRefusesTheOrganizationDefaultProject(t *testing.T) {
	sandboxCredsForTest(t)
	destroy, sweep := &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{}
	rt, store, workspace := liveTeardownRuntime(t, destroy, sweep)

	workDir := filepath.Join(workspace, "live", "dep-org")
	require.NoError(t, os.MkdirAll(workDir, 0o755))
	writeReapLiveState(t, workDir, reapOrgID)

	now := time.Now()
	d := livestore.Deployment{
		ID: "dep-org", Scenario: "web-live-paris", ProjectID: reapOrgID,
		WorkDir: workDir, CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute),
	}
	require.NoError(t, store.Put(d))

	_, failures := tearDownDeployment(context.Background(), rt, store, d)

	require.NotEmpty(t, failures)
	assert.Contains(t, failures[0].Detail, "organization's default project")
	assert.Zero(t, destroy.calls)
}

// A record whose state has vanished must stay in the registry. Its
// resources may still be running, and releasing it would retire the only
// evidence that says so.
func TestTeardownKeepsTheRecordWhenTheStateIsMissing(t *testing.T) {
	sandboxCredsForTest(t)
	destroy, sweep := &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{}
	rt, store, workspace := liveTeardownRuntime(t, destroy, sweep)

	d := liveDeploymentWithState(t, store, workspace, "dep-lost", -time.Minute)
	require.NoError(t, os.Remove(filepath.Join(d.WorkDir, "terraform-live.tfstate")))

	_, failures := tearDownDeployment(context.Background(), rt, store, d)

	require.NotEmpty(t, failures)
	assert.Contains(t, failures[0].Detail, "may still be running")
	assert.Zero(t, destroy.calls)

	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.NotEqual(t, livestore.StateReleased, got.State, "the leak stays visible")
	assert.True(t, got.Reapable(time.Now()), "and it is retried next pass")
}

func TestTeardownWithoutAWorkDirIsUnreclaimable(t *testing.T) {
	sandboxCredsForTest(t)
	destroy, sweep := &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{}
	rt, store, _ := liveTeardownRuntime(t, destroy, sweep)

	now := time.Now()
	d := livestore.Deployment{
		ID: "dep-nowhere", Scenario: "web-live-paris", ProjectID: reapProjectID,
		CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute),
	}
	require.NoError(t, store.Put(d))

	_, failures := tearDownDeployment(context.Background(), rt, store, d)

	require.NotEmpty(t, failures)
	assert.Contains(t, failures[0].Detail, "records no work_dir")
	assert.Zero(t, destroy.calls)
}

// A failed destroy must not release the record: the resources are still
// there.
func TestTeardownDoesNotReleaseWhenDestroyFails(t *testing.T) {
	sandboxCredsForTest(t)
	destroy := &fakeSandboxDestroyHarness{err: harness.ErrSandboxDestroyFailed}
	sweep := &fakeOrphanSweep{}
	rt, store, workspace := liveTeardownRuntime(t, destroy, sweep)
	d := liveDeploymentWithState(t, store, workspace, "dep-stuck", -time.Minute)

	_, failures := tearDownDeployment(context.Background(), rt, store, d)

	require.NotEmpty(t, failures)
	assert.Zero(t, sweep.calls, "no point verifying an account the destroy did not clear")

	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.NotEqual(t, livestore.StateReleased, got.State)
}

func TestLiveReapWithNothingExpiredSucceedsAndSaysSo(t *testing.T) {
	sandboxCredsForTest(t)
	destroy, sweep := &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{}
	rt, store, workspace := liveTeardownRuntime(t, destroy, sweep)
	liveDeploymentWithState(t, store, workspace, "dep-fresh", time.Hour)

	var out strings.Builder
	require.NoError(t, runLiveReap(t, rt, &out))

	assert.Contains(t, out.String(), "Nothing has expired.")
	assert.Zero(t, destroy.calls)
}

func TestLiveReapTearsDownOnlyExpiredDeployments(t *testing.T) {
	sandboxCredsForTest(t)
	destroy, sweep := &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{}
	rt, store, workspace := liveTeardownRuntime(t, destroy, sweep)

	expired := liveDeploymentWithState(t, store, workspace, "dep-old", -time.Minute)
	fresh := liveDeploymentWithState(t, store, workspace, "dep-new", time.Hour)

	var out strings.Builder
	require.NoError(t, runLiveReap(t, rt, &out))

	assert.Equal(t, 1, destroy.calls, "exactly one deployment was expired")
	assert.Contains(t, out.String(), "dep-old", "and the reaper names what it removed")
	assert.NotContains(t, out.String(), "tearing down dep-new")

	gotExpired, err := store.Get(expired.ID)
	require.NoError(t, err)
	assert.Equal(t, livestore.StateReleased, gotExpired.State)

	gotFresh, err := store.Get(fresh.ID)
	require.NoError(t, err)
	assert.Equal(t, livestore.StateLive, gotFresh.State, "an unexpired deployment is untouched")
}

// --dry-run must be inert, for the same reason reap's is: pointing it at
// the estate to see what it would do has to be safe.
func TestLiveReapDryRunDestroysNothing(t *testing.T) {
	sandboxCredsForTest(t)
	destroy, sweep := &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{}
	rt, store, workspace := liveTeardownRuntime(t, destroy, sweep)
	d := liveDeploymentWithState(t, store, workspace, "dep-old", -time.Minute)

	var out strings.Builder
	require.NoError(t, runLiveReap(t, rt, &out, "--dry-run"))

	assert.Zero(t, destroy.calls)
	assert.Contains(t, out.String(), "would tear down dep-old")
	assert.Contains(t, out.String(), "nothing destroyed")

	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.Equal(t, livestore.StateLive, got.State)
}

// An unreadable record may describe running infrastructure the reaper
// cannot reach. Reporting success while one exists would tell the
// operator the estate is clean when it might not be.
func TestLiveReapFailsWhenARecordCannotBeRead(t *testing.T) {
	sandboxCredsForTest(t)
	destroy, sweep := &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{}
	rt, store, workspace := liveTeardownRuntime(t, destroy, sweep)
	liveDeploymentWithState(t, store, workspace, "dep-old", -time.Minute)
	require.NoError(t, os.WriteFile(filepath.Join(store.Root, "mystery.json"), []byte("{oops"), 0o644))

	var out strings.Builder
	err := runLiveReap(t, rt, &out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "provably clean")
	assert.Equal(t, 1, destroy.calls, "the readable expired deployment is still torn down")
}

func TestLiveReapSkipsAlreadyReleasedDeployments(t *testing.T) {
	sandboxCredsForTest(t)
	destroy, sweep := &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{}
	rt, store, workspace := liveTeardownRuntime(t, destroy, sweep)
	d := liveDeploymentWithState(t, store, workspace, "dep-done", -time.Minute)
	require.NoError(t, store.MarkReleased(d.ID))

	var out strings.Builder
	require.NoError(t, runLiveReap(t, rt, &out))

	assert.Zero(t, destroy.calls)
	assert.Contains(t, out.String(), "Nothing has expired.")
}

// A dry run that exits 0 with an unaccounted record is the failure the
// fail-closed rule exists to prevent: a CI wrapper that dry-runs first
// sees green while something that may be running is unaccounted for.
func TestLiveReapDryRunStillFailsOnUnreadableRecords(t *testing.T) {
	sandboxCredsForTest(t)
	destroy, sweep := &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{}
	rt, store, workspace := liveTeardownRuntime(t, destroy, sweep)
	liveDeploymentWithState(t, store, workspace, "dep-old", -time.Minute)
	require.NoError(t, os.WriteFile(filepath.Join(store.Root, "mystery.json"), []byte("{oops"), 0o644))

	var out strings.Builder
	err := runLiveReap(t, rt, &out, "--dry-run")

	require.Error(t, err, "dry-run must not report success while a record is unaccounted for")
	assert.Zero(t, destroy.calls, "and it still destroys nothing")
}

// Releasing after a destroy could fail (read-only disk, say). The retry
// the message used to promise was impossible: destroy has emptied the
// state, so the next pass read no project and reported the deployment as
// a leak forever -- about a project that no longer existed.
func TestTeardownOfAnAlreadyDestroyedDeploymentReleasesInsteadOfCryingLeak(t *testing.T) {
	sandboxCredsForTest(t)
	destroy, sweep := &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{}
	rt, store, workspace := liveTeardownRuntime(t, destroy, sweep)
	d := liveDeploymentWithState(t, store, workspace, "dep-twice", -time.Minute)

	// Destroy leaves an empty state behind; simulate that.
	require.NoError(t, os.WriteFile(
		filepath.Join(d.WorkDir, "terraform-live.tfstate"), []byte(`{"resources":[]}`), 0o600))

	stages, failures := tearDownDeployment(context.Background(), rt, store, d)

	assert.Empty(t, failures, "an empty state means destroy already ran, not that something leaked")
	assert.Zero(t, destroy.calls, "and there is nothing left to destroy")
	require.NotEmpty(t, stages)
	assert.Contains(t, stages[len(stages)-1].Detail, "already destroyed")

	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.Equal(t, livestore.StateReleased, got.State, "so it stops being reaped every pass")
}

// The regression this closes: the empty-state shortcut released the
// record with a PASS without re-running the sweep, so a teardown whose
// orphan sweep had FAILED was laundered green on the next pass and the
// orphans became invisible.
func TestTeardownOfAnAlreadyDestroyedDeploymentReVerifiesTheAccount(t *testing.T) {
	sandboxCredsForTest(t)
	destroy := &fakeSandboxDestroyHarness{}
	sweep := &fakeOrphanSweep{err: harness.ErrOrphanSweepFailed}
	rt, store, workspace := liveTeardownRuntime(t, destroy, sweep)
	d := liveDeploymentWithState(t, store, workspace, "dep-laundered", -time.Minute)
	require.NoError(t, os.WriteFile(
		filepath.Join(d.WorkDir, "terraform-live.tfstate"), []byte(`{"resources":[]}`), 0o600))

	_, failures := tearDownDeployment(context.Background(), rt, store, d)

	assert.Equal(t, 1, sweep.calls, "the account is re-verified, not assumed clean")
	require.NotEmpty(t, failures, "a failing sweep must not be laundered into a release")

	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.NotEqual(t, livestore.StateReleased, got.State, "the record stays, so the orphans stay visible")
}

// An undecodable record is reapable by design but not reclaimable, so
// without an escape it fails every pass forever. `live forget` is that
// escape: it releases without destroying and says what it gives up.
func TestLiveForgetReleasesARecordTheToolingCannotAct(t *testing.T) {
	sandboxCredsForTest(t)
	rt, store, _ := liveTeardownRuntime(t, &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{})
	require.NoError(t, os.MkdirAll(store.Root, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(store.Root, "mystery.json"), []byte("{oops"), 0o644))

	expired, _, err := store.Reapable(time.Now())
	require.NoError(t, err)
	require.Len(t, expired, 1, "precondition: it is reapable but unreclaimable")

	var out bytes.Buffer
	cmd := &cobra.Command{Use: "forget"}
	cmd.Flags().String("output", string(OutputModeHuman), "")
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(context.Background())
	require.NoError(t, runLiveForgetCommand(cmd, []string{"mystery"}, rt))

	assert.Contains(t, out.String(), "WITHOUT destroying anything")
	after, _, err := store.Reapable(time.Now())
	require.NoError(t, err)
	assert.Empty(t, after, "and it stops failing every pass")
	assert.FileExists(t, filepath.Join(store.Root, "mystery.json.unreadable"),
		"the unparseable bytes are preserved, not overwritten")
}

// Both reviewers found this independently. A sweep that failed found
// strays OUTSIDE the run project, computed from state that destroy has
// since emptied — so a later pass cannot recompute them, and releasing on
// an empty state would launder that failure into a clean result.
func TestTeardownRefusesToReleaseWhenAnEarlierSweepFailed(t *testing.T) {
	sandboxCredsForTest(t)
	destroy, sweep := &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{}
	rt, store, workspace := liveTeardownRuntime(t, destroy, sweep)
	d := liveDeploymentWithState(t, store, workspace, "dep-strays", -time.Minute)
	d.SweepVerificationFailed = true
	require.NoError(t, store.Put(d))
	require.NoError(t, os.WriteFile(
		filepath.Join(d.WorkDir, "terraform-live.tfstate"), []byte(`{"resources":[]}`), 0o600))

	_, failures := tearDownDeployment(context.Background(), rt, store, d)

	require.NotEmpty(t, failures)
	assert.Contains(t, failures[0].Detail, "orphan sweep FAIL")
	assert.Zero(t, sweep.calls, "a sweep that cannot see the strays proves nothing")

	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.NotEqual(t, livestore.StateReleased, got.State, "the strays stay tracked")
}

// A failing sweep must set the sticky flag, or the next pass takes the
// empty-state shortcut and releases.
func TestTeardownRecordsThatASweepFailed(t *testing.T) {
	sandboxCredsForTest(t)
	destroy := &fakeSandboxDestroyHarness{}
	sweep := &fakeOrphanSweep{err: harness.ErrOrphanSweepFailed}
	rt, store, workspace := liveTeardownRuntime(t, destroy, sweep)
	d := liveDeploymentWithState(t, store, workspace, "dep-sticky", -time.Minute)

	_, failures := tearDownDeployment(context.Background(), rt, store, d)

	require.NotEmpty(t, failures)
	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.True(t, got.SweepVerificationFailed, "sticky, so no later pass can release on an empty state")
}

// live forget abandons tracking. Pointed at a healthy deployment it would
// be a one-command permanent leak, so it refuses what teardown can handle.
func TestLiveForgetRefusesAReclaimableDeployment(t *testing.T) {
	sandboxCredsForTest(t)
	rt, store, workspace := liveTeardownRuntime(t, &fakeSandboxDestroyHarness{}, &fakeOrphanSweep{})
	d := liveDeploymentWithState(t, store, workspace, "dep-healthy", time.Hour)

	var out bytes.Buffer
	cmd := &cobra.Command{Use: "forget"}
	cmd.Flags().String("output", string(OutputModeHuman), "")
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(context.Background())

	err := runLiveForgetCommand(cmd, []string{d.ID}, rt)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "live teardown")

	got, getErr := store.Get(d.ID)
	require.NoError(t, getErr)
	assert.Equal(t, livestore.StateLive, got.State, "still tracked, still reapable")
	assert.True(t, got.Reapable(time.Now().Add(2*time.Hour)))
}
