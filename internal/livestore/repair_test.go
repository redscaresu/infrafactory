package livestore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type obsSpec struct {
	offset time.Duration
	status ObservationStatus
	detail string
	drift  bool
}

func repairDeployment(upgradeOffset time.Duration, specs ...obsSpec) Deployment {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	d := Deployment{
		ID: "dep-1", Scenario: "web-live-paris", Cloud: "scaleway",
		ProjectID: "proj-1", WorkDir: "/tmp/work",
		AddressResource: "scaleway_lb_ip",
		State:           StateLive,
		CreatedAt:       base.Add(-time.Hour), ExpiresAt: base.Add(time.Hour),
		UpgradedAt:       base.Add(upgradeOffset),
		UpgradeStartedAt: base.Add(upgradeOffset - 30*time.Second),
		UpgradeSucceeded: true,
	}
	for _, s := range specs {
		o := Observation{At: base.Add(s.offset), Status: s.status, Detail: s.detail}
		if s.drift {
			o.Version = VersionUnconfirmed
		} else if s.status == ObservationHealthy {
			o.Version = VersionConfirmed
		}
		d.Observations = append(d.Observations, o)
	}
	return d
}

// The shape S156d exists for: failing, upgraded, healthy. Terraform
// reported success both times, so only the observations can tell these
// apart.
func TestRepairsFindsAnUpgradeThatClearedAFailure(t *testing.T) {
	d := repairDeployment(0,
		obsSpec{offset: -2 * time.Minute, status: ObservationUnhealthy, detail: "HTTP 503 from /healthz"},
		obsSpec{offset: -time.Minute, status: ObservationUnhealthy, detail: "HTTP 503 from /healthz"},
		obsSpec{offset: time.Minute, status: ObservationHealthy},
		obsSpec{offset: 2 * time.Minute, status: ObservationHealthy},
	)

	got := Repairs([]Deployment{d}, nil)

	require.Len(t, got, 1)
	assert.Equal(t, "HTTP 503 from /healthz", got[0].Detail)
	assert.Equal(t, 2, got[0].ObservationsBefore)
	assert.Equal(t, 2, got[0].ObservationsAfter)
}

// An upgrade with nothing wrong before it fixed nothing. Rolling a
// version forward is a fine thing to have done and teaches no remedy.
func TestRepairsIgnoresAnUpgradeThatFixedNothing(t *testing.T) {
	d := repairDeployment(0,
		obsSpec{offset: -time.Minute, status: ObservationHealthy},
		obsSpec{offset: time.Minute, status: ObservationHealthy},
		obsSpec{offset: 2 * time.Minute, status: ObservationHealthy},
	)

	assert.Empty(t, Repairs([]Deployment{d}, nil),
		"there was no failure for the upgrade to have fixed")
}

// Still broken afterwards is not a fix, and writing it as one would
// teach a remedy that was demonstrably not one.
func TestRepairsIgnoresAnUpgradeThatDidNotHelp(t *testing.T) {
	d := repairDeployment(0,
		obsSpec{offset: -time.Minute, status: ObservationUnhealthy, detail: "HTTP 503"},
		obsSpec{offset: time.Minute, status: ObservationHealthy},
		obsSpec{offset: 2 * time.Minute, status: ObservationUnhealthy, detail: "HTTP 503"},
	)

	assert.Empty(t, Repairs([]Deployment{d}, nil))
}

// One healthy probe is a lucky sample, not evidence.
func TestRepairsWaitsForMoreThanOneHealthyProbe(t *testing.T) {
	d := repairDeployment(0,
		obsSpec{offset: -time.Minute, status: ObservationUnhealthy, detail: "HTTP 503"},
		obsSpec{offset: time.Minute, status: ObservationHealthy},
	)

	assert.Empty(t, Repairs([]Deployment{d}, nil),
		"observing it more may still qualify it; saying yes now would be as wrong as saying no")
}

// The most dangerous case, and the one every other signal calls healthy.
// A service answering fine while running a version other than the one
// deployed is close to evidence the upgrade did NOT take.
func TestRepairsRefusesWhenTheServiceIsHealthyButOnTheWrongVersion(t *testing.T) {
	d := repairDeployment(0,
		obsSpec{offset: -time.Minute, status: ObservationUnhealthy, detail: "HTTP 503"},
		obsSpec{offset: time.Minute, status: ObservationHealthy, drift: true},
		obsSpec{offset: 2 * time.Minute, status: ObservationHealthy, drift: true},
	)

	assert.Empty(t, Repairs([]Deployment{d}, nil),
		"the apply reported success and the running service did not move")
}

// A probe stamped at the upgrade instant describes the NEW
// configuration. Counting it as evidence of the old failure would
// attribute the fault to the thing that fixed it.
func TestRepairsCountsAProbeAtTheUpgradeInstantAsAfter(t *testing.T) {
	d := repairDeployment(0,
		obsSpec{offset: -time.Minute, status: ObservationUnhealthy, detail: "HTTP 503"},
		obsSpec{offset: 0, status: ObservationHealthy},
		obsSpec{offset: time.Minute, status: ObservationHealthy},
	)

	got := Repairs([]Deployment{d}, nil)
	require.Len(t, got, 1)
	assert.Equal(t, 1, got[0].ObservationsBefore)
	assert.Equal(t, 2, got[0].ObservationsAfter)
}

// The failure the service was exhibiting AT the upgrade, which is the
// one the new configuration was written against.
func TestRepairsTakesTheFailureTheUpgradeWasWrittenAgainst(t *testing.T) {
	d := repairDeployment(0,
		obsSpec{offset: -3 * time.Minute, status: ObservationUnhealthy, detail: "an older problem"},
		obsSpec{offset: -2 * time.Minute, status: ObservationUnhealthy, detail: "an older problem"},
		obsSpec{offset: -time.Minute, status: ObservationUnreachable, detail: "connection refused"},
		obsSpec{offset: time.Minute, status: ObservationHealthy},
		obsSpec{offset: 2 * time.Minute, status: ObservationHealthy},
	)

	got := Repairs([]Deployment{d}, nil)
	require.Len(t, got, 1)
	assert.Equal(t, "connection refused", got[0].Detail)
}

// A deployment that was never upgraded has no before/after pair at all.
func TestRepairsIgnoresADeploymentThatWasNeverUpgraded(t *testing.T) {
	d := repairDeployment(0,
		obsSpec{offset: -time.Minute, status: ObservationUnhealthy, detail: "HTTP 503"},
		obsSpec{offset: time.Minute, status: ObservationHealthy},
		obsSpec{offset: 2 * time.Minute, status: ObservationHealthy},
	)
	d.UpgradedAt = time.Time{}

	assert.Empty(t, Repairs([]Deployment{d}, nil))
}

// Without a working directory there is no previous HCL to diff against,
// so there is nothing prescriptive to extract.
func TestRepairsIgnoresADeploymentWithNoWorkDir(t *testing.T) {
	d := repairDeployment(0,
		obsSpec{offset: -time.Minute, status: ObservationUnhealthy, detail: "HTTP 503"},
		obsSpec{offset: time.Minute, status: ObservationHealthy},
		obsSpec{offset: 2 * time.Minute, status: ObservationHealthy},
	)
	d.WorkDir = ""

	assert.Empty(t, Repairs([]Deployment{d}, nil))
}

// A service that broke, recovered on its own, and was then upgraded was
// not fixed by that upgrade. Crediting the new configuration would
// attach a remedy to a failure it never addressed.
func TestRepairsIgnoresAFailureThatHadAlreadyRecovered(t *testing.T) {
	d := repairDeployment(0,
		obsSpec{offset: -3 * time.Minute, status: ObservationUnhealthy, detail: "HTTP 503"},
		obsSpec{offset: -2 * time.Minute, status: ObservationHealthy},
		obsSpec{offset: -time.Minute, status: ObservationHealthy},
		obsSpec{offset: time.Minute, status: ObservationHealthy},
		obsSpec{offset: 2 * time.Minute, status: ObservationHealthy},
	)

	assert.Empty(t, Repairs([]Deployment{d}, nil),
		"it had already recovered; the upgrade cleared nothing")
}

// The upgrade's OWN downtime must not be learned as the failure it was
// meant to fix. `live observe` can run during an apply that takes
// minutes, and those probes are stamped before UpgradedAt because that
// is written after the apply returns.
func TestRepairsDiscardsProbesTakenDuringTheApply(t *testing.T) {
	d := repairDeployment(0,
		obsSpec{offset: -time.Minute, status: ObservationUnhealthy, detail: "HTTP 503 from /healthz"},
		// Mid-apply: after UpgradeStartedAt (-30s), before UpgradedAt.
		obsSpec{offset: -15 * time.Second, status: ObservationUnreachable, detail: "connection refused"},
		obsSpec{offset: time.Minute, status: ObservationHealthy},
		obsSpec{offset: 2 * time.Minute, status: ObservationHealthy},
	)

	got := Repairs([]Deployment{d}, nil)

	require.Len(t, got, 1)
	assert.Equal(t, "HTTP 503 from /healthz", got[0].Detail,
		"the changeover describes neither configuration")
	assert.Equal(t, 1, got[0].ObservationsBefore)
}

// Without the boundary there is no way to tell the two apart, so the
// record is declined rather than guessed at.
func TestRepairsDeclinesWhenTheApplyWindowIsUnknown(t *testing.T) {
	d := repairDeployment(0,
		obsSpec{offset: -time.Minute, status: ObservationUnhealthy, detail: "HTTP 503"},
		obsSpec{offset: time.Minute, status: ObservationHealthy},
		obsSpec{offset: 2 * time.Minute, status: ObservationHealthy},
	)
	d.UpgradeStartedAt = time.Time{}

	assert.Empty(t, Repairs([]Deployment{d}, nil),
		"guessing a boundary would teach remedies for outages the upgrade caused")
}

// A rule claiming three probes reported a failure when one did overstates
// its own evidence, and the corpus is read as guidance.
func TestRepairsCountsOnlyTheProbesThatReportedTheFailure(t *testing.T) {
	d := repairDeployment(0,
		obsSpec{offset: -4 * time.Minute, status: ObservationHealthy},
		obsSpec{offset: -3 * time.Minute, status: ObservationHealthy},
		obsSpec{offset: -2 * time.Minute, status: ObservationUnhealthy, detail: "HTTP 503"},
		obsSpec{offset: -time.Minute, status: ObservationUnhealthy, detail: "HTTP 503"},
		obsSpec{offset: time.Minute, status: ObservationHealthy},
		obsSpec{offset: 2 * time.Minute, status: ObservationHealthy},
	)

	got := Repairs([]Deployment{d}, nil)
	require.Len(t, got, 1)
	assert.Equal(t, 2, got[0].ObservationsBefore, "two reported it, not four")
}

// `live upgrade` updates the record whenever the apply got past plan,
// because a failed apply may still have created resources. So an upgrade
// having HAPPENED is not an upgrade having worked -- and after a partial
// apply the running infrastructure is a mixture of the two
// configurations, so the diff describes a change that was never made.
func TestRepairsDeclinesAnUpgradeThatDidNotComplete(t *testing.T) {
	d := repairDeployment(0,
		obsSpec{offset: -time.Minute, status: ObservationUnhealthy, detail: "HTTP 503"},
		obsSpec{offset: time.Minute, status: ObservationHealthy},
		obsSpec{offset: 2 * time.Minute, status: ObservationHealthy},
	)
	d.UpgradeSucceeded = false

	assert.Empty(t, Repairs([]Deployment{d}, nil),
		"crediting recovery to HCL that was never applied is a false remedy")
}
