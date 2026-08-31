package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

type fakeRunProject struct {
	created      harness.RunProject
	createErr    error
	deleteErr    error
	deleteErrs   []error
	describeErr  error
	describeGone bool
	creates      int
	deletes      int
	lastOrg      string
	lastScen     string
	deletedID    string
}

func (f *fakeRunProject) Create(_ context.Context, _, organizationID, scenario, _ string) (harness.RunProject, error) {
	f.creates++
	f.lastOrg, f.lastScen = organizationID, scenario
	if f.createErr != nil {
		return harness.RunProject{}, f.createErr
	}
	return f.created, nil
}

func (f *fakeRunProject) Describe(_ context.Context, _, projectID string) (harness.ProjectProvenance, error) {
	if f.describeErr != nil {
		return harness.ProjectProvenance{}, f.describeErr
	}
	if f.describeGone {
		return harness.ProjectProvenance{Exists: false}, nil
	}
	return harness.ProjectProvenance{
		Exists: true, Name: harness.RunProjectNamePrefix + "x", Description: harness.RunProjectDescription,
	}, nil
}

func (f *fakeRunProject) Delete(_ context.Context, _, projectID string) error {
	i := f.deletes
	f.deletes++
	f.deletedID = projectID
	if i < len(f.deleteErrs) {
		return f.deleteErrs[i]
	}
	return f.deleteErr
}

// releaseFixture gives releaseRunProject what the guard now needs: a
// workdir carrying the marker, and sandbox credentials.
func releaseFixture(t *testing.T) (string, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, harness.WriteRunProjectMarker(dir,
		harness.RunProject{ID: "proj-1", Name: harness.RunProjectNamePrefix + "release"}))
	return dir, map[string]string{"SCW_SECRET_KEY": "secret"}
}

func TestEnsureRunProjectCreatesAndReportsIt(t *testing.T) {
	sandboxCredsForTest(t)
	fake := &fakeRunProject{created: harness.RunProject{ID: "proj-1", Name: "if-run-web-live-paris-x"}}
	rt := &CommandRuntime{Deps: RuntimeDependencies{RunProject: fake}}

	id, stages, failures := ensureRunProject(context.Background(), rt, "web-live-paris", t.TempDir())

	assert.Equal(t, "proj-1", id)
	assert.Empty(t, failures)
	require.Len(t, stages, 1)
	assert.Equal(t, StageStatusPass, stages[0].Status)
	assert.Contains(t, stages[0].Detail, "proj-1", "the operator can see which project the run owns")
	assert.Equal(t, "web-live-paris", fake.lastScen)
	assert.NotEmpty(t, fake.lastOrg, "a project must be created in an organization")
}

// Falling back to the shared project would put this run's strays next to
// every other run's, which is what the run-owned project exists to stop.
func TestEnsureRunProjectFailsRatherThanFallingBack(t *testing.T) {
	sandboxCredsForTest(t)
	fake := &fakeRunProject{createErr: errors.New("http 403: insufficient permissions")}
	rt := &CommandRuntime{Deps: RuntimeDependencies{RunProject: fake}}

	id, stages, failures := ensureRunProject(context.Background(), rt, "web-live-paris", t.TempDir())

	assert.Empty(t, id)
	require.Len(t, failures, 1)
	assert.Contains(t, failures[0].Detail, "insufficient permissions", "the API message survives")
	require.Len(t, stages, 1)
	assert.Equal(t, StageStatusFail, stages[0].Status)
}

func TestReleaseRunProjectDeletesAndReports(t *testing.T) {
	fake := &fakeRunProject{}
	rt := &CommandRuntime{Deps: RuntimeDependencies{RunProject: fake}}

	workDir, env := releaseFixture(t)

	stages, failures := releaseRunProject(context.Background(), rt, workDir, "proj-1", env)

	assert.Empty(t, failures)
	require.Len(t, stages, 1)
	assert.Equal(t, StageStatusPass, stages[0].Status)
	assert.Equal(t, "proj-1", fake.deletedID)
}

func TestReleaseRunProjectIsInertWithoutAProject(t *testing.T) {
	fake := &fakeRunProject{}
	rt := &CommandRuntime{Deps: RuntimeDependencies{RunProject: fake}}

	workDir, env := releaseFixture(t)

	stages, failures := releaseRunProject(context.Background(), rt, workDir, "", env)

	assert.Empty(t, stages)
	assert.Empty(t, failures)
	assert.Zero(t, fake.deletes)
}

// An empty project is free, but it does not clean itself up — a failed
// delete is reported rather than swallowed.
func TestReleaseRunProjectReportsAFailedDelete(t *testing.T) {
	fake := &fakeRunProject{deleteErr: errors.New("http 412: resource_still_in_use")}
	rt := &CommandRuntime{Deps: RuntimeDependencies{RunProject: fake}}

	workDir, env := releaseFixture(t)

	stages, failures := releaseRunProject(context.Background(), rt, workDir, "proj-1", env)

	require.Len(t, failures, 1)
	assert.Contains(t, failures[0].Detail, "resource_still_in_use")
	require.Len(t, stages, 1)
	assert.Equal(t, StageStatusFail, stages[0].Status)
}

func TestRunProjectStampIsSortableAndSafe(t *testing.T) {
	at := time.Date(2026, 8, 31, 1, 2, 3, 0, time.UTC)
	assert.Equal(t, "20260831t010203z", runProjectStamp(at))
}

// The leak codex pass 22 found: an apply that fails at preflight, init or
// plan writes no state, so the destroy branch never runs — and the
// project it created would be left behind on exactly the runs most likely
// to be repeated.
func TestReleaseRunProjectRunsWhenNoStateWasEverWritten(t *testing.T) {
	// liveStateMayHoldResources is false for a directory with no state
	// file, which is the condition the cleanup keys off.
	assert.False(t, liveStateMayHoldResources(t.TempDir()),
		"no state means nothing was created, so the project is safe to delete")
}

// Apply and destroy must use the SAME provider default project. They were
// asymmetric once: the apply used the run-owned project while destroy
// rebuilt its environment from the shared fallback, so resources with no
// project_id of their own would be looked for in the wrong project --
// failing teardown or leaving them behind.
//
// A source audit rather than a unit test because the asymmetry lives in
// which helper each call site picks, which is exactly what drifts. Same
// idiom as cloud_prefix_lockstep_test.go.
func TestPipelineNeverBuildsSandboxEnvWithoutTheRunProject(t *testing.T) {
	source, err := os.ReadFile("test_command.go")
	require.NoError(t, err)

	for i, line := range strings.Split(string(source), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "func sandboxCommandEnv(") {
			continue
		}
		assert.NotContains(t, trimmed, "sandboxCommandEnv(runtime)",
			"test_command.go:%d builds the Layer 3 environment without a run project; "+
				"use sandboxCommandEnvForProject so apply and destroy agree", i+1)
	}
}

// A cancelled run is exactly when cleanup matters most. Delete must not
// inherit the cancelled context, or the project survives every Ctrl-C.
func TestReleaseRunProjectSurvivesACancelledRunContext(t *testing.T) {
	fake := &fakeRunProject{}
	rt := &CommandRuntime{Deps: RuntimeDependencies{RunProject: fake}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	workDir, env := releaseFixture(t)

	stages, failures := releaseRunProject(ctx, rt, workDir, "proj-1", env)

	assert.Empty(t, failures)
	assert.Equal(t, 1, fake.deletes, "the delete still reaches the API after cancellation")
	require.Len(t, stages, 1)
	assert.Equal(t, StageStatusPass, stages[0].Status)
}

// A configuration that is going to be rejected must not leave a real
// project behind. ensureRunProject is only reached once the sealed
// environment validates, so a missing credential produces no API call at
// all rather than residue cleaned up on a best-effort basis.
func TestNoProjectIsCreatedWhenTheSandboxEnvIsInvalid(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SCW_SECRET_KEY", "secret")
	t.Setenv("SCW_DEFAULT_ORGANIZATION_ID", "22222222-2222-2222-2222-222222222222")
	t.Setenv("SCW_ACCESS_KEY", "") // the missing piece

	rt := &CommandRuntime{}
	_, err := sandboxCommandEnvForProject(rt, "")

	require.Error(t, err, "the environment is rejected before any project could be created")
	assert.Contains(t, err.Error(), "SCW_ACCESS_KEY")
}

// Every destroy path goes through one guard, so none can carry a weaker
// check. An unreachable API refuses rather than proceeding: refusing
// costs a retry, proceeding wrongly costs a project nobody meant to
// destroy.
func TestAssertRunProjectDeletableRefusesWhenTheAPICannotBeReached(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, harness.WriteRunProjectMarker(dir,
		harness.RunProject{ID: "proj-1", Name: harness.RunProjectNamePrefix + "x"}))

	rt := &CommandRuntime{Deps: RuntimeDependencies{
		RunProject: &fakeRunProject{describeErr: errors.New("dial tcp: connection refused")},
	}}

	err := assertRunProjectDeletable(context.Background(), rt, dir, "proj-1",
		map[string]string{"SCW_SECRET_KEY": "s", "SCW_DEFAULT_ORGANIZATION_ID": "org"})

	require.Error(t, err)
	assert.ErrorIs(t, err, harness.ErrProtectedProject)
	assert.Contains(t, err.Error(), "could not verify")
}

// No marker means nothing says this run created the project.
func TestAssertRunProjectDeletableRefusesWithoutAMarkerFile(t *testing.T) {
	rt := &CommandRuntime{Deps: RuntimeDependencies{RunProject: &fakeRunProject{}}}

	err := assertRunProjectDeletable(context.Background(), rt, t.TempDir(), "proj-1",
		map[string]string{"SCW_SECRET_KEY": "s"})

	require.Error(t, err)
	assert.ErrorIs(t, err, harness.ErrProtectedProject)
}

// A project the API says is already gone is the outcome asked for.
func TestAssertRunProjectDeletableAllowsAnAlreadyGoneProject(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, harness.WriteRunProjectMarker(dir,
		harness.RunProject{ID: "proj-1", Name: harness.RunProjectNamePrefix + "x"}))

	rt := &CommandRuntime{Deps: RuntimeDependencies{RunProject: &fakeRunProject{describeGone: true}}}

	assert.NoError(t, assertRunProjectDeletable(context.Background(), rt, dir, "proj-1",
		map[string]string{"SCW_SECRET_KEY": "s"}))
}

// If the marker cannot be written, the project exists but nothing can
// ever authorise deleting it — so it is deleted immediately, while
// nothing has been applied into it and the id is still in hand.
func TestEnsureRunProjectDeletesTheProjectIfTheMarkerCannotBeWritten(t *testing.T) {
	sandboxCredsForTest(t)
	fake := &fakeRunProject{created: harness.RunProject{ID: "proj-1", Name: "if-run-x"}}
	rt := &CommandRuntime{Deps: RuntimeDependencies{RunProject: fake}}

	// A file where the workdir should be makes the marker write fail.
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o600))

	id, _, failures := ensureRunProject(context.Background(), rt, "web-live-paris", blocked)

	assert.Empty(t, id)
	require.Len(t, failures, 1)
	assert.Equal(t, 1, fake.deletes, "the project is reclaimed while it still can be")
	assert.Contains(t, failures[0].Detail, "deleted again, so nothing was left behind")
}

// D6, arrived at from the other direction. Before the cutover the 412
// landed on `tofu destroy` and destroySandbox purged and retried. Now
// destroy succeeds -- the project is not its resource -- and the 412
// lands here instead. Without this purge every run declaring compute
// leaks a project again, and quietly: nothing billable survives, so cost
// checks keep reporting clean.
func TestReleaseRunProjectPurgesTheAutoCreatedBlockerAndRetries(t *testing.T) {
	fake := &fakeRunProject{deleteErrs: []error{errors.New("http 412: resource_still_in_use")}}
	purge := &fakePurge{removed: []string{"security_group 142eef7b (Default security group) in fr-par-1"}}
	rt := &CommandRuntime{Deps: RuntimeDependencies{RunProject: fake, AutoCreated: purge}}
	workDir, env := releaseFixture(t)

	stages, failures := releaseRunProject(context.Background(), rt, workDir, "proj-1", env)

	assert.Empty(t, failures)
	assert.Equal(t, 2, fake.deletes, "the delete is retried once the blocker is gone")
	assert.Equal(t, "proj-1", purge.gotProj, "the purge is scoped to the run's project")
	require.Len(t, stages, 2)
	// A teardown that silently deleted things nobody asked it to delete
	// would be worse than the leak it fixes.
	assert.Contains(t, stages[0].Detail, "Default security group")
	assert.Equal(t, StageStatusPass, stages[1].Status)
}

// A purge that removes nothing means the delete failed on its own
// merits, so the original error is what the operator needs to see.
func TestReleaseRunProjectReportsTheOriginalErrorWhenNothingWasAutoCreated(t *testing.T) {
	fake := &fakeRunProject{deleteErr: errors.New("http 500: boom")}
	purge := &fakePurge{}
	rt := &CommandRuntime{Deps: RuntimeDependencies{RunProject: fake, AutoCreated: purge}}
	workDir, env := releaseFixture(t)

	_, failures := releaseRunProject(context.Background(), rt, workDir, "proj-1", env)

	require.Len(t, failures, 1)
	assert.Contains(t, failures[0].Detail, "http 500: boom")
	assert.Equal(t, 1, fake.deletes, "no blocker was removed, so a retry would only repeat the failure")
}

// The guard lives inside releaseRunProject because four paths reach it
// and a check that can be forgotten will be.
func TestReleaseRunProjectRefusesAProjectNoMarkerNames(t *testing.T) {
	fake := &fakeRunProject{}
	rt := &CommandRuntime{Deps: RuntimeDependencies{RunProject: fake}}
	_, env := releaseFixture(t)

	_, failures := releaseRunProject(context.Background(), rt, t.TempDir(), "proj-1", env)

	require.Len(t, failures, 1)
	assert.Contains(t, failures[0].Detail, "refusing to delete it")
	assert.Zero(t, fake.deletes, "nothing proves this project is the run's")
}
