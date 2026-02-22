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
			{result: CommandResult{Stdout: []byte("apply complete")}},
		},
	}
	mockClient := &fakeMockStateClient{
		statePayload: []byte(`{"state":"ok"}`),
	}

	h := NewMockDeployHarness(runner, mockClient)
	out, err := h.Run(context.Background(), "/tmp/workdir", nil)
	if err != nil {
		t.Fatalf("run mock deploy harness: %v", err)
	}

	if !mockClient.resetCalled {
		t.Fatal("expected reset to be called")
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one apply command call, got %d", len(runner.calls))
	}
	got := append([]string{runner.calls[0].Name}, runner.calls[0].Args...)
	expected := []string{"tofu", "apply", "-auto-approve"}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("unexpected apply command: got %v want %v", got, expected)
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
		runnerErr     error
		expectedStage string
	}{
		{
			name:          "reset failure",
			mockErrReset:  errors.New("reset failed"),
			expectedStage: "reset",
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
			_, err := h.Run(context.Background(), "/tmp/workdir", nil)
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

type fakeMockStateClient struct {
	resetCalled  bool
	stateCalled  bool
	errReset     error
	errState     error
	statePayload []byte
}

func (f *fakeMockStateClient) Reset(_ context.Context) error {
	f.resetCalled = true
	return f.errReset
}

func (f *fakeMockStateClient) State(_ context.Context) ([]byte, error) {
	f.stateCalled = true
	if f.errState != nil {
		return nil, f.errState
	}
	return f.statePayload, nil
}
