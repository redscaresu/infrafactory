package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sequencedDestroy returns a different outcome per call so a retry is
// distinguishable from a single attempt.
type sequencedDestroy struct {
	errs  []error
	calls int
}

func (s *sequencedDestroy) Run(context.Context, string, map[string]string) (*harness.SandboxDestroyResult, error) {
	i := s.calls
	s.calls++
	if i < len(s.errs) && s.errs[i] != nil {
		return nil, s.errs[i]
	}
	return &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}}, nil
}

type fakePurge struct {
	removed   []string
	err       error
	calls     int
	gotProj   string
	gotSecret string
}

func (f *fakePurge) Run(_ context.Context, projectID, secretKey string) ([]string, error) {
	f.calls++
	f.gotProj = projectID
	f.gotSecret = secretKey
	return f.removed, f.err
}

func retryRuntime(t *testing.T, destroy *sequencedDestroy, purge *fakePurge) *CommandRuntime {
	t.Helper()
	return &CommandRuntime{Deps: RuntimeDependencies{
		SandboxDestroy: destroy, AutoCreated: purge, RunProject: &fakeRunProject{},
	}}
}

// purgeWorkDir is a workdir carrying the run-project marker the guard
// now reads instead of the state file (ADR-0025).
func purgeWorkDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := harness.WriteRunProjectMarker(dir, harness.RunProject{
		ID: purgeProjectID, Name: harness.RunProjectNamePrefix + "purge",
	}); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	return dir
}

var destroyEnv = map[string]string{"SCW_SECRET_KEY": "secret"}

// The case this whole path exists for: tofu cannot delete a project that
// still holds Scaleway's auto-created default security group.
func TestDestroyRetriesAfterPurgingAutoCreatedResources(t *testing.T) {
	destroy := &sequencedDestroy{errs: []error{errors.New("precondition failed: resource is still in use")}}
	purge := &fakePurge{removed: []string{"security_group 142eef7b (Default security group) in fr-par-1"}}

	result, purged, err := destroySandbox(context.Background(), retryRuntime(t, destroy, purge), purgeWorkDir(t), destroyEnv, purgeProjectID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, destroy.calls, "destroy must be retried once the blocker is gone")
	// A teardown that silently deleted things nobody asked it to delete
	// would be worse than the leak it fixes.
	require.Len(t, purged, 1, "what the purge removed must reach the stage summary")
	assert.Contains(t, autoCreatedPurgeStage(purged).Detail, "Default security group")
	assert.Equal(t, 1, purge.calls)
	assert.Equal(t, purgeProjectID, purge.gotProj, "the purge must be scoped to the run's project")
	assert.Equal(t, "secret", purge.gotSecret)
}

// If nothing was auto-created, the destroy failed on its own merits.
// Retrying would only produce a second identical failure and bury the
// first one's diagnostics.
func TestDestroyDoesNotRetryWhenNothingWasPurged(t *testing.T) {
	wantErr := errors.New("genuine destroy bug")
	destroy := &sequencedDestroy{errs: []error{wantErr, nil}}
	purge := &fakePurge{}

	_, _, err := destroySandbox(context.Background(), retryRuntime(t, destroy, purge), purgeWorkDir(t), destroyEnv, purgeProjectID)

	require.ErrorIs(t, err, wantErr)
	assert.Equal(t, 1, destroy.calls)
}

func TestDestroySkipsPurgeWhenItSucceeds(t *testing.T) {
	destroy := &sequencedDestroy{}
	purge := &fakePurge{}

	_, _, err := destroySandbox(context.Background(), retryRuntime(t, destroy, purge), purgeWorkDir(t), destroyEnv, purgeProjectID)

	require.NoError(t, err)
	assert.Equal(t, 1, destroy.calls)
	assert.Zero(t, purge.calls, "a successful destroy must not touch the API")
}

// Without a project id AND without a marker there is no blast radius to
// scope a purge to, so deleting anything would be guessing.
//
// The empty-id case alone no longer stops it, and that boundary moved
// deliberately on 2026-09-10. An empty id is not a caller saying "do
// nothing" -- every caller derives it from CaptureSweepTarget, so it
// means that capture failed, and treating it as a refusal silently
// disabled the purge and the poweroff together on a run that needed
// both. The marker is the same provenance assertRunProjectDeletable
// reads, and that check still runs, so recovery is scoped by the guard
// rather than by an accident of capture.
func TestDestroyWithoutProjectIDOrMarkerDoesNotPurge(t *testing.T) {
	wantErr := errors.New("destroy failed")
	destroy := &sequencedDestroy{errs: []error{wantErr, nil}}
	purge := &fakePurge{}

	// t.TempDir(), not purgeWorkDir: no marker, so nothing to recover.
	_, _, err := destroySandbox(context.Background(), retryRuntime(t, destroy, purge), t.TempDir(), destroyEnv, "")

	require.ErrorIs(t, err, wantErr)
	assert.Zero(t, purge.calls)
}

// ...and with a marker, an empty caller id recovers rather than
// disabling remediation. This is the outage: no purge stage and no
// poweroff stage on a run whose destroy failed for a reason both would
// have cleared.
func TestDestroyRecoversTheProjectFromTheMarkerWhenTheCallerHasNone(t *testing.T) {
	destroy := &sequencedDestroy{errs: []error{errors.New("precondition failed: resource is still in use")}}
	purge := &fakePurge{removed: []string{"security_group 142eef7b (Default security group) in fr-par-1"}}

	_, purged, err := destroySandbox(context.Background(), retryRuntime(t, destroy, purge), purgeWorkDir(t), destroyEnv, "")

	require.NoError(t, err, "the retry should succeed once the blocker is purged")
	assert.Equal(t, 1, purge.calls, "an empty caller id must not disable the purge when the marker names a project")
	assert.NotEmpty(t, purged)
}

// A destroy that fails twice is a real failure, and the second error is
// the one that describes the account's actual state.
func TestDestroyReportsSecondFailure(t *testing.T) {
	second := errors.New("still stuck")
	destroy := &sequencedDestroy{errs: []error{errors.New("first"), second}}
	purge := &fakePurge{removed: []string{"security_group 142eef7b"}}

	_, _, err := destroySandbox(context.Background(), retryRuntime(t, destroy, purge), purgeWorkDir(t), destroyEnv, purgeProjectID)

	require.ErrorIs(t, err, second)
	assert.Equal(t, 2, destroy.calls)
}

const purgeProjectID = "6c4390c9-664e-4289-a34f-cdc865653fc7"

// Every real-cloud teardown must go through destroySandbox.
//
// The purge/retry was added to the ordinary destroy paths first and the
// interrupt-cleanup path was missed -- which is the path where a leak
// matters most, because nothing else is coming to clean up after it.
// Bypassing the wrapper reintroduces exactly that bug, so make it a
// failing test rather than a convention.
func TestAllDestroyCallsGoThroughTheRetryWrapper(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	var offenders []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		// The wrapper is the one place allowed to call the harness.
		if name == "destroy_retry.go" {
			continue
		}
		src, err := os.ReadFile(name)
		require.NoError(t, err)
		for i, line := range strings.Split(string(src), "\n") {
			if strings.Contains(line, "SandboxDestroy.Run(") {
				offenders = append(offenders, fmt.Sprintf("%s:%d: %s", name, i+1, strings.TrimSpace(line)))
			}
		}
	}

	assert.Empty(t, offenders,
		"call destroySandbox instead: a raw SandboxDestroy.Run skips the auto-created-resource purge, "+
			"so the run leaks its project whenever the API put something in it that Terraform does not own")
}

// The purge deletes real resources over HTTP with Terraform nowhere in
// the loop, so a project id that fails AssertProjectDeletable must not
// reach it. reap asserted this; run, test and the interrupt path did
// not, so the guard lives in the wrapper.
func TestDestroyRefusesToPurgeTheOrganizationDefaultProject(t *testing.T) {
	wantErr := errors.New("destroy failed")
	destroy := &sequencedDestroy{errs: []error{wantErr, nil}}
	purge := &fakePurge{removed: []string{"security_group would-have-deleted"}}
	env := map[string]string{
		"SCW_SECRET_KEY": "secret",
		// The state names the organization's own default project.
		"SCW_DEFAULT_ORGANIZATION_ID": purgeProjectID,
	}

	_, purged, err := destroySandbox(
		context.Background(), retryRuntime(t, destroy, purge), "/work", env, purgeProjectID)

	require.ErrorIs(t, err, wantErr)
	assert.Zero(t, purge.calls, "the organization default project must never be purged")
	assert.Empty(t, purged)
	assert.Equal(t, 1, destroy.calls, "and no retry should follow a refused purge")
}

// The reason has to reach a channel somebody reads.
//
// The previous attempt put skip reasons in a StageSummary, and `run`
// does not persist sandbox stages -- so a real run produced the string
// "poweroff" ZERO times anywhere on disk, and answering "did it run?"
// took a live API probe. A guard that explains itself into a channel
// with no reader is worse than a silent one, because it looks fixed.
func TestThePurgeDecisionReachesTheLog(t *testing.T) {
	var sink bytes.Buffer
	rt := retryRuntime(t, &sequencedDestroy{errs: []error{errors.New("boom")}}, &fakePurge{})
	rt.Logger = NewAppLogger(&sink)

	_, _, _ = destroySandbox(context.Background(), rt, purgeWorkDir(t), destroyEnv, purgeProjectID)

	assert.Contains(t, sink.String(), "layer3_auto_created_purge",
		"the purge decision must be on the record: `run` does not persist sandbox stages, "+
			"so a decision that exists only as a stage is invisible on disk")
}
