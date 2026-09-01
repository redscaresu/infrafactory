package livestore

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// obs builds one failing observation.
func obs(status ObservationStatus, detail string) Observation {
	return Observation{At: time.Now(), Status: status, Detail: detail}
}

func healthy() Observation {
	return Observation{At: time.Now(), Status: ObservationHealthy}
}

func deploymentWith(id, scenario string, os ...Observation) Deployment {
	return Deployment{ID: id, Scenario: scenario, Observations: os}
}

var testRule = PromotionRule{ConsecutiveProbes: 3, DistinctDeployments: 2}

// A single 502 never becomes a pitfall. Without this gate one broken
// deployment emits the same lesson forever and the corpus rots.
func TestOneObservationIsNeverPromoted(t *testing.T) {
	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web", obs(ObservationUnhealthy, "returned HTTP 502")),
	}, testRule)

	assert.Empty(t, got)
}

// Persistent on one deployment: enough probes in a row that it is not a
// restart or a blip.
func TestPersistenceOnOneDeploymentPromotes(t *testing.T) {
	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web",
			obs(ObservationUnhealthy, "returned HTTP 502"),
			obs(ObservationUnhealthy, "returned HTTP 502"),
			obs(ObservationUnhealthy, "returned HTTP 502"),
		),
	}, testRule)

	require.Len(t, got, 1)
	assert.Equal(t, 3, got[0].LongestRun)
	assert.Equal(t, PromotedByPersistence, got[0].Reason)
	assert.Equal(t, []string{"dep-1"}, got[0].Deployments)
}

// A healthy probe between two failures means the service RECOVERED,
// which is precisely the blip this gate exists to reject. Counting them
// as consecutive would promote a flapping service as a structural fact.
func TestARecoveryBreaksTheRun(t *testing.T) {
	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web",
			obs(ObservationUnhealthy, "returned HTTP 502"),
			obs(ObservationUnhealthy, "returned HTTP 502"),
			healthy(),
			obs(ObservationUnhealthy, "returned HTTP 502"),
			obs(ObservationUnhealthy, "returned HTTP 502"),
		),
	}, testRule)

	assert.Empty(t, got, "four failures, but never three in a row")
}

// A different failure in between breaks the run too: only one thing can
// be true of a service at a given probe.
func TestADifferentFailureBreaksTheRun(t *testing.T) {
	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web",
			obs(ObservationUnhealthy, "returned HTTP 502"),
			obs(ObservationUnhealthy, "returned HTTP 502"),
			obs(ObservationUnhealthy, "returned HTTP 503"),
			obs(ObservationUnhealthy, "returned HTTP 502"),
		),
	}, testRule)

	assert.Empty(t, got)
}

// Two separate deployments reporting the same thing is a property of the
// shape rather than of one machine — the other half of the rule, and it
// does not need persistence.
func TestBreadthAcrossDeploymentsPromotes(t *testing.T) {
	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web", obs(ObservationUnhealthy, "returned HTTP 502")),
		deploymentWith("dep-2", "web", obs(ObservationUnhealthy, "returned HTTP 502")),
	}, testRule)

	require.Len(t, got, 1)
	assert.Equal(t, PromotedByBreadth, got[0].Reason)
	assert.Equal(t, []string{"dep-1", "dep-2"}, got[0].Deployments)
	assert.Equal(t, 1, got[0].LongestRun)
}

// The same deployment observed twice is not two deployments.
func TestTheSameDeploymentTwiceIsNotBreadth(t *testing.T) {
	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web",
			obs(ObservationUnhealthy, "returned HTTP 502"),
			healthy(),
			obs(ObservationUnhealthy, "returned HTTP 502"),
		),
	}, testRule)

	assert.Empty(t, got, "two failures on one deployment, with a recovery between")
}

// "It told us it is broken" and "we got no answer" are different facts
// (ADR-0024). Merging them would produce a lesson about neither.
func TestUnhealthyAndUnreachableDoNotMerge(t *testing.T) {
	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web", obs(ObservationUnhealthy, "the same words")),
		deploymentWith("dep-2", "web", obs(ObservationUnreachable, "the same words")),
	}, testRule)

	assert.Empty(t, got, "one of each is not two of either")
}

// Grouping is on the NORMALIZED detail, so the same underlying failure
// with shifting line numbers reproduces with itself.
func TestNormalizationGroupsTheSameFailure(t *testing.T) {
	rule := testRule
	rule.Normalize = func(s string) string {
		// Stand-in for feedback.NormalizeDetail: drop line references.
		if i := strings.Index(s, " on line "); i >= 0 {
			return s[:i]
		}
		return s
	}

	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web", obs(ObservationUnhealthy, "backend down on line 12")),
		deploymentWith("dep-2", "web", obs(ObservationUnhealthy, "backend down on line 47")),
	}, rule)

	require.Len(t, got, 1)
	assert.Equal(t, "backend down", got[0].Detail, "the identity is the normalized form")
	assert.Contains(t, got[0].Example, "line 12", "but a human needs the real words")
}

// A lesson blamed on a version nobody verified is a falsehood (S155a),
// so attribution travels with the candidate rather than filtering it:
// something WAS broken either way, and the extractor decides.
func TestAttributionIsRecordedRatherThanFiltered(t *testing.T) {
	confirmed := obs(ObservationUnhealthy, "returned HTTP 502")
	confirmed.Version = VersionConfirmed

	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web", obs(ObservationUnhealthy, "returned HTTP 502")),
		deploymentWith("dep-2", "web", confirmed),
	}, testRule)
	require.Len(t, got, 1)
	assert.True(t, got[0].Attributable, "one confirmed exhibiting deployment is enough to attribute")

	unattributable := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web", obs(ObservationUnhealthy, "returned HTTP 502")),
		deploymentWith("dep-2", "web", obs(ObservationUnhealthy, "returned HTTP 502")),
	}, testRule)
	require.Len(t, unattributable, 1)
	assert.False(t, unattributable[0].Attributable, "still promoted, but marked")
}

// A released deployment's observations happened while it was live, and
// the record keeps its history on purpose. Dropping them would discard
// exactly the reproduced evidence the gate looks for.
func TestReleasedDeploymentsStillCount(t *testing.T) {
	released := deploymentWith("dep-1", "web", obs(ObservationUnhealthy, "returned HTTP 502"))
	released.State = StateReleased

	got := PromotionCandidates([]Deployment{
		released,
		deploymentWith("dep-2", "web", obs(ObservationUnhealthy, "returned HTTP 502")),
	}, testRule)

	require.Len(t, got, 1)
	assert.Contains(t, got[0].Deployments, "dep-1")
}

// Both halves at once is the strongest evidence available here, and a
// reader should not have to infer which rule fired.
func TestBothHalvesAreReportedDistinctly(t *testing.T) {
	three := []Observation{
		obs(ObservationUnhealthy, "returned HTTP 502"),
		obs(ObservationUnhealthy, "returned HTTP 502"),
		obs(ObservationUnhealthy, "returned HTTP 502"),
	}
	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web", three...),
		deploymentWith("dep-2", "other", three...),
	}, testRule)

	require.Len(t, got, 1)
	assert.Equal(t, PromotedByBoth, got[0].Reason)
	assert.Equal(t, []string{"other", "web"}, got[0].Scenarios,
		"more than one scenario is strong evidence of a shape-level problem")
}

// Strongest evidence first: an operator reading a list wants the most
// reproduced thing at the top.
func TestCandidatesAreOrderedByStrengthOfEvidence(t *testing.T) {
	twice := []Observation{
		obs(ObservationUnhealthy, "weak"),
		obs(ObservationUnhealthy, "weak"),
		obs(ObservationUnhealthy, "weak"),
	}
	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web", twice...),
		deploymentWith("dep-2", "web", obs(ObservationUnhealthy, "strong")),
		deploymentWith("dep-3", "web", obs(ObservationUnhealthy, "strong")),
		deploymentWith("dep-4", "web", obs(ObservationUnhealthy, "strong")),
	}, testRule)

	require.Len(t, got, 2)
	assert.Equal(t, "strong", got[0].Detail, "three deployments beats one long run")
	assert.Equal(t, "weak", got[1].Detail)
}

// A healthy service has no detail and nothing to teach.
func TestHealthyObservationsAreNeverCandidates(t *testing.T) {
	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web", healthy(), healthy(), healthy(), healthy()),
		deploymentWith("dep-2", "web", healthy()),
	}, testRule)

	assert.Empty(t, got)
}

// A rule with no thresholds must promote nothing rather than everything:
// a misconfigured gate that opens is worse than one that closes.
func TestAZeroRulePromotesNothing(t *testing.T) {
	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web",
			obs(ObservationUnhealthy, "returned HTTP 502"),
			obs(ObservationUnhealthy, "returned HTTP 502"),
			obs(ObservationUnhealthy, "returned HTTP 502"),
		),
		deploymentWith("dep-2", "web", obs(ObservationUnhealthy, "returned HTTP 502")),
	}, PromotionRule{})

	assert.Empty(t, got)
}

// The most dangerous shape live observation can find: the service
// answers perfectly and is running something other than what the record
// claims. Every other signal in the system reports it as healthy, which
// is exactly why the gate must not.
//
// This is the S155b canary's shape — an apply that SUCCEEDED while the
// service kept serving the old version.
func TestVersionDriftIsPromotableEvenThoughTheServiceIsHealthy(t *testing.T) {
	drift := func() Observation {
		o := healthy()
		o.Version = VersionUnconfirmed
		o.Detail = `the record claims nginx:1.28 but / does not mention "1.28"`
		return o
	}

	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web", drift(), drift(), drift()),
	}, testRule)

	require.Len(t, got, 1, "a healthy service running the wrong version is a lesson")
	assert.True(t, got[0].VersionDrift)
	assert.Equal(t, ObservationHealthy, got[0].Status, "healthy, and still wrong")
	assert.Contains(t, got[0].Example, "does not mention")
}

// Drift and a health failure are different problems with different
// fixes, and their details come from different probes.
func TestVersionDriftDoesNotMergeWithAHealthFailure(t *testing.T) {
	drift := healthy()
	drift.Version = VersionUnconfirmed
	drift.Detail = "same words"

	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web", drift),
		deploymentWith("dep-2", "web", obs(ObservationUnhealthy, "same words")),
	}, testRule)

	assert.Empty(t, got, "one of each is not two of either")
}

// An observation that is BOTH unhealthy and version-unconfirmed is not
// drift: its detail describes the health failure, which is the more
// urgent story, and it must group with other instances of that failure
// rather than splitting off on a version field.
func TestAnUnhealthyObservationGroupsByItsFailureNotItsVersion(t *testing.T) {
	both := obs(ObservationUnhealthy, "returned HTTP 502")
	both.Version = VersionUnconfirmed

	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web", both),
		deploymentWith("dep-2", "web", obs(ObservationUnhealthy, "returned HTTP 502")),
	}, testRule)

	require.Len(t, got, 1, "the health failure is the story; the version is incidental")
	assert.False(t, got[0].VersionDrift)
	assert.Len(t, got[0].Deployments, 2)
}

// `unchecked` is not adverse: nobody looked, which is not evidence of
// anything (S155a).
func TestAnUncheckedVersionIsNotDrift(t *testing.T) {
	unchecked := healthy()
	unchecked.Detail = "irrelevant"

	got := PromotionCandidates([]Deployment{
		deploymentWith("dep-1", "web", unchecked, unchecked, unchecked),
		deploymentWith("dep-2", "web", unchecked),
	}, testRule)

	assert.Empty(t, got, "nobody having looked is not a failure")
}
