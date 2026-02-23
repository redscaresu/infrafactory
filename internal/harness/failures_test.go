package harness

import (
	"context"
	"errors"
	"strings"
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

func TestStageFailureDetailSanitizesANSIAndTruncates(t *testing.T) {
	t.Parallel()

	stderr := "\x1b[31mError:\x1b[0m invalid value"
	detail := stageFailureDetail(errors.New("validate failed"), stderr)
	if strings.Contains(detail, "\x1b[31m") {
		t.Fatalf("expected ansi escapes removed, got %q", detail)
	}
	if !strings.Contains(detail, "Error: invalid value") {
		t.Fatalf("expected normalized stderr text, got %q", detail)
	}

	long := strings.Repeat("a", failureStderrMaxChars+10)
	detail = stageFailureDetail(errors.New("validate failed"), long)
	if !strings.HasSuffix(detail, "...") {
		t.Fatalf("expected truncated stderr suffix, got %q", detail)
	}
}
