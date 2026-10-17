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
	"github.com/stretchr/testify/require"
)

func TestCommandOutputGoldenSnapshots(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		build     func(*CommandTestHarness) *cobra.Command
		args      func(*CommandTestHarness, OutputMode) []string
		expectErr bool
		normalize func(*CommandTestHarness, string) string
		// modes is the output modes the case runs in: both when nil.
		modes []OutputMode
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
						cfg.Paths.Output = filepath.Join(h.WorkspaceDir, "output")
						cfg.Validation.Layers.SandboxDeploy.Enabled = true
						return cfg, nil
					},
					runstoreRoot:   filepath.Join(h.WorkspaceDir, ".infrafactory", "runs"),
					scenarioLoader: defaultScenarioLoader,
					deps: RuntimeDependencies{
						Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
							return &generator.GeneratedCode{Files: map[string][]byte{
								"main.tf":   []byte("terraform {}\n"),
								"volume.tf": []byte("resource \"scaleway_block_volume\" \"sandbox\" { size_in_gb = 1 }\n"),
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
		{
			name: "rehearse_prepare",
			build: func(h *CommandTestHarness) *cobra.Command {
				return goldenPrepareRoot(h, "resource \"terraform_data\" \"x\" {}\n")
			},
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return append([]string{"rehearse", "prepare", "envs/prod", "--output", string(mode)}, prepareFlags...)
			},
			normalize: normalizeGoldenWorkspace,
		},
		{
			name: "rehearse_prepare_refused",
			build: func(h *CommandTestHarness) *cobra.Command {
				return goldenPrepareRoot(h, "terraform {\n  cloud {}\n}\n")
			},
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return append([]string{"rehearse", "prepare", "envs/prod", "--output", string(mode)}, prepareFlags...)
			},
			expectErr: true,
			normalize: normalizeGoldenWorkspace,
		},
		{
			name: "rehearse_read",
			build: func(h *CommandTestHarness) *cobra.Command {
				return goldenReadRoot(h, readAccount)
			},
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return append([]string{"rehearse", "read", "--out", readOutDir, "--output", string(mode)}, readFlags()...)
			},
			normalize: normalizeGoldenWorkspace,
		},
		{
			name: "rehearse_read_refused",
			build: func(h *CommandTestHarness) *cobra.Command {
				return goldenReadRoot(h, "210987654321")
			},
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return append([]string{"rehearse", "read", "--out", readOutDir, "--output", string(mode)}, readFlags()...)
			},
			expectErr: true,
		},
		{
			name: "rehearse_end",
			build: func(h *CommandTestHarness) *cobra.Command {
				return goldenEndRoot(h, true)
			},
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return []string{"rehearse", "end", prepareID, "--output", string(mode)}
			},
		},
		{
			name: "rehearse_end_missing",
			build: func(h *CommandTestHarness) *cobra.Command {
				return goldenEndRoot(h, false)
			},
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return []string{"rehearse", "end", "missing-id", "--output", string(mode)}
			},
		},
		{
			name: "rehearse_end_bad_id",
			build: func(h *CommandTestHarness) *cobra.Command {
				return goldenEndRoot(h, false)
			},
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return []string{"rehearse", "end", "../x", "--output", string(mode)}
			},
			expectErr: true,
		},
		{
			// Passthrough refuses --output json, so the case runs in
			// human mode only: tofu's output, and nothing after it.
			name:  "rehearse_plan",
			build: goldenPassthroughRoot,
			args: func(h *CommandTestHarness, mode OutputMode) []string {
				return []string{"rehearse", "plan", prepareID, "--output", string(mode), "--", "-out=p"}
			},
			modes: []OutputMode{OutputModeHuman},
		},
	}

	for _, tc := range cases {
		tc := tc
		modes := tc.modes
		if modes == nil {
			modes = []OutputMode{OutputModeHuman, OutputModeJSON}
		}
		for _, mode := range modes {
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

// goldenPrepareRoot makes the workspace a repo whose envs/prod/main.tf
// holds mainTF, and returns a root that runs prepare from it.
func goldenPrepareRoot(h *CommandTestHarness, mainTF string) *cobra.Command {
	mustWriteFile(h.T, filepath.Join(h.WorkspaceDir, ".git", "HEAD"), "ref: refs/heads/main\n")
	mustWriteFile(h.T, filepath.Join(h.WorkspaceDir, "envs", "prod", "main.tf"), mainTF)
	mustWriteFile(h.T, filepath.Join(h.WorkspaceDir, "envs", "prod", "provider.tf"), prepareProvider)
	mustWriteFile(h.T, filepath.Join(h.WorkspaceDir, "envs", "prod", rehearsalLockFile), prepareLock)
	tofu := harness.CommandRunnerFunc(func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
		if isSchemaCall(c) {
			return harness.CommandResult{Stdout: []byte(prepareSchema)}, nil
		}
		return harness.CommandResult{}, nil
	})
	return prepareRoot(h.T, stubWalls(rehearsePrepare{runner: tofu, wd: h.WorkspaceDir, newID: func() string { return prepareID }}))
}

// goldenReadRoot makes the workspace a repo holding the read's key file
// and read set, and returns a root that reads from it through a doer
// answering as a key of account.
func goldenReadRoot(h *CommandTestHarness, account string) *cobra.Command {
	mustWriteFile(h.T, filepath.Join(h.WorkspaceDir, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeReadInputs(h.T, h.WorkspaceDir, readVPCEntry)
	root := readRoot(h.T, rehearseRead{doer: &readDoer{account: account}, wd: h.WorkspaceDir})
	root.SetErr(&bytes.Buffer{})
	return root
}

// goldenEndRoot makes the workspace a repo and returns a root that ends
// from it. With planted, it holds rehearsal prepareID with endPlanted and
// a record of a live fakeaws stub.
func goldenEndRoot(h *CommandTestHarness, planted bool) *cobra.Command {
	mustWriteFile(h.T, filepath.Join(h.WorkspaceDir, ".git", "HEAD"), "ref: refs/heads/main\n")
	if planted {
		dir := filepath.Join(h.WorkspaceDir, ".infrafactory", "rehearsals", prepareID)
		for _, rel := range endPlanted {
			mustWriteFile(h.T, filepath.Join(dir, rel), rel)
		}
		require.NoError(h.T, endStub(h.T, "fakeaws").write(dir), "write record")
	}
	return endRoot(h.T, rehearseEnd{wd: h.WorkspaceDir})
}

// goldenPassthroughRoot prepares a rehearsal with a live fakeaws stub
// and returns a root whose passthrough runs from that repo through a
// tofu stub that streams a plan's last line.
func goldenPassthroughRoot(h *CommandTestHarness) *cobra.Command {
	repo, _ := regatePreparedLive(h.T, passthroughToken)
	tofu := harness.CommandRunnerFunc(func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
		_, err := fmt.Fprintln(c.Stdout, "Plan: 1 to add, 0 to change, 0 to destroy.")
		return harness.CommandResult{}, err
	})
	return passthroughRoot(h.T, rehearsePassthrough{runner: tofu, wd: repo})
}

// normalizeGoldenWorkspace replaces the workspace, resolved, as prepare
// names it.
func normalizeGoldenWorkspace(h *CommandTestHarness, out string) string {
	resolved, err := filepath.EvalSymlinks(h.WorkspaceDir)
	if err != nil {
		h.T.Fatalf("resolve workspace: %v", err)
	}
	return strings.ReplaceAll(out, resolved, "<WORKSPACE>")
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
