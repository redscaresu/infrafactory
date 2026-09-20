package harness

import (
	"context"
	"errors"
	"fmt"
)

var ErrMockDeployFailed = errors.New("mock deploy failed")

// MockStateClient manages mockway lifecycle state for harnesses.
// Snapshot is captured by the CLI at run start for incremental baseline artifacts;
// the deploy/destroy harnesses themselves only reset, restore, and read state.
type MockStateClient interface {
	Reset(context.Context) error
	Snapshot(context.Context) error
	Restore(context.Context) error
	State(context.Context) ([]byte, error)
}

type MockDeployMode string

const (
	MockDeployModeClean       MockDeployMode = "clean"
	MockDeployModeIncremental MockDeployMode = "incremental"
)

type MockDeployHarness struct {
	runner CommandRunner
	mock   MockStateClient
}

func NewMockDeployHarness(runner CommandRunner, mock MockStateClient) *MockDeployHarness {
	return &MockDeployHarness{
		runner: runner,
		mock:   mock,
	}
}

type MockDeployResult struct {
	Init          StageResult
	Apply         StageResult
	StateSnapshot []byte

	// Converge is the second plan, run immediately after the apply.
	Converge StageResult

	// Drifted reports that the second plan was NOT empty.
	//
	// After a successful apply, the config, the state and what the API
	// reports should all agree -- so a non-empty plan means one of them
	// is lying, and against a mock the mock is the likely candidate. The
	// classic shape is a mock that accepts a field on write and returns
	// a different one on read: the apply succeeds, and every plan from
	// then on proposes the same change to a resource nobody touched.
	//
	// Reported, never acted on here. Whether a drifting stack stops the
	// run is policy, and policy belongs to the caller -- the harness's
	// job is to answer the question, which nothing in this pipeline
	// previously asked.
	Drifted bool
}

// driftDetectedExitCode is `tofu plan -detailed-exitcode` reporting that
// the plan is not empty. 0 is no changes, 1 is an error.
const driftDetectedExitCode = 2

type MockDeployError struct {
	Stage    string
	Converge StageResult
	Init     StageResult
	Apply    StageResult
	Err      error
}

func (e *MockDeployError) Error() string {
	if e == nil {
		return ErrMockDeployFailed.Error()
	}
	return fmt.Sprintf("%s: %s: %v", ErrMockDeployFailed, e.Stage, e.Err)
}

func (e *MockDeployError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *MockDeployError) Is(target error) bool {
	return target == ErrMockDeployFailed
}

func (h *MockDeployHarness) Run(ctx context.Context, workDir string, env map[string]string, mode MockDeployMode) (*MockDeployResult, error) {
	switch mode {
	case MockDeployModeIncremental:
		if err := h.mock.Restore(ctx); err != nil {
			return nil, &MockDeployError{
				Stage: "restore",
				Err:   err,
			}
		}
	default:
		// Clean deploy flow starts from an empty mock environment.
		if err := h.mock.Reset(ctx); err != nil {
			return nil, &MockDeployError{
				Stage: "reset",
				Err:   err,
			}
		}
	}

	initCmd := Command{
		Name: "tofu",
		Args: []string{"init"},
		Dir:  workDir,
		Env:  env,
	}
	initResult, err := h.runner.Run(ctx, initCmd)
	initStage := StageResult{
		Stage:  "init",
		Cmd:    []string{"tofu", "init"},
		Stdout: string(initResult.Stdout),
		Stderr: string(initResult.Stderr),
	}
	if err != nil {
		return nil, &MockDeployError{
			Stage: "init",
			Init:  initStage,
			Err:   err,
		}
	}

	cmd := Command{
		Name: "tofu",
		Args: []string{"apply", "-auto-approve"},
		Dir:  workDir,
		Env:  env,
	}
	applyResult, err := h.runner.Run(ctx, cmd)
	stage := StageResult{
		Stage:  "apply",
		Cmd:    []string{"tofu", "apply", "-auto-approve"},
		Stdout: string(applyResult.Stdout),
		Stderr: string(applyResult.Stderr),
	}
	if err != nil {
		return nil, &MockDeployError{
			Stage: "apply",
			Init:  initStage,
			Apply: stage,
			Err:   err,
		}
	}

	convergeArgs := []string{"plan", "-detailed-exitcode", "-input=false", "-no-color"}
	convergeResult, convergeErr := h.runner.Run(ctx, Command{
		Name: "tofu",
		Args: convergeArgs,
		Dir:  workDir,
		Env:  env,
	})
	convergeStage := StageResult{
		Stage:  "converge",
		Cmd:    append([]string{"tofu"}, convergeArgs...),
		Stdout: string(convergeResult.Stdout),
		Stderr: string(convergeResult.Stderr),
	}
	// Exit 2 is the FINDING, not a failure to produce one: the plan ran
	// and reported changes. Exit 1 (or anything else non-zero) means the
	// plan itself broke, and reporting that as drift would claim an
	// observation the run never made.
	drifted := false
	switch {
	case convergeErr == nil:
	case convergeResult.ExitCode == driftDetectedExitCode:
		drifted = true
	default:
		return nil, &MockDeployError{
			Stage:    "converge",
			Init:     initStage,
			Apply:    stage,
			Converge: convergeStage,
			Err:      convergeErr,
		}
	}

	stateSnapshot, err := h.mock.State(ctx)
	if err != nil {
		return nil, &MockDeployError{
			Stage: "state",
			Init:  initStage,
			Apply: stage,
			Err:   err,
		}
	}

	return &MockDeployResult{
		Init:          initStage,
		Apply:         stage,
		StateSnapshot: stateSnapshot,
		Converge:      convergeStage,
		Drifted:       drifted,
	}, nil
}
