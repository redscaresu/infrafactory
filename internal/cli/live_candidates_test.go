package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/livestore"
)

func candidatesRuntime(t *testing.T) (*CommandRuntime, *livestore.FilesystemStore) {
	t.Helper()
	h := newCommandTestHarness(t)
	return &CommandRuntime{livestoreRoot: h.LivestoreRoot()},
		livestore.NewFilesystemStore(h.LivestoreRoot())
}

func failingDeployment(t *testing.T, store *livestore.FilesystemStore, id, detail string, n int) {
	t.Helper()
	now := time.Now()
	d := livestore.Deployment{
		ID: id, Scenario: "web-live-paris",
		ProjectID: "7c98d82e-ad6d-4f4c-99ea-d1886b0f38e5",
		State:     livestore.StateLive,
		CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour),
	}
	for i := 0; i < n; i++ {
		d.RecordObservation(livestore.Observation{
			At: now, Status: livestore.ObservationUnhealthy, Detail: detail,
		})
	}
	require.NoError(t, store.Put(d))
}

func runCandidates(t *testing.T, rt *CommandRuntime, out *strings.Builder, args ...string) error {
	t.Helper()
	cmd := &cobra.Command{Use: "candidates"}
	cmd.Flags().Int("consecutive", livestore.DefaultPromotionRule.ConsecutiveProbes, "")
	cmd.Flags().Int("deployments", livestore.DefaultPromotionRule.DistinctDeployments, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	require.NoError(t, cmd.ParseFlags(args))
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetContext(context.Background())
	return runLiveCandidatesCommand(cmd, nil, rt)
}

// The gate's verdict has to be visible before anything acts on it: a
// promotion mechanism whose output only appears once it has written to
// the corpus is one nobody can calibrate.
func TestCandidatesReportsWhatPromotedAndWhy(t *testing.T) {
	rt, store := candidatesRuntime(t)
	failingDeployment(t, store, "dep-1", "returned HTTP 502", 3)

	var out strings.Builder
	require.NoError(t, runCandidates(t, rt, &out))

	assert.Contains(t, out.String(), "1 observation(s) have reproduced")
	assert.Contains(t, out.String(), "persistent")
	assert.Contains(t, out.String(), "longest run 3")
	assert.Contains(t, out.String(), "HTTP 502", "the real words, not just the normalized form")
}

// A lesson blamed on a version nobody verified is a falsehood, so the
// report says so rather than leaving it to be assumed.
func TestCandidatesSaysWhenNothingMayBeBlamedOnATag(t *testing.T) {
	rt, store := candidatesRuntime(t)
	failingDeployment(t, store, "dep-1", "returned HTTP 502", 3)

	var out strings.Builder
	require.NoError(t, runCandidates(t, rt, &out))

	assert.Contains(t, out.String(), "version UNCONFIRMED")
	assert.Contains(t, out.String(), "nothing may be blamed on a tag")
}

// Nothing reproduced is a normal, successful answer — and it states the
// threshold, so a reader can tell "nothing is wrong" from "the bar is
// set too high".
func TestCandidatesStatesTheThresholdWhenNothingReproduced(t *testing.T) {
	rt, store := candidatesRuntime(t)
	failingDeployment(t, store, "dep-1", "returned HTTP 502", 1)

	var out strings.Builder
	require.NoError(t, runCandidates(t, rt, &out))

	assert.Contains(t, out.String(), "nothing has reproduced")
	assert.Contains(t, out.String(), "3 consecutive probes, or 2 distinct deployments")
}

func TestCandidatesHonoursThresholdFlags(t *testing.T) {
	rt, store := candidatesRuntime(t)
	failingDeployment(t, store, "dep-1", "returned HTTP 502", 2)

	var out strings.Builder
	require.NoError(t, runCandidates(t, rt, &out, "--consecutive", "2"))
	assert.Contains(t, out.String(), "1 observation(s) have reproduced")

	var stricter strings.Builder
	require.NoError(t, runCandidates(t, rt, &stricter, "--consecutive", "5"))
	assert.Contains(t, stricter.String(), "nothing has reproduced")
}

// Grouping goes through the same normalizer the run loop uses, so one
// problem with shifting line numbers reproduces with itself.
func TestCandidatesGroupsThroughTheSharedNormalizer(t *testing.T) {
	rt, store := candidatesRuntime(t)
	// Two deployments, same failure, different line references — the
	// exact variation feedback.NormalizeDetail exists to collapse.
	failingDeployment(t, store, "dep-1",
		"exit status 1 | stderr: Error: backend unhealthy on line 12", 1)
	failingDeployment(t, store, "dep-2",
		"exit status 1 | stderr: Error: backend unhealthy on line 47", 1)

	var out strings.Builder
	require.NoError(t, runCandidates(t, rt, &out))

	assert.Contains(t, out.String(), "1 observation(s) have reproduced",
		"line numbers must not make one problem look like two")
	assert.Contains(t, out.String(), "across 2 deployment(s)")
}

// "healthy" alone would read as the opposite of a finding, and version
// drift is the shape every other signal in the system already reports as
// fine. The report has to name it.
func TestCandidatesNamesVersionDriftRatherThanCallingItHealthy(t *testing.T) {
	rt, store := candidatesRuntime(t)

	now := time.Now()
	d := livestore.Deployment{
		ID: "dep-drift", Scenario: "web-live-paris",
		ProjectID: "7c98d82e-ad6d-4f4c-99ea-d1886b0f38e5",
		State:     livestore.StateLive,
		CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour),
	}
	for i := 0; i < 3; i++ {
		d.RecordObservation(livestore.Observation{
			At: now, Status: livestore.ObservationHealthy,
			Version: livestore.VersionUnconfirmed,
			Detail:  `the record claims nginx:1.28 but / does not mention "1.28"`,
		})
	}
	require.NoError(t, store.Put(d))

	var out strings.Builder
	require.NoError(t, runCandidates(t, rt, &out))

	assert.Contains(t, out.String(), "version drift (service healthy)")
	assert.Contains(t, out.String(), "does not mention")
	assert.NotContains(t, out.String(), "nothing has reproduced")
}
