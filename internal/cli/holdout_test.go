package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/feedback"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

const unseenHoldoutYAML = `scenario: web-live-paris-unseen
type: holdout
references: web-live-paris
version: "1.0"
cloud: scaleway
description: criteria the generator never saw
acceptance_criteria:
  - type: connectivity
    from: public_internet
    to: compute
    port: 22
    expect: blocked
`

func writeHoldout(t *testing.T, h *CommandTestHarness, body string) {
	t.Helper()
	dir := filepath.Join(h.WorkspaceDir, "scenarios", "holdout")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "unseen.yaml"), []byte(body), 0o600))
}

func holdoutRuntime(t *testing.T, h *CommandTestHarness, probe RealProbeHarnessRunner) *CommandRuntime {
	t.Helper()
	cfg := config.Default()
	cfg.Paths.Scenarios = filepath.Join(h.WorkspaceDir, "scenarios")
	return &CommandRuntime{
		Config:         cfg,
		scenarioLoader: defaultScenarioLoader,
		Deps:           RuntimeDependencies{RealProbe: probe},
	}
}

// A holdout that finds nothing is reported as a pass with a count, not
// as silence. "No holdout ran" and "the holdout passed" are different
// facts about the same green screen.
func TestHoldoutReportsWhatItChecked(t *testing.T) {
	h := newCommandTestHarness(t)
	writeHoldout(t, h, unseenHoldoutYAML)
	probe := &fakeRealProbeHarness{result: &harness.RealProbeResult{}}

	stages, failures, _ := runHoldouts(context.Background(), holdoutRuntime(t, h, probe), "web-live-paris", h.OutputDir())

	assert.Empty(t, failures)
	assert.Equal(t, 1, probe.calls)
	assert.Contains(t, stageDetail(stages, "holdout", "discovery"), "1 holdout(s)")
	assert.Contains(t, stageDetail(stages, "holdout", "web-live-paris-unseen"), "1 unseen check(s) passed")
}

// The probe must receive the holdout's OWN criteria. A holdout that
// silently ran the training scenario's checks would pass every time and
// measure nothing.
func TestHoldoutProbesItsOwnCriteria(t *testing.T) {
	h := newCommandTestHarness(t)
	writeHoldout(t, h, unseenHoldoutYAML)
	probe := &fakeRealProbeHarness{result: &harness.RealProbeResult{}}

	_, _, _ = runHoldouts(context.Background(), holdoutRuntime(t, h, probe), "web-live-paris", h.OutputDir())

	require.Len(t, probe.lastChecks, 1)
	assert.Equal(t, harness.ProbeCheck{
		Type: "connectivity", Expect: "blocked",
		From: "public_internet", To: "compute", Port: 22,
	}, probe.lastChecks[0])
	assert.Equal(t, h.OutputDir(), probe.lastDir, "the holdout reads the RUNNING stack, not a fresh apply")
}

// Matching moved to the scenario name because a path mismatch is
// silent: discovery returns nothing, the run says "0 holdouts", and
// that is indistinguishable from having none.
func TestHoldoutMatchesOnScenarioName(t *testing.T) {
	h := newCommandTestHarness(t)
	writeHoldout(t, h, unseenHoldoutYAML)
	probe := &fakeRealProbeHarness{result: &harness.RealProbeResult{}}
	rt := holdoutRuntime(t, h, probe)

	_, failures, _ := runHoldouts(context.Background(), rt, "web-live-paris", h.OutputDir())
	assert.Empty(t, failures)
	assert.Equal(t, 1, probe.calls, "the name matches however the scenario was invoked")

	stages, _, _ := runHoldouts(context.Background(), rt, "some-other-scenario", h.OutputDir())
	assert.Contains(t, stageDetail(stages, "holdout", "discovery"), "0 holdout(s)")
}

// A criterion the probe cannot evaluate is REPORTED, not skipped. A
// holdout that quietly drops half its checks is the false coverage the
// whole idea exists to remove.
func TestHoldoutRefusesCriteriaItCannotProbe(t *testing.T) {
	h := newCommandTestHarness(t)
	writeHoldout(t, h, `scenario: unprobeable
type: holdout
references: web-live-paris
version: "1.0"
cloud: scaleway
description: a criterion that cannot be probed against a running stack
acceptance_criteria:
  - type: policy
    check: region_restriction
    expect: pass
`)
	probe := &fakeRealProbeHarness{result: &harness.RealProbeResult{}}

	_, failures, _ := runHoldouts(context.Background(), holdoutRuntime(t, h, probe), "web-live-paris", h.OutputDir())

	require.Len(t, failures, 1)
	assert.Contains(t, failures[0].Detail, "cannot be probed")
	assert.Zero(t, probe.calls, "nothing probeable, so nothing was probed")
}

// A holdout directory that cannot be read is not an empty one.
func TestHoldoutFailsWhenItCannotReadTheDirectory(t *testing.T) {
	h := newCommandTestHarness(t)
	dir := filepath.Join(h.WorkspaceDir, "scenarios", "holdout")
	require.NoError(t, os.MkdirAll(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := os.ReadDir(dir); err == nil {
		t.Skip("running as a user that can read a 0000 directory")
	}

	stages, failures, _ := runHoldouts(context.Background(),
		holdoutRuntime(t, h, &fakeRealProbeHarness{}), "web-live-paris", h.OutputDir())

	require.Len(t, failures, 1)
	assert.Contains(t, failures[0].Detail, "unknown whether")
	assert.Equal(t, StageStatusFail, stages[0].Status)
}

// THE central property. A holdout failure must not reach the repair
// loop: feeding it back makes the unseen check seen, the generator
// fixes against it, and the number keeps looking good while meaning
// nothing. At Layer 3 it also costs a real apply per lap.
func TestHoldoutFailureEndsTheRunWithoutARepair(t *testing.T) {
	h := newCommandTestHarness(t)
	sandboxCredsForTest(t)

	scenarioPath := filepath.Join(h.WorkspaceDir, "scenarios", "training", "web-live-paris.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(scenarioPath), 0o755))
	require.NoError(t, os.WriteFile(scenarioPath, []byte(liveServiceScenarioYAML), 0o600))
	writeHoldout(t, h, unseenHoldoutYAML)

	generated := 0
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Paths.Scenarios = filepath.Join(h.WorkspaceDir, "scenarios")
		cfg.Validation.Layers.SandboxDeploy.Enabled = true
		cfg.Agent.RepairIterationsMax = 5
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			generated++
			// A real provider pin: the Layer 3 shape gate refuses HCL
			// that would resolve the provider binary implicitly, and
			// it runs before anything this test is about.
			return &generator.GeneratedCode{Files: map[string][]byte{
				"main.tf": []byte(`terraform {
  required_providers {
    scaleway = {
      source  = "scaleway/scaleway"
      version = "` + layer3ScalewayProviderVersion + `"
    }
  }
}
`),
				harness.LiveStateFilename: []byte(`{"resources":[{"type":"scaleway_instance_server"}]}`),
			}}, nil
		}),
		Static: &fakeStaticHarness{result: &harness.StaticResult{
			Stages:   []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}},
			PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`),
		}},
		MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{
			Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{}`),
		}},
		Destroy: &fakeDestroyHarness{result: &harness.DestroyResult{
			Destroy: harness.StageResult{Stage: "destroy"}, StateSnapshot: []byte(`{"instance":{"servers":[]}}`),
		}},
		RunProject:     &fakeRunProject{created: harness.RunProject{ID: "p-1", Name: "if-run-1"}},
		SandboxDeploy:  &fakeSandboxDeployHarness{result: &harness.SandboxDeployResult{Apply: harness.StageResult{Stage: "apply"}}},
		SandboxDestroy: &fakeSandboxDestroyHarness{result: &harness.SandboxDestroyResult{}},
		OrphanSweep:    &fakeOrphanSweep{},
		// The scenario's own probe passes; the holdout's does not.
		// That is the whole shape of overfitting.
		RealProbe: &fakeRealProbeHarness{
			result: &harness.RealProbeResult{},
			// Keyed on the check, not the scenario name: the holdout
			// probes under the TRAINING name so `{{scenario_name}}`
			// resolves against the stack that exists, which means the
			// name no longer tells the two callers apart.
			failWhen: func(checks []harness.ProbeCheck) *harness.RealProbeResult {
				for _, c := range checks {
					if c.Type == "connectivity" && c.Port == 22 {
						return &harness.RealProbeResult{Failures: []feedback.Failure{{
							Check:  "connectivity",
							Detail: "public_internet->compute:22 expected false got true",
						}}}
					}
				}
				return nil
			},
		},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath, "--holdout"})

	err := cmd.Execute()

	require.Error(t, err, "a failed holdout fails the run")
	assert.Contains(t, err.Error(), "was not shown")
	assert.Equal(t, 1, generated,
		"the repair loop must never be given the holdout: fixing against it is exactly the overfitting being measured")
	assert.Contains(t, stdout.String(), "holdout/web-live-paris-unseen: fail")
}

// Both commands that can run a holdout have to accept the flag, or the
// UI's deploy path fails with "flag accessed but not defined" -- which
// is how adding this flag broke three callers at once.
func TestHoldoutFlagIsDefinedWhereverItIsRead(t *testing.T) {
	cfg := &rootConfig{}
	for _, tc := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "run", cmd: newRunCmd(cfg)},
		{name: "deploy", cmd: newDeployCmd(cfg)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NotNil(t, tc.cmd.Flags().Lookup("holdout"), "%s reads --holdout", tc.name)
		})
	}
}

// Three states, not two. A bool would render "no holdout ran" and "the
// holdout passed" identically on the estate page, which is the false
// coverage a holdout exists to prevent.
func TestHoldoutResultIsRecordedOnTheDeployment(t *testing.T) {
	h := newCommandTestHarness(t)
	store := livestore.NewFilesystemStore(h.LivestoreRoot())
	workDir := filepath.Join(h.WorkspaceDir, "wd")
	require.NoError(t, os.MkdirAll(workDir, 0o755))

	for _, tc := range []struct{ id, result string }{
		{id: "dep-none", result: ""},
		{id: "dep-pass", result: livestore.HoldoutPass},
		{id: "dep-fail", result: livestore.HoldoutFail},
	} {
		_, failures := registerDeployment(store, scenarioWithService(t), tc.id, workDir, "p-1", time.Hour, tc.result)
		require.Empty(t, failures)
	}

	deployments, _, err := store.List()
	require.NoError(t, err)
	got := map[string]string{}
	for _, d := range deployments {
		got[d.ID] = d.Holdout
	}
	assert.Equal(t, "", got["dep-none"], "not run is its own state")
	assert.Equal(t, livestore.HoldoutPass, got["dep-pass"])
	assert.Equal(t, livestore.HoldoutFail, got["dep-fail"])
}

// The estate page must never say "unseen checks passed" for a
// deployment nothing probed. A scenario with no holdout files produces
// a clean discovery and no failures, so anything keying off the
// --holdout REQUEST rather than the result would record a pass.
func TestHoldoutOutcomeDistinguishesRanFromRequested(t *testing.T) {
	discoveryOnly := []StageSummary{{Layer: "holdout", Stage: "discovery", Status: StageStatusPass, Detail: "0 holdout(s)"}}
	assert.Equal(t, "", holdoutOutcome(discoveryOnly), "discovering nothing is not passing")

	skipped := []StageSummary{
		{Layer: "holdout", Stage: "discovery", Status: StageStatusPass},
		{Layer: "holdout", Stage: "skipped", Status: StageStatusSkip},
	}
	assert.Equal(t, "", holdoutOutcome(skipped), "a skip is not a pass")

	passed := append([]StageSummary{}, discoveryOnly...)
	passed = append(passed, StageSummary{Layer: "holdout", Stage: "unseen", Status: StageStatusPass})
	assert.Equal(t, livestore.HoldoutPass, holdoutOutcome(passed))

	failed := append([]StageSummary{}, passed...)
	failed = append(failed, StageSummary{Layer: "holdout", Stage: "other", Status: StageStatusFail})
	assert.Equal(t, livestore.HoldoutFail, holdoutOutcome(failed), "one failure outranks any number of passes")

	assert.Equal(t, "", holdoutOutcome([]StageSummary{{Layer: "sandbox_deploy", Stage: "apply", Status: StageStatusPass}}),
		"only holdout stages count")
}

// --holdout is accepted on every run, so a run that skips destruction
// must not silently skip the holdout and still report success.
//
// Driven through executeTest rather than by calling holdoutAfterCriteria
// directly: the risk is a MISSING CALL on one of the two criteria
// paths, and a test that calls the helper itself cannot see that. A
// first version did exactly that, and deleting the call site left it
// passing.
func TestHoldoutRunsEvenWhenDestructionIsSkipped(t *testing.T) {
	h := newCommandTestHarness(t)
	sandboxCredsForTest(t)
	writeHoldout(t, h, unseenHoldoutYAML)
	scenarioPath := writeLiveServiceScenario(t, h)

	probe := &fakeRealProbeHarness{result: &harness.RealProbeResult{}}
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Paths.Output = h.OutputDir()
			cfg.Paths.Scenarios = filepath.Join(h.WorkspaceDir, "scenarios")
			cfg.Validation.Layers.SandboxDeploy.Enabled = true
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		livestoreRoot:  h.LivestoreRoot(),
		deps: RuntimeDependencies{
			MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{
				Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{}`),
			}},
			Destroy:        &fakeDestroyHarness{result: &harness.DestroyResult{StateSnapshot: []byte(`{}`)}},
			RunProject:     &fakeRunProject{created: harness.RunProject{ID: "p-1", Name: "if-run-1"}},
			SandboxDeploy:  &fakeSandboxDeployHarness{result: &harness.SandboxDeployResult{Apply: harness.StageResult{Stage: "apply"}}},
			SandboxDestroy: &fakeSandboxDestroyHarness{result: &harness.SandboxDestroyResult{}},
			OrphanSweep:    &fakeOrphanSweep{},
			RealProbe:      probe,
		},
	}

	cmd := &cobra.Command{Use: "test <scenario>"}
	cmd.Flags().String("config", h.ConfigPath, "")
	rt, err := buildRuntime(cmd, opts)
	require.NoError(t, err)

	// The Layer 3 HCL preflight refuses a configuration that would
	// resolve the provider binary implicitly, and it returns before
	// either criteria path -- so without real HCL here this test would
	// pass by never reaching the code it is about.
	_, err = rt.LoadScenario(scenarioPath)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(rt.OutputDir(), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rt.OutputDir(), "providers.tf"), []byte(`terraform {
  required_providers {
    scaleway = {
      source  = "scaleway/scaleway"
      version = "`+layer3ScalewayProviderVersion+`"
    }
  }
}
`), 0o600))

	result, _ := executeTest(t.Context(), rt, scenarioPath,
		testExecutionOptions{Holdout: true, SkipDestroy: true})

	assert.Contains(t, stageDetail(result.Stages, "holdout", "discovery"), "1 holdout(s)",
		"the holdout must not depend on the destroy branch")
	assert.Equal(t, StageStatusSkip, stageStatus(result.Stages, "destruction", "disabled"),
		"and this is the path where destruction really is skipped")
}

func writeLiveServiceScenario(t *testing.T, h *CommandTestHarness) string {
	t.Helper()
	path := filepath.Join(h.WorkspaceDir, "scenarios", "training", "web-live-paris.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(liveServiceScenarioYAML), 0o600))
	return path
}

// The third return value is what stops a deployment being recorded as
// "unseen checks passed" when nothing was probed. It counts CHECKS, not
// files: a scenario with no holdout, and a holdout whose every
// criterion is unprobeable, both ran nothing.
func TestHoldoutReportsWhetherAnythingActuallyRan(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{name: "no holdout files", body: "", want: false},
		{name: "probeable criteria", body: unseenHoldoutYAML, want: true},
		{
			name: "holdout with nothing probeable",
			want: false,
			body: `scenario: unprobeable
type: holdout
references: web-live-paris
version: "1.0"
cloud: scaleway
description: nothing here can be probed against a running stack
acceptance_criteria:
  - type: policy
    check: region_restriction
    expect: pass
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newCommandTestHarness(t)
			if tc.body != "" {
				writeHoldout(t, h, tc.body)
			} else {
				require.NoError(t, os.MkdirAll(filepath.Join(h.WorkspaceDir, "scenarios", "holdout"), 0o755))
			}
			probe := &fakeRealProbeHarness{result: &harness.RealProbeResult{}}

			_, _, probed := runHoldouts(context.Background(),
				holdoutRuntime(t, h, probe), "web-live-paris", h.OutputDir())

			assert.Equal(t, tc.want, probed)
		})
	}
}

// A holdout is mostly `expect: blocked`, and a blocked check passes
// TRIVIALLY against a stack that is not serving. The scenario's own
// probe is what waits for the load balancer, so probing after an
// earlier failure would report the strongest possible pass for the
// weakest possible reason.
func TestHoldoutDoesNotProbeAStackThatIsNotServing(t *testing.T) {
	h := newCommandTestHarness(t)
	writeHoldout(t, h, unseenHoldoutYAML)
	probe := &fakeRealProbeHarness{result: &harness.RealProbeResult{}}

	rt := holdoutRuntime(t, h, probe)
	rt.Config.Validation.Layers.SandboxDeploy.Enabled = true
	sc, err := rt.LoadScenario(writeLiveServiceScenario(t, h))
	require.NoError(t, err)

	priorFailure := []FailureSummary{{Layer: "sandbox_deploy", Stage: "real_probe", Check: "http_probe"}}
	stages, failures := holdoutAfterCriteria(context.Background(), rt, sc,
		testExecutionOptions{Holdout: true}, priorFailure)

	assert.Empty(t, failures)
	assert.Zero(t, probe.calls, "a blocked-port check against a dead stack passes for the wrong reason")
	assert.Equal(t, StageStatusSkip, stageStatus(stages, "holdout", "skipped"))
	assert.Contains(t, stageDetail(stages, "holdout", "skipped"), "not known to be serving")
}

// RealProbeHarness substitutes the scenario name into
// `{{scenario_name}}`, and the DNS records belong to the stack the
// TRAINING scenario built. Passing the holdout's own name would
// resolve `web-live-paris-unseen.example.com` for a record called
// `web-live-paris.example.com` and report whatever that lookup did.
func TestHoldoutProbesUnderTheTrainingScenarioName(t *testing.T) {
	h := newCommandTestHarness(t)
	writeHoldout(t, h, `scenario: web-live-paris-unseen
type: holdout
references: web-live-paris
version: "1.0"
cloud: scaleway
description: a dns check that uses the scenario-name placeholder
acceptance_criteria:
  - type: dns_resolution
    domain: "{{scenario_name}}.example.com"
    expect: resolves
`)
	probe := &fakeRealProbeHarness{result: &harness.RealProbeResult{}}

	_, _, _ = runHoldouts(context.Background(), holdoutRuntime(t, h, probe), "web-live-paris", h.OutputDir())

	assert.Equal(t, "web-live-paris", probe.lastName,
		"the placeholder resolves against the stack the training scenario built")
}

// A flag that cannot be honoured is a usage error, and usage errors
// are only cheap while they are early. Accepting --holdout without
// Layer 3 and skipping it would let a run report target_reached having
// probed nothing.
func TestHoldoutIsRefusedWithoutLayer3(t *testing.T) {
	h := newCommandTestHarness(t)
	rt := holdoutRuntime(t, h, &fakeRealProbeHarness{})

	rt.Config.Validation.Layers.SandboxDeploy.Enabled = false
	err := assertHoldoutRunnable(true, rt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox_deploy.enabled")

	assert.NoError(t, assertHoldoutRunnable(false, rt), "not asking for it is always fine")

	rt.Config.Validation.Layers.SandboxDeploy.Enabled = true
	assert.NoError(t, assertHoldoutRunnable(true, rt))
}

// The whole point of the state-policy path: a criterion named in
// acceptance_criteria must actually be evaluated against what the
// provider created, not merely routed somewhere and dropped.
func TestStatePolicyEvaluatesTheNamedPolicyAgainstMockState(t *testing.T) {
	h := newCommandTestHarness(t)
	rt := holdoutRuntime(t, h, &fakeRealProbeHarness{result: &harness.RealProbeResult{}})
	rt.Config.Paths.Policies = filepath.Join("..", "..", "policies")
	// The same mappings infrafactory.yaml carries. Declared here
	// rather than inherited, so the test states what it depends on.
	rt.Config.ConstraintPolicies = map[string]string{
		"region_restriction": "scaleway/region_restriction.rego",
		"encryption_at_rest": "scaleway/encryption_at_rest.rego",
	}

	// A VPC the provider placed outside the requested region. The PLAN
	// said fr-par; this is what came back.
	state, err := json.Marshal(map[string]any{
		"vpc": map[string]any{"vpcs": []any{map[string]any{"name": "stray", "region": "nl-ams"}}},
	})
	require.NoError(t, err)

	specs := []scenario.ExecutableCheckSpec{{
		Type:   "policy",
		Expect: "pass",
		Policy: &scenario.PolicyCheckSpec{
			Check:  "region_restriction",
			Params: map[string]any{"region": "fr-par"},
		},
	}}

	stages, failures, _ := evaluateStatePolicyCriteria(context.Background(), rt, "scaleway", state, specs)

	require.Len(t, failures, 1, "the criterion must be evaluated against deployed state")
	assert.Contains(t, failures[0].Detail, "stray")
	assert.Contains(t, failures[0].Detail, "nl-ams")
	assert.Empty(t, stages, "it has a state rule, so nothing is skipped")
}

// A plan-only policy is REPORTED as such. Rego rules are undefined
// rather than false when absent and return zero results, identical to
// a rule that found nothing -- so without this the run says "pass" for
// a criterion nothing evaluated.
func TestStatePolicyReportsAPlanOnlyPolicyInsteadOfPassingIt(t *testing.T) {
	h := newCommandTestHarness(t)
	rt := holdoutRuntime(t, h, &fakeRealProbeHarness{result: &harness.RealProbeResult{}})
	rt.Config.Paths.Policies = filepath.Join("..", "..", "policies")
	rt.Config.ConstraintPolicies = map[string]string{
		"encryption_at_rest": "scaleway/encryption_at_rest.rego",
	}

	state, err := json.Marshal(map[string]any{"vpc": map[string]any{"vpcs": []any{}}})
	require.NoError(t, err)

	specs := []scenario.ExecutableCheckSpec{{
		Type:   "policy",
		Expect: "pass",
		// Mapped, and plan-only: it has `deny` and no `deny_state`.
		Policy: &scenario.PolicyCheckSpec{Check: "encryption_at_rest"},
	}}

	stages, failures, _ := evaluateStatePolicyCriteria(context.Background(), rt, "scaleway", state, specs)

	assert.Empty(t, failures, "a plan-only policy is not a failure")
	require.Len(t, stages, 1)
	assert.Equal(t, StageStatusSkip, stages[0].Status)
	assert.Contains(t, stages[0].Detail, "encryption_at_rest")
	assert.Contains(t, stages[0].Detail, "NOT checked")
}

// `expect: fail` asks the policy to DENY. A plan-only policy can never
// deny, so skipping it would let the run report success for an
// assertion nothing could ever satisfy -- strictly worse than the old
// behaviour, where the absent rule returned zero denials and the
// expectation check caught it.
func TestStatePolicyFailsAnExpectFailAgainstAPlanOnlyPolicy(t *testing.T) {
	h := newCommandTestHarness(t)
	rt := holdoutRuntime(t, h, &fakeRealProbeHarness{result: &harness.RealProbeResult{}})
	rt.Config.Paths.Policies = filepath.Join("..", "..", "policies")
	rt.Config.ConstraintPolicies = map[string]string{
		"encryption_at_rest": "scaleway/encryption_at_rest.rego",
	}

	state, err := json.Marshal(map[string]any{"vpc": map[string]any{"vpcs": []any{}}})
	require.NoError(t, err)

	specs := []scenario.ExecutableCheckSpec{{
		Type:   "policy",
		Expect: "fail",
		Policy: &scenario.PolicyCheckSpec{Check: "encryption_at_rest"},
	}}

	stages, failures, evaluated := evaluateStatePolicyCriteria(context.Background(), rt, "scaleway", state, specs)

	require.Len(t, failures, 1, "an expectation nothing can satisfy is a failure, not a skip")
	assert.Contains(t, failures[0].Detail, "can never deny")
	assert.Empty(t, stages, "it failed; it was not skipped")
	assert.Zero(t, evaluated)
}

// A skip beside a pass is two contradictory claims about one check,
// and the green one is what people read -- so a scenario mixing an
// evaluated policy with a plan-only one would look entirely checked.
// Green has to mean every named policy was evaluated.
func TestStatePolicyEmitsOneStageForAMixedSet(t *testing.T) {
	h := newCommandTestHarness(t)
	rt := holdoutRuntime(t, h, &fakeRealProbeHarness{result: &harness.RealProbeResult{}})
	rt.Config.Paths.Policies = filepath.Join("..", "..", "policies")
	rt.Config.ConstraintPolicies = map[string]string{
		"region_restriction": "scaleway/region_restriction.rego", // has deny_state
		"encryption_at_rest": "scaleway/encryption_at_rest.rego", // plan-only
	}

	state, err := json.Marshal(map[string]any{
		"instance": map[string]any{"servers": []any{map[string]any{"name": "web", "zone": "fr-par-1"}}},
	})
	require.NoError(t, err)

	specs := []scenario.ExecutableCheckSpec{
		{Type: "policy", Expect: "pass", Policy: &scenario.PolicyCheckSpec{
			Check: "region_restriction", Params: map[string]any{"region": "fr-par"}}},
		{Type: "policy", Expect: "pass", Policy: &scenario.PolicyCheckSpec{Check: "encryption_at_rest"}},
	}

	stages, failures, evaluated := evaluateStatePolicyCriteria(context.Background(), rt, "scaleway", state, specs)

	assert.Empty(t, failures, "the evaluated policy passed and the other was not run")
	assert.Equal(t, 1, evaluated)
	require.Len(t, stages, 1, "one check, one stage")
	assert.Equal(t, StageStatusSkip, stages[0].Status,
		"anything unchecked means this is not a pass")
	assert.Contains(t, stages[0].Detail, "1 of 2 evaluated")
	assert.Contains(t, stages[0].Detail, "encryption_at_rest")
}

// ...and when everything named really was evaluated, it is green.
func TestStatePolicyIsGreenOnlyWhenEverythingWasChecked(t *testing.T) {
	h := newCommandTestHarness(t)
	rt := holdoutRuntime(t, h, &fakeRealProbeHarness{result: &harness.RealProbeResult{}})
	rt.Config.Paths.Policies = filepath.Join("..", "..", "policies")
	rt.Config.ConstraintPolicies = map[string]string{
		"region_restriction": "scaleway/region_restriction.rego",
	}

	state, err := json.Marshal(map[string]any{
		"instance": map[string]any{"servers": []any{map[string]any{"name": "web", "zone": "fr-par-1"}}},
	})
	require.NoError(t, err)

	specs := []scenario.ExecutableCheckSpec{{Type: "policy", Expect: "pass",
		Policy: &scenario.PolicyCheckSpec{Check: "region_restriction", Params: map[string]any{"region": "fr-par"}}}}

	stages, failures, evaluated := evaluateStatePolicyCriteria(context.Background(), rt, "scaleway", state, specs)

	assert.Empty(t, failures)
	assert.Equal(t, 1, evaluated)
	require.Len(t, stages, 1)
	assert.Equal(t, StageStatusPass, stages[0].Status)
}
