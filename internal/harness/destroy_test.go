package harness

import (
	"context"
	"errors"
	"testing"
)

func TestDestroyHarnessRunSuccess(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		responses: []runnerResponse{
			{result: CommandResult{Stdout: []byte("destroy complete")}},
		},
	}
	mockClient := &fakeMockStateClient{
		statePayload: []byte(`{"instance":{"servers":[]}}`),
	}

	h := NewDestroyHarness(runner, mockClient)
	out, err := h.Run(context.Background(), "/tmp/workdir", nil)
	if err != nil {
		t.Fatalf("run destroy harness: %v", err)
	}

	if out.OrphanCount != 0 {
		t.Fatalf("expected no orphans, got %d", out.OrphanCount)
	}
	if out.Destroy.Stage != "destroy" {
		t.Fatalf("expected destroy stage result, got %+v", out.Destroy)
	}
}

func TestDestroyHarnessRunFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		runnerErr     error
		statePayload  []byte
		mockErrState  error
		expectedStage string
	}{
		{
			name:          "destroy command failure",
			runnerErr:     errors.New("destroy failed"),
			statePayload:  []byte(`{"instance":{"servers":[]}}`),
			expectedStage: "destroy",
		},
		{
			name:          "state fetch failure",
			statePayload:  []byte(`{"instance":{"servers":[]}}`),
			mockErrState:  errors.New("state failed"),
			expectedStage: "state",
		},
		{
			name:          "orphans detected",
			statePayload:  []byte(`{"instance":{"servers":[{"id":"srv-1"}]}}`),
			expectedStage: "orphan_check",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{
				responses: []runnerResponse{
					{
						result: CommandResult{Stdout: []byte("destroy"), Stderr: []byte("stderr")},
						err:    tc.runnerErr,
					},
				},
			}
			mockClient := &fakeMockStateClient{
				statePayload: tc.statePayload,
				errState:     tc.mockErrState,
			}

			h := NewDestroyHarness(runner, mockClient)
			_, err := h.Run(context.Background(), "/tmp/workdir", nil)
			if err == nil {
				t.Fatal("expected destroy error")
			}
			if !errors.Is(err, ErrDestroyFailed) {
				t.Fatalf("expected ErrDestroyFailed, got %v", err)
			}

			var destroyErr *DestroyError
			if !errors.As(err, &destroyErr) {
				t.Fatalf("expected *DestroyError, got %T", err)
			}
			if destroyErr.Stage != tc.expectedStage {
				t.Fatalf("expected stage %q, got %q", tc.expectedStage, destroyErr.Stage)
			}
		})
	}
}

func TestCountOrphansIgnoresNonResourceArrays(t *testing.T) {
	t.Parallel()

	state := []byte(`{
  "metadata": {
    "messages": ["ok", "note"]
  },
  "instance": {
    "servers": [],
    "events": [{"id":"e-1"}],
    "servers_meta": {
      "history": ["a", "b"]
    }
  }
}`)

	count, err := countOrphans(state)
	if err != nil {
		t.Fatalf("count orphans: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 orphans, got %d", count)
	}
}

func TestCountOrphansCountsKnownCollectionsOnly(t *testing.T) {
	t.Parallel()

	state := []byte(`{
  "instance": {
    "servers": [{"id":"srv-1"}, {"id":"srv-2"}],
    "events": [{"id":"e-1"}]
  },
  "lb": {
    "lbs": [{"id":"lb-1"}],
    "metrics": [{"id":"m-1"}]
  },
  "rdb": {
    "instances": [{"id":"db-1"}]
  }
}`)

	count, err := countOrphans(state)
	if err != nil {
		t.Fatalf("count orphans: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected 4 orphans from known collections, got %d", count)
	}
}
