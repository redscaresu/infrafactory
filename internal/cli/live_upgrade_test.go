package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
)

// stagingVersionProbe answers with whatever version is "running" now, so
// a test can change it between the before and after probes exactly as an
// apply would.
type stagingVersionProbe struct {
	running     string
	unreachable bool
	probes      int
}

func (p *stagingVersionProbe) Probe(_ context.Context, _ string, _ int, _ string) (harness.ServiceProbeResult, error) {
	p.probes++
	if p.unreachable {
		return harness.ServiceProbeResult{}, nil
	}
	return harness.ServiceProbeResult{
		Reachable: true, Healthy: true, Body: p.running, BodyComplete: true,
	}, nil
}

func upgradeRuntime(t *testing.T, probe ServiceProbeRunner, deploy *fakeSandboxDeployHarness) (*CommandRuntime, *livestore.FilesystemStore) {
	t.Helper()
	h := newCommandTestHarness(t)
	cfg, err := config.Load(h.ConfigPath)
	require.NoError(t, err)
	cfg.Validation.Layers.SandboxDeploy.Enabled = true

	rt := &CommandRuntime{
		Config:        cfg,
		livestoreRoot: h.LivestoreRoot(),
		Deps: RuntimeDependencies{
			ServiceProbe: probe, SandboxDeploy: deploy,
			// Provenance is the half local files cannot forge, so every
			// upgrade asks the API before applying.
			RunProject: &fakeRunProject{},
		},
	}
	return rt, livestore.NewFilesystemStore(h.LivestoreRoot())
}

// upgradeableDeployment writes a record plus a workdir holding the HCL it
// is currently running.
func upgradeableDeployment(t *testing.T, store *livestore.FilesystemStore, id, tag string) livestore.Deployment {
	t.Helper()
	workDir := filepath.Join(t.TempDir(), "wd")
	require.NoError(t, os.MkdirAll(workDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "providers.tf"), []byte(deployableProvidersTF), 0o644))
	// Every post-cutover deployment has one: ensureRunProject writes it
	// before the apply, and failing to is fatal there.
	require.NoError(t, harness.WriteRunProjectMarker(workDir, harness.RunProject{
		ID: "7c98d82e-ad6d-4f4c-99ea-d1886b0f38e5", Name: harness.RunProjectNamePrefix + "fixture",
	}))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "main.tf"),
		[]byte("resource \"scaleway_block_volume\" \"v\" { size_in_gb = 1 }\n"), 0o644))

	now := time.Now()
	d := livestore.Deployment{
		ID: id, Scenario: "web-live-paris",
		ProjectID:   "7c98d82e-ad6d-4f4c-99ea-d1886b0f38e5",
		Address:     "1.2.3.4",
		Port:        80,
		HealthPath:  "/",
		VersionPath: "/version",
		Image:       "nginx", Tag: tag,
		State:     livestore.StateLive,
		WorkDir:   workDir,
		CreatedAt: now.Add(-time.Hour),
		ExpiresAt: now.Add(time.Hour),
	}
	require.NoError(t, store.Put(d))
	return d
}

// deployableProvidersTF is what the Layer 3 shape gate requires of any
// configuration that reaches the real API: an exact provider pin, not a
// range. Every fixture here carries it, because an upgrade runs the same
// deny-by-default checks a first deploy runs.
const deployableProvidersTF = `terraform {
  required_version = ">= 1.6.0"

  required_providers {
    scaleway = {
      source  = "scaleway/scaleway"
      version = "2.81.0"
    }
  }
}

provider "scaleway" {
  region = "fr-par"
  zone   = "fr-par-1"
}
`

// newHCLDir is where the upgrade's replacement configuration comes from.
func newHCLDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "providers.tf"), []byte(deployableProvidersTF), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"),
		[]byte("resource \"scaleway_block_volume\" \"v\" { size_in_gb = 2 }\n"), 0o644))
	return dir
}

func runUpgrade(t *testing.T, rt *CommandRuntime, id string, out *strings.Builder, args ...string) error {
	t.Helper()
	cmd := &cobra.Command{Use: "upgrade"}
	cmd.Flags().String("from", "", "")
	cmd.Flags().String("tag", "", "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	require.NoError(t, cmd.ParseFlags(args))
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetContext(context.Background())
	return runLiveUpgradeCommand(cmd, []string{id}, rt)
}

// The whole point: a successful apply is not the service running the new
// version. The instance may not have restarted, the image may not have
// been pulled, the user data may never have run.
func TestUpgradeFailsWhenTheApplySucceedsButTheVersionDidNotChange(t *testing.T) {
	sandboxCredsForTest(t)
	probe := &stagingVersionProbe{running: "nginx/1.27.4"} // never moves to 1.28
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, probe, deploy)
	d := upgradeableDeployment(t, store, "dep-stuck", "1.27")

	var out strings.Builder
	err := runUpgrade(t, rt, d.ID, &out, "--from", newHCLDir(t), "--tag", "1.28")

	require.Error(t, err, "an apply that changed nothing is not an upgrade")
	assert.Equal(t, 1, deploy.calls, "the apply did run")
	assert.Contains(t, out.String(), "upgrade_verify")
	assert.Contains(t, out.String(), "is not the service running the new version")

	// The record moved to the new tag anyway: something was applied, and
	// a record naming the old version would send the next observation
	// looking for the wrong thing.
	got, storeErr := store.Get(d.ID)
	require.NoError(t, storeErr)
	assert.Equal(t, "1.28", got.Tag)
	assert.False(t, got.UpgradedAt.IsZero())
}

func TestUpgradeConfirmsTheNewVersionIsActuallyRunning(t *testing.T) {
	sandboxCredsForTest(t)
	probe := &stagingVersionProbe{running: "nginx/1.27.4"}
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, probe, deploy)
	d := upgradeableDeployment(t, store, "dep-rolled", "1.27")

	// The apply is what changes what is serving, so the fake moves the
	// running version at that moment rather than before it.
	deploy.onRun = func() { probe.running = "nginx/1.28.0" }

	var out strings.Builder
	require.NoError(t, runUpgrade(t, rt, d.ID, &out, "--from", newHCLDir(t), "--tag", "1.28"))

	assert.Contains(t, out.String(), "confirms nginx:1.27 before the upgrade")
	assert.Contains(t, out.String(), "now confirms nginx:1.28")
}

// Upgrading from a version the service contradicts would record a v1→v2
// transition that never happened.
func TestUpgradeRefusesWhenTheStartingVersionIsContradicted(t *testing.T) {
	sandboxCredsForTest(t)
	probe := &stagingVersionProbe{running: "SimpleHTTP/0.6 Python/3.10.12"}
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, probe, deploy)
	d := upgradeableDeployment(t, store, "dep-lying", "1.27")

	var out strings.Builder
	err := runUpgrade(t, rt, d.ID, &out, "--from", newHCLDir(t), "--tag", "1.28")

	require.Error(t, err)
	assert.Zero(t, deploy.calls, "nothing is applied on top of a state nobody understands")
	assert.Contains(t, out.String(), "would record a transition that never happened")

	got, storeErr := store.Get(d.ID)
	require.NoError(t, storeErr)
	assert.Equal(t, "1.27", got.Tag, "a refused upgrade must not move the record")
}

// The pair either side of one change is the diff S156 needs and cannot
// reconstruct.
func TestUpgradeKeepsThePreviousConfiguration(t *testing.T) {
	sandboxCredsForTest(t)
	probe := &stagingVersionProbe{running: "nginx/1.27.4"}
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, probe, deploy)
	d := upgradeableDeployment(t, store, "dep-diffable", "1.27")
	deploy.onRun = func() { probe.running = "nginx/1.28.0" }

	require.NoError(t, runUpgrade(t, rt, d.ID, &strings.Builder{},
		"--from", newHCLDir(t), "--tag", "1.28"))

	previous, err := os.ReadFile(filepath.Join(d.WorkDir, PreviousHCLDirname, "main.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(previous), "size_in_gb = 1", "the configuration that was running")

	current, err := os.ReadFile(filepath.Join(d.WorkDir, "main.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(current), "size_in_gb = 2", "and the one that replaced it")
}

// Copying over the top would leave a resource behind that the new
// configuration no longer declares, and tofu would keep managing it --
// an upgrade that silently kept something the operator deleted.
func TestUpgradeRemovesSupersededHCL(t *testing.T) {
	sandboxCredsForTest(t)
	probe := &stagingVersionProbe{running: "nginx/1.27.4"}
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, probe, deploy)
	d := upgradeableDeployment(t, store, "dep-pruned", "1.27")
	deploy.onRun = func() { probe.running = "nginx/1.28.0" }

	// A second file the replacement does not carry.
	require.NoError(t, os.WriteFile(filepath.Join(d.WorkDir, "extra.tf"),
		[]byte("resource \"scaleway_block_volume\" \"gone\" { size_in_gb = 1 }\n"), 0o644))

	require.NoError(t, runUpgrade(t, rt, d.ID, &strings.Builder{},
		"--from", newHCLDir(t), "--tag", "1.28"))

	_, err := os.Stat(filepath.Join(d.WorkDir, "extra.tf"))
	assert.True(t, os.IsNotExist(err), "a resource the new configuration drops must not stay managed")
	// But it is still recoverable for the diff.
	_, err = os.Stat(filepath.Join(d.WorkDir, PreviousHCLDirname, "extra.tf"))
	assert.NoError(t, err)
}

func TestUpgradeRefusesAReleasedDeployment(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, &stagingVersionProbe{}, deploy)
	d := upgradeableDeployment(t, store, "dep-gone", "1.27")
	require.NoError(t, store.MarkReleased(d.ID))

	err := runUpgrade(t, rt, d.ID, &strings.Builder{}, "--from", newHCLDir(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing running to upgrade")
	assert.Zero(t, deploy.calls)
}

func TestUpgradeRequiresNewConfiguration(t *testing.T) {
	sandboxCredsForTest(t)
	rt, store := upgradeRuntime(t, &stagingVersionProbe{}, &fakeSandboxDeployHarness{})
	d := upgradeableDeployment(t, store, "dep-nosource", "1.27")

	err := runUpgrade(t, rt, d.ID, &strings.Builder{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--from is required")
}

// A failure at init or plan changed nothing on the cloud. Advancing the
// tag there would make the record claim a version that was never
// deployed -- the exact falsehood S155a exists to prevent.
func TestUpgradeDoesNotAdvanceTheTagWhenTheApplyNeverRan(t *testing.T) {
	sandboxCredsForTest(t)
	probe := &stagingVersionProbe{running: "nginx/1.27.4"}
	deploy := &fakeSandboxDeployHarness{err: &harness.SandboxDeployError{
		Stage: "plan", Err: errors.New("invalid configuration"),
	}}
	rt, store := upgradeRuntime(t, probe, deploy)
	d := upgradeableDeployment(t, store, "dep-planfail", "1.27")

	require.Error(t, runUpgrade(t, rt, d.ID, &strings.Builder{},
		"--from", newHCLDir(t), "--tag", "1.28"))

	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.Equal(t, "1.27", got.Tag, "nothing was applied, so the record must not claim 1.28")
	assert.True(t, got.UpgradedAt.IsZero())
}

// A failure DURING apply may have changed a great deal, so the record
// moves: an observation looking for the old version would look for the
// wrong thing.
func TestUpgradeAdvancesTheTagWhenTheApplyFailedPartway(t *testing.T) {
	sandboxCredsForTest(t)
	probe := &stagingVersionProbe{running: "nginx/1.27.4"}
	deploy := &fakeSandboxDeployHarness{err: &harness.SandboxDeployError{
		Stage: "apply", Err: errors.New("timeout waiting for instance"),
	}}
	rt, store := upgradeRuntime(t, probe, deploy)
	d := upgradeableDeployment(t, store, "dep-partial", "1.27")

	require.Error(t, runUpgrade(t, rt, d.ID, &strings.Builder{},
		"--from", newHCLDir(t), "--tag", "1.28"))

	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.Equal(t, "1.28", got.Tag, "something may be running; the record must not hide it")
}

// Replacement HCL can recreate the load balancer. Verifying against the
// address captured at first deploy would probe infrastructure this
// deployment no longer owns, and point every later observation there too.
func TestUpgradeRefreshesTheAddressWhenTheEndpointMoves(t *testing.T) {
	sandboxCredsForTest(t)
	probe := &stagingVersionProbe{running: "nginx/1.27.4"}
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, probe, deploy)
	d := upgradeableDeployment(t, store, "dep-moved", "1.27")

	deploy.onRun = func() {
		probe.running = "nginx/1.28.0"
		writeUpgradeStateWithLBAddress(t, d.WorkDir, "9.9.9.9")
	}

	var out strings.Builder
	require.NoError(t, runUpgrade(t, rt, d.ID, &out, "--from", newHCLDir(t), "--tag", "1.28"))

	assert.Contains(t, out.String(), "moved from 1.2.3.4 to 9.9.9.9")
	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.Equal(t, "9.9.9.9", got.Address)
}

// writeUpgradeStateWithLBAddress fakes what an apply leaves behind when
// the load balancer's IP has changed.
func writeUpgradeStateWithLBAddress(t *testing.T, workDir, address string) {
	t.Helper()
	state := `{"version":4,"outputs":{},"resources":[{"type":"scaleway_lb_ip","name":"front",
	  "instances":[{"attributes":{"id":"ip-1","ip_address":"` + address + `"}}]}]}`
	require.NoError(t, os.WriteFile(
		filepath.Join(workDir, harness.LiveStateFilename), []byte(state), 0o600))
}

// replaceDeployedHCL removes the superseded .tf files before copying the
// new ones in, so a --from pointing at the workdir deletes the files it
// is about to read -- leaving no configuration at all for infrastructure
// that is still running.
func TestUpgradeRefusesASourceInsideTheDeploymentWorkdir(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, &stagingVersionProbe{running: "nginx/1.27.4"}, deploy)
	d := upgradeableDeployment(t, store, "dep-selfref", "1.27")

	for name, source := range map[string]string{
		"the workdir itself": d.WorkDir,
		"a path inside it":   filepath.Join(d.WorkDir, PreviousHCLDirname),
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, os.MkdirAll(source, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(source, "providers.tf"), []byte(deployableProvidersTF), 0o644))

			err := runUpgrade(t, rt, d.ID, &strings.Builder{}, "--from", source, "--tag", "1.28")

			require.Error(t, err)
			assert.Contains(t, err.Error(), "deployment's own workdir")
			assert.Zero(t, deploy.calls)

			// And the configuration it was running is untouched.
			_, statErr := os.Stat(filepath.Join(d.WorkDir, "main.tf"))
			assert.NoError(t, statErr, "the running configuration must survive a refused upgrade")
		})
	}
}

// An init or plan failure never reached the cloud, so the deployment is
// still running the old configuration. Leaving the rejected one in the
// workdir would make every later operation plan against something that
// was never applied.
func TestUpgradeRestoresTheOldHCLWhenNothingWasApplied(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{err: &harness.SandboxDeployError{
		Stage: "plan", Err: errors.New("invalid configuration"),
	}}
	rt, store := upgradeRuntime(t, &stagingVersionProbe{running: "nginx/1.27.4"}, deploy)
	d := upgradeableDeployment(t, store, "dep-reverted", "1.27")

	var out strings.Builder
	require.Error(t, runUpgrade(t, rt, d.ID, &out, "--from", newHCLDir(t), "--tag", "1.28"))

	current, err := os.ReadFile(filepath.Join(d.WorkDir, "main.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(current), "size_in_gb = 1",
		"the workdir must hold what is actually running, not what was rejected")
	assert.Contains(t, out.String(), "keeps the configuration it was already running")
}

// A failure DURING apply may have changed the cloud, so the new
// configuration stays: it is the one that describes what is out there.
func TestUpgradeKeepsTheNewHCLWhenTheApplyRan(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{err: &harness.SandboxDeployError{
		Stage: "apply", Err: errors.New("timeout"),
	}}
	rt, store := upgradeRuntime(t, &stagingVersionProbe{running: "nginx/1.27.4"}, deploy)
	d := upgradeableDeployment(t, store, "dep-kept", "1.27")

	require.Error(t, runUpgrade(t, rt, d.ID, &strings.Builder{}, "--from", newHCLDir(t), "--tag", "1.28"))

	current, err := os.ReadFile(filepath.Join(d.WorkDir, "main.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(current), "size_in_gb = 2",
		"something may be running from this configuration; reverting would hide it")
}

// An upgrade applies to real infrastructure exactly as a first deploy
// does. A gate that guards one entry point and not the other guards
// nothing.
func TestUpgradeRequiresTheRealDeployOptIn(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, &stagingVersionProbe{running: "nginx/1.27.4"}, deploy)
	rt.Config.Validation.Layers.SandboxDeploy.Enabled = false
	d := upgradeableDeployment(t, store, "dep-optout", "1.27")

	err := runUpgrade(t, rt, d.ID, &strings.Builder{}, "--from", newHCLDir(t), "--tag", "1.28")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox_deploy.enabled")
	assert.Zero(t, deploy.calls, "Layer 3 is off, so nothing reaches the real project")

	// And the workdir is untouched: the refusal came before the swap.
	current, readErr := os.ReadFile(filepath.Join(d.WorkDir, "main.tf"))
	require.NoError(t, readErr)
	assert.Contains(t, string(current), "size_in_gb = 1")
}

// Everything that can fail without touching the workdir happens before
// the destructive swap, so an environment problem cannot leave the
// workdir holding configuration that was never applied.
func TestUpgradeLeavesTheWorkdirAloneWhenTheEnvironmentIsUnusable(t *testing.T) {
	sandboxCredsForTest(t)
	// No organisation id: sandboxCommandEnvForProject refuses.
	t.Setenv("SCW_DEFAULT_ORGANIZATION_ID", "")
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, &stagingVersionProbe{running: "nginx/1.27.4"}, deploy)
	d := upgradeableDeployment(t, store, "dep-noenv", "1.27")

	require.Error(t, runUpgrade(t, rt, d.ID, &strings.Builder{},
		"--from", newHCLDir(t), "--tag", "1.28"))

	assert.Zero(t, deploy.calls)
	current, err := os.ReadFile(filepath.Join(d.WorkDir, "main.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(current), "size_in_gb = 1",
		"a failure before the apply must not leave unapplied configuration behind")

	got, storeErr := store.Get(d.ID)
	require.NoError(t, storeErr)
	assert.Equal(t, "1.27", got.Tag)
}

// The record's project id is the half a stale or edited file can change.
// This call applies REAL infrastructure into whatever it names, so a
// disagreement with the marker is refused rather than resolved in the
// record's favour.
func TestUpgradeRefusesWhenTheRecordAndMarkerDisagreeOnTheProject(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, &stagingVersionProbe{running: "nginx/1.27.4"}, deploy)
	d := upgradeableDeployment(t, store, "dep-mismatch", "1.27")
	require.NoError(t, harness.WriteRunProjectMarker(d.WorkDir, harness.RunProject{
		ID: "99999999-9999-9999-9999-999999999999", Name: harness.RunProjectNamePrefix + "other",
	}))

	err := runUpgrade(t, rt, d.ID, &strings.Builder{}, "--from", newHCLDir(t), "--tag", "1.28")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "could change infrastructure belonging to another deployment")
	assert.Zero(t, deploy.calls)

	current, readErr := os.ReadFile(filepath.Join(d.WorkDir, "main.tf"))
	require.NoError(t, readErr)
	assert.Contains(t, string(current), "size_in_gb = 1", "refused before the swap")
}

// Without --tag the record still names the old version, so confirming it
// proves the service is running what the record says and proves nothing
// about a transition. Calling that "upgraded" would be a green built from
// checking that nothing changed.
func TestUpgradeWithoutANewTagDoesNotClaimAVersionChange(t *testing.T) {
	sandboxCredsForTest(t)
	probe := &stagingVersionProbe{running: "nginx/1.27.4"}
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, probe, deploy)
	d := upgradeableDeployment(t, store, "dep-notag", "1.27")

	var out strings.Builder
	require.NoError(t, runUpgrade(t, rt, d.ID, &out, "--from", newHCLDir(t)))

	assert.Contains(t, out.String(), "unchanged rather than upgraded")
	assert.NotContains(t, out.String(), "now confirms")
}

// Required, not merely preferred. Falling back to the record would leave
// the editable half deciding where real infrastructure gets applied.
//
// `live teardown` does fall back, deliberately: refusing there strands a
// pre-cutover record whose resources are real, and destroy is bounded by
// its own state anyway. Neither argument holds for applying, and an
// operator who cannot upgrade can still tear down and deploy again.
func TestUpgradeRefusesWithoutAReadableMarker(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, &stagingVersionProbe{running: "nginx/1.27.4"}, deploy)
	d := upgradeableDeployment(t, store, "dep-unmarked", "1.27")
	require.NoError(t, os.Remove(filepath.Join(d.WorkDir, harness.RunProjectMarkerFilename)))

	err := runUpgrade(t, rt, d.ID, &strings.Builder{}, "--from", newHCLDir(t), "--tag", "1.28")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "which is editable")
	assert.Zero(t, deploy.calls)
}

// The marker and the record are both local files, so together they prove
// only that two local files agree. Provenance asks the API, which is the
// half that cannot be forged from a text editor.
func TestUpgradeRefusesAProjectTheAPIDoesNotVouchFor(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, &stagingVersionProbe{running: "nginx/1.27.4"}, deploy)
	d := upgradeableDeployment(t, store, "dep-unstamped", "1.27")

	// A real project that is not one of ours: exactly what editing two
	// local files would point an apply at.
	rt.Deps.RunProject = &fakeRunProject{describeUnstamped: true}

	err := runUpgrade(t, rt, d.ID, &strings.Builder{}, "--from", newHCLDir(t), "--tag", "1.28")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not carry infrafactory's stamp")
	assert.NotContains(t, err.Error(), "refusing to delete",
		"the message must describe the operation being refused")
	assert.Zero(t, deploy.calls)
}

// "We could not check" must never behave like "it is fine".
func TestUpgradeRefusesWhenProvenanceCannotBeChecked(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, &stagingVersionProbe{running: "nginx/1.27.4"}, deploy)
	d := upgradeableDeployment(t, store, "dep-unreachable-api", "1.27")
	rt.Deps.RunProject = &fakeRunProject{describeErr: errors.New("dial tcp: connection refused")}

	err := runUpgrade(t, rt, d.ID, &strings.Builder{}, "--from", newHCLDir(t), "--tag", "1.28")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not verify project")
	assert.Zero(t, deploy.calls)
}

// Applying into a project that is already gone is not an upgrade. The
// deletion guard treats gone as success; this is the opposite case.
func TestUpgradeRefusesAProjectThatNoLongerExists(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, &stagingVersionProbe{running: "nginx/1.27.4"}, deploy)
	d := upgradeableDeployment(t, store, "dep-vanished", "1.27")
	rt.Deps.RunProject = &fakeRunProject{describeGone: true}

	err := runUpgrade(t, rt, d.ID, &strings.Builder{}, "--from", newHCLDir(t), "--tag", "1.28")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no longer exists")
	assert.Zero(t, deploy.calls)
}

// An upgrade holds its record across a real apply -- minutes, not
// microseconds -- so a teardown finishing in that window would be
// overwritten by the upgrade's write, resurrecting a deployment that is
// already gone.
func TestUpgradeDoesNotResurrectARecordTornDownWhileApplying(t *testing.T) {
	sandboxCredsForTest(t)
	probe := &stagingVersionProbe{running: "nginx/1.27.4"}
	deploy := &fakeSandboxDeployHarness{}
	rt, store := upgradeRuntime(t, probe, deploy)
	d := upgradeableDeployment(t, store, "dep-racing-upgrade", "1.27")

	// Teardown lands while the apply is running.
	deploy.onRun = func() { require.NoError(t, store.MarkReleased(d.ID)) }

	var out strings.Builder
	require.Error(t, runUpgrade(t, rt, d.ID, &out, "--from", newHCLDir(t), "--tag", "1.28"))

	got, err := store.Get(d.ID)
	require.NoError(t, err)
	assert.Equal(t, livestore.StateReleased, got.State, "the teardown stands")
	assert.Equal(t, "1.27", got.Tag, "and the upgrade does not write over it")
	assert.Contains(t, out.String(), "was torn down while this upgrade was applying")
	assert.Contains(t, out.String(), "check project")
}
