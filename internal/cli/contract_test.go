package cli

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestExitCodeForErrorTofuExitCode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "carried 2", err: &TofuExitError{ExitCode: 2}, want: 2},
		{name: "carried 2 wrapped", err: fmt.Errorf("rehearse plan: %w", &TofuExitError{ExitCode: 2}), want: 2},
		{name: "carried 255", err: &TofuExitError{ExitCode: 255}, want: 255},
		{name: "carried 0", err: &TofuExitError{ExitCode: 0}, want: ExitCodeRuntime},
		{name: "carried -1", err: &TofuExitError{ExitCode: -1}, want: ExitCodeRuntime},
		{name: "carried 256", err: &TofuExitError{ExitCode: 256}, want: ExitCodeRuntime},
		{name: "carried 1000", err: &TofuExitError{ExitCode: 1000}, want: ExitCodeRuntime},
		{name: "carried 3 wrapped", err: fmt.Errorf("x: %w", &TofuExitError{ExitCode: 3}), want: 3},
		{name: "carried 2 joined with another failure", err: errors.Join(&TofuExitError{ExitCode: 2}, errors.New("tighten")), want: ExitCodeRuntime},
		{name: "carried 2 beside another failure in one Errorf", err: fmt.Errorf("%w: %w", &TofuExitError{ExitCode: 2}, errors.New("tighten")), want: ExitCodeRuntime},
		{name: "usage joined with carried 2", err: errors.Join(&CLIError{Code: errorCodeUsage, Err: errors.New("usage")}, &TofuExitError{ExitCode: 2}), want: ExitCodeUsage},
		{name: "usage wrapping carried 3", err: &CLIError{Code: errorCodeUsage, Err: &TofuExitError{ExitCode: 3}}, want: ExitCodeUsage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, ExitCodeForError(tc.err))
		})
	}
}

func TestReportError(t *testing.T) {
	t.Parallel()

	usageErr := &CLIError{Op: "run", Code: errorCodeUsage, Err: errors.New("expected exactly 1 scenario path argument")}
	carrierAndTighten := errors.Join(&TofuExitError{ExitCode: 2}, errors.New("tighten"))
	usageAndCarrier := errors.Join(usageErr, &TofuExitError{ExitCode: 2})
	usageWrapsCarrier := &CLIError{Op: "rehearse plan", Code: errorCodeUsage, Err: &TofuExitError{ExitCode: 3}}
	cases := []struct {
		name       string
		err        error
		wantCode   int
		wantStderr string
	}{
		{name: "nil", err: nil, wantCode: ExitCodeSuccess, wantStderr: ""},
		{name: "quiet tofu exit", err: &TofuExitError{ExitCode: 2}, wantCode: 2, wantStderr: ""},
		{name: "quiet tofu exit wrapped", err: fmt.Errorf("rehearse plan: %w", &TofuExitError{ExitCode: 3}), wantCode: 3, wantStderr: ""},
		{name: "tofu exit -1 is written", err: &TofuExitError{ExitCode: -1}, wantCode: ExitCodeRuntime, wantStderr: "tofu exited with code -1\n"},
		{name: "usage error is written", err: usageErr, wantCode: ExitCodeUsage, wantStderr: usageErr.Error() + "\n"},
		{name: "other error is written", err: errors.New("boom"), wantCode: ExitCodeRuntime, wantStderr: "boom\n"},
		{name: "carrier joined with another failure is written", err: carrierAndTighten, wantCode: ExitCodeRuntime, wantStderr: carrierAndTighten.Error() + "\n"},
		{name: "usage joined with carrier is written", err: usageAndCarrier, wantCode: ExitCodeUsage, wantStderr: usageAndCarrier.Error() + "\n"},
		{name: "usage wrapping carrier is written", err: usageWrapsCarrier, wantCode: ExitCodeUsage, wantStderr: usageWrapsCarrier.Error() + "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			assert.Equal(t, tc.wantCode, ReportError(tc.err, &stderr))
			assert.Equal(t, tc.wantStderr, stderr.String())
		})
	}
}
