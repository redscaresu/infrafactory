package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

func keepRuntime(t *testing.T, h *CommandTestHarness, sandbox bool) *CommandRuntime {
	t.Helper()
	cfg := config.Default()
	cfg.Paths.Output = h.OutputDir()
	cfg.Validation.Layers.SandboxDeploy.Enabled = sandbox
	return &CommandRuntime{Config: cfg, outputDir: h.OutputDir(), livestoreRoot: h.LivestoreRoot()}
}

// Every one of these is knowable before the run spends LLM time and
// real money, and a --keep discovered to be impossible at the end would
// destroy the stack it was asked to preserve.
func TestAssertKeepableRefusesWhatTheRunCannotHonour(t *testing.T) {
	h := newCommandTestHarness(t)
	withService := scenarioWithService(t)
	noService := withService
	noService.Service = nil

	badTTL := withService
	spec := *withService.Service
	spec.TTL = "not-a-duration"
	badTTL.Service = &spec

	for _, tc := range []struct {
		name     string
		keep     bool
		sandbox  bool
		sc       scenario.Scenario
		expected string
	}{
		{name: "no keep is always fine", keep: false, sandbox: false, sc: noService},
		{name: "keep with everything present", keep: true, sandbox: true, sc: withService},
		{
			name: "keep without layer 3", keep: true, sandbox: false, sc: withService,
			expected: "sandbox_deploy.enabled",
		},
		{
			name: "keep without a service block", keep: true, sandbox: true, sc: noService,
			expected: "would have no TTL",
		},
		{
			name: "keep with an unusable ttl", keep: true, sandbox: true, sc: badTTL,
			expected: "could not be given a deadline",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := assertKeepable(tc.keep, keepRuntime(t, h, tc.sandbox), tc.sc)
			if tc.expected == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expected)
		})
	}
}

// Both skip a destroy and they mean opposite things afterwards, so
// accepting both would leave it ambiguous which was wanted -- on the
// flag pair where being wrong costs money.
func TestResolveRunControlsRejectsKeepWithNoDestroy(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "run"}
	registerRunFlags(cmd, false)
	require.NoError(t, cmd.Flags().Set("keep", "true"))
	require.NoError(t, cmd.Flags().Set("no-destroy", "true"))

	_, err := resolveRunControls(cmd, &CommandRuntime{Config: config.Default()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestRegisterKeptRunRecordsTheProjectAndCopiesTheStateOut(t *testing.T) {
	h := newCommandTestHarness(t)
	runtime := keepRuntime(t, h, true)

	require.NoError(t, os.MkdirAll(runtime.OutputDir(), 0o755))
	require.NoError(t, harness.WriteRunProjectMarker(runtime.OutputDir(),
		harness.RunProject{ID: "proj-kept", Name: "if-run-kept"}))
	require.NoError(t, os.WriteFile(filepath.Join(runtime.OutputDir(), harness.LiveStateFilename),
		[]byte(`{"resources":[{"type":"scaleway_instance_server"}]}`), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(runtime.OutputDir(), ".terraform", "providers"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(runtime.OutputDir(), ".terraform", "providers", "plugin"), []byte("binary"), 0o755))

	stages, failures := registerKeptRun(runtime, scenarioWithService(t))
	assert.Empty(t, failures)

	store := livestore.NewFilesystemStore(h.LivestoreRoot())
	deployments, unreadable, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, unreadable)
	require.Len(t, deployments, 1)

	d := deployments[0]
	assert.Equal(t, "proj-kept", d.ProjectID, "the marker is the only thing that names the run's project")
	assert.Equal(t, livestore.StateLive, d.State)
	assert.False(t, d.ExpiresAt.IsZero(), "a kept stack with no deadline is a leak with a nicer name")

	// The next run of this scenario regenerates into the output dir and
	// overwrites the state in place. The state is the only thing that
	// can destroy these resources, so the record must not point at it.
	assert.NotEqual(t, runtime.OutputDir(), d.WorkDir)
	assert.FileExists(t, filepath.Join(d.WorkDir, harness.LiveStateFilename))
	assert.FileExists(t, filepath.Join(d.WorkDir, harness.RunProjectMarkerFilename))
	assert.FileExists(t, filepath.Join(d.WorkDir, ".terraform", "providers", "plugin"),
		"teardown runs destroy without an init, so the initialised providers have to come with it")

	require.NotEmpty(t, stages)
	assert.Equal(t, StageStatusPass, stages[len(stages)-1].Status)
	assert.Contains(t, stages[len(stages)-1].Detail, "live teardown ")
}

// A stack that is running and NOT tracked is the one outcome --keep
// must never produce quietly.
func TestRegisterKeptRunFailsLoudlyWhenNoProjectWasRecorded(t *testing.T) {
	h := newCommandTestHarness(t)
	runtime := keepRuntime(t, h, true)
	require.NoError(t, os.MkdirAll(runtime.OutputDir(), 0o755))

	_, failures := registerKeptRun(runtime, scenarioWithService(t))

	require.Len(t, failures, 1)
	assert.Contains(t, failures[0].Detail, "cannot be reaped")
}

// The copy is what makes the record survive the next run. If it cannot
// be made the resources still exist, so the record is written anyway --
// and the run says so instead of reporting a clean keep.
func TestRegisterKeptRunStillRecordsAStackItCouldNotCopy(t *testing.T) {
	h := newCommandTestHarness(t)
	runtime := keepRuntime(t, h, true)
	require.NoError(t, os.MkdirAll(runtime.OutputDir(), 0o755))
	require.NoError(t, harness.WriteRunProjectMarker(runtime.OutputDir(),
		harness.RunProject{ID: "proj-uncopyable", Name: "if-run-uncopyable"}))

	// os.CopyFS refuses a source it cannot walk.
	unreadable := filepath.Join(runtime.OutputDir(), "locked")
	require.NoError(t, os.MkdirAll(unreadable, 0o000))
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })
	if _, err := os.ReadDir(unreadable); err == nil {
		t.Skip("running as a user that can read a 0000 directory")
	}

	stages, failures := registerKeptRun(runtime, scenarioWithService(t))

	store := livestore.NewFilesystemStore(h.LivestoreRoot())
	deployments, _, err := store.List()
	require.NoError(t, err)
	require.Len(t, deployments, 1, "the resources exist, so the record has to")
	assert.Equal(t, "proj-uncopyable", deployments[0].ProjectID)

	require.Len(t, failures, 1)
	assert.Contains(t, failures[0].Detail, "tear this down BEFORE running the scenario again")
	assert.Equal(t, StageStatusFail, stages[len(stages)-1].Status)
}

// --keep must not become a way to leave infrastructure with no record,
// so the flag's own help has to say what it does.
func TestKeepFlagIsDocumented(t *testing.T) {
	cmd := &cobra.Command{Use: "run"}
	registerRunFlags(cmd, true)
	flag := cmd.Flags().Lookup("keep")
	require.NotNil(t, flag)
	assert.True(t, strings.Contains(flag.Usage, "service.ttl"))
}

// --keep must keep the REAL stack and still destroy the mock. Sharing
// one switch with --no-destroy would leave mockway dirty for the next
// run in exchange for nothing: Layer 2 holds no resources and costs
// nothing to tear down.
func TestKeepSandboxSkipsTheRealDestroyAndNotTheMockOne(t *testing.T) {
	h := newCommandTestHarness(t)
	scenarioPath := writeUnsupportedCriteriaScenario(t, h.WorkspaceDir)
	sandboxCredsForTest(t)

	sandboxDestroy := &fakeSandboxDestroyHarness{
		result: &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}},
	}
	mockDestroy := &fakeDestroyHarness{
		result: &harness.DestroyResult{
			Destroy:       harness.StageResult{Stage: "destroy"},
			StateSnapshot: []byte(`{"instance":{"servers":[]}}`),
		},
	}
	runProject := &fakeRunProject{created: harness.RunProject{ID: "run-proj-keep", Name: "if-run-keep"}}
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Validation.Layers.SandboxDeploy.Enabled = true
			cfg.Paths.Output = h.OutputDir()
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		livestoreRoot:  h.LivestoreRoot(),
		deps: RuntimeDependencies{
			MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{
				Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{}`),
			}},
			Destroy:    mockDestroy,
			RunProject: runProject,
			SandboxDeploy: &fakeSandboxDeployHarness{result: &harness.SandboxDeployResult{
				Init: harness.StageResult{Stage: "init"}, Apply: harness.StageResult{Stage: "apply"},
			}},
			SandboxDestroy: sandboxDestroy,
			OrphanSweep:    &fakeOrphanSweep{},
			RealProbe:      &fakeRealProbeHarness{result: &harness.RealProbeResult{}},
		},
	}

	cmd := &cobra.Command{Use: "test <scenario>"}
	cmd.Flags().String("config", h.ConfigPath, "")
	runtime, err := buildRuntime(cmd, opts)
	require.NoError(t, err)

	// LoadScenario is what resolves the per-scenario output dir.
	_, err = runtime.LoadScenario(scenarioPath)
	require.NoError(t, err)

	// State the destroy would have acted on, so the skip is a decision
	// rather than the "nothing to destroy" shortcut.
	require.NoError(t, os.MkdirAll(runtime.OutputDir(), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(runtime.OutputDir(), harness.LiveStateFilename),
		[]byte(`{"resources":[{"type":"scaleway_instance_server"}]}`), 0o600))

	result, err := executeTest(t.Context(), runtime, scenarioPath, testExecutionOptions{KeepSandbox: true})
	require.NoError(t, err)

	assert.Zero(t, sandboxDestroy.calls, "--keep exists to leave the real stack running")
	assert.Equal(t, 1, mockDestroy.calls, "the mock is free to tear down and dirty for the next run if it is not")
	assert.Zero(t, runProject.deletes, "the project is the handle to what was kept")

	assert.Contains(t, stageDetail(result.Stages, "sandbox_deploy", "destroy"), "kept by --keep")
	assert.Contains(t, stageDetail(result.Stages, "sandbox_deploy", "run_project_delete"), "on purpose")
}

func stageDetail(stages []StageSummary, layer, stage string) string {
	for _, s := range stages {
		if s.Layer == layer && s.Stage == stage {
			return s.Detail
		}
	}
	return ""
}

// The wiring, end to end: a green `run --keep` must leave the stack up
// AND leave a record of it. Tested at the command because the two
// halves are separate code -- skipping the destroy lives in
// executeTest, registering lives in the run loop -- and a keep with
// only the first half is exactly the untracked leak `--keep` exists to
// avoid.
func TestRunKeepLeavesTheStackUpAndRegistersIt(t *testing.T) {
	h := newCommandTestHarness(t)
	sandboxCredsForTest(t)

	scenarioPath := filepath.Join(h.WorkspaceDir, "scenarios", "training", "web-live-paris.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(scenarioPath), 0o755))
	require.NoError(t, os.WriteFile(scenarioPath, []byte(liveServiceScenarioYAML), 0o600))

	sandboxDestroy := &fakeSandboxDestroyHarness{
		result: &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}},
	}
	mockDestroy := &fakeDestroyHarness{result: &harness.DestroyResult{
		Destroy:       harness.StageResult{Stage: "destroy"},
		StateSnapshot: []byte(`{"instance":{"servers":[]}}`),
	}}
	runProject := &fakeRunProject{created: harness.RunProject{ID: "run-proj-keep", Name: "if-run-keep"}}

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Validation.Layers.SandboxDeploy.Enabled = true
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{
				"main.tf": []byte("terraform {}\n"),
				// Resources in state, so the destroy this keeps is one
				// that would otherwise have had something to do.
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
		Destroy:        mockDestroy,
		RunProject:     runProject,
		SandboxDeploy:  &fakeSandboxDeployHarness{result: &harness.SandboxDeployResult{Apply: harness.StageResult{Stage: "apply"}}},
		SandboxDestroy: sandboxDestroy,
		OrphanSweep:    &fakeOrphanSweep{},
		RealProbe:      &fakeRealProbeHarness{result: &harness.RealProbeResult{}},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath, "--keep"})

	require.NoError(t, cmd.Execute(), "a kept run is a successful run:\n%s", stdout.String())

	assert.Zero(t, sandboxDestroy.calls, "--keep exists to leave the stack running")
	assert.Zero(t, runProject.deletes, "deleting the project would take the kept stack with it")

	// A holdout runs against the SAME output directory: with Layer 3
	// on it would create its own project, overwrite the marker, apply
	// over this run's state and destroy -- taking the kept stack with
	// it and recording the wrong project.
	assert.Contains(t, stdout.String(), "holdout/skipped: skip")
	assert.Contains(t, stdout.String(), "would destroy the kept stack")

	deployments, _, err := livestore.NewFilesystemStore(h.LivestoreRoot()).List()
	require.NoError(t, err)
	require.Len(t, deployments, 1, "a kept stack nothing recorded is the leak --keep exists to avoid")
	assert.Equal(t, "run-proj-keep", deployments[0].ProjectID)
	assert.Equal(t, livestore.StateLive, deployments[0].State)
	assert.Contains(t, stdout.String(), "live/keep: pass")
}

// The guard is only worth having if it runs BEFORE the run spends
// anything. Asserted at the command, because assertKeepable passing its
// own unit tests says nothing about whether the run loop calls it --
// and it is a usage error that would otherwise surface after minutes of
// LLM time and a real apply, at which point the run destroys the stack
// the operator asked to keep.
func TestRunKeepRefusesBeforeItSpendsAnything(t *testing.T) {
	h := newCommandTestHarness(t)
	sandboxCredsForTest(t)

	scenarioPath := filepath.Join(h.WorkspaceDir, "scenarios", "training", "block-paris.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(scenarioPath), 0o755))
	require.NoError(t, os.WriteFile(scenarioPath, []byte(infraOnlyScenarioYAML), 0o600))

	generated := 0
	runProject := &fakeRunProject{created: harness.RunProject{ID: "run-proj-never", Name: "if-run-never"}}
	sandboxDeploy := &fakeSandboxDeployHarness{result: &harness.SandboxDeployResult{}}

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Validation.Layers.SandboxDeploy.Enabled = true
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			generated++
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		Static:        &fakeStaticHarness{result: &harness.StaticResult{PlanJSON: []byte(`{}`)}},
		MockDeploy:    &fakeMockDeployHarness{result: &harness.MockDeployResult{StateSnapshot: []byte(`{}`)}},
		Destroy:       &fakeDestroyHarness{result: &harness.DestroyResult{StateSnapshot: []byte(`{}`)}},
		RunProject:    runProject,
		SandboxDeploy: sandboxDeploy,
	}

	cmd := newRunCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath, "--keep"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "would have no TTL")

	assert.Zero(t, generated, "refused before the LLM was asked for anything")
	assert.Zero(t, runProject.creates, "refused before a real project existed")
	assert.Zero(t, sandboxDeploy.calls, "refused before anything was applied")
}

// A keep whose record cannot destroy what it kept is not a success.
//
// The fallback record points at the run's own output directory, which
// the NEXT run of this scenario overwrites -- so a green exit here
// invites exactly the re-run that destroys the only teardown state the
// kept stack has. Reported as a failure, and the run fails with it.
func TestRunKeepFailsTheRunWhenTheRecordCannotDestroyWhatItKept(t *testing.T) {
	h := newCommandTestHarness(t)
	sandboxCredsForTest(t)

	scenarioPath := filepath.Join(h.WorkspaceDir, "scenarios", "training", "web-live-paris.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(scenarioPath), 0o755))
	require.NoError(t, os.WriteFile(scenarioPath, []byte(liveServiceScenarioYAML), 0o600))

	// A FILE where the per-deployment workdirs go. The copy out of the
	// run's output dir fails; the record beside it (<root>/<id>.json)
	// still writes, so this isolates the copy failure from every other
	// way the store can break.
	require.NoError(t, os.MkdirAll(h.LivestoreRoot(), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(h.LivestoreRoot(), "workdirs"), []byte("not a dir"), 0o600))

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Validation.Layers.SandboxDeploy.Enabled = true
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{
				"main.tf":                 []byte("terraform {}\n"),
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
		RunProject:     &fakeRunProject{created: harness.RunProject{ID: "run-proj-keep", Name: "if-run-keep"}},
		SandboxDeploy:  &fakeSandboxDeployHarness{result: &harness.SandboxDeployResult{Apply: harness.StageResult{Stage: "apply"}}},
		SandboxDestroy: &fakeSandboxDestroyHarness{result: &harness.SandboxDestroyResult{}},
		OrphanSweep:    &fakeOrphanSweep{},
		RealProbe:      &fakeRealProbeHarness{result: &harness.RealProbeResult{}},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath, "--keep"})

	err := cmd.Execute()
	require.Error(t, err, "a keep the run cannot record faithfully is not a success:\n%s", stdout.String())
	assert.Contains(t, stdout.String(), "live/keep_workdir: fail")

	// Recorded anyway: the resources exist, and the project id alone is
	// enough for teardown to delete the project and sweep the account.
	deployments, _, listErr := livestore.NewFilesystemStore(h.LivestoreRoot()).List()
	require.NoError(t, listErr)
	require.Len(t, deployments, 1)
	assert.Equal(t, "run-proj-keep", deployments[0].ProjectID)
}

// --keep keeps a SUCCESSFUL stack, and a failing iteration is not one.
//
// Skipping the destroy here would leak with no record at all: the
// repair loop generates again, and generation does os.RemoveAll on the
// output directory -- taking the live state AND the run-project marker
// with it -- so what that iteration applied becomes untraceable before
// anything registers it. Only the terminal success is ever registered.
func TestKeepStillDestroysAnIterationThatFailed(t *testing.T) {
	h := newCommandTestHarness(t)
	scenarioPath := writeUnsupportedCriteriaScenario(t, h.WorkspaceDir)
	sandboxCredsForTest(t)

	sandboxDestroy := &fakeSandboxDestroyHarness{
		result: &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}},
	}
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Validation.Layers.SandboxDeploy.Enabled = true
			cfg.Paths.Output = h.OutputDir()
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		livestoreRoot:  h.LivestoreRoot(),
		deps: RuntimeDependencies{
			MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{
				Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{}`),
			}},
			Destroy:    &fakeDestroyHarness{result: &harness.DestroyResult{StateSnapshot: []byte(`{"instance":{"servers":[]}}`)}},
			RunProject: &fakeRunProject{created: harness.RunProject{ID: "run-proj-fail", Name: "if-run-fail"}},
			SandboxDeploy: &fakeSandboxDeployHarness{result: &harness.SandboxDeployResult{
				Apply: harness.StageResult{Stage: "apply"},
			}},
			SandboxDestroy: sandboxDestroy,
			OrphanSweep:    &fakeOrphanSweep{},
			// The apply succeeded and the PROBE failed: resources are
			// real and running, and this iteration is not the one being
			// kept.
			RealProbe: &fakeRealProbeHarness{err: errors.New("probe: connection refused")},
		},
	}

	cmd := &cobra.Command{Use: "test <scenario>"}
	cmd.Flags().String("config", h.ConfigPath, "")
	runtime, err := buildRuntime(cmd, opts)
	require.NoError(t, err)
	_, err = runtime.LoadScenario(scenarioPath)
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(runtime.OutputDir(), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(runtime.OutputDir(), harness.LiveStateFilename),
		[]byte(`{"resources":[{"type":"scaleway_instance_server"}]}`), 0o600))

	result, _ := executeTest(t.Context(), runtime, scenarioPath, testExecutionOptions{KeepSandbox: true})

	require.NotEmpty(t, result.Failures, "the fixture must actually fail, or this proves nothing")
	assert.Equal(t, 1, sandboxDestroy.calls,
		"a failing iteration's resources are destroyed even under --keep: nothing would ever record them")
	assert.NotContains(t, stageDetail(result.Stages, "sandbox_deploy", "destroy"), "kept by --keep")
}
