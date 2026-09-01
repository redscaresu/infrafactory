package livestore

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Silence is not health. A deployment nobody probed must say so, in a
// word, rather than reading as fine.
func TestHealthOfAnUnobservedDeploymentSaysUnobserved(t *testing.T) {
	got := Deployment{ID: "dep-1"}.Health()

	assert.Equal(t, HealthUnobserved, got.Status)
	assert.False(t, got.Observed())
	assert.Zero(t, got.Observations)
	assert.Nil(t, got.At, "there is no time at which nobody looked")
}

// `omitempty` does not omit a zero time.Time -- it is a struct, so the
// tag has no effect and it marshals as year 1. A page showing a
// never-probed deployment as "last observed 0001-01-01" is worse than a
// blank cell, because it looks like data.
func TestHealthNeverSerialisesAYearOneTimestamp(t *testing.T) {
	payload, err := json.Marshal(Deployment{ID: "dep-1"}.Health())
	require.NoError(t, err)

	assert.NotContains(t, string(payload), "0001-01-01")
	assert.Contains(t, string(payload), `"at":null`)
}

// The load-bearing case for the UI. `VersionUnchecked` is the empty
// string on the record -- correct there, and a falsehood in a view: a
// blank cell beside a `confirmed` one invites the reader to read
// nothing-was-checked as nothing-is-wrong.
func TestHealthNeverSerialisesAVersionAsAnEmptyString(t *testing.T) {
	for name, d := range map[string]Deployment{
		"never observed": {ID: "dep-1"},
		"observed with no version path": {ID: "dep-2", Observations: []Observation{
			{At: time.Now(), Status: ObservationHealthy, Version: VersionUnchecked},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			payload, err := json.Marshal(d.Health())
			require.NoError(t, err)
			assert.Contains(t, string(payload), `"version":"unchecked"`)
		})
	}
}

// The most dangerous state this system can be in, and the one every
// other signal calls healthy.
func TestHealthKeepsVersionDriftVisibleBesideAHealthyStatus(t *testing.T) {
	got := Deployment{Observations: []Observation{
		{At: time.Now(), Status: ObservationHealthy, Version: VersionUnconfirmed},
	}}.Health()

	assert.Equal(t, string(ObservationHealthy), got.Status)
	assert.Equal(t, string(VersionUnconfirmed), got.Version,
		"a reader must be able to see the two disagree")
}

// The LAST observation, and the count, so one lucky sample is
// distinguishable from a settled picture.
func TestHealthReportsTheLatestObservationAndHowManyThereWere(t *testing.T) {
	base := time.Now()
	got := Deployment{Observations: []Observation{
		{At: base.Add(-time.Minute), Status: ObservationUnhealthy, Detail: "old"},
		{At: base, Status: ObservationUnreachable, Detail: "connection refused"},
	}}.Health()

	assert.Equal(t, string(ObservationUnreachable), got.Status)
	assert.Equal(t, "connection refused", got.Detail)
	assert.Equal(t, 2, got.Observations)
	require.NotNil(t, got.At)
	assert.Equal(t, base, *got.At)
}
