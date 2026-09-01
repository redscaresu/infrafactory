package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/livestore"
)

// repairWorkDir lays out the before/after pair S155b leaves behind: the
// configuration that was running, stashed under .infrafactory-previous,
// and the one that replaced it.
func repairWorkDir(t *testing.T, before, after string) string {
	t.Helper()
	work := t.TempDir()
	previous := filepath.Join(work, PreviousHCLDirname)
	require.NoError(t, os.MkdirAll(previous, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(previous, "main.tf"), []byte(before), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(work, "main.tf"), []byte(after), 0o644))
	return work
}

func seedRepair(t *testing.T, store *livestore.FilesystemStore, workDir string, healthyAfter int) {
	t.Helper()
	// The detail a REAL ServiceProbe produces: it names no Terraform
	// address and no attribute, which is the whole attribution problem.
	seedRepairWithDetail(t, store, workDir, healthyAfter,
		"health path http://51.15.0.1/healthz returned HTTP 503")
}

func seedRepairWithDetail(t *testing.T, store *livestore.FilesystemStore, workDir string, healthyAfter int, detail string) {
	t.Helper()
	base := time.Now().Add(-time.Hour)
	d := livestore.Deployment{
		ID: "dep-repair", Scenario: "web-live-paris", Cloud: "scaleway",
		ProjectID: "proj-1", WorkDir: workDir,
		AddressResource: "scaleway_lb_ip",
		State:           livestore.StateLive,
		CreatedAt:       base.Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour),
		UpgradedAt:       base,
		UpgradeStartedAt: base.Add(-30 * time.Second),
		UpgradeSucceeded: true,
	}
	d.Observations = append(d.Observations, livestore.Observation{
		At: base.Add(-time.Minute), Status: livestore.ObservationUnhealthy, Detail: detail,
	})
	for i := 1; i <= healthyAfter; i++ {
		d.Observations = append(d.Observations, livestore.Observation{
			At: base.Add(time.Duration(i) * time.Minute), Status: livestore.ObservationHealthy,
			Version: livestore.VersionConfirmed,
		})
	}
	require.NoError(t, store.Put(d))
}

const backendBefore = `resource "scaleway_lb_backend" "app" {
  name             = "app"
  forward_protocol = "http"
  forward_port     = 80
}
`

const backendAfter = `resource "scaleway_lb_backend" "app" {
  name             = "app"
  forward_protocol = "http"
  forward_port     = 80
  health_check_http {
    uri = "/healthz"
  }
}
`

// The point of the slice: an upgrade that demonstrably fixed something
// becomes a rule that says what to DO, not merely what went wrong.
func TestLearnWritesARemedyWhenAnUpgradeClearedAFailure(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)
	seedRepair(t, store, repairWorkDir(t, backendBefore, backendAfter), 2)

	var out strings.Builder
	require.NoError(t, runLearn(t, rt, &out))

	entries := corpus(t, pitfalls)
	require.NotEmpty(t, entries)

	var remedy string
	for _, e := range entries {
		if strings.Contains(e.Rule, "health_check_http") {
			remedy = e.Rule
		}
	}
	require.NotEmpty(t, remedy, "the rule must carry the configuration that fixed it: %+v", entries)
	assert.Contains(t, remedy, "cleared", "and the evidence that it did")
	assert.Contains(t, out.String(), "remedy")
}

// Retirable, not immortal. ADR-0019's `fix` entries are permanent; this
// describes a RUNNING service, and what is true of one stops being true.
func TestLearnedRemediesStayRetirable(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)
	seedRepair(t, store, repairWorkDir(t, backendBefore, backendAfter), 2)

	require.NoError(t, runLearn(t, rt, &strings.Builder{}))

	for _, e := range corpus(t, pitfalls) {
		assert.Equal(t, "live", e.Source,
			"a remedy tagged `fix` could never be retired, breaking S156c's rule")
		assert.NotEmpty(t, e.LastSeen, "retirement acts on last_seen")
		assert.NotEmpty(t, e.ObservedKey)
	}
}

// One healthy probe is a lucky sample. The remedy waits.
func TestLearnWritesNoRemedyOnASingleHealthyProbe(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)
	seedRepair(t, store, repairWorkDir(t, backendBefore, backendAfter), 1)

	require.NoError(t, runLearn(t, rt, &strings.Builder{}))
	assert.Empty(t, corpus(t, pitfalls))
}

// A repair whose configurations cannot be told apart yields no remedy,
// and says so rather than going quiet.
func TestLearnSaysSoWhenARepairCannotBeAttributed(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)
	seedRepair(t, store, repairWorkDir(t, backendBefore, backendBefore), 2)

	var out strings.Builder
	require.NoError(t, runLearn(t, rt, &out))

	assert.Empty(t, corpus(t, pitfalls))
	assert.Contains(t, out.String(), "no single attributable change was found")
}

// Several resources changing is an ordinary upgrade, not a fault -- but
// the diff cannot say which one cleared the failure, and picking one
// would be a guess presented as a finding.
func TestLearnWritesNoRemedyWhenSeveralResourcesChanged(t *testing.T) {
	before := backendBefore + `
resource "scaleway_lb_frontend" "app" {
  inbound_port = 80
}
`
	after := backendAfter + `
resource "scaleway_lb_frontend" "app" {
  inbound_port = 443
}
`
	rt, store, pitfalls := learnRuntime(t)
	seedRepair(t, store, repairWorkDir(t, before, after), 2)

	var out strings.Builder
	require.NoError(t, runLearn(t, rt, &out))

	assert.Empty(t, corpus(t, pitfalls))
	assert.Contains(t, out.String(), "no single attributable change was found")
}

// --dry-run must not write a remedy either. Writing to the corpus is the
// one irreversible act in this arc.
func TestLearnDryRunWritesNoRemedy(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)
	seedRepair(t, store, repairWorkDir(t, backendBefore, backendAfter), 2)

	var out strings.Builder
	require.NoError(t, runLearn(t, rt, &out, "--dry-run"))

	assert.Empty(t, corpus(t, pitfalls))
	assert.Contains(t, out.String(), "would learn a REMEDY")
}

// A deletion is a change. An upgrade that removes one resource and
// modifies another is not "exactly one changed", and attributing the fix
// to whichever survived would be a guess -- deletion-as-fix is a real
// shape, which is why ADR-0019 has `avoid` entries at all.
func TestLearnWritesNoRemedyWhenAnUpgradeAlsoDeletedAResource(t *testing.T) {
	before := backendBefore + `
resource "scaleway_lb_frontend" "legacy" {
  inbound_port = 8080
}
`
	rt, store, pitfalls := learnRuntime(t)
	seedRepair(t, store, repairWorkDir(t, before, backendAfter), 2)

	var out strings.Builder
	require.NoError(t, runLearn(t, rt, &out))

	assert.Empty(t, corpus(t, pitfalls),
		"the deletion may be what cleared the failure; the diff cannot say")
	assert.Contains(t, out.String(), "no single attributable change was found")
}

// A record naming no cloud cannot be filed anywhere. It must skip the
// way the descriptive path does, not fail the whole command over one
// partial record -- `Cloud` is optional on the schema and older records
// really do lack it.
func TestLearnSkipsARepairedRecordWithNoCloud(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)
	seedRepair(t, store, repairWorkDir(t, backendBefore, backendAfter), 2)

	d, err := store.Get("dep-repair")
	require.NoError(t, err)
	d.Cloud = ""
	require.NoError(t, store.Put(d))

	var out strings.Builder
	require.NoError(t, runLearn(t, rt, &out), "one partial record must not fail the command")

	assert.Empty(t, corpus(t, pitfalls))
	assert.Contains(t, out.String(), "names no cloud")
}

// A repair is not always an addition. An upgrade that REMOVES something
// is just as much a fix, and ADR-0019 has a vocabulary for it.
//
// The avoid extractor attributes strictly -- the removed attribute must
// be NAMED in the failure detail, a rule added after a false positive in
// S63 -- so this fires only when the detail carries it.
func TestLearnWritesARemedyWhenTheFixWasARemovalTheFailureNamed(t *testing.T) {
	before := `resource "scaleway_lb_backend" "app" {
  forward_port     = 80
  sticky_sessions  = "cookie"
}
`
	after := `resource "scaleway_lb_backend" "app" {
  forward_port = 80
}
`
	rt, store, pitfalls := learnRuntime(t)
	seedRepairWithDetail(t, store, repairWorkDir(t, before, after), 2,
		"backend rejected requests: sticky_sessions requires a cookie name")

	var out strings.Builder
	require.NoError(t, runLearn(t, rt, &out))

	entries := corpus(t, pitfalls)
	require.NotEmpty(t, entries, "a removal that cleared a live failure is a lesson")
	assert.Contains(t, entries[0].Rule, "sticky_sessions")
	assert.Equal(t, "live", entries[0].Source, "still retirable, like every live entry")
}

// And the limit, stated as a test rather than left to be discovered: a
// health probe says `returned HTTP 503` and names no attribute, so the
// avoid extractor's strict attribution cannot fire. The run loop depends
// on that strictness, so loosening it to suit the live path is not a
// trade this slice is entitled to make.
func TestLearnCannotAttributeARemovalTheProbeDidNotName(t *testing.T) {
	before := `resource "scaleway_lb_backend" "app" {
  forward_port    = 80
  sticky_sessions = "cookie"
}
`
	after := `resource "scaleway_lb_backend" "app" {
  forward_port = 80
}
`
	rt, store, pitfalls := learnRuntime(t)
	seedRepair(t, store, repairWorkDir(t, before, after), 2)

	var out strings.Builder
	require.NoError(t, runLearn(t, rt, &out))

	assert.Empty(t, corpus(t, pitfalls))
	assert.Contains(t, out.String(), "could not be turned into a rule")
}

// An ABSENT "before" is not an empty one. `loadResourceBlocks` returns an
// empty map for a missing directory, so without this check every resource
// in the current configuration looks newly added and the corpus gains a
// rule claiming the whole configuration was the remedy.
func TestLearnWritesNoRemedyWithoutTheStashedPreviousConfiguration(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(work, "main.tf"), []byte(backendAfter), 0o644))

	rt, store, pitfalls := learnRuntime(t)
	seedRepair(t, store, work, 2)

	var out strings.Builder
	require.NoError(t, runLearn(t, rt, &out))

	assert.Empty(t, corpus(t, pitfalls),
		"a rule claiming the entire configuration was the fix is corpus corruption")
	assert.Contains(t, out.String(), "nothing to compare against")
}

// Two upgrades can clear the same symptom by different changes, and
// those are two lessons. Keyed on the symptom alone the second would
// silently refresh over the first.
func TestLearnKeepsTwoDifferentRemediesForTheSameSymptom(t *testing.T) {
	rt, store, pitfalls := learnRuntime(t)

	base := time.Now().Add(-time.Hour)
	for i, spec := range []struct{ before, after string }{
		{backendBefore, backendAfter},
		{backendBefore, `resource "scaleway_lb_backend" "app" {
  name             = "app"
  forward_protocol = "http"
  forward_port     = 8080
}
`},
	} {
		work := repairWorkDir(t, spec.before, spec.after)
		d := livestore.Deployment{
			ID: fmt.Sprintf("dep-%d", i), Scenario: "web-live-paris", Cloud: "scaleway",
			ProjectID: "proj-1", WorkDir: work, AddressResource: "scaleway_lb_ip",
			State:     livestore.StateLive,
			CreatedAt: base.Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour),
			UpgradedAt: base, UpgradeStartedAt: base.Add(-30 * time.Second),
			UpgradeSucceeded: true,
		}
		d.Observations = append(d.Observations, livestore.Observation{
			At: base.Add(-time.Minute), Status: livestore.ObservationUnhealthy,
			Detail: "health path http://51.15.0.1/healthz returned HTTP 503",
		})
		for n := 1; n <= 2; n++ {
			d.Observations = append(d.Observations, livestore.Observation{
				At: base.Add(time.Duration(n) * time.Minute), Status: livestore.ObservationHealthy,
				Version: livestore.VersionConfirmed,
			})
		}
		require.NoError(t, store.Put(d))
	}

	require.NoError(t, runLearn(t, rt, &strings.Builder{}))

	// Three entries, not two: both deployments reported the same symptom,
	// so the DESCRIPTIVE path correctly promotes it on breadth as well.
	// Count the remedies specifically.
	remedies := 0
	for _, e := range corpus(t, pitfalls) {
		if strings.Contains(e.Rule, "this configuration change cleared") {
			remedies++
		}
	}
	assert.Equal(t, 2, remedies,
		"one symptom, two remedies, and neither may overwrite the other")
}
