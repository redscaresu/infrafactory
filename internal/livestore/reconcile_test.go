package livestore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func reconcileDeployment(id, projectID string, state State) Deployment {
	return Deployment{
		ID: id, Scenario: "web-live-paris", Cloud: "scaleway",
		ProjectID: projectID, State: state,
		CreatedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour),
	}
}

// The expensive case, and the reason this exists: the store was wiped and
// the infrastructure was not.
func TestReconcileFindsAStampedProjectNoRecordExplains(t *testing.T) {
	projects := []StampedProject{
		{ID: "proj-a", Name: "if-run-block-paris-1", Ours: true},
	}

	got := Reconcile(projects, nil)

	require.Len(t, got.Unrecorded, 1)
	assert.Equal(t, "proj-a", got.Unrecorded[0].ProjectID)
	assert.False(t, got.Clean())
}

// infrafactory does not reason about projects it did not create. An
// unstamped project in this report would invite somebody to delete it.
func TestReconcileNeverConsidersAProjectWithoutTheStamp(t *testing.T) {
	projects := []StampedProject{
		{ID: "openclaw-prod", Name: "openclaw", Ours: false},
		{ID: "someone-elses", Name: "production", Ours: false},
	}

	got := Reconcile(projects, nil)

	assert.Empty(t, got.Unrecorded, "these are not ours to report on, let alone to delete")
	assert.True(t, got.Clean())
}

// A record naming a project the API does not have is not a leak, but it
// makes `live ls` report something that is gone.
func TestReconcileFindsARecordWhoseProjectIsGone(t *testing.T) {
	deployments := []Deployment{reconcileDeployment("dep-1", "proj-vanished", StateLive)}

	got := Reconcile(nil, deployments)

	require.Len(t, got.Vanished, 1)
	assert.Equal(t, "dep-1", got.Vanished[0].ID)
	assert.Zero(t, got.Accounted)
}

// A released deployment still ACCOUNTS for its project. Teardown records
// the release, but ADR-0024's unreclaimable case is exactly a project
// that outlives it -- so ignoring released records would send an operator
// to investigate something the store already explains.
func TestReconcileLetsAReleasedRecordAccountForItsProject(t *testing.T) {
	projects := []StampedProject{{ID: "proj-a", Name: "if-run-x", Ours: true}}
	deployments := []Deployment{reconcileDeployment("dep-1", "proj-a", StateReleased)}

	got := Reconcile(projects, deployments)

	assert.Empty(t, got.Unrecorded, "the store explains this project; it is not unaccounted for")
	assert.Equal(t, 1, got.Accounted)
	assert.True(t, got.Clean())
}

// A record with no project id is reported elsewhere (ADR-0024 calls it
// reapable-but-damaged) and cannot be reconciled in either direction.
func TestReconcileIgnoresARecordWithNoProjectID(t *testing.T) {
	deployments := []Deployment{reconcileDeployment("dep-broken", "", StateLive)}

	got := Reconcile(nil, deployments)

	assert.Empty(t, got.Vanished)
	assert.Zero(t, got.Accounted)
	assert.True(t, got.Clean())
}

// The mixed case an operator actually meets, and the ordering that makes
// two runs comparable.
func TestReconcileReportsBothDirectionsInAStableOrder(t *testing.T) {
	projects := []StampedProject{
		{ID: "proj-z", Name: "if-run-z", Ours: true},
		{ID: "proj-a", Name: "if-run-a", Ours: true},
		{ID: "proj-known", Name: "if-run-known", Ours: true},
		{ID: "not-ours", Name: "production", Ours: false},
	}
	deployments := []Deployment{
		reconcileDeployment("dep-known", "proj-known", StateLive),
		reconcileDeployment("dep-z", "gone-2", StateLive),
		reconcileDeployment("dep-a", "gone-1", StateLive),
	}

	got := Reconcile(projects, deployments)

	assert.Equal(t, []string{"proj-a", "proj-z"},
		[]string{got.Unrecorded[0].ProjectID, got.Unrecorded[1].ProjectID})
	assert.Equal(t, []string{"dep-a", "dep-z"},
		[]string{got.Vanished[0].ID, got.Vanished[1].ID})
	assert.Equal(t, 1, got.Accounted)
}
