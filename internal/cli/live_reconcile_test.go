package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
)

func reconcileRuntime(t *testing.T, fake *fakeRunProject) (*CommandRuntime, *livestore.FilesystemStore) {
	t.Helper()
	h := newCommandTestHarness(t)
	rt := &CommandRuntime{livestoreRoot: h.LivestoreRoot()}
	rt.Deps.RunProject = fake

	t.Setenv("SCW_SECRET_KEY", "test-secret")
	t.Setenv("SCW_DEFAULT_ORGANIZATION_ID", "org-1")
	return rt, livestore.NewFilesystemStore(h.LivestoreRoot())
}

func runReconcile(t *testing.T, rt *CommandRuntime, out *strings.Builder) error {
	t.Helper()
	cmd := &cobra.Command{Use: "reconcile"}
	cmd.Flags().String("output", string(OutputModeHuman), "")
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetContext(context.Background())
	return runLiveReconcileCommand(cmd, nil, rt)
}

func stampedListing(id, name string) harness.ListedProject {
	return harness.ListedProject{ID: id, Name: name, Description: harness.RunProjectDescription}
}

func seedReconcileDeployment(t *testing.T, store *livestore.FilesystemStore, id, projectID string) {
	t.Helper()
	require.NoError(t, store.Put(livestore.Deployment{
		ID: id, Scenario: "web-live-paris", Cloud: "scaleway",
		ProjectID: projectID, State: livestore.StateLive,
		CreatedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour),
	}))
}

// The whole point: infrastructure the store cannot explain is found, and
// the command FAILS rather than mentioning it in passing.
func TestReconcileReportsAStampedProjectNoRecordExplains(t *testing.T) {
	rt, _ := reconcileRuntime(t, &fakeRunProject{
		listed: []harness.ListedProject{stampedListing("proj-orphan", "if-run-block-paris-9")},
	})

	var out strings.Builder
	err := runReconcile(t, rt, &out)

	require.Error(t, err, "something is running that nothing will reap")
	assert.Contains(t, out.String(), "proj-orphan")
	assert.Contains(t, out.String(), "Nothing was destroyed")
}

// Never destroys. An unrecorded project is by definition something the
// records do not explain, and destroying what you cannot explain is how a
// reconciler becomes the incident.
func TestReconcileDestroysNothing(t *testing.T) {
	fake := &fakeRunProject{
		listed: []harness.ListedProject{stampedListing("proj-orphan", "if-run-x")},
	}
	rt, _ := reconcileRuntime(t, fake)

	_ = runReconcile(t, rt, &strings.Builder{})

	assert.Zero(t, fake.deletes, "reporting is the whole contract")
}

// A missing credential makes every project invisible, which renders as
// "nothing unaccounted for" -- the exact false green this command exists
// to prevent.
func TestReconcileRefusesWithoutCredentialsRatherThanReportingClean(t *testing.T) {
	rt, _ := reconcileRuntime(t, &fakeRunProject{})
	t.Setenv("SCW_SECRET_KEY", "")

	var out strings.Builder
	err := runReconcile(t, rt, &out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "would look accounted for")
}

// An unreachable API must never read as an empty estate, for the same
// reason.
func TestReconcileFailsWhenTheCloudCannotBeRead(t *testing.T) {
	rt, _ := reconcileRuntime(t, &fakeRunProject{
		listErr: assertAnError(),
	})

	require.Error(t, runReconcile(t, rt, &strings.Builder{}))
}

// infrafactory does not reason about projects it did not create.
func TestReconcileIgnoresProjectsWithoutTheStamp(t *testing.T) {
	rt, _ := reconcileRuntime(t, &fakeRunProject{
		listed: []harness.ListedProject{
			{ID: "openclaw-prod", Name: "openclaw", Description: "our real project"},
		},
	})

	var out strings.Builder
	require.NoError(t, runReconcile(t, rt, &out))
	assert.NotContains(t, out.String(), "openclaw")
}

// The clean answer states what was EXAMINED. "0 unrecorded" out of zero
// projects and out of forty read identically and mean opposite things.
func TestReconcileCleanAnswerSaysWhatItLookedAt(t *testing.T) {
	fake := &fakeRunProject{
		listed: []harness.ListedProject{stampedListing("proj-a", "if-run-a")},
	}
	rt, store := reconcileRuntime(t, fake)
	seedReconcileDeployment(t, store, "dep-a", "proj-a")

	var out strings.Builder
	require.NoError(t, runReconcile(t, rt, &out))

	assert.Contains(t, out.String(), "examined 1 project(s)")
	assert.Contains(t, out.String(), "agree")
	assert.Equal(t, "org-1", fake.lastOrg)
}

// A record naming a project the API does not have makes `live ls` report
// something that is gone.
func TestReconcileReportsARecordWhoseProjectIsGone(t *testing.T) {
	rt, store := reconcileRuntime(t, &fakeRunProject{})
	seedReconcileDeployment(t, store, "dep-ghost", "proj-vanished")

	var out strings.Builder
	err := runReconcile(t, rt, &out)

	require.Error(t, err)
	assert.Contains(t, out.String(), "dep-ghost")
	assert.Contains(t, out.String(), "does not exist")
}

// The defect exactly as it was observed: `live reconcile` returned a
// CLIError and "the cloud and the store disagree" for a deployment that
// had just been torn down correctly.
//
// Teardown destroys the project and THEN marks the record released, so
// this is the steady state after every successful teardown -- and the
// livestore tests could not catch it, because the symptom was the
// command's exit status and its summary line.
func TestReconcileIsCleanAfterASuccessfulTeardown(t *testing.T) {
	rt, store := reconcileRuntime(t, &fakeRunProject{})
	seedReconcileDeployment(t, store, "dep-torn-down", "proj-destroyed")
	require.NoError(t, store.MarkReleased("dep-torn-down"))

	var out strings.Builder
	err := runReconcile(t, rt, &out)

	require.NoError(t, err, "a destroyed project is what teardown is for")
	assert.Contains(t, out.String(), "the cloud and the store agree")
	// And it is still COUNTED. Reporting "0 record(s)" for a store that
	// holds one reads exactly like an empty or unreadable store.
	assert.Contains(t, out.String(), "1 record(s)")
}

// `live forget` releases a record WITHOUT destroying its project, so a
// released record whose project still exists may be a load balancer and
// an instance still billing with nothing that will ever reap them.
// Reported -- not a disagreement, and not silence either.
func TestReconcileSaysWhenAReleasedRecordsProjectIsStillAlive(t *testing.T) {
	rt, store := reconcileRuntime(t, &fakeRunProject{
		listed: []harness.ListedProject{stampedListing("proj-forgotten", "if-run-web-live-paris-1")},
	})
	seedReconcileDeployment(t, store, "dep-forgotten", "proj-forgotten")
	require.NoError(t, store.MarkReleased("dep-forgotten"))

	var out strings.Builder
	err := runReconcile(t, rt, &out)

	require.NoError(t, err, "forget is a deliberate act, not a disagreement")
	assert.Contains(t, out.String(), "nothing will reap them")
}
