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
	"github.com/redscaresu/infrafactory/internal/scenario"
)

func TestRehearseIsRegisteredAndPrintsItsHelp(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()
	found, _, err := root.Find([]string{"rehearse"})
	require.NoError(t, err)
	assert.Equal(t, "rehearse", found.Name())

	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"rehearse"})

	require.NoError(t, root.Execute())
	assert.Contains(t, stdout.String(), found.Short)
}

// The child stands in for `prepare`: a subcommand's flag parse error,
// missing required flag and broken flag group must exit 2 too, and a valid
// call must still run.
func TestRehearseUsageErrorsExitTwo(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{"rehearse", "bogus"}, ExitCodeUsage},
		{[]string{"rehearse", "--nope"}, ExitCodeUsage},
		{[]string{"--output", "yaml", "rehearse"}, ExitCodeUsage},
		{[]string{"rehearse", "child", "--need", "v", "--nope"}, ExitCodeUsage},
		{[]string{"rehearse", "child"}, ExitCodeUsage},
		{[]string{"rehearse", "child", "--need", "v", "--x", "--y"}, ExitCodeUsage},
		{[]string{"rehearse", "child", "--need", "v"}, ExitCodeSuccess},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			t.Parallel()

			root := NewRootCmd()
			group, _, err := root.Find([]string{"rehearse"})
			require.NoError(t, err)
			child := &cobra.Command{Use: "child", RunE: func(*cobra.Command, []string) error { return nil }}
			child.Flags().String("need", "", "")
			require.NoError(t, child.MarkFlagRequired("need"))
			child.Flags().Bool("x", false, "")
			child.Flags().Bool("y", false, "")
			child.MarkFlagsMutuallyExclusive("x", "y")
			group.AddCommand(child)
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			root.SetArgs(tc.args)

			assert.Equal(t, tc.want, ExitCodeForError(root.Execute()))
		})
	}
}

// A subcommand's own persistent pre-run would shadow the group's, which
// holds the family's exit-2 rule and the root's --output check (ADR-0042).
func TestRehearseSubcommandsSetNoPersistentPreRun(t *testing.T) {
	t.Parallel()

	group, _, err := NewRootCmd().Find([]string{"rehearse"})
	require.NoError(t, err)
	assert.Empty(t, ownPersistentPreRuns(group))

	// The walk must see a hook at any depth, or the check above is vacuous.
	nested := &cobra.Command{Use: "nested"}
	nested.AddCommand(&cobra.Command{Use: "deep", PersistentPreRunE: func(*cobra.Command, []string) error { return nil }})
	group.AddCommand(&cobra.Command{Use: "planted", PersistentPreRun: func(*cobra.Command, []string) {}}, nested)
	assert.ElementsMatch(t, []string{"infrafactory rehearse planted", "infrafactory rehearse nested deep"}, ownPersistentPreRuns(group))
}

func ownPersistentPreRuns(group *cobra.Command) []string {
	var paths []string
	for _, c := range group.Commands() {
		if c.PersistentPreRunE != nil || c.PersistentPreRun != nil {
			paths = append(paths, c.CommandPath())
		}
		paths = append(paths, ownPersistentPreRuns(c)...)
	}
	return paths
}

func TestNewRootCmdHasExpectedCommands(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()
	expected := []string{"init", "generate", "validate", "test", "run", "mock", "ui"}

	// Find returns the deepest match without error, so a missing `mock <sub>`
	// comes back as `mock`: the name is the check, not the error.
	for _, name := range expected {
		found, _, err := root.Find([]string{name})
		require.NoError(t, err, name)
		require.Equal(t, name, found.Name())
	}

	for _, sub := range []string{"start", "stop", "status", "logs"} {
		found, _, err := root.Find([]string{"mock", sub})
		require.NoError(t, err, "mock "+sub)
		require.Equal(t, sub, found.Name(), "mock "+sub)
	}
}

func TestInitWritesScaffoldAndPrintsNextSteps(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scenarioPath := filepath.Join(tmpDir, "scenarios", "training", "new-scenario.yaml")

	root := NewRootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"init", "--path", scenarioPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute init: %v", err)
	}

	content, err := os.ReadFile(scenarioPath)
	if err != nil {
		t.Fatalf("read scaffold: %v", err)
	}
	if string(content) != defaultScenarioScaffold() {
		t.Fatalf("unexpected scaffold content\nexpected:\n%s\nactual:\n%s", defaultScenarioScaffold(), string(content))
	}

	schemaPath := filepath.Join("..", "..", scenario.DefaultSchemaPath)
	if _, err := scenario.LoadWithSchema(scenarioPath, schemaPath); err != nil {
		t.Fatalf("expected scaffold to pass schema validation: %v", err)
	}

	output := stdout.String()
	expectedLines := []string{
		"Created scenario scaffold: " + scenarioPath,
		"Next steps:",
		"1. Edit " + scenarioPath + " and replace placeholder values.",
		"2. infrafactory generate " + scenarioPath + " --config " + config.DefaultPath,
		"3. infrafactory run " + scenarioPath + " --config " + config.DefaultPath,
	}
	for _, expected := range expectedLines {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected output to contain %q, got: %s", expected, output)
		}
	}
}

func TestInitReturnsErrorWhenScaffoldAlreadyExists(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scenarioPath := filepath.Join(tmpDir, "scenarios", "training", "existing.yaml")
	if err := os.MkdirAll(filepath.Dir(scenarioPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(scenarioPath, []byte("scenario: preexisting\n"), 0o644); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"init", "--path", scenarioPath})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "create scenario scaffold") {
		t.Fatalf("expected create scenario scaffold error, got: %v", err)
	}
}

func TestGenerateCommandReturnsRuntimeErrorWithDefaultGenerator(t *testing.T) {
	t.Parallel()

	configPath := writeConfigFixture(t)
	root := NewRootCmd(withSchemaRunner(recordedSchemaRunner{}))
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"generate", filepath.Join("..", "..", "scenarios", "training", "web-app-paris.yaml"), "--config", configPath})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected concrete generator failure, got not implemented: %v", err)
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *CLIError, got %T (%v)", err, err)
	}
	if cliErr.Op != "generate" {
		t.Fatalf("expected generate op, got %q", cliErr.Op)
	}
}

// NewRootCmd hands withSchemaRunner's runner to the runtimes its commands
// build, with injected dependencies or without them, as
// CommandTestHarness.Run uses it.
func TestRootCommandExtractsWithSchemaRunner(t *testing.T) {
	t.Parallel()

	t.Run("injected generator gets the recorded schema", func(t *testing.T) {
		t.Parallel()
		h := newCommandTestHarness(t)
		var got []byte
		root := NewRootCmd(withSchemaRunner(recordedSchemaRunner{}), WithRuntimeDependencies(RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(_ context.Context, req generator.Request) (*generator.GeneratedCode, error) {
				got = req.ProviderSchemaJSON
				return nil, errors.New("stop after the request")
			}),
		}))
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs([]string{"generate", h.ScenarioPath, "--config", h.ConfigPath})

		require.ErrorContains(t, root.Execute(), "stop after the request")
		assert.Equal(t, recordedProviderSchema, got)
	})

	t.Run("default dependencies use the runner", func(t *testing.T) {
		t.Parallel()
		h := newCommandTestHarness(t)
		// The default generator is the real Claude adapter: give it the
		// repo's prompts and a claude that refuses, so the run ends at a
		// known refusal and never at a real LLM.
		claude := filepath.Join(t.TempDir(), "claude")
		writeScript(t, claude, "#!/bin/sh\necho stub claude refused >&2\nexit 1\n")
		prompts, err := filepath.Abs(filepath.Join("..", "..", "prompts"))
		require.NoError(t, err)
		raw, err := os.ReadFile(h.ConfigPath)
		require.NoError(t, err)
		raw = []byte(strings.Replace(string(raw), "  type: claude-code\n",
			"  type: claude-code\n  claude:\n    command: "+claude+"\n", 1) +
			"paths:\n  prompts: " + prompts + "\n  output: " + h.OutputDir() + "\n")
		require.NoError(t, os.WriteFile(h.ConfigPath, raw, 0o600))
		runner := &countingSchemaRunner{}
		root := NewRootCmd(withSchemaRunner(runner))
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs([]string{"generate", h.ScenarioPath, "--config", h.ConfigPath})

		err = root.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stub claude refused")
		assert.Equal(t, 1, runner.schemas)
	})
}

func TestConfigBackedCommandsReturnConfigLoadError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
	}{
		{name: "generate", args: []string{"generate", filepath.Join("..", "..", "scenarios", "training", "web-app-paris.yaml")}},
		{name: "validate", args: []string{"validate", filepath.Join("..", "..", "scenarios", "training", "web-app-paris.yaml")}},
		{name: "test", args: []string{"test", filepath.Join("..", "..", "scenarios", "training", "web-app-paris.yaml")}},
		{name: "run", args: []string{"run", filepath.Join("..", "..", "scenarios", "training", "web-app-paris.yaml")}},
		{name: "mock start", args: []string{"mock", "start"}},
		{name: "ui", args: []string{"ui"}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := NewRootCmd()
			root.SetArgs(append(tc.args, "--config", filepath.Join(t.TempDir(), "missing.yaml")))
			err := root.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			var validationErr *config.ValidationError
			if errors.As(err, &validationErr) {
				t.Fatalf("expected file read error, got validation error: %v", err)
			}
		})
	}
}

func TestValidateCommandReturnsRuntimeErrorWhenToolingMissing(t *testing.T) {
	t.Parallel()

	configPath := writeConfigFixture(t)
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"validate", filepath.Join("..", "..", "scenarios", "training", "web-app-paris.yaml"), "--config", configPath})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *CLIError, got %T (%v)", err, err)
	}
	if cliErr.Op != "validate" {
		t.Fatalf("expected validate op, got %q", cliErr.Op)
	}
	if ExitCodeForError(err) != ExitCodeRuntime {
		t.Fatalf("expected runtime exit code, got %d", ExitCodeForError(err))
	}
}

func TestTestCommandReturnsRuntimeErrorWhenDependenciesFail(t *testing.T) {
	t.Parallel()

	configPath := writeConfigFixture(t)
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"test", filepath.Join("..", "..", "scenarios", "training", "web-app-paris.yaml"), "--config", configPath})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *CLIError, got %T (%v)", err, err)
	}
	if cliErr.Op != "test" {
		t.Fatalf("expected test op, got %q", cliErr.Op)
	}
	if ExitCodeForError(err) != ExitCodeRuntime {
		t.Fatalf("expected runtime exit code, got %d", ExitCodeForError(err))
	}
}

func TestRunCommandReturnsRuntimeErrorWhenSkeletonFails(t *testing.T) {
	t.Parallel()

	configPath := writeConfigFixture(t)
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"run", filepath.Join("..", "..", "scenarios", "training", "web-app-paris.yaml"), "--config", configPath})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *CLIError, got %T (%v)", err, err)
	}
	if cliErr.Op != "run" {
		t.Fatalf("expected run op, got %q", cliErr.Op)
	}
	if ExitCodeForError(err) != ExitCodeRuntime {
		t.Fatalf("expected runtime exit code, got %d", ExitCodeForError(err))
	}
}

func TestGenerateCommandReturnsConfigInvalidCode(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "infrafactory.yaml")
	if err := os.WriteFile(configPath, []byte("version: \"1.0\"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"generate", filepath.Join("..", "..", "scenarios", "training", "web-app-paris.yaml"), "--config", configPath})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *CLIError, got %T (%v)", err, err)
	}
	if cliErr.Op != "generate" || cliErr.Code != errorCodeConfigInvalid {
		t.Fatalf("expected generate/%s CLI error, got op=%q code=%q", errorCodeConfigInvalid, cliErr.Op, cliErr.Code)
	}
}

func TestGenerateCommandReturnsScenarioMalformedCode(t *testing.T) {
	t.Parallel()

	configPath := writeConfigFixture(t)
	tmpDir := t.TempDir()
	scenarioPath := filepath.Join(tmpDir, "malformed.yaml")
	if err := os.WriteFile(scenarioPath, []byte("scenario: [\n"), 0o644); err != nil {
		t.Fatalf("write scenario: %v", err)
	}

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"generate", scenarioPath, "--config", configPath})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *CLIError, got %T (%v)", err, err)
	}
	if cliErr.Op != "generate" || cliErr.Code != errorCodeScenarioMalformed {
		t.Fatalf("expected generate/%s CLI error, got op=%q code=%q", errorCodeScenarioMalformed, cliErr.Op, cliErr.Code)
	}
}

func writeConfigFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "infrafactory.yaml")
	content := `version: "1.0"
agent:
  type: claude-code
mockway:
  url: http://localhost:8080
`

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	return path
}

// rehearseRootWith is the root command with its `rehearse name` subcommand
// replaced by sub. It fails the test if `rehearse name` is not registered,
// since cobra's Find falls back to the nearest parent without an error.
func rehearseRootWith(t *testing.T, name string, sub *cobra.Command) *cobra.Command {
	t.Helper()
	root := NewRootCmd()
	group, _, err := root.Find([]string{"rehearse"})
	require.NoError(t, err)
	require.Equal(t, "rehearse", group.Name())
	old, _, err := group.Find([]string{name})
	require.NoError(t, err)
	require.Equal(t, name, old.Name(), "rehearse %s is not registered", name)
	group.RemoveCommand(old)
	group.AddCommand(sub)
	return root
}
