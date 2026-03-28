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
	if len(runner.calls) != 2 {
		t.Fatalf("expected init+apply command calls, got %d", len(runner.calls))
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
	if string(out.StateSnapshot) != `{"state":"ok"}` {
		t.Fatalf("unexpected state snapshot: %s", string(out.StateSnapshot))
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
