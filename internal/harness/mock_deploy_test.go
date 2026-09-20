package harness

import (
	"context"
	"errors"
	"testing"
)

func TestMockDeployHarnessRunSuccess(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		responses: []runnerResponse{
			{result: CommandResult{Stdout: []byte("init complete")}},
			{result: CommandResult{Stdout: []byte("apply complete")}},
			{result: CommandResult{Stdout: []byte("No changes.")}},
		},
	}
	mockClient := &fakeMockStateClient{
		statePayload: []byte(`{"state":"ok"}`),
	}

	h := NewMockDeployHarness(runner, mockClient)
	out, err := h.Run(context.Background(), "/tmp/workdir", nil, MockDeployModeClean)
	if err != nil {
		t.Fatalf("run mock deploy harness: %v", err)
	}

	if !mockClient.resetCalled {
		t.Fatal("expected reset to be called")
	}
	if len(runner.calls) != 3 {
		t.Fatalf("expected init+apply+converge command calls, got %d", len(runner.calls))
	}
	gotInit := append([]string{runner.calls[0].Name}, runner.calls[0].Args...)
	expectedInit := []string{"tofu", "init"}
	for i := range expectedInit {
		if gotInit[i] != expectedInit[i] {
			t.Fatalf("unexpected init command: got %v want %v", gotInit, expectedInit)
		}
	}
	gotApply := append([]string{runner.calls[1].Name}, runner.calls[1].Args...)
	expectedApply := []string{"tofu", "apply", "-auto-approve"}
	for i := range expectedApply {
		if gotApply[i] != expectedApply[i] {
			t.Fatalf("unexpected apply command: got %v want %v", gotApply, expectedApply)
		}
	}
	gotConverge := append([]string{runner.calls[2].Name}, runner.calls[2].Args...)
	expectedConverge := []string{"tofu", "plan", "-detailed-exitcode", "-input=false", "-no-color"}
	for i := range expectedConverge {
		if gotConverge[i] != expectedConverge[i] {
			t.Fatalf("unexpected converge command: got %v want %v", gotConverge, expectedConverge)
		}
	}
	if out.Drifted {
		t.Fatal("an empty plan is not drift")
	}
	if string(out.StateSnapshot) != `{"state":"ok"}` {
		t.Fatalf("unexpected state snapshot: %s", string(out.StateSnapshot))
	}
}

// Exit 2 is the FINDING -- the plan ran and reported changes -- so it
// must come back as a result the caller can act on, not an error.
// Reporting it as an error would make a drifting stack indistinguishable
// from a plan that could not run.
func TestMockDeployHarnessReportsDriftWithoutFailing(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		responses: []runnerResponse{
			{result: CommandResult{Stdout: []byte("init complete")}},
			{result: CommandResult{Stdout: []byte("apply complete")}},
			{
				result: CommandResult{
					Stdout:   []byte("# scaleway_lb.main will be updated in-place"),
					ExitCode: 2,
				},
				err: errors.New("exit status 2"),
			},
		},
	}
	mockClient := &fakeMockStateClient{statePayload: []byte(`{"state":"ok"}`)}

	h := NewMockDeployHarness(runner, mockClient)
	out, err := h.Run(context.Background(), "/tmp/workdir", nil, MockDeployModeClean)
	if err != nil {
		t.Fatalf("drift is a result, not an error: %v", err)
	}
	if !out.Drifted {
		t.Fatal("expected Drifted")
	}
	if out.Converge.Stdout == "" {
		t.Fatal("the plan output is the only thing that says WHICH attribute drifts")
	}
}

// Exit 1 means the plan itself broke. Calling that drift would claim an
// observation the run never made.
func TestMockDeployHarnessDistinguishesABrokenPlanFromDrift(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		responses: []runnerResponse{
			{result: CommandResult{Stdout: []byte("init complete")}},
			{result: CommandResult{Stdout: []byte("apply complete")}},
			{
				result: CommandResult{Stderr: []byte("Error: connection refused"), ExitCode: 1},
				err:    errors.New("exit status 1"),
			},
		},
	}
	mockClient := &fakeMockStateClient{statePayload: []byte(`{"state":"ok"}`)}

	h := NewMockDeployHarness(runner, mockClient)
	_, err := h.Run(context.Background(), "/tmp/workdir", nil, MockDeployModeClean)

	var deployErr *MockDeployError
	if !errors.As(err, &deployErr) {
		t.Fatalf("expected *MockDeployError, got %T", err)
	}
	if deployErr.Stage != "converge" {
		t.Fatalf("expected converge stage, got %q", deployErr.Stage)
	}
}

func TestMockDeployHarnessRunFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		mockErrReset  error
		mockErrState  error
		initErr       error
		runnerErr     error
		expectedStage string
	}{
		{
			name:          "reset failure",
			mockErrReset:  errors.New("reset failed"),
			expectedStage: "reset",
		},
		{
			name:          "init failure",
			initErr:       errors.New("init failed"),
			expectedStage: "init",
		},
		{
			name:          "apply failure",
			runnerErr:     errors.New("apply failed"),
			expectedStage: "apply",
		},
		{
			name:          "state failure",
			mockErrState:  errors.New("state failed"),
			expectedStage: "state",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{
				responses: []runnerResponse{
					{
						result: CommandResult{Stdout: []byte("init"), Stderr: []byte("init stderr")},
						err:    tc.initErr,
					},
					{
						result: CommandResult{Stdout: []byte("apply"), Stderr: []byte("stderr")},
						err:    tc.runnerErr,
					},
					{result: CommandResult{Stdout: []byte("No changes.")}},
				},
			}
			mockClient := &fakeMockStateClient{
				errReset: tc.mockErrReset,
				errState: tc.mockErrState,
			}

			h := NewMockDeployHarness(runner, mockClient)
			_, err := h.Run(context.Background(), "/tmp/workdir", nil, MockDeployModeClean)
			if err == nil {
				t.Fatal("expected deploy error")
			}
			if !errors.Is(err, ErrMockDeployFailed) {
				t.Fatalf("expected ErrMockDeployFailed, got %v", err)
			}

			var deployErr *MockDeployError
			if !errors.As(err, &deployErr) {
				t.Fatalf("expected *MockDeployError, got %T", err)
			}
			if deployErr.Stage != tc.expectedStage {
				t.Fatalf("expected stage %q, got %q", tc.expectedStage, deployErr.Stage)
			}
		})
	}
}

func TestMockDeployHarnessRunIncrementalUsesRestore(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		responses: []runnerResponse{
			{result: CommandResult{Stdout: []byte("init complete")}},
			{result: CommandResult{Stdout: []byte("apply complete")}},
			{result: CommandResult{Stdout: []byte("No changes.")}},
		},
	}
	mockClient := &fakeMockStateClient{
		statePayload: []byte(`{"state":"ok"}`),
	}

	h := NewMockDeployHarness(runner, mockClient)
	_, err := h.Run(context.Background(), "/tmp/workdir", nil, MockDeployModeIncremental)
	if err != nil {
		t.Fatalf("run mock deploy harness: %v", err)
	}
	if !mockClient.restoreCalled {
		t.Fatal("expected restore to be called")
	}
	if mockClient.resetCalled {
		t.Fatal("did not expect reset to be called in incremental mode")
	}
}

type fakeMockStateClient struct {
	resetCalled   bool
	restoreCalled bool
	stateCalled   bool
	errReset      error
	errRestore    error
	errState      error
	statePayload  []byte
}

func (f *fakeMockStateClient) Reset(_ context.Context) error {
	f.resetCalled = true
	return f.errReset
}

func (f *fakeMockStateClient) Snapshot(_ context.Context) error {
	return nil
}

func (f *fakeMockStateClient) Restore(_ context.Context) error {
	f.restoreCalled = true
	return f.errRestore
}

func (f *fakeMockStateClient) State(_ context.Context) ([]byte, error) {
	f.stateCalled = true
	if f.errState != nil {
		return nil, f.errState
	}
	return f.statePayload, nil
}
