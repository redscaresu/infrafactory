package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/livestore"
)

func learnRuntime(t *testing.T) (*CommandRuntime, *livestore.FilesystemStore, string) {
	t.Helper()
	h := newCommandTestHarness(t)
	pitfalls := filepath.Join(h.WorkspaceDir, "pitfalls")
	require.NoError(t, os.MkdirAll(pitfalls, 0o755))
	payload, err := yaml.Marshal(generator.PitfallsFile{Provider: "scaleway"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(pitfalls, "scaleway.yaml"), payload, 0o644))

	rt := &CommandRuntime{
		Config:        config.Config{Paths: config.PathsConfig{Pitfalls: pitfalls}},
		livestoreRoot: h.LivestoreRoot(),
	}
	return rt, livestore.NewFilesystemStore(h.LivestoreRoot()), pitfalls
}

type learnDeployment struct {
	id, detail, resource, cloud string
	observations                int
	drift                       bool
}

func seedLearnDeployment(t *testing.T, store *livestore.FilesystemStore, spec learnDeployment) {
	t.Helper()
	now := time.Now()
	d := livestore.Deployment{
		ID: spec.id, Scenario: "web-live-paris", Cloud: spec.cloud,
		ProjectID:       "7c98d82e-ad6d-4f4c-99ea-d1886b0f38e5",
		AddressResource: spec.resource,
		State:           livestore.StateLive,
		CreatedAt:       now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour),
	}
	for i := 0; i < spec.observations; i++ {
		o := livestore.Observation{At: now, Status: livestore.ObservationUnhealthy, Detail: spec.detail}
		if spec.drift {
			o.Status = livestore.ObservationHealthy
			o.Version = livestore.VersionUnconfirmed
		}
		d.RecordObservation(o)
	}
	require.NoError(t, store.Put(d))
}

func runLearn(t *testing.T, rt *CommandRuntime, out *strings.Builder, args ...string) error {
	t.Helper()
	cmd := &cobra.Command{Use: "learn"}
	cmd.Flags().Int("consecutive", livestore.DefaultPromotionRule.ConsecutiveProbes, "")
	cmd.Flags().Int("deployments", livestore.DefaultPromotionRule.DistinctDeployments, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	require.NoError(t, cmd.ParseFlags(args))
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetContext(context.Background())
	return runLiveLearnCommand(cmd, nil, rt)
}

func corpus(t *testing.T, dir string) []generator.PitfallEntry {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(dir, "scaleway.yaml"))
	require.NoError(t, err)
	var pf generator.PitfallsFile
	require.NoError(t, yaml.Unmarshal(payload, &pf))
	return pf.Pitfalls
}

// The loop closes here: a reproduced observation becomes a rule the
// generator will see.
func TestLearnWritesAReproducedObservationAsSourceLive(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)
	seedLearnDeployment(t, store, learnDeployment{
		id: "dep-1", detail: "health path returned HTTP 503",
		resource: "scaleway_lb_ip", cloud: "scaleway", observations: 3,
	})

	var out strings.Builder
	require.NoError(t, runLearn(t, rt, &out))

	entries := corpus(t, pitfalls)
	require.Len(t, entries, 1)
	assert.Equal(t, "scaleway_lb_ip", entries[0].Resource, "the resource the address was resolved from")
	assert.Equal(t, generator.LiveSource, entries[0].Source)
	assert.Contains(t, entries[0].Rule, "RUNNING deployment")
	assert.Contains(t, entries[0].Rule, "HTTP 503")
}

// An entry with no `last_seen` is never retired (S156a), so a live entry
// written without one would be immortal — undoing the slice built to
// bound the corpus.
func TestLearnStampsEveryEntrySoItCanBeRetired(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)
	seedLearnDeployment(t, store, learnDeployment{
		id: "dep-1", detail: "health path returned HTTP 503",
		resource: "scaleway_lb_ip", cloud: "scaleway", observations: 3,
	})

	require.NoError(t, runLearn(t, rt, &strings.Builder{}))

	entries := corpus(t, pitfalls)
	require.Len(t, entries, 1)
	require.NotEmpty(t, entries[0].LastSeen, "an unstamped live entry can never be retired")

	// And retirement can actually act on it.
	retired, err := generator.RetireStaleLivePitfalls(pitfalls, "scaleway", time.Nanosecond,
		time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.Len(t, retired, 1, "the inflow must produce something the outflow can remove")
}

// The corpus is keyed by resource and a live observation names none.
// ExtractDescriptivePitfall refuses to invent one; so does this.
func TestLearnWritesNothingWhenNoResourceCanBeAttributed(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)
	seedLearnDeployment(t, store, learnDeployment{
		id: "dep-1", detail: "health path returned HTTP 503",
		resource: "", cloud: "scaleway", observations: 3,
	})

	var out strings.Builder
	require.NoError(t, runLearn(t, rt, &out))

	assert.Empty(t, corpus(t, pitfalls), "a lesson nobody can attribute is not written")
	assert.Contains(t, out.String(), "no resource can be attributed",
		"and the operator is told, rather than the corpus looking complete")
}

// If one deployment served from an lb_ip and another from an
// instance_ip, the failure they share is not a fact about either.
func TestLearnRefusesWhenDeploymentsDisagreeOnTheResource(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)
	for _, r := range []struct{ id, resource string }{
		{"dep-1", "scaleway_lb_ip"},
		{"dep-2", "scaleway_instance_ip"},
	} {
		seedLearnDeployment(t, store, learnDeployment{
			id: r.id, detail: "health path returned HTTP 503",
			resource: r.resource, cloud: "scaleway", observations: 1,
		})
	}

	require.NoError(t, runLearn(t, rt, &strings.Builder{}))
	assert.Empty(t, corpus(t, pitfalls), "attributing it to whichever came first would be a guess")
}

// The corpus is per-cloud. A candidate spanning two is not one lesson.
func TestLearnRefusesWhenDeploymentsDisagreeOnTheCloud(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)
	for _, c := range []struct{ id, cloud string }{
		{"dep-1", "scaleway"},
		{"dep-2", "gcp"},
	} {
		seedLearnDeployment(t, store, learnDeployment{
			id: c.id, detail: "health path returned HTTP 503",
			resource: "scaleway_lb_ip", cloud: c.cloud, observations: 1,
		})
	}

	var out strings.Builder
	require.NoError(t, runLearn(t, rt, &out))
	assert.Empty(t, corpus(t, pitfalls))
	assert.Contains(t, out.String(), "do not agree on a cloud")
}

// Nothing reproduced is a normal answer, and writing nothing is correct.
func TestLearnWritesNothingWhenNothingReproduced(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)
	seedLearnDeployment(t, store, learnDeployment{
		id: "dep-1", detail: "health path returned HTTP 503",
		resource: "scaleway_lb_ip", cloud: "scaleway", observations: 1,
	})

	var out strings.Builder
	require.NoError(t, runLearn(t, rt, &out))
	assert.Empty(t, corpus(t, pitfalls))
	assert.Contains(t, out.String(), "nothing has reproduced")
}

// Writing to the corpus is the one irreversible act in this arc, so it
// can be previewed.
func TestLearnDryRunWritesNothing(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)
	seedLearnDeployment(t, store, learnDeployment{
		id: "dep-1", detail: "health path returned HTTP 503",
		resource: "scaleway_lb_ip", cloud: "scaleway", observations: 3,
	})

	var out strings.Builder
	require.NoError(t, runLearn(t, rt, &out, "--dry-run"))

	assert.Empty(t, corpus(t, pitfalls))
	assert.Contains(t, out.String(), "would learn for scaleway_lb_ip")
}

// Learning twice must refresh rather than duplicate, or the corpus grows
// once per cron tick and retention means "first observed".
func TestLearnTwiceRefreshesRatherThanDuplicating(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)
	seedLearnDeployment(t, store, learnDeployment{
		id: "dep-1", detail: "health path returned HTTP 503",
		resource: "scaleway_lb_ip", cloud: "scaleway", observations: 3,
	})

	require.NoError(t, runLearn(t, rt, &strings.Builder{}))
	first := corpus(t, pitfalls)
	require.Len(t, first, 1)

	require.NoError(t, runLearn(t, rt, &strings.Builder{}))
	assert.Len(t, corpus(t, pitfalls), 1, "the same lesson twice is one lesson")
}

// The version-drift rule says the thing that is actually surprising: the
// apply succeeded and the service did not change.
func TestLearnDescribesVersionDriftAsWhatItIs(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)
	seedLearnDeployment(t, store, learnDeployment{
		id: "dep-1", detail: `the record claims nginx:1.28 but / does not mention "1.28"`,
		resource: "scaleway_lb_ip", cloud: "scaleway", observations: 3, drift: true,
	})

	require.NoError(t, runLearn(t, rt, &strings.Builder{}))

	entries := corpus(t, pitfalls)
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Rule, "answered normally while serving a version")
	assert.Contains(t, entries[0].Rule, "does not mean the service restarted")
}

// A descriptive rule that invents a remedy is worse than one that admits
// it has none; the prescriptive form comes from an upgrade diff (S156d).
func TestLearnDoesNotInventARemedy(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)
	seedLearnDeployment(t, store, learnDeployment{
		id: "dep-1", detail: "health path returned HTTP 503",
		resource: "scaleway_lb_ip", cloud: "scaleway", observations: 3,
	})

	require.NoError(t, runLearn(t, rt, &strings.Builder{}))

	rule := corpus(t, pitfalls)[0].Rule
	for _, imperative := range []string{"you should", "add a", "set the", "use a"} {
		assert.NotContains(t, strings.ToLower(rule), imperative,
			"a descriptive rule states what was seen, not what to do")
	}
	assert.Contains(t, rule, "never confirmed", "and it says what it could not establish")
}
