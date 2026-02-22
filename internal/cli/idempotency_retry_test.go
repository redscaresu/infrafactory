package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/spf13/cobra"
)

func TestGenerateCommandRetryAfterFailureAndRepeatDeterministic(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	outputRoot := filepath.Join(h.WorkspaceDir, "output")
	calls := 0
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Paths.Output = outputRoot
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				calls++
				if calls == 1 {
					return nil, errors.New("transient generator failure")
				}
				return &generator.GeneratedCode{
					Files: map[string][]byte{
						"main.tf":        []byte("terraform {}\n"),
						"modules/vpc.tf": []byte("resource \"x\" \"y\" {}\n"),
					},
				}, nil
			}),
		},
	}

	run := func() (string, error) {
		cmd := newGenerateCommandForTest(opts)
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})
		return stdout.String(), cmd.Execute()
	}

	if _, err := run(); err == nil {
		t.Fatal("expected first generate call to fail")
	}
	secondOut, err := run()
	if err != nil {
		t.Fatalf("expected second generate call to succeed, got: %v", err)
	}

	generatedDir := filepath.Join(outputRoot, "example-scenario")
	stalePath := filepath.Join(generatedDir, "stale.tf")
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	thirdOut, err := run()
	if err != nil {
		t.Fatalf("expected third generate call to succeed, got: %v", err)
	}
	if secondOut != thirdOut {
		t.Fatalf("expected deterministic generate output on repeated success\nsecond:\n%s\nthird:\n%s", secondOut, thirdOut)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("expected stale file to be removed on repeated generate, stat err=%v", err)
	}
}

func TestValidateAndTestRetryAfterFailure(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)

	staticCalls := 0
	static := &fakeStaticHarness{}
	static.result = &harness.StaticResult{
		Stages: []harness.StageResult{
			{Stage: "init"},
			{Stage: "validate"},
			{Stage: "plan"},
			{Stage: "show"},
		},
		PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`),
	}
	static.err = nil

	validateOpts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Static: StaticHarnessRunnerFunc(func(ctx context.Context, dir string, env map[string]string) (*harness.StaticResult, error) {
				staticCalls++
				if staticCalls == 1 {
					return &harness.StaticResult{
							Stages: []harness.StageResult{
								{Stage: "init"},
								{Stage: "validate", Cmd: []string{"tofu", "validate"}},
							},
						}, &harness.StageError{
							StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}},
							Err:         errors.New("transient validate failure"),
						}
				}
				return static.Run(ctx, dir, env)
			}),
		},
	}

	validateRun := func() (string, error) {
		cmd := newValidateCommandForTest(validateOpts)
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})
		return stdout.String(), cmd.Execute()
	}
	if _, err := validateRun(); err == nil {
		t.Fatal("expected first validate call to fail")
	}
	validateSecond, err := validateRun()
	if err != nil {
		t.Fatalf("expected second validate call to succeed, got: %v", err)
	}
	validateThird, err := validateRun()
	if err != nil {
		t.Fatalf("expected third validate call to succeed, got: %v", err)
	}
	if validateSecond != validateThird {
		t.Fatalf("expected deterministic validate output on repeated success")
	}

	mockCalls := 0
	destroy := &fakeDestroyHarness{
		result: &harness.DestroyResult{
			Destroy:       harness.StageResult{Stage: "destroy"},
			StateSnapshot: []byte(`{}`),
			OrphanCount:   0,
		},
	}

	testOpts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockDeploy: MockDeployHarnessRunnerFunc(func(context.Context, string, map[string]string) (*harness.MockDeployResult, error) {
				mockCalls++
				if mockCalls == 1 {
					return nil, &harness.MockDeployError{
						Stage: "apply",
						Apply: harness.StageResult{Stage: "apply", Cmd: []string{"tofu", "apply"}},
						Err:   errors.New("transient apply failure"),
					}
				}
				return &harness.MockDeployResult{
					Apply:         harness.StageResult{Stage: "apply"},
					StateSnapshot: []byte(`{"mock":true}`),
				}, nil
			}),
			Destroy: destroy,
		},
	}

	testRun := func() (string, error) {
		cmd := newTestCommandForTest(testOpts)
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})
		return stdout.String(), cmd.Execute()
	}
	if _, err := testRun(); err == nil {
		t.Fatal("expected first test call to fail")
	}
	testSecond, err := testRun()
	if err != nil {
		t.Fatalf("expected second test call to succeed, got: %v", err)
	}
	testThird, err := testRun()
	if err != nil {
		t.Fatalf("expected third test call to succeed, got: %v", err)
	}
	if testSecond != testThird {
		t.Fatalf("expected deterministic test output on repeated success")
	}
}

func TestRunAndMockCommandsRemainSafeOnRepeatedExecution(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)

	runOpts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Validation.Layers.SandboxDeploy.Enabled = true
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
	}

	runExec := func() (string, error) {
		cmd := newRunCommandForTest(runOpts)
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})
		return stdout.String(), cmd.Execute()
	}

	firstOut, firstErr := runExec()
	if firstErr == nil {
		t.Fatal("expected blocked run error")
	}
	secondOut, secondErr := runExec()
	if secondErr == nil {
		t.Fatal("expected blocked run error on repeated execution")
	}
	if firstOut != secondOut {
		t.Fatalf("expected deterministic blocked-run output on repeat")
	}

	starter := &fakeMockStarter{err: errors.New("transient docker failure")}
	startOpts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockStart: MockStarterFunc(func(context.Context, config.MockwayConfig) error {
				if starter.err != nil {
					err := starter.err
					starter.err = nil
					return err
				}
				return nil
			}),
			MockStop:   &fakeMockLifecycle{},
			MockStatus: &fakeMockLifecycle{statusValue: "Up"},
			MockLogs:   &fakeMockLifecycle{logsValue: "line1"},
		},
	}

	mockStart := func() (string, error) {
		cmd := newMockStartCommandForTest(startOpts)
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"--config", h.ConfigPath})
		return stdout.String(), cmd.Execute()
	}
	if _, err := mockStart(); err == nil {
		t.Fatal("expected first mock start to fail")
	}
	if out, err := mockStart(); err != nil {
		t.Fatalf("expected retry mock start success, got %v (output=%s)", err, out)
	}

	for name, build := range map[string]func() *cobra.Command{
		"mock stop":   func() *cobra.Command { return newMockStopCommandForTest(startOpts) },
		"mock status": func() *cobra.Command { return newMockStatusCommandForTest(startOpts) },
		"mock logs":   func() *cobra.Command { return newMockLogsCommandForTest(startOpts) },
	} {
		run := func() (string, error) {
			cmd := build()
			stdout := &bytes.Buffer{}
			cmd.SetOut(stdout)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{"--config", h.ConfigPath})
			return stdout.String(), cmd.Execute()
		}
		out1, err1 := run()
		if err1 != nil {
			t.Fatalf("%s first execution failed: %v", name, err1)
		}
		out2, err2 := run()
		if err2 != nil {
			t.Fatalf("%s second execution failed: %v", name, err2)
		}
		if strings.TrimSpace(out1) != strings.TrimSpace(out2) {
			t.Fatalf("%s output not deterministic on repeat\nfirst:\n%s\nsecond:\n%s", name, out1, out2)
		}
	}
}

type StaticHarnessRunnerFunc func(context.Context, string, map[string]string) (*harness.StaticResult, error)

func (f StaticHarnessRunnerFunc) Run(ctx context.Context, workDir string, env map[string]string) (*harness.StaticResult, error) {
	return f(ctx, workDir, env)
}

type MockDeployHarnessRunnerFunc func(context.Context, string, map[string]string) (*harness.MockDeployResult, error)

func (f MockDeployHarnessRunnerFunc) Run(ctx context.Context, outputDir string, env map[string]string) (*harness.MockDeployResult, error) {
	return f(ctx, outputDir, env)
}

type MockStarterFunc func(context.Context, config.MockwayConfig) error

func (f MockStarterFunc) Start(ctx context.Context, cfg config.MockwayConfig) error {
	return f(ctx, cfg)
}
