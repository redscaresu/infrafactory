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
)

// alternatingStaticHarness fails with one of two errors based on the
// 0-indexed call count modulo 2. Used to simulate the model alternating
// between two incomplete fixes (oscillation).
type alternatingStaticHarness struct {
	calls int
	errs  [2]error
}

func (a *alternatingStaticHarness) Run(ctx context.Context, workDir string, env map[string]string) (*harness.StaticResult, error) {
	idx := a.calls % 2
	a.calls++
	return nil, a.errs[idx]
}

// TestRunCommandLearnsPitfallFromOscillation drives the run loop with a
// stub static harness that alternates between two distinct failures over
// four iterations (A, B, A, B), exhausts the repair budget, and verifies
// the run loop extracts a pitfall from the oscillating signature whose
// detail matches a known ExtractLearnedPitfall pattern.
func TestRunCommandLearnsPitfallFromOscillation(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)

	pitfallsDir := filepath.Join(h.WorkspaceDir, "pitfalls")
	if err := os.MkdirAll(pitfallsDir, 0o755); err != nil {
		t.Fatalf("mkdir pitfalls: %v", err)
	}

	// Detail A is extractable: ExtractLearnedPitfall recognizes the K8s
	// minor-version + auto_upgrade pattern when it sees both phrases plus
	// a scaleway_ resource name in the text.
	detailA := `exit status 1 | stderr: minor version 1.31 must only be used with auto upgrade enabled, on resource scaleway_k8s_cluster.main`
	// Detail B is *generic* and intentionally not extractable. The oscillation
	// detector still surfaces it as oscillating, but ExtractLearnedPitfall
	// returns nil for it, exercising the "skip non-extractable" branch in
	// run_command.go without polluting the learned pitfall file.
	detailB := `exit status 1 | stderr: validation failed`

	stageErrA := &harness.StageError{
		StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}, Stderr: detailA},
		Err:         errors.New(detailA),
	}
	stageErrB := &harness.StageError{
		StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}, Stderr: detailB},
		Err:         errors.New(detailB),
	}

	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Agent.RepairIterationsMax = 4
			cfg.Paths.Pitfalls = pitfallsDir
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
			}),
			Static:     &alternatingStaticHarness{errs: [2]error{stageErrA, stageErrB}},
			MockDeploy: &fakeMockDeployHarness{},
			Destroy:    &fakeDestroyHarness{},
		},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run failure when budget is exhausted")
	}

	if !strings.Contains(stdout.String(), "run/terminal_reason: pass (repair_budget_exhausted)") {
		t.Fatalf("expected repair_budget_exhausted terminal reason, got:\n%s", stdout.String())
	}

	// Scenario default cloud is scaleway, so pitfalls file lands at
	// pitfalls/scaleway.yaml. The K8s detail (errA) should produce one
	// learned pitfall; errB is generic and should be skipped.
	pitfallsPath := filepath.Join(pitfallsDir, "scaleway.yaml")
	data, err := os.ReadFile(pitfallsPath)
	if err != nil {
		t.Fatalf("expected learned pitfalls file at %s: %v", pitfallsPath, err)
	}
	contents := string(data)
	for _, want := range []string{
		"resource: scaleway_k8s_cluster",
		"source: learned",
		"discovered_from: example-scenario",
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("expected learned pitfalls to contain %q, got:\n%s", want, contents)
		}
	}
	// The generic errB rule must NOT have been written.
	if strings.Contains(contents, "validation failed") {
		t.Fatalf("expected non-extractable generic failure to be skipped, got:\n%s", contents)
	}
}

// TestRunCommandSkipsOscillationLearningWhenNoOscillation ensures the
// oscillation pitfall path stays inert when failures are sustained
// (linear) — only target_reached/iter-N->N-1 path should write learned
// pitfalls in that case, and that path is unrelated to oscillation.
func TestRunCommandSkipsOscillationLearningWhenNoOscillation(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)

	pitfallsDir := filepath.Join(h.WorkspaceDir, "pitfalls")
	if err := os.MkdirAll(pitfallsDir, 0o755); err != nil {
		t.Fatalf("mkdir pitfalls: %v", err)
	}

	detailA := `exit status 1 | stderr: minor version 1.31 must only be used with auto upgrade enabled, on resource scaleway_k8s_cluster.main`
	stageErr := &harness.StageError{
		StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}, Stderr: detailA},
		Err:         errors.New(detailA),
	}

	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Agent.RepairIterationsMax = 4
			cfg.Paths.Pitfalls = pitfallsDir
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
			}),
			Static:     &fakeStaticHarness{err: stageErr},
			MockDeploy: &fakeMockDeployHarness{},
			Destroy:    &fakeDestroyHarness{},
		},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected run failure")
	}

	// Stuck detection fires before repair budget when failures repeat.
	if !strings.Contains(stdout.String(), "run/terminal_reason: pass (stuck)") {
		t.Fatalf("expected stuck terminal reason for sustained failures, got:\n%s", stdout.String())
	}

	// No oscillation → oscillation-pitfall logic must not have produced
	// a pitfalls file.
	pitfallsPath := filepath.Join(pitfallsDir, "scaleway.yaml")
	if _, err := os.Stat(pitfallsPath); err == nil {
		t.Fatalf("expected no pitfalls file when no oscillation occurred")
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected stat error: %v", err)
	}
}
