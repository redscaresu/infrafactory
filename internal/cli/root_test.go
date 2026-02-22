package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
)

func TestNewRootCmdHasExpectedCommands(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()
	expected := []string{"init", "generate", "validate", "test", "run", "mock"}

	for _, name := range expected {
		if _, _, err := root.Find([]string{name}); err != nil {
			t.Fatalf("expected command %q to be wired: %v", name, err)
		}
	}

	if _, _, err := root.Find([]string{"mock", "start"}); err != nil {
		t.Fatalf("expected command %q to be wired: %v", "mock start", err)
	}
}

func TestStubLeafCommandsReturnNotImplemented(t *testing.T) {
	t.Parallel()

	configPath := writeConfigFixture(t)

	cases := []struct {
		name          string
		args          []string
		needsConfig   bool
		expectedError error
	}{
		{name: "init", args: []string{"init"}, expectedError: ErrNotImplemented},
		{name: "generate", args: []string{"generate"}, needsConfig: true, expectedError: ErrNotImplemented},
		{name: "validate", args: []string{"validate"}, needsConfig: true, expectedError: ErrNotImplemented},
		{name: "test", args: []string{"test"}, needsConfig: true, expectedError: ErrNotImplemented},
		{name: "run", args: []string{"run"}, needsConfig: true, expectedError: ErrNotImplemented},
		{name: "mock start", args: []string{"mock", "start"}, needsConfig: true, expectedError: ErrNotImplemented},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := NewRootCmd()
			args := tc.args
			if tc.needsConfig {
				args = append(args, "--config", configPath)
			}
			root.SetArgs(args)
			err := root.Execute()
			if !errors.Is(err, tc.expectedError) {
				t.Fatalf("expected %v, got: %v", tc.expectedError, err)
			}
		})
	}
}

func TestConfigBackedCommandsReturnConfigLoadError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
	}{
		{name: "generate", args: []string{"generate"}},
		{name: "validate", args: []string{"validate"}},
		{name: "test", args: []string{"test"}},
		{name: "run", args: []string{"run"}},
		{name: "mock start", args: []string{"mock", "start"}},
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
