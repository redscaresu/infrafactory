package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/spf13/cobra"
)

func TestCommandOutputGoldenSnapshots(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		build     func(*CommandTestHarness) *cobra.Command
		args      func(*CommandTestHarness, OutputMode) []string
		expectErr bool
		normalize func(*CommandTestHarness, string) string
	}{
		{
			name: "init",
			build: func(h *CommandTestHarness) *cobra.Command {
				root := NewRootCmd()
				root.SetErr(&bytes.Buffer{})
				return root
			},
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return []string{
					"init",
					"--path", filepath.Join(h.WorkspaceDir, "scenarios", "training", "golden-init.yaml"),
					"--output", string(mode),
				}
			},
			normalize: func(h *CommandTestHarness, out string) string {
				return strings.ReplaceAll(out, h.WorkspaceDir, "<WORKSPACE>")
			},
		},
		{
			name: "generate",
			build: func(h *CommandTestHarness) *cobra.Command {
				opts := runtimeOptions{
					configLoader: func(path string) (config.Config, error) {
						cfg, err := config.Load(path)
						if err != nil {
							return config.Config{}, err
						}
						cfg.Paths.Output = filepath.Join(h.WorkspaceDir, "output")
						return cfg, nil
					},
					scenarioLoader: defaultScenarioLoader,
					deps: RuntimeDependencies{
						Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
							return &generator.GeneratedCode{
								Files: map[string][]byte{
									"main.tf": []byte("terraform {}\n"),
								},
							}, nil
						}),
					},
				}
				cmd := newGenerateCommandForTest(opts)
				cmd.SetErr(&bytes.Buffer{})
				return cmd
			},
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return []string{h.ScenarioPath, "--config", h.ConfigPath, "--output", string(mode)}
			},
		},
		{
			name: "validate",
			build: func(h *CommandTestHarness) *cobra.Command {
				opts := runtimeOptions{
					configLoader: func(path string) (config.Config, error) {
						cfg, err := config.Load(path)
						if err != nil {
							return config.Config{}, err
						}
						cfg.Validation.Layers.Static.PolicyPaths = nil
						return cfg, nil
					},
					scenarioLoader: defaultScenarioLoader,
					deps: RuntimeDependencies{
						Static: &fakeStaticHarness{
							result: &harness.StaticResult{
								Stages: []harness.StageResult{
									{Stage: "init"},
									{Stage: "validate"},
									{Stage: "plan"},
									{Stage: "show"},
								},
								PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`),
							},
						},
					},
				}
				cmd := newValidateCommandForTest(opts)
				cmd.SetErr(&bytes.Buffer{})
				return cmd
			},
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return []string{h.ScenarioPath, "--config", h.ConfigPath, "--output", string(mode)}
			},
		},
		{
			name: "test",
			build: func(h *CommandTestHarness) *cobra.Command {
				opts := runtimeOptions{
					configLoader: func(path string) (config.Config, error) {
						cfg, err := config.Load(path)
						if err != nil {
							return config.Config{}, err
						}
						cfg.Validation.Layers.MockDeploy.Enabled = false
						cfg.Validation.Layers.Destruction.Enabled = false
						return cfg, nil
					},
					scenarioLoader: defaultScenarioLoader,
					deps: RuntimeDependencies{
						MockDeploy: &fakeMockDeployHarness{},
						Destroy:    &fakeDestroyHarness{},
					},
				}
				cmd := newTestCommandForTest(opts)
				cmd.SetErr(&bytes.Buffer{})
				return cmd
			},
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return []string{h.ScenarioPath, "--config", h.ConfigPath, "--output", string(mode)}
			},
		},
		{
			name: "run",
			build: func(h *CommandTestHarness) *cobra.Command {
				opts := runtimeOptions{
					configLoader: func(path string) (config.Config, error) {
						cfg, err := config.Load(path)
						if err != nil {
							return config.Config{}, err
						}
						cfg.Validation.Layers.SandboxDeploy.Enabled = true
						return cfg, nil
					},
					scenarioLoader: defaultScenarioLoader,
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
							result: &harness.MockDeployResult{
								Apply:         harness.StageResult{Stage: "apply"},
								StateSnapshot: []byte(`{}`),
							},
						},
						Destroy: &fakeDestroyHarness{
							result: &harness.DestroyResult{
								Destroy:       harness.StageResult{Stage: "destroy"},
								StateSnapshot: []byte(`{}`),
								OrphanCount:   0,
							},
						},
						MockState: &fakeRunMockStateClient{statePayload: []byte(`{"instance":{"servers":[]}}`)},
					},
				}
				cmd := newRunCommandForTest(opts)
				cmd.SetErr(&bytes.Buffer{})
				return cmd
			},
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return []string{h.ScenarioPath, "--config", h.ConfigPath, "--output", string(mode)}
			},
			expectErr: true,
			normalize: func(h *CommandTestHarness, out string) string {
				out = strings.ReplaceAll(out, h.WorkspaceDir, "<WORKSPACE>")
				runIDPattern := regexp.MustCompile(`\b20\d{6}T\d{6}Z(?:[+-]\d{4})?\b`)
				return runIDPattern.ReplaceAllString(out, "<RUN_ID>")
			},
		},
		{
			name: "mock_start",
			build: func(h *CommandTestHarness) *cobra.Command {
				cmd := newMockStartCommandForTest(runtimeOptions{
					configLoader:   config.Load,
					scenarioLoader: defaultScenarioLoader,
					deps: RuntimeDependencies{
						MockStart: &fakeMockStarter{},
					},
				})
				cmd.SetErr(&bytes.Buffer{})
				return cmd
			},
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return []string{"--config", h.ConfigPath, "--output", string(mode)}
			},
		},
		{
			name: "mock_stop",
			build: func(h *CommandTestHarness) *cobra.Command {
				cmd := newMockStopCommandForTest(runtimeOptions{
					configLoader:   config.Load,
					scenarioLoader: defaultScenarioLoader,
					deps: RuntimeDependencies{
						MockStop: &fakeMockLifecycle{},
					},
				})
				cmd.SetErr(&bytes.Buffer{})
				return cmd
			},
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return []string{"--config", h.ConfigPath, "--output", string(mode)}
			},
		},
		{
			name: "mock_status",
			build: func(h *CommandTestHarness) *cobra.Command {
				cmd := newMockStatusCommandForTest(runtimeOptions{
					configLoader:   config.Load,
					scenarioLoader: defaultScenarioLoader,
					deps: RuntimeDependencies{
						MockStatus: &fakeMockLifecycle{statusValue: "Up 1 minute"},
					},
				})
				cmd.SetErr(&bytes.Buffer{})
				return cmd
			},
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return []string{"--config", h.ConfigPath, "--output", string(mode)}
			},
		},
		{
			name: "mock_logs",
			build: func(h *CommandTestHarness) *cobra.Command {
				cmd := newMockLogsCommandForTest(runtimeOptions{
					configLoader:   config.Load,
					scenarioLoader: defaultScenarioLoader,
					deps: RuntimeDependencies{
						MockLogs: &fakeMockLifecycle{logsValue: "line1\nline2"},
					},
				})
				cmd.SetErr(&bytes.Buffer{})
				return cmd
			},
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return []string{"--config", h.ConfigPath, "--output", string(mode)}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		for _, mode := range []OutputMode{OutputModeHuman, OutputModeJSON} {
			mode := mode
			t.Run(fmt.Sprintf("%s_%s", tc.name, mode), func(t *testing.T) {
				t.Parallel()
				h := newCommandTestHarness(t)
				cmd := tc.build(h)
				stdout := &bytes.Buffer{}
				cmd.SetOut(stdout)
				cmd.SetArgs(tc.args(h, mode))

				err := cmd.Execute()
				if tc.expectErr && err == nil {
					t.Fatalf("expected error for %s", tc.name)
				}
				if !tc.expectErr && err != nil {
					t.Fatalf("unexpected error for %s: %v", tc.name, err)
				}

				output := stdout.String()
				if tc.normalize != nil {
					output = tc.normalize(h, output)
				}

				assertCommandGolden(t, fmt.Sprintf("%s.%s.txt", tc.name, mode), []byte(output))
			})
		}
	}
}

func assertCommandGolden(t *testing.T, name string, actual []byte) {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file path")
	}
	path := filepath.Join(filepath.Dir(thisFile), "testdata", "golden", "commands", name)
	update := os.Getenv("UPDATE_GOLDEN") == "1"
	if update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}
		if err := os.WriteFile(path, actual, 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
	}

	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file %q: %v", path, err)
	}
	if string(expected) != string(actual) {
		t.Fatalf("golden mismatch for %s (set UPDATE_GOLDEN=1 to refresh)", path)
	}
}
