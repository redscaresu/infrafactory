package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
)

type fakeServiceProbe struct {
	result   harness.ServiceProbeResult
	err      error
	calls    int
	lastAddr string
	lastPort int
	lastPath string
}

func (f *fakeServiceProbe) Probe(_ context.Context, address string, port int, healthPath string) (harness.ServiceProbeResult, error) {
	f.calls++
	f.lastAddr, f.lastPort, f.lastPath = address, port, healthPath
	return f.result, f.err
}

// observeRuntime builds a runtime whose live store is workspace-scoped,
// so parallel tests cannot share deployment records.
func observeRuntime(t *testing.T, probe *fakeServiceProbe) (*CommandRuntime, *livestore.FilesystemStore) {
	t.Helper()
	h := newCommandTestHarness(t)
	rt := &CommandRuntime{
		livestoreRoot: h.LivestoreRoot(),
		Deps:          RuntimeDependencies{ServiceProbe: probe},
	}
	return rt, livestore.NewFilesystemStore(h.LivestoreRoot())
}

func observableDeployment(t *testing.T, store *livestore.FilesystemStore, id string) livestore.Deployment {
	t.Helper()
	now := time.Now()
	d := livestore.Deployment{
		ID:         id,
		Scenario:   "web-live-paris",
		ProjectID:  "7c98d82e-ad6d-4f4c-99ea-d1886b0f38e5",
		Address:    "1.2.3.4",
		Port:       80,
		HealthPath: "/healthz",
		Image:      "nginx",
		Tag:        "1.27",
		State:      livestore.StateLive,
		CreatedAt:  now.Add(-time.Hour),
		ExpiresAt:  now.Add(time.Hour),
	}
	require.NoError(t, store.Put(d))
	return d
}

func runObserve(t *testing.T, rt *CommandRuntime, out *strings.Builder) error {
	t.Helper()
	cmd := &cobra.Command{Use: "observe"}
	cmd.Flags().String("output", string(OutputModeHuman), "")
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetContext(context.Background())
	return runLiveObserveCommand(cmd, nil, rt)
}

func TestObserveRecordsAHealthyProbeAgainstTheDeployment(t *testing.T) {
	probe := &fakeServiceProbe{result: harness.ServiceProbeResult{Reachable: true, Healthy: true, Status: 200}}
	rt, store := observeRuntime(t, probe)
	d := observableDeployment(t, store, "dep-healthy")

	var out strings.Builder
	require.NoError(t, runObserve(t, rt, &out))

	// Probed at the address and health path the RECORD carries, not
	// whatever the scenario file says today.
	assert.Equal(t, "1.2.3.4", probe.lastAddr)
	assert.Equal(t, 80, probe.lastPort)
	assert.Equal(t, "/healthz", probe.lastPath)

	got, err := store.Get(d.ID)
	require.NoError(t, err)
	require.Len(t, got.Observations, 1)
	assert.Equal(t, livestore.ObservationHealthy, got.Observations[0].Status)
	assert.Empty(t, got.Observations[0].Detail, "a healthy probe has nothing to say")
}

// An unhealthy service is a finding, not a note. A command that exits
// zero while something it is watching is down has not observed anything
// useful.
func TestObserveReportsAnUnhealthyServiceAsAFailure(t *testing.T) {
	probe := &fakeServiceProbe{result: harness.ServiceProbeResult{
		Reachable: true, Status: 503,
		Detail: "health path http://1.2.3.4:80/healthz returned HTTP 503",
	}}
	rt, store := observeRuntime(t, probe)
	d := observableDeployment(t, store, "dep-sick")

	var out strings.Builder
	err := runObserve(t, rt, &out)

	require.Error(t, err)
	assert.Contains(t, out.String(), "HTTP 503")

	got, storeErr := store.Get(d.ID)
	require.NoError(t, storeErr)
	require.Len(t, got.Observations, 1)
	assert.Equal(t, livestore.ObservationUnhealthy, got.Observations[0].Status)
}

// "It told us it is broken" and "we got no answer" are different facts,
// and the record has to keep them apart or S156 learns the wrong lesson.
func TestObserveDistinguishesUnreachableFromUnhealthy(t *testing.T) {
	probe := &fakeServiceProbe{result: harness.ServiceProbeResult{
		Detail: "health path http://1.2.3.4:80/healthz is unreachable: connection refused",
	}}
	rt, store := observeRuntime(t, probe)
	d := observableDeployment(t, store, "dep-gone")

	require.Error(t, runObserve(t, rt, &strings.Builder{}))

	got, err := store.Get(d.ID)
	require.NoError(t, err)
	require.Len(t, got.Observations, 1)
	assert.Equal(t, livestore.ObservationUnreachable, got.Observations[0].Status)
}

// A probe that could not run is not a service that is down, and must not
// be written to the record as if it were.
func TestObserveRecordsNothingWhenTheProbeItselfFailed(t *testing.T) {
	probe := &fakeServiceProbe{err: errors.New("no address recorded")}
	rt, store := observeRuntime(t, probe)
	d := observableDeployment(t, store, "dep-unprobeable")

	var out strings.Builder
	require.Error(t, runObserve(t, rt, &out))
	assert.Contains(t, out.String(), "could not be probed")

	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Observations, "an observation that did not happen must not be recorded")
}

// Probing a released deployment's old address would attribute whatever
// answers there now to a service that no longer exists.
func TestObserveSkipsReleasedAndUnprobeableRecords(t *testing.T) {
	probe := &fakeServiceProbe{result: harness.ServiceProbeResult{Reachable: true, Healthy: true}}
	rt, store := observeRuntime(t, probe)

	released := observableDeployment(t, store, "dep-released")
	released.State = livestore.StateReleased
	require.NoError(t, store.Put(released))

	// A record from before S154 carries no port.
	legacy := observableDeployment(t, store, "dep-legacy")
	legacy.Port = 0
	require.NoError(t, store.Put(legacy))

	var out strings.Builder
	require.NoError(t, runObserve(t, rt, &out), "skipping is not failing")
	assert.Zero(t, probe.calls)
	assert.Contains(t, out.String(), "released")
	assert.Contains(t, out.String(), "nothing to probe")
}

// An expired deployment that is still answering means the reaper has not
// run, which is exactly what this command exists to surface.
func TestObserveStillProbesAnExpiredDeployment(t *testing.T) {
	probe := &fakeServiceProbe{result: harness.ServiceProbeResult{Reachable: true, Healthy: true}}
	rt, store := observeRuntime(t, probe)

	d := observableDeployment(t, store, "dep-overdue")
	d.ExpiresAt = time.Now().Add(-time.Minute)
	require.NoError(t, store.Put(d))

	require.NoError(t, runObserve(t, rt, &strings.Builder{}))
	assert.Equal(t, 1, probe.calls, "still running is still worth recording")
}

func TestObserveWithNoDeploymentsSucceeds(t *testing.T) {
	probe := &fakeServiceProbe{}
	rt, _ := observeRuntime(t, probe)

	assert.NoError(t, runObserve(t, rt, &strings.Builder{}))
	assert.Zero(t, probe.calls)
}

// The ring is what stops a permanently broken deployment turning its
// record into an append-only log that grows until the disk does.
func TestObservationsAreCappedOldestFirst(t *testing.T) {
	var d livestore.Deployment
	for i := 0; i < livestore.MaxObservations+10; i++ {
		d.RecordObservation(livestore.Observation{
			At:     time.Unix(int64(i), 0),
			Status: livestore.ObservationHealthy,
		})
	}

	require.Len(t, d.Observations, livestore.MaxObservations)
	assert.Equal(t, int64(10), d.Observations[0].At.Unix(), "the oldest go first")
	assert.Equal(t, int64(livestore.MaxObservations+9), d.Observations[len(d.Observations)-1].At.Unix())
}

// Observations survive the round trip, or the reproduction gate S156
// builds on them counts nothing.
func TestObservationsPersistAcrossAReload(t *testing.T) {
	probe := &fakeServiceProbe{result: harness.ServiceProbeResult{Reachable: true, Healthy: true}}
	rt, store := observeRuntime(t, probe)
	d := observableDeployment(t, store, "dep-twice")

	require.NoError(t, runObserve(t, rt, &strings.Builder{}))
	require.NoError(t, runObserve(t, rt, &strings.Builder{}))

	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.Len(t, got.Observations, 2, "each pass appends rather than replacing")
}

// An observation nobody can see is not a signal. Without this column the
// only way to learn a live service is failing is to read the JSON by
// hand, which nobody does until they already know.
func TestLiveListShowsTheLastObservation(t *testing.T) {
	probe := &fakeServiceProbe{result: harness.ServiceProbeResult{
		Reachable: true, Status: 503, Detail: "returned HTTP 503",
	}}
	rt, store := observeRuntime(t, probe)
	observableDeployment(t, store, "dep-visible")

	var out strings.Builder
	require.NoError(t, renderLiveTable(&out, []livestore.Deployment{
		{ID: "never-probed", State: livestore.StateLive, ExpiresAt: time.Now().Add(time.Hour)},
	}, nil, time.Now()))
	assert.Contains(t, out.String(), "HEALTH")
	assert.Contains(t, out.String(), "unobserved", "a record with no probes says so rather than looking fine")

	require.Error(t, runObserve(t, rt, &strings.Builder{}))

	observed, _, err := store.List()
	require.NoError(t, err)
	var table strings.Builder
	require.NoError(t, renderLiveTable(&table, observed, nil, time.Now()))
	assert.Contains(t, table.String(), string(livestore.ObservationUnhealthy))
}

// `live observe` spans every deployment, so it belongs to no one
// scenario. A blank "Scenario:" line reads as a value that failed to
// render rather than one that does not apply.
func TestHumanSummaryOmitsTheScenarioLineWhenThereIsNoScenario(t *testing.T) {
	withScenario := RenderHumanSummary(OutputResult{
		Command: "test", Scenario: "block-paris", Status: CommandStatusSuccess,
	})
	assert.Contains(t, withScenario, "Scenario: block-paris")

	spanning := RenderHumanSummary(OutputResult{
		Command: "live observe", Status: CommandStatusSuccess,
	})
	assert.NotContains(t, spanning, "Scenario:")
	assert.Contains(t, spanning, "Command: live observe")
}

// `live observe` is the command most likely to be on a cron, so it is the
// one most likely to be mid-probe when an operator runs `live teardown`.
// A read-modify-write over a slow probe would put `state: live` back over
// a record teardown had just released.
func TestObserveDoesNotResurrectARecordReleasedWhileItWasProbing(t *testing.T) {
	rt, store := observeRuntime(t, nil)
	d := observableDeployment(t, store, "dep-racing")

	// The probe stands in for a slow one: teardown lands while it runs.
	rt.Deps.ServiceProbe = &releasingProbe{store: store, id: d.ID}

	var out strings.Builder
	require.NoError(t, runObserve(t, rt, &out))

	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.Equal(t, livestore.StateReleased, got.State, "the release stands")
	assert.Empty(t, got.Observations, "and no observation is written over it")
	assert.Contains(t, out.String(), "released while it was being probed")
}

// releasingProbe releases the deployment during the probe, reproducing
// the interleaving without a sleep.
type releasingProbe struct {
	store *livestore.FilesystemStore
	id    string
}

func (p *releasingProbe) Probe(context.Context, string, int, string) (harness.ServiceProbeResult, error) {
	if err := p.store.MarkReleased(p.id); err != nil {
		return harness.ServiceProbeResult{}, err
	}
	return harness.ServiceProbeResult{Reachable: true, Healthy: true}, nil
}
