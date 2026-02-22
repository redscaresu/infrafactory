package harness

import (
	"context"
	"errors"
	"testing"
)

func TestStaticHarnessRunSuccess(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		responses: []runnerResponse{
			{result: CommandResult{}},
			{result: CommandResult{}},
			{result: CommandResult{}},
			{result: CommandResult{Stdout: []byte(`{"planned_values":{"root_module":{}}}`)}},
		},
	}

	h := NewStaticHarness(runner)
	out, err := h.Run(context.Background(), "/tmp/workdir", map[string]string{"SCW_API_URL": "http://localhost:8080"})
	if err != nil {
		t.Fatalf("run static harness: %v", err)
	}

	if len(out.Stages) != 4 {
		t.Fatalf("expected 4 stages, got %d", len(out.Stages))
	}
	if string(out.PlanJSON) != `{"planned_values":{"root_module":{}}}` {
		t.Fatalf("unexpected plan json: %s", string(out.PlanJSON))
	}

	expected := [][]string{
		{"tofu", "init"},
		{"tofu", "validate"},
		{"tofu", "plan", "-out=tfplan"},
		{"tofu", "show", "-json", "tfplan"},
	}
	if len(runner.calls) != len(expected) {
		t.Fatalf("expected %d runner calls, got %d", len(expected), len(runner.calls))
	}
	for i := range expected {
		got := append([]string{runner.calls[i].Name}, runner.calls[i].Args...)
		if len(got) != len(expected[i]) {
			t.Fatalf("call %d length mismatch: got %v want %v", i, got, expected[i])
		}
		for j := range expected[i] {
			if got[j] != expected[i][j] {
				t.Fatalf("call %d command mismatch: got %v want %v", i, got, expected[i])
			}
		}
	}
}

func TestStaticHarnessRunFailsAtStage(t *testing.T) {
	t.Parallel()

	rootErr := errors.New("validate failed")
	runner := &fakeRunner{
		responses: []runnerResponse{
			{result: CommandResult{}},
			{result: CommandResult{Stderr: []byte("bad config")}, err: rootErr},
		},
	}

	h := NewStaticHarness(runner)
	out, err := h.Run(context.Background(), "/tmp/workdir", nil)
	if err == nil {
		t.Fatal("expected stage error")
	}
	if !errors.Is(err, ErrStaticStageFailed) {
		t.Fatalf("expected ErrStaticStageFailed, got %v", err)
	}
	if !errors.Is(err, rootErr) {
		t.Fatalf("expected wrapped root err, got %v", err)
	}

	var stageErr *StageError
	if !errors.As(err, &stageErr) {
		t.Fatalf("expected *StageError, got %T", err)
	}
	if stageErr.Stage != "validate" {
		t.Fatalf("expected validate stage, got %q", stageErr.Stage)
	}
	if stageErr.Stderr != "bad config" {
		t.Fatalf("expected stderr capture, got %q", stageErr.Stderr)
	}
	if len(out.Stages) != 2 {
		t.Fatalf("expected two recorded stages before failure, got %d", len(out.Stages))
	}
}

func TestStaticHarnessRunFailsOnInvalidPlanJSON(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		responses: []runnerResponse{
			{result: CommandResult{}},
			{result: CommandResult{}},
			{result: CommandResult{}},
			{result: CommandResult{Stdout: []byte("not-json")}},
		},
	}

	h := NewStaticHarness(runner)
	_, err := h.Run(context.Background(), "/tmp/workdir", nil)
	if err == nil {
		t.Fatal("expected stage error")
	}

	var stageErr *StageError
	if !errors.As(err, &stageErr) {
		t.Fatalf("expected *StageError, got %T", err)
	}
	if stageErr.Stage != "show" {
		t.Fatalf("expected show stage failure, got %q", stageErr.Stage)
	}
}

type fakeRunner struct {
	responses []runnerResponse
	calls     []Command
}

type runnerResponse struct {
	result CommandResult
	err    error
}

func (f *fakeRunner) Run(_ context.Context, cmd Command) (CommandResult, error) {
	f.calls = append(f.calls, cmd)

	idx := len(f.calls) - 1
	if idx >= len(f.responses) {
		return CommandResult{}, errors.New("unexpected command execution")
	}

	resp := f.responses[idx]
	return resp.result, resp.err
}
