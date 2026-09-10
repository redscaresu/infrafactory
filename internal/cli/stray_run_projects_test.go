package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type listingRunProject struct {
	fakeRunProject
	projects []harness.ListedProject
	err      error
}

func (l *listingRunProject) List(ctx context.Context, secretKey, orgID string) ([]harness.ListedProject, error) {
	return l.projects, l.err
}

func strayRuntime(t *testing.T, rp RunProjectManager) *CommandRuntime {
	t.Helper()
	t.Setenv("SCW_SECRET_KEY", "secret")
	t.Setenv("SCW_DEFAULT_ORGANIZATION_ID", "org")
	rt := &CommandRuntime{Deps: RuntimeDependencies{RunProject: rp}}
	rt.livestoreRoot = t.TempDir()
	return rt
}

// The leak this exists for: a run project nothing explains. Eight of
// these accumulated over two days, four still running an instance, and
// they surfaced only because a human listed projects on a hunch.
func TestStrayRunProjectsFailsOnAnUnexplainedProject(t *testing.T) {
	rt := strayRuntime(t, &listingRunProject{projects: []harness.ListedProject{
		{ID: "p-1", Name: harness.RunProjectNamePrefix + "web-live-paris-20260909t110101z", Description: harness.RunProjectDescription},
	}})

	stages, failures := reportStrayRunProjects(context.Background(), rt)

	require.Len(t, failures, 1)
	assert.Contains(t, failures[0].Detail, "p-1")
	assert.Contains(t, failures[0].Detail, "Nothing was destroyed",
		"a reconciler that deletes what it cannot explain becomes the incident")
	require.Len(t, stages, 1)
	assert.Equal(t, StageStatusFail, stages[0].Status)
}

// A clean estate passes, and says how many it checked -- "found nothing"
// and "looked at nothing" must not read alike.
func TestStrayRunProjectsPassesWhenNothingIsUnexplained(t *testing.T) {
	rt := strayRuntime(t, &listingRunProject{projects: nil})

	stages, failures := reportStrayRunProjects(context.Background(), rt)

	assert.Empty(t, failures)
	require.Len(t, stages, 1)
	assert.Equal(t, StageStatusPass, stages[0].Status)
	assert.Contains(t, stages[0].Detail, "accounted for")
}

// "Could not check" and "nothing leaked" must never look alike -- the
// S139 lesson, and the reason this reports a failure rather than a pass
// when the API cannot be read.
func TestStrayRunProjectsFailsWhenItCannotList(t *testing.T) {
	rt := strayRuntime(t, &listingRunProject{err: errors.New("401 unauthorized")})

	stages, failures := reportStrayRunProjects(context.Background(), rt)

	assert.Empty(t, stages)
	require.Len(t, failures, 1)
	assert.Contains(t, failures[0].Detail, "cannot be ruled out")
}

// A project without infrafactory's stamp is somebody else's. The
// ownership test is shared with the teardown guard so "infrafactory
// created this" has one definition.
func TestStrayRunProjectsIgnoresUnstampedProjects(t *testing.T) {
	rt := strayRuntime(t, &listingRunProject{projects: []harness.ListedProject{
		{ID: "p-prod", Name: "openclaw-prod", Description: "not ours"},
	}})

	_, failures := reportStrayRunProjects(context.Background(), rt)

	assert.Empty(t, failures, "an unstamped project must never be reported as infrafactory's litter")
}

// A Layer 2 run creates no project, so checking would be noise.
func TestStrayCheckSkippedWhenLayer3IsOff(t *testing.T) {
	assert.False(t, sandboxRunEnabled(&CommandRuntime{}))
}

// The pass line is the evidence an operator reads for "nothing leaked",
// so it must count what was CHECKED, not every project in the
// organization.
func TestStrayRunProjectsCountsOnlyStampedProjects(t *testing.T) {
	rt := strayRuntime(t, &listingRunProject{projects: []harness.ListedProject{
		{ID: "p-prod", Name: "openclaw-prod", Description: "not ours"},
		{ID: "p-default", Name: "default", Description: ""},
	}})

	stages, failures := reportStrayRunProjects(context.Background(), rt)

	require.Empty(t, failures)
	require.Len(t, stages, 1)
	assert.Contains(t, stages[0].Detail, "0 project(s) carry infrafactory's stamp",
		"none of these are ours, and the summary must not imply they were checked as ours")
}

// A stray must fail a run that otherwise reached its target. The first
// version of the stray check appended its failures after the status had
// been decided, so a green run could carry an unexplained project --
// billing infrastructure with no TTL, reported as success.
func TestRunReportsFailureWhenAStrayProjectSurvives(t *testing.T) {
	assert.Equal(t, CommandStatusSuccess, runCommandStatus("target_reached", false, false))
	assert.Equal(t, CommandStatusFailed, runCommandStatus("target_reached", false, true),
		"a stray run project is billing infrastructure nothing will reap")
	assert.Equal(t, CommandStatusFailed, runCommandStatus("target_reached", true, false))
	assert.Equal(t, CommandStatusFailed, runCommandStatus("stuck", false, false))
}

// An unreadable live record cannot be told from a clean estate unless it
// is reported. With no unrecorded project the pass path would otherwise
// return, discarding the failure and calling an estate clean that was
// never fully read.
func TestStrayRunProjectsFailsOnAnUnreadableLiveRecord(t *testing.T) {
	rt := strayRuntime(t, &listingRunProject{projects: nil})
	// A record the store cannot parse.
	require.NoError(t, os.MkdirAll(rt.livestoreRoot, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rt.livestoreRoot, "broken.json"), []byte("{not json"), 0o600))

	stages, failures := reportStrayRunProjects(context.Background(), rt)

	require.NotEmpty(t, failures, "an unreadable record is not a clean estate")
	assert.Contains(t, failures[0].Detail, "cannot be told from a stray")
	require.Len(t, stages, 1)
	assert.Equal(t, StageStatusFail, stages[0].Status)
}
