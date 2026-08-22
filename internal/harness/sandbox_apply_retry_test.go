package harness

import (
	"context"
	"errors"
	"testing"
)

// scriptedRunner replays one response per call and can cancel the run's
// context when a chosen command runs, so the interrupt path is testable.
type scriptedRunner struct {
	responses  []runnerResponse
	calls      []Command
	cancelOn   int // 1-based call index that cancels the context; 0 disables
	cancelFunc context.CancelFunc
}

func (r *scriptedRunner) Run(_ context.Context, cmd Command) (CommandResult, error) {
	r.calls = append(r.calls, cmd)
	idx := len(r.calls) - 1
	if r.cancelOn != 0 && len(r.calls) == r.cancelOn && r.cancelFunc != nil {
		r.cancelFunc()
	}
	if idx >= len(r.responses) {
		return CommandResult{}, errors.New("unexpected extra command execution")
	}
	return r.responses[idx].result, r.responses[idx].err
}

func (r *scriptedRunner) applyCalls() int {
	n := 0
	for _, c := range r.calls {
		if len(c.Args) > 0 && c.Args[0] == "apply" {
			n++
		}
	}
	return n
}

func okResp() runnerResponse  { return runnerResponse{} }
func errResp() runnerResponse { return runnerResponse{err: errors.New("exit status 1")} }

// Real Scaleway returned a create error *after* the block volume existed,
// leaving it tainted (S143 run 2). A second apply replaces the tainted
// resource and succeeds; without a retry a single blip fails the run.
func TestSandboxApplyRetriesOnceAfterTransientFailure(t *testing.T) {
	runner := &scriptedRunner{responses: []runnerResponse{
		okResp(),  // init
		okResp(),  // plan
		errResp(), // apply #1 -- transient
		okResp(),  // apply #2 -- succeeds
	}}

	result, err := NewSandboxDeployHarness(runner).Run(context.Background(), t.TempDir(), nil)

	if err != nil {
		t.Fatalf("expected the retry to recover the apply, got %v", err)
	}
	if runner.applyCalls() != 2 {
		t.Errorf("got %d apply calls, want 2", runner.applyCalls())
	}
	if result.Attempts != 2 {
		t.Errorf("got Attempts=%d, want 2 so the retry is visible", result.Attempts)
	}
}

// One retry, not an unbounded loop: a genuinely broken plan must fail fast
// rather than bill for every attempt.
func TestSandboxApplyStopsAfterBoundedAttempts(t *testing.T) {
	runner := &scriptedRunner{responses: []runnerResponse{
		okResp(),  // init
		okResp(),  // plan
		errResp(), // apply #1
		errResp(), // apply #2
	}}

	_, err := NewSandboxDeployHarness(runner).Run(context.Background(), t.TempDir(), nil)

	if err == nil {
		t.Fatal("expected the deploy to fail after both attempts")
	}
	if runner.applyCalls() != sandboxApplyAttempts {
		t.Errorf("got %d apply calls, want %d", runner.applyCalls(), sandboxApplyAttempts)
	}
	deployErr := &SandboxDeployError{}
	if !errors.As(err, &deployErr) {
		t.Fatalf("got %T, want *SandboxDeployError", err)
	}
	if deployErr.Attempts != sandboxApplyAttempts {
		t.Errorf("got Attempts=%d, want %d", deployErr.Attempts, sandboxApplyAttempts)
	}
}

// The interrupt guard exists to stop touching the API and get to destroy.
// Retrying a cancelled apply would create exactly what the operator just
// asked us to stop creating.
func TestSandboxApplyDoesNotRetryAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runner := &scriptedRunner{
		responses: []runnerResponse{
			okResp(),  // init
			okResp(),  // plan
			errResp(), // apply #1 -- fails because the run was cancelled
			okResp(),  // must never be reached
		},
		cancelOn:   3, // cancel as the apply runs
		cancelFunc: cancel,
	}

	_, err := NewSandboxDeployHarness(runner).Run(ctx, t.TempDir(), nil)

	if err == nil {
		t.Fatal("expected a cancelled apply to fail")
	}
	if runner.applyCalls() != 1 {
		t.Errorf("got %d apply calls after cancellation, want 1", runner.applyCalls())
	}
}

func TestSandboxApplyRecordsSingleAttemptOnFirstTrySuccess(t *testing.T) {
	runner := &scriptedRunner{responses: []runnerResponse{okResp(), okResp(), okResp()}}

	result, err := NewSandboxDeployHarness(runner).Run(context.Background(), t.TempDir(), nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Attempts != 1 {
		t.Errorf("got Attempts=%d, want 1", result.Attempts)
	}
}
