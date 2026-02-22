package cli

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"
)

func TestScenarioCommandsRequireScenarioArg(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	cases := []struct {
		name string
		args []string
	}{
		{name: "generate", args: []string{"generate"}},
		{name: "validate", args: []string{"validate"}},
		{name: "test", args: []string{"test"}},
		{name: "run", args: []string{"run"}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := h.Run(append(tc.args, "--config", h.ConfigPath)...)
			err := result.Err
			if err == nil {
				t.Fatal("expected error")
			}
			var cliErr *CLIError
			if !errors.As(err, &cliErr) {
				t.Fatalf("expected *CLIError, got %T (%v)", err, err)
			}
			if cliErr.Code != errorCodeUsage {
				t.Fatalf("expected usage code, got %q", cliErr.Code)
			}
			if ExitCodeForError(err) != ExitCodeUsage {
				t.Fatalf("expected usage exit code, got %d", ExitCodeForError(err))
			}
		})
	}
}

func TestOutputModeValidation(t *testing.T) {
	t.Parallel()

	validModes := []string{string(OutputModeHuman), string(OutputModeJSON)}
	for _, mode := range validModes {
		mode := mode
		t.Run("valid_"+mode, func(t *testing.T) {
			t.Parallel()

			root := NewRootCmd()
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			scenarioPath := filepath.Join(t.TempDir(), "scenario.yaml")
			root.SetArgs([]string{"init", "--path", scenarioPath, "--output", mode})
			if err := root.Execute(); err != nil {
				t.Fatalf("expected valid mode %q to pass, got: %v", mode, err)
			}
		})
	}

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"init", "--path", filepath.Join(t.TempDir(), "bad.yaml"), "--output", "yaml"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected invalid output mode error")
	}
	if !errors.Is(err, ErrInvalidOutputMode) {
		t.Fatalf("expected ErrInvalidOutputMode, got: %v", err)
	}
	if ExitCodeForError(err) != ExitCodeUsage {
		t.Fatalf("expected usage exit code, got %d", ExitCodeForError(err))
	}
}

func TestExitCodeForError(t *testing.T) {
	t.Parallel()

	if ExitCodeForError(nil) != ExitCodeSuccess {
		t.Fatalf("expected success exit code %d", ExitCodeSuccess)
	}
	if ExitCodeForError(errors.New("boom")) != ExitCodeRuntime {
		t.Fatalf("expected runtime exit code %d", ExitCodeRuntime)
	}

	usageErr := &CLIError{Op: "run", Code: errorCodeUsage, Err: errors.New("usage")}
	if ExitCodeForError(usageErr) != ExitCodeUsage {
		t.Fatalf("expected usage exit code %d", ExitCodeUsage)
	}
}
