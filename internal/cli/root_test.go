package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

func TestNewRootCmdHasExpectedCommands(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()
	expected := []string{"init", "generate", "validate", "test", "run", "mock", "ui"}

	for _, name := range expected {
		if _, _, err := root.Find([]string{name}); err != nil {
			t.Fatalf("expected command %q to be wired: %v", name, err)
		}
	}

	for _, sub := range []string{"start", "stop", "status", "logs"} {
		if _, _, err := root.Find([]string{"mock", sub}); err != nil {
			t.Fatalf("expected command %q to be wired: %v", "mock "+sub, err)
		}
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
	root := NewRootCmd()
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
