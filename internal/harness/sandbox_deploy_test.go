package harness

import (
	"context"
	"errors"
	"testing"
)

func TestSandboxDeployHarnessRunSuccess(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		responses: []runnerResponse{
			{result: CommandResult{Stdout: []byte("init complete")}},
			{result: CommandResult{Stdout: []byte("plan complete")}},
			{result: CommandResult{Stdout: []byte("apply complete")}},
		},
	}

	h := NewSandboxDeployHarness(runner)
	out, err := h.Run(context.Background(), "/tmp/workdir", map[string]string{"SCW_ACCESS_KEY": "real"})
	if err != nil {
		t.Fatalf("run sandbox deploy harness: %v", err)
	}
	if out.Init.Stage != "init" || out.Plan.Stage != "plan" || out.Apply.Stage != "apply" {
		t.Fatalf("unexpected result: %+v", out)
	}

	expected := [][]string{
		{"tofu", "init"},
		{"tofu", "plan", "-state=" + LiveStateFilename},
		{"tofu", "apply", "-auto-approve", "-state=" + LiveStateFilename},
	}
	for i := range expected {
		got := append([]string{runner.calls[i].Name}, runner.calls[i].Args...)
		for j := range expected[i] {
			if got[j] != expected[i][j] {
				t.Fatalf("call %d mismatch: got %v want %v", i, got, expected[i])
			}
		}
	}
}

func TestSandboxDeployHarnessRunFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		responses     []runnerResponse
		expectedStage string
	}{
		{
			name: "init failure",
			responses: []runnerResponse{
				{result: CommandResult{Stderr: []byte("init stderr")}, err: errors.New("init failed")},
			},
			expectedStage: "init",
		},
		{
			name: "plan failure",
			responses: []runnerResponse{
				{result: CommandResult{Stdout: []byte("init complete")}},
				{result: CommandResult{Stderr: []byte("plan stderr")}, err: errors.New("plan failed")},
			},
			expectedStage: "plan",
		},
		{
			name: "apply failure",
			responses: []runnerResponse{
				{result: CommandResult{Stdout: []byte("init complete")}},
				{result: CommandResult{Stdout: []byte("plan complete")}},
				{result: CommandResult{Stderr: []byte("apply stderr")}, err: errors.New("apply failed")},
			},
			expectedStage: "apply",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := NewSandboxDeployHarness(&fakeRunner{responses: tc.responses})
			_, err := h.Run(context.Background(), "/tmp/workdir", nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, ErrSandboxDeployFailed) {
				t.Fatalf("expected ErrSandboxDeployFailed, got %v", err)
			}

			var deployErr *SandboxDeployError
			if !errors.As(err, &deployErr) {
				t.Fatalf("expected *SandboxDeployError, got %T", err)
			}
			if deployErr.Stage != tc.expectedStage {
				t.Fatalf("expected stage %q, got %q", tc.expectedStage, deployErr.Stage)
			}
		})
	}
}
