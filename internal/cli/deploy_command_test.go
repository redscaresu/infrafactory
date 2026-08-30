package cli

import (
	"bytes"
	"context"
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
	"github.com/redscaresu/infrafactory/internal/scenario"
)

const liveServiceScenarioYAML = `scenario: web-live-paris
version: "1.0"
cloud: scaleway
description: A versioned web application behind a load balancer.
service:
  image: nginx
  tag: "1.27"
  port: 80
  ttl: 4h
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`

const infraOnlyScenarioYAML = `scenario: block-paris
version: "1.0"
cloud: scaleway
description: Infrastructure only, no versioned application.
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`

// deployTestRuntime wires a Layer-3-enabled runtime with fakes and
// workspace-scoped output and live-store roots.
func deployTestRuntime(t *testing.T, scenarioYAML string, deploy *fakeSandboxDeployHarness) (*CommandRuntime, *livestore.FilesystemStore, string) {
	t.Helper()
	h := newCommandTestHarness(t)

	scenarioPath := filepath.Join(h.WorkspaceDir, "scenario.yaml")
	require.NoError(t, os.WriteFile(scenarioPath, []byte(scenarioYAML), 0o600))

	cfg, err := config.Load(h.ConfigPath)
	require.NoError(t, err)
	cfg.Paths.Output = h.OutputDir()
	cfg.Validation.Layers.SandboxDeploy.Enabled = true

	rt := &CommandRuntime{
		Config:         cfg,
		scenarioLoader: defaultScenarioLoader,
		livestoreRoot:  h.LivestoreRoot(),
		Deps:           RuntimeDependencies{SandboxDeploy: deploy},
	}
	_, err = rt.LoadScenario(scenarioPath)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(rt.OutputDir(), 0o755))

	return rt, livestore.NewFilesystemStore(h.LivestoreRoot()), scenarioPath
}

// writeDeployableHCL puts allowlisted HCL in the output dir, standing in
// for what `infrafactory run` would have generated and validated.
func writeDeployableHCL(t *testing.T, dir string) {
	t.Helper()
	body := `terraform {
  required_providers {
    scaleway = {
      source  = "scaleway/scaleway"
      version = "2.81.0"
    }
  }
}

resource "scaleway_account_project" "run" {
  name = "if-live"
}

resource "scaleway_block_volume" "data" {
  name       = "if-live-data"
  size_in_gb = 10
  iops       = 5000
  project_id = scaleway_account_project.run.id
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(body), 0o600))
}

func runDeploy(t *testing.T, rt *CommandRuntime, scenarioPath string, out *strings.Builder, args ...string) error {
	t.Helper()
	cmd := &cobra.Command{Use: "deploy"}
	cmd.Flags().String("ttl", "", "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	require.NoError(t, cmd.ParseFlags(args))
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetContext(context.Background())
	return runDeployCommand(cmd, []string{scenarioPath}, rt)
}

func TestDeployAppliesAndRecordsWithoutDestroying(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{}
	rt, store, scenarioPath := deployTestRuntime(t, liveServiceScenarioYAML, deploy)
	writeDeployableHCL(t, rt.OutputDir())

	var out strings.Builder
	require.NoError(t, runDeploy(t, rt, scenarioPath, &out))

	assert.Equal(t, 1, deploy.calls)

	records, unreadable, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, unreadable)
	require.Len(t, records, 1, "the deployment is recorded")

	d := records[0]
	assert.Equal(t, "web-live-paris", d.Scenario)
	assert.Equal(t, "nginx", d.Image)
	assert.Equal(t, "1.27", d.Tag)
	assert.Equal(t, livestore.StateLive, d.State)
	assert.Equal(t, "test-run-project", d.ProjectID, "taken from the live state, not the record")
	assert.InDelta(t, 4*time.Hour, d.ExpiresAt.Sub(d.CreatedAt), float64(time.Second),
		"the TTL comes from service.ttl")
	assert.Contains(t, out.String(), "nginx:1.27")
}

// The reason each deployment gets its own workdir: a shared one would
// let a second apply overwrite the first's state, orphaning real
// resources with nothing left that knows how to destroy them.
func TestDeployGivesEachDeploymentItsOwnWorkdir(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{}
	rt, store, scenarioPath := deployTestRuntime(t, liveServiceScenarioYAML, deploy)
	writeDeployableHCL(t, rt.OutputDir())

	var out strings.Builder
	// Back to back, no sleep. The previous version slept 1100ms to get
	// past second-resolution ids, which dodged the collision bug rather
	// than testing that it is gone.
	require.NoError(t, runDeploy(t, rt, scenarioPath, &out))
	require.NoError(t, runDeploy(t, rt, scenarioPath, &out))

	records, _, err := store.List()
	require.NoError(t, err)
	require.Len(t, records, 2, "two distinct deployments")
	assert.NotEqual(t, records[0].ID, records[1].ID)
	assert.NotEqual(t, records[0].WorkDir, records[1].WorkDir,
		"separate state, so neither can overwrite the other")

	for _, d := range records {
		_, err := os.Stat(filepath.Join(d.WorkDir, harness.LiveStateFilename))
		assert.NoError(t, err, "each workdir holds its own live state")
	}
}

// A scenario with no versioned application is not deployable: there
// would be nothing to roll forward, and deploy would mean "apply and
// forget to destroy".
func TestDeployRefusesAnInfrastructureOnlyScenario(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{}
	rt, store, scenarioPath := deployTestRuntime(t, infraOnlyScenarioYAML, deploy)
	writeDeployableHCL(t, rt.OutputDir())

	var out strings.Builder
	err := runDeploy(t, rt, scenarioPath, &out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "declares no service")
	assert.Zero(t, deploy.calls)

	records, _, listErr := store.List()
	require.NoError(t, listErr)
	assert.Empty(t, records)
}

func TestDeployRefusesWhenNothingHasBeenGenerated(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{}
	rt, _, scenarioPath := deployTestRuntime(t, liveServiceScenarioYAML, deploy)

	var out strings.Builder
	err := runDeploy(t, rt, scenarioPath, &out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "infrafactory run")
	assert.Zero(t, deploy.calls, "an empty apply would still create a project")
}

// deploy applies HCL a previous command validated, but "already
// validated" is a claim about a previous command. The allowlist is
// re-checked here, deny-by-default, exactly as the PR gate does.
func TestDeployRefusesHCLOutsideTheAllowlist(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{}
	rt, _, scenarioPath := deployTestRuntime(t, liveServiceScenarioYAML, deploy)
	require.NoError(t, os.WriteFile(filepath.Join(rt.OutputDir(), "main.tf"), []byte(
		`resource "scaleway_account_project" "run" {}
resource "scaleway_rdb_instance" "expensive" {}
`), 0o600))

	var out strings.Builder
	err := runDeploy(t, rt, scenarioPath, &out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "hcl validation")
	assert.Zero(t, deploy.calls)
}

func TestDeployRefusesWhenLayer3IsDisabled(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{}
	rt, _, scenarioPath := deployTestRuntime(t, liveServiceScenarioYAML, deploy)
	rt.Config.Validation.Layers.SandboxDeploy.Enabled = false
	writeDeployableHCL(t, rt.OutputDir())

	var out strings.Builder
	err := runDeploy(t, rt, scenarioPath, &out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox_deploy.enabled")
	assert.Zero(t, deploy.calls)
}

func TestDeployTTLOverride(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{}
	rt, store, scenarioPath := deployTestRuntime(t, liveServiceScenarioYAML, deploy)
	writeDeployableHCL(t, rt.OutputDir())

	var out strings.Builder
	require.NoError(t, runDeploy(t, rt, scenarioPath, &out, "--ttl", "30m"))

	records, _, err := store.List()
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.InDelta(t, 30*time.Minute, records[0].ExpiresAt.Sub(records[0].CreatedAt), float64(time.Second))
}

// An override still goes through the schema's bounds: --ttl must not be
// a way around "there is no unbounded form".
func TestDeployRefusesAnOutOfBoundsTTLOverride(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{}
	rt, _, scenarioPath := deployTestRuntime(t, liveServiceScenarioYAML, deploy)
	writeDeployableHCL(t, rt.OutputDir())

	for _, bad := range []string{"0s", "-1h", "400h", "forever"} {
		t.Run(bad, func(t *testing.T) {
			var out strings.Builder
			err := runDeploy(t, rt, scenarioPath, &out, "--ttl", bad)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "--ttl")
		})
	}
	assert.Zero(t, deploy.calls)
}

// An apply that fails before creating anything leaves no state, so there
// is nothing to record and nothing to reap -- but the operator is still
// told how to clean up if they think otherwise.
func TestDeployRecordsNothingWhenTheApplyCreatedNothing(t *testing.T) {
	sandboxCredsForTest(t)
	deploy := &fakeSandboxDeployHarness{err: harness.ErrSandboxDeployFailed}
	rt, store, scenarioPath := deployTestRuntime(t, liveServiceScenarioYAML, deploy)
	writeDeployableHCL(t, rt.OutputDir())

	var out strings.Builder
	err := runDeploy(t, rt, scenarioPath, &out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to tear down",
		"pointing at teardown when no record exists reads like a typo, not like nothing was created")

	records, _, listErr := store.List()
	require.NoError(t, listErr)
	assert.Empty(t, records, "no state means nothing was created")
}

// partialApplyHarness fails the way a real apply does when it dies with
// resources already created: an error AND a state file naming them.
type partialApplyHarness struct{ calls int }

func (p *partialApplyHarness) Run(_ context.Context, workDir string, _ map[string]string) (*harness.SandboxDeployResult, error) {
	p.calls++
	_ = os.MkdirAll(workDir, 0o755)
	_ = os.WriteFile(filepath.Join(workDir, harness.LiveStateFilename), []byte(
		`{"resources":[{"type":"scaleway_account_project","instances":[{"attributes":{"id":"half-made-project"}}]}]}`), 0o600)
	return nil, harness.ErrSandboxDeployFailed
}

// The leak-prevention path, and the reason registration is not on the
// happy path only. A half-finished apply leaves real resources with a
// real project; the record is the only thing that will bring the reaper
// back to them.
func TestDeployRecordsAPartialApplySoItCanBeReaped(t *testing.T) {
	sandboxCredsForTest(t)
	partial := &partialApplyHarness{}
	rt, store, scenarioPath := deployTestRuntime(t, liveServiceScenarioYAML, nil)
	rt.Deps.SandboxDeploy = partial
	writeDeployableHCL(t, rt.OutputDir())

	var out strings.Builder
	err := runDeploy(t, rt, scenarioPath, &out)

	require.Error(t, err, "the deploy still fails")
	assert.Contains(t, err.Error(), "live teardown")

	records, _, listErr := store.List()
	require.NoError(t, listErr)
	require.Len(t, records, 1, "but what it created is recorded anyway")
	assert.Equal(t, "half-made-project", records[0].ProjectID)
	assert.True(t, records[0].Reapable(records[0].ExpiresAt.Add(time.Second)),
		"so the reaper will come back for it")
}

// scenarioWithService loads the live-service fixture for direct calls
// into registerDeployment.
func scenarioWithService(t *testing.T) scenario.Scenario {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "scenario.yaml")
	require.NoError(t, os.WriteFile(path, []byte(liveServiceScenarioYAML), 0o600))
	sc, err := scenario.LoadWithSchema(path, filepath.Join("..", "..", "scenario.schema.json"))
	require.NoError(t, err)
	return sc
}

// State that names no project cannot be reaped, so recording it silently
// would leave something running that nothing tracks.
func TestRegisterDeploymentFailsWhenStateNamesNoProject(t *testing.T) {
	h := newCommandTestHarness(t)
	store := livestore.NewFilesystemStore(h.LivestoreRoot())
	workDir := filepath.Join(h.WorkspaceDir, "wd")
	require.NoError(t, os.MkdirAll(workDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, harness.LiveStateFilename),
		[]byte(`{"resources":[]}`), 0o600))

	sc := scenarioWithService(t)
	stages, failures := registerDeployment(store, sc, "dep-x", workDir, time.Hour)

	require.Len(t, failures, 1)
	assert.Contains(t, failures[0].Detail, "cannot be recorded or reaped")
	require.Len(t, stages, 1)
	assert.Equal(t, StageStatusFail, stages[0].Status)
}

func TestRegisterDeploymentSkipsWhenNothingWasCreated(t *testing.T) {
	h := newCommandTestHarness(t)
	store := livestore.NewFilesystemStore(h.LivestoreRoot())
	workDir := filepath.Join(h.WorkspaceDir, "empty")
	require.NoError(t, os.MkdirAll(workDir, 0o755))

	stages, failures := registerDeployment(store, scenarioWithService(t), "dep-y", workDir, time.Hour)

	assert.Empty(t, failures)
	require.Len(t, stages, 1)
	assert.Equal(t, StageStatusSkip, stages[0].Status)
	assert.Contains(t, stages[0].Detail, "nothing was created")
}

func TestNewDeploymentIDIsScenarioScopedAndTimestamped(t *testing.T) {
	at := time.Date(2026, 8, 30, 17, 4, 5, 0, time.UTC)
	id := newDeploymentID("web-live-paris", at)
	assert.True(t, strings.HasPrefix(id, "web-live-paris-20260830T170405Z-"), "got %q", id)
}

// The leak this closes: a second-resolution id made two deploys of one
// scenario in the same second share a record path AND a workdir, so the
// second apply adopted the first's state and left a project running with
// nothing that knew how to destroy it.
func TestDeploymentIDsAreUniqueWithinTheSameSecond(t *testing.T) {
	at := time.Date(2026, 8, 30, 17, 4, 5, 0, time.UTC)
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		id := newDeploymentID("web-live-paris", at)
		require.False(t, seen[id], "collision at iteration %d: %s", i, id)
		seen[id] = true
	}
}

// Defence in depth behind the id: even given a collision, an apply must
// never adopt another deployment's state.
func TestCopyDeploySourceRefusesAWorkdirHoldingLiveState(t *testing.T) {
	source, workDir := t.TempDir(), t.TempDir()
	writeDeployableHCL(t, source)
	require.NoError(t, os.WriteFile(filepath.Join(workDir, harness.LiveStateFilename), []byte(`{}`), 0o600))

	err := copyDeploySource(source, workDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to reuse it")
}

// stubNotify simulates a signal arriving during the apply: the returned
// context is already cancelled, which is what signal.NotifyContext gives
// the apply once SIGINT lands.
func stubNotify(cancelled bool) func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
	return func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(parent)
		if cancelled {
			cancel()
		}
		return ctx, cancel
	}
}

// The leak this closes: without a signal handler, Ctrl-C during a ~140s
// apply killed the process outright. The project already existed, the
// record is written after the apply returns, so nothing was written at
// all -- `live ls` showed nothing and the project billed indefinitely.
func TestDeployApplyRunsUnderASignalGuardAndReportsInterruption(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{Use: "deploy"}
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	var sawCtx context.Context
	_, err := runDeployApply(cmd, context.Background(), stubNotify(true),
		func(ctx context.Context) (*harness.SandboxDeployResult, error) {
			sawCtx = ctx
			return nil, ctx.Err()
		})

	require.Error(t, err, "the apply unwinds rather than the process dying")
	require.NotNil(t, sawCtx)
	assert.Error(t, sawCtx.Err(), "the apply is handed a cancelled context")
	assert.Contains(t, out.String(), "Recording whatever was created so it can be reaped")
}

func TestDeployApplyIsTransparentWhenNotInterrupted(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{Use: "deploy"}
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	result, err := runDeployApply(cmd, context.Background(), stubNotify(false),
		func(context.Context) (*harness.SandboxDeployResult, error) {
			return &harness.SandboxDeployResult{}, nil
		})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotContains(t, out.String(), "Interrupted")
}
