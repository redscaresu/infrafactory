package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// TestRunCommand_ParallelSubtestsAreWorkspaceIsolated guards against
// the regression diagnosed in the May 2026 CI failure: when two
// parallel subtests share the same `./output` and `.infrafactory/runs`
// roots (the relative-path defaults), one subtest's `os.RemoveAll`
// wipes another's freshly-written `project.tf`, which surfaces as a
// layer-3 "missing scaleway_account_project" failure on Linux that
// the macOS filesystem masks.
//
// This test runs the run command in 8 parallel t.Run subtests using
// the same workspace-isolated configuration as
// TestCommandOutputGoldenSnapshots/run. If anyone changes that
// pattern in a way that re-shares the relative-path defaults, this
// test fails on Linux (and intermittently on macOS), surfacing the
// regression before it reaches CI.
func TestRunCommand_ParallelSubtestsAreWorkspaceIsolated(t *testing.T) {
	t.Parallel()

	for i := 0; i < 8; i++ {
		i := i
		t.Run("parallel_invocation", func(t *testing.T) {
			t.Parallel()

			h := newCommandTestHarness(t)
			opts := runtimeOptions{
				configLoader: func(path string) (config.Config, error) {
					cfg, err := config.Load(path)
					if err != nil {
						return config.Config{}, err
					}
					cfg.Paths.Output = filepath.Join(h.WorkspaceDir, "output")
					cfg.Validation.Layers.SandboxDeploy.Enabled = true
					return cfg, nil
				},
				scenarioLoader: defaultScenarioLoader,
				runstoreRoot:   filepath.Join(h.WorkspaceDir, ".infrafactory", "runs"),
				deps: RuntimeDependencies{
					Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
						return &generator.GeneratedCode{Files: map[string][]byte{
							"main.tf":    []byte("terraform {}\n"),
							"project.tf": []byte("resource \"scaleway_account_project\" \"sandbox\" { name = \"test\" }\n"),
						}}, nil
					}),
					Static: &fakeStaticHarness{result: &harness.StaticResult{
						Stages:   []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}},
						PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`),
					}},
					MockDeploy: &fakeMockDeployHarness{
						result: &harness.MockDeployResult{Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{}`)},
					},
					Destroy: &fakeDestroyHarness{
						result: &harness.DestroyResult{Destroy: harness.StageResult{Stage: "destroy"}, StateSnapshot: []byte(`{}`)},
					},
					MockState: &fakeRunMockStateClient{statePayload: []byte(`{"instance":{"servers":[]}}`)},
				},
			}
			cmd := newRunCommandForTest(opts)
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			cmd.SetOut(stdout)
			cmd.SetErr(stderr)
			cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath, "--output", string(OutputModeJSON)})

			_ = cmd.Execute()
			out := stdout.String()

			// The hermetic seed-generator produces a project.tf with
			// scaleway_account_project, so the layer-3 gate must never
			// trip — its presence in stdout means a parallel subtest
			// clobbered our output dir. Similarly, "directory not empty"
			// on the runstore reset means parallel subtests shared the
			// same .infrafactory/runs/<scenario>/<runID> dir.
			if strings.Contains(out, "scaleway_account_project resource") {
				t.Fatalf("invocation %d: layer-3 fired — cfg.Paths.Output is not workspace-isolated; output:\n%s", i, out)
			}
			if strings.Contains(out, "directory not empty") {
				t.Fatalf("invocation %d: runstore reset failed — runstoreRoot is not workspace-isolated; output:\n%s", i, out)
			}
		})
	}
}
