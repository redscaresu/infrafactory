package harness

import (
	"context"
	"errors"
	"testing"
)

func TestSandboxDestroyHarnessRunSuccess(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		responses: []runnerResponse{
			{result: CommandResult{Stdout: []byte("destroy complete")}},
		},
	}

	h := NewSandboxDestroyHarness(runner)
	out, err := h.Run(context.Background(), "/tmp/workdir", map[string]string{"SCW_ACCESS_KEY": "real"})
	if err != nil {
		t.Fatalf("run sandbox destroy harness: %v", err)
	}
	if out.Destroy.Stage != "destroy" {
		t.Fatalf("unexpected result: %+v", out)
	}

	got := append([]string{runner.calls[0].Name}, runner.calls[0].Args...)
	expected := []string{"tofu", "destroy", "-auto-approve", "-state=" + LiveStateFilename}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("command mismatch: got %v want %v", got, expected)
		}
	}
}

func TestSandboxDestroyHarnessRunFailure(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		responses: []runnerResponse{
			{result: CommandResult{Stderr: []byte("destroy stderr")}, err: errors.New("destroy failed")},
		},
	}

	h := NewSandboxDestroyHarness(runner)
	_, err := h.Run(context.Background(), "/tmp/workdir", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrSandboxDestroyFailed) {
		t.Fatalf("expected ErrSandboxDestroyFailed, got %v", err)
	}

	var destroyErr *SandboxDestroyError
	if !errors.As(err, &destroyErr) {
		t.Fatalf("expected *SandboxDestroyError, got %T", err)
	}
	if destroyErr.Stage != "destroy" {
		t.Fatalf("expected destroy stage, got %q", destroyErr.Stage)
	}
}
