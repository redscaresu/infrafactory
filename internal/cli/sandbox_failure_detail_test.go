package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// The S143 run 2 canary hit a transient block-volume create error against
// real Scaleway. The volume was created server-side, the provider errored
// on the post-create read, and infrafactory reported the whole thing as
// `exit status 1` with no provider message -- the one diagnostic that
// matters when reproducing the failure costs real money.
func TestSandboxDeployFailureDetailIncludesApplyStderr(t *testing.T) {
	err := &harness.SandboxDeployError{
		Stage: "apply",
		Init:  harness.StageResult{Stage: "init"},
		Plan:  harness.StageResult{Stage: "plan"},
		Apply: harness.StageResult{
			Stage:  "apply",
			Stderr: "Error: creating block volume: scaleway-sdk-go: internal server error",
		},
		Err: errors.New("exit status 1"),
	}

	detail := sandboxDeployFailureDetail(err)

	if !strings.Contains(detail, "exit status 1") {
		t.Errorf("detail lost the exec error: %q", detail)
	}
	if !strings.Contains(detail, "creating block volume") {
		t.Errorf("detail lost the provider message: %q", detail)
	}
	if !strings.Contains(detail, "| stderr: ") {
		t.Errorf("detail missing the stderr separator used by the other layers: %q", detail)
	}
}

// Only the failing stage's output is worth surfacing -- Layer 3 runs
// three commands against the real API and the other two succeeded.
func TestSandboxDeployFailureDetailSelectsFailingStageStderr(t *testing.T) {
	err := &harness.SandboxDeployError{
		Stage: "plan",
		Init:  harness.StageResult{Stage: "init", Stderr: "init noise"},
		Plan:  harness.StageResult{Stage: "plan", Stderr: "plan exploded"},
		Apply: harness.StageResult{Stage: "apply", Stderr: "apply noise"},
		Err:   errors.New("exit status 1"),
	}

	detail := sandboxDeployFailureDetail(err)

	if !strings.Contains(detail, "plan exploded") {
		t.Errorf("detail lost the failing stage stderr: %q", detail)
	}
	if strings.Contains(detail, "init noise") || strings.Contains(detail, "apply noise") {
		t.Errorf("detail leaked a non-failing stage's stderr: %q", detail)
	}
}

func TestSandboxDeployFailureDetailWithoutStderrStaysBare(t *testing.T) {
	err := &harness.SandboxDeployError{
		Stage: "apply",
		Apply: harness.StageResult{Stage: "apply"},
		Err:   errors.New("exit status 1"),
	}

	if got := sandboxDeployFailureDetail(err); got != "exit status 1" {
		t.Errorf("got %q, want %q", got, "exit status 1")
	}
}

// A failed real-money destroy is the orphaned-billing case. It used to
// report `exit status 1` too.
func TestSandboxDestroyFailureSurfacesStderr(t *testing.T) {
	destroyErr := &harness.SandboxDestroyError{
		Stage:   "destroy",
		Destroy: harness.StageResult{Stage: "destroy", Stderr: "Error: project not empty"},
		Err:     errors.New("exit status 1"),
	}

	_, failures := appendSandboxDestroyResult(nil, nil, nil, destroyErr)

	if len(failures) != 1 {
		t.Fatalf("got %d failures, want 1", len(failures))
	}
	if !strings.Contains(failures[0].Detail, "project not empty") {
		t.Errorf("destroy failure detail lost the provider message: %q", failures[0].Detail)
	}
}

// The stage summary an operator actually reads must carry the message,
// not just the helper in isolation.
func TestAppendSandboxDeployResultSurfacesStderrInFailure(t *testing.T) {
	deployErr := &harness.SandboxDeployError{
		Stage: "apply",
		Init:  harness.StageResult{Stage: "init"},
		Plan:  harness.StageResult{Stage: "plan"},
		Apply: harness.StageResult{Stage: "apply", Stderr: "Error: quota exceeded"},
		Err:   errors.New("exit status 1"),
	}

	stages, failures := appendSandboxDeployResult(nil, nil, nil, deployErr)

	if len(failures) != 1 {
		t.Fatalf("got %d failures, want 1", len(failures))
	}
	if failures[0].Layer != "sandbox_deploy" || failures[0].Stage != "apply" {
		t.Errorf("got layer=%q stage=%q, want sandbox_deploy/apply", failures[0].Layer, failures[0].Stage)
	}
	if !strings.Contains(failures[0].Detail, "quota exceeded") {
		t.Errorf("failure detail lost the provider message: %q", failures[0].Detail)
	}

	var applyStatus StageStatus
	var found bool
	for _, stage := range stages {
		if stage.Stage == "apply" {
			applyStatus, found = stage.Status, true
		}
	}
	if !found {
		t.Fatalf("no apply stage in summary: %+v", stages)
	}
	if applyStatus != StageStatusFail {
		t.Errorf("got apply status %q, want %q", applyStatus, StageStatusFail)
	}
}

// ANSI codes must not eat the truncation budget, and the budget must
// still bound the detail. Same M86 reasoning as the Layer 2 path.
func TestStderrFailureDetailStripsAnsiAndTruncates(t *testing.T) {
	noisy := "\x1b[31m" + strings.Repeat("x", failureStderrDetailMaxChars+500) + "\x1b[0m"

	detail := stderrFailureDetail(errors.New("exit status 1"), noisy)

	if strings.Contains(detail, "\x1b[") {
		t.Errorf("detail kept ANSI escapes: %q", detail[:40])
	}
	if !strings.HasSuffix(detail, "...") {
		t.Errorf("expected truncation marker, got tail %q", detail[len(detail)-10:])
	}
	if len(detail) >= failureStderrDetailMaxChars+100 {
		t.Errorf("detail not bounded by the budget: len=%d", len(detail))
	}
}
