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
	// Surfaced rather than counted as agreement: see Released's field
	// comment. Still Clean(), because `live forget` is a deliberate act
	// and failing every later reconcile for it would be the
	// permanent-red defect this command just had.
	require.Len(t, got.Released, 1)
	assert.True(t, got.Clean())
	assert.Equal(t, 1, got.Examined(), "and it is still a record that was looked at")
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

// `live teardown` destroys the project and THEN marks the record
// released, so a released record naming a project the API no longer
// knows is the success path -- not a disagreement.
//
// Without this, every successful teardown left `live reconcile`
// permanently non-zero, reporting "the record outlived its
// infrastructure" about the one case where that is precisely what
// should have happened. Found by running the S156e validation deploy
// end to end: no unit test covered a released record, because none of
// them tore one down first.
func TestAReleasedRecordIsNotAVanishedOne(t *testing.T) {
	// Neither project exists any more: one because teardown destroyed
	// it, one because something else did.
	got := Reconcile(nil, []Deployment{
		reconcileDeployment("dep-released", "proj-destroyed", StateReleased),
		reconcileDeployment("dep-live", "proj-vanished", StateLive),
	})

	require.Len(t, got.Vanished, 1,
		"only the LIVE record is a disagreement")
	assert.Equal(t, "dep-live", got.Vanished[0].ID,
		"a released record's project is supposed to be gone")
	assert.False(t, got.Clean(), "the live one is still a real disagreement")
}

func TestAReleasedRecordAloneReconcilesClean(t *testing.T) {
	got := Reconcile(nil, []Deployment{
		reconcileDeployment("dep-released", "proj-destroyed", StateReleased),
	})

	assert.True(t, got.Clean(),
		"a torn-down deployment must not leave reconcile red for the rest of time")
}

// The two released cases are opposites and the ordering is what keeps
// them apart: project GONE is the success path, project ALIVE is
// ADR-0024's unreclaimable case. Skipping released records outright
// would silence the expensive one.
func TestAReleasedRecordWhoseProjectSurvivedIsStillAccountedFor(t *testing.T) {
	projects := []StampedProject{{ID: "proj-a", Name: "if-run-x", Ours: true}}

	got := Reconcile(projects, []Deployment{
		reconcileDeployment("dep-released-gone", "proj-vanished", StateReleased),
		reconcileDeployment("dep-released-alive", "proj-a", StateReleased),
	})

	assert.Empty(t, got.Vanished, "a destroyed project is not a disagreement")
	assert.Empty(t, got.Unrecorded, "and a surviving one is still explained by its record")
	// Explained is not the same as agreed. `live forget` releases
	// WITHOUT destroying, so a surviving project may be billing with
	// nothing that will reap it -- reported, not counted as agreement.
	require.Len(t, got.Released, 1)
	assert.Equal(t, "dep-released-alive", got.Released[0].ID)
	assert.Zero(t, got.Accounted, "Accounted is for LIVE records")
}

// After any successful teardown the store holds a released record whose
// project is gone. It belongs in neither Accounted nor Vanished, so the
// summary's `Accounted + Vanished` total reported "0 record(s)" for a
// store that held one -- indistinguishable from an empty or unreadable
// store, which is the false signal reconcileSummary exists to prevent.
func TestExaminedCountsEveryRecordItLookedAt(t *testing.T) {
	projects := []StampedProject{{ID: "proj-alive", Name: "if-run-x", Ours: true}}

	got := Reconcile(projects, []Deployment{
		reconcileDeployment("dep-live", "proj-alive", StateLive),
		reconcileDeployment("dep-torn-down", "proj-gone", StateReleased),
		reconcileDeployment("dep-forgotten", "proj-alive", StateReleased),
	})

	assert.Equal(t, 3, got.Examined(), "every record was looked at, whatever became of it")
	assert.Equal(t, 1, got.Accounted)
	assert.Len(t, got.Released, 1)
	assert.Empty(t, got.Vanished)
}
