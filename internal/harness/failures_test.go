package harness

import (
	"context"
	"errors"
	"testing"
)

func TestStaticFailureFromError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		responses     []runnerResponse
		expectedStage string
		expectedCheck string
	}{
		{
			name: "validate stage failure",
			responses: []runnerResponse{
				{result: CommandResult{}},
				{result: CommandResult{Stderr: []byte("validate stderr")}, err: errors.New("validate failed")},
			},
			expectedStage: "validate",
			expectedCheck: "validate",
		},
		{
			name: "plan stage failure",
			responses: []runnerResponse{
				{result: CommandResult{}},
				{result: CommandResult{}},
				{result: CommandResult{Stderr: []byte("plan stderr")}, err: errors.New("plan failed")},
			},
			expectedStage: "plan",
			expectedCheck: "plan",
		},
		{
			name: "show stage invalid json",
			responses: []runnerResponse{
				{result: CommandResult{}},
				{result: CommandResult{}},
				{result: CommandResult{}},
				{result: CommandResult{Stdout: []byte("not-json"), Stderr: []byte("show stderr")}},
			},
			expectedStage: "show",
			expectedCheck: "show",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := NewStaticHarness(&fakeRunner{responses: tc.responses})
			_, err := h.Run(context.Background(), "/tmp/workdir", nil)
			if err == nil {
				t.Fatal("expected stage error")
			}

			failure, ok := StaticFailureFromError(err)
			if !ok {
				t.Fatalf("expected static failure conversion, got %v", err)
			}
			if failure.Layer != "static" || failure.Status != "fail" {
				t.Fatalf("unexpected failure layer/status: %+v", failure)
			}
			if failure.Stage != tc.expectedStage || failure.Check != tc.expectedCheck {
				t.Fatalf("unexpected stage/check: %+v", failure)
			}
			if failure.Command == "" {
				t.Fatalf("expected command to be populated: %+v", failure)
			}
		})
	}
}
