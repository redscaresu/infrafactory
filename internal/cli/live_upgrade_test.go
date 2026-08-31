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
		Deps:          RuntimeDependencies{ServiceProbe: probe, SandboxDeploy: deploy},
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
