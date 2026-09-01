package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
)

// The live commands are coupled through a RECORD, not through function
// calls. `deploy` writes it, `live observe` appends to it, `live upgrade`
// rewrites three of its fields, `live teardown` releases it, and
// `live reap` acts on whatever it finds.
//
// Every command's own tests build the record they read. That is exactly
// the shape that hides defects, and this arc produced three:
//
//   - `observe` failed on a record `deploy` had legitimately written
//     without an address (S154 pass 44)
//   - `upgrade` wrote back a record it had read before a minutes-long
//     apply, discarding observations appended in between (S155b pass 57)
//   - the record's project id and the marker's could disagree, and three
//     passes were needed to settle which an apply may trust (passes 53-55)
//
// None could have been caught by a unit test, because each is an
// interaction between two commands. This is the test that would have.
//
// It cannot run against mockway: assertRealScalewayEndpoint refuses any
// Layer 3 apply not pointed at api.scaleway.com, checking the passed env,
// the inherited SCW_API_URL and the scw config file. That is ADR-0023
// containment and pointing this test at a mock would mean weakening it.
// So the CLOUD is faked and everything that touches the record is real.

// lifecycleRig wires the real commands and the real livestore to fake
// cloud harnesses.
type lifecycleRig struct {
	t        *testing.T
	runtime  *CommandRuntime
	store    *livestore.FilesystemStore
	deploy   *fakeSandboxDeployHarness
	destroy  *fakeSandboxDestroyHarness
	sweep    *fakeOrphanSweep
	projects *fakeRunProject
	probe    *stagingVersionProbe
	scenario string
	source   string
}

func newLifecycleRig(t *testing.T) *lifecycleRig {
	t.Helper()
	sandboxCredsForTest(t)
	h := newCommandTestHarness(t)

	cfg, err := config.Load(h.ConfigPath)
	require.NoError(t, err)
	cfg.Paths.Output = h.OutputDir()
	cfg.Validation.Layers.SandboxDeploy.Enabled = true

	rig := &lifecycleRig{
		t:        t,
		store:    livestore.NewFilesystemStore(h.LivestoreRoot()),
		deploy:   &fakeSandboxDeployHarness{},
		destroy:  &fakeSandboxDestroyHarness{result: &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}}},
		sweep:    &fakeOrphanSweep{},
		projects: &fakeRunProject{created: harness.RunProject{ID: lifecycleProjectID, Name: harness.RunProjectNamePrefix + "journey"}},
		probe:    &stagingVersionProbe{running: "nginx/1.27.0"},
	}
	rig.runtime = &CommandRuntime{
		Config:         cfg,
		scenarioLoader: defaultScenarioLoader,
		livestoreRoot:  h.LivestoreRoot(),
		Deps: RuntimeDependencies{
			SandboxDeploy:  rig.deploy,
			SandboxDestroy: rig.destroy,
			OrphanSweep:    rig.sweep,
			RunProject:     rig.projects,
			ServiceProbe:   rig.probe,
		},
	}

	rig.scenario = filepath.Join(h.WorkspaceDir, "web-live-paris.yaml")
	require.NoError(t, os.WriteFile(rig.scenario, []byte(lifecycleScenarioYAML), 0o600))
	_, err = rig.runtime.LoadScenario(rig.scenario)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(rig.runtime.OutputDir(), 0o755))

	// The HCL `deploy` will copy into the deployment's own workdir.
	writeLifecycleHCL(t, rig.runtime.OutputDir(), 1)
	rig.source = t.TempDir()
	writeLifecycleHCL(t, rig.source, 2)

	// A real apply leaves state naming the load balancer, and `deploy`
	// reads the address out of it. Without this the fake produces a
	// deployment with no endpoint -- a legitimate shape, but a different
	// journey from the one under test, and one the second test covers
	// deliberately.
	//
	// Keyed off the workdir the apply is GIVEN, because a first deploy
	// picks its own and nothing can name it in advance.
	rig.deploy.onRunDir = func(workDir string) {
		writeUpgradeStateWithLBAddress(t, workDir, lifecycleAddress)
	}

	return rig
}

const lifecycleAddress = "51.15.0.1"

const lifecycleProjectID = "7c98d82e-ad6d-4f4c-99ea-d1886b0f38e5"

const lifecycleScenarioYAML = `scenario: web-live-paris
version: "1.0"
cloud: scaleway
description: >
  The live journey: deploy, observe, upgrade, observe again, tear down.
service:
  image: nginx
  tag: "1.27"
  port: 80
  health_path: /
  version_path: /version
  ttl: 4h
resources:
  compute:
    purpose: web-server
    size: small
    count: 1
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`

func writeLifecycleHCL(t *testing.T, dir string, sizeGB int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "providers.tf"), []byte(deployableProvidersTF), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"),
		[]byte(fmt.Sprintf("resource \"scaleway_block_volume\" \"v\" { size_in_gb = %d }\n", sizeGB)), 0o644))
}

func (r *lifecycleRig) run(args ...string) (string, error) {
	r.t.Helper()
	cmd := &cobra.Command{Use: args[0]}
	cmd.Flags().String("output", string(OutputModeHuman), "")
	cmd.Flags().String("ttl", "", "")
	cmd.Flags().String("from", "", "")
	cmd.Flags().String("tag", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	require.NoError(r.t, cmd.ParseFlags(args[1:]))
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(context.Background())

	positional := cmd.Flags().Args()
	var err error
	switch args[0] {
	case "deploy":
		err = runDeployCommand(cmd, []string{r.scenario}, r.runtime)
	case "ls":
		err = runLiveListCommand(cmd, nil, r.runtime)
	case "observe":
		err = runLiveObserveCommand(cmd, nil, r.runtime)
	case "upgrade":
		err = runLiveUpgradeCommand(cmd, positional, r.runtime)
	case "teardown":
		err = runLiveTeardownCommand(cmd, positional, r.runtime)
	case "reap":
		err = runLiveReapCommand(cmd, nil, r.runtime)
	default:
		r.t.Fatalf("unknown command %q", args[0])
	}
	return out.String(), err
}

func (r *lifecycleRig) only() livestore.Deployment {
	r.t.Helper()
	ds, unreadable, err := r.store.List()
	require.NoError(r.t, err)
	require.Empty(r.t, unreadable)
	require.Len(r.t, ds, 1)
	return ds[0]
}

// TestLiveLifecycleJourney runs the whole live sequence through the real
// commands and the real record.
//
// The assertions are deliberately about what each step LEAVES for the
// next one, rather than about each step in isolation — the isolation
// cases already have their own tests, and they are not where the defects
// were.
func TestLiveLifecycleJourney(t *testing.T) {
	rig := newLifecycleRig(t)

	// 1. deploy — the record is created, with everything later steps read.
	out, err := rig.run("deploy", "--ttl", "30m")
	require.NoError(t, err, out)

	d := rig.only()
	assert.Equal(t, livestore.StateLive, d.State)
	assert.Equal(t, lifecycleProjectID, d.ProjectID)
	assert.Equal(t, 80, d.Port, "observe reads this")
	assert.Equal(t, "/", d.HealthPath, "observe reads this")
	assert.Equal(t, "/version", d.VersionPath, "the version check reads this")
	assert.Equal(t, "nginx", d.Image)
	assert.Equal(t, "1.27", d.Tag)
	assert.False(t, d.ExpiresAt.IsZero(), "the reaper reads this")

	// The marker `upgrade` and `teardown` will demand.
	marker, err := harness.ReadRunProjectMarker(d.WorkDir)
	require.NoError(t, err, "deploy must leave the marker every later command requires")
	assert.Equal(t, d.ProjectID, marker.ProjectID, "record and marker must agree from the start")

	// 2. ls — before anything has looked, health is `unobserved`, not blank.
	out, err = rig.run("ls")
	require.NoError(t, err, out)
	assert.Contains(t, out, d.ID)
	assert.Contains(t, out, "unobserved", "silence must not read as healthy")

	// 3. observe — an observation lands on the record deploy wrote.
	out, err = rig.run("observe")
	require.NoError(t, err, out)
	assert.Contains(t, out, "confirms nginx:1.27")

	d = rig.only()
	require.Len(t, d.Observations, 1)
	assert.Equal(t, livestore.ObservationHealthy, d.Observations[0].Status)
	assert.Equal(t, livestore.VersionConfirmed, d.Observations[0].Version)

	// 4. upgrade — and an observation lands DURING the apply, which is
	//    the pass-57 defect: writing back a stale copy would drop it.
	// onRunDir fires first, so the applied state exists before the
	// concurrent observation reads the record.
	//
	// The observation happens BEFORE the served version flips, on
	// purpose. An `observe` that lands after the service has moved but
	// before the upgrade has written the new tag sees the record and the
	// world genuinely disagree, and reports `unconfirmed` — correct, and
	// a real transient during any upgrade. Folding it into this step
	// would test two things at once; it is noted here because it is
	// exactly the single-blip noise S156b's reproduction gate exists to
	// filter out.
	rig.deploy.onRun = func() {
		_, obsErr := rig.run("observe")
		require.NoError(t, obsErr)
		rig.probe.running = "nginx/1.28.0"
	}
	out, err = rig.run("upgrade", d.ID, "--from", rig.source, "--tag", "1.28")
	require.NoError(t, err, out)
	assert.Contains(t, out, "confirms nginx:1.27 before the upgrade")
	assert.Contains(t, out, "now confirms nginx:1.28")

	d = rig.only()
	assert.Equal(t, "1.28", d.Tag)
	assert.False(t, d.UpgradedAt.IsZero())
	assert.Len(t, d.Observations, 2,
		"the observation recorded during the apply must survive the upgrade's write")

	// The superseded configuration is kept — the diff S156d needs.
	previous, err := os.ReadFile(filepath.Join(d.WorkDir, PreviousHCLDirname, "main.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(previous), "size_in_gb = 1")
	current, err := os.ReadFile(filepath.Join(d.WorkDir, "main.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(current), "size_in_gb = 2")

	// 5. observe again — appends rather than replacing.
	rig.deploy.onRun = nil
	out, err = rig.run("observe")
	require.NoError(t, err, out)
	assert.Len(t, rig.only().Observations, 3)

	// 6. teardown — destroy, delete the project, sweep, release.
	out, err = rig.run("teardown", d.ID)
	require.NoError(t, err, out)
	assert.Equal(t, 1, rig.destroy.calls)
	assert.Equal(t, 1, rig.projects.deletes, "tofu cannot delete the project; teardown must")
	assert.Equal(t, 1, rig.sweep.calls)

	// 7. and afterwards the record is released, not deleted, so the
	//    reaper can prove it acted.
	released := rig.only()
	assert.Equal(t, livestore.StateReleased, released.State)
	assert.Len(t, released.Observations, 3, "the history survives release")

	out, err = rig.run("ls")
	require.NoError(t, err, out)
	assert.Contains(t, out, "released")

	// 8. observe skips it now — probing a released address would
	//    attribute whatever answers there to a service that is gone.
	probesBefore := rig.probe.probes
	out, err = rig.run("observe")
	require.NoError(t, err, out)
	assert.Equal(t, probesBefore, rig.probe.probes, "a released deployment is not probed")
	assert.Contains(t, out, "released")
}

// A deployment whose apply produced no address is a record `deploy`
// legitimately writes, and the S154 pass-44 defect was `observe` treating
// it as nothing to see. The journey catches it because the record comes
// from deploy rather than from the test.
func TestLiveLifecycleObserveRefusesADeploymentDeployCouldNotAddress(t *testing.T) {
	rig := newLifecycleRig(t)

	// No state at all, so LiveEndpoint finds no load balancer -- the shape
	// a real apply leaves when it fails before creating one.
	rig.deploy.onRunDir = nil
	rig.deploy.err = &harness.SandboxDeployError{Stage: "apply", Err: assertAnError()}

	out, _ := rig.run("deploy", "--ttl", "30m")
	d := rig.only()
	require.Empty(t, d.Address, "the apply produced no endpoint, and deploy records that honestly")

	out, err := rig.run("observe")

	require.Error(t, err, "a live deployment nobody can monitor is a finding, not a skip")
	assert.Contains(t, out, "cannot be observed")
	assert.Contains(t, out, d.ProjectID, "the operator gets the handle to what is running")
}

func assertAnError() error { return &lifecycleErr{} }

type lifecycleErr struct{}

func (*lifecycleErr) Error() string { return "apply failed after creating nothing addressable" }
