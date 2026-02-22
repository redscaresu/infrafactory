package harness

import (
	"context"
	"errors"
	"fmt"
)

var ErrMockDeployFailed = errors.New("mock deploy failed")

type MockStateClient interface {
	Reset(context.Context) error
	State(context.Context) ([]byte, error)
}

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
}

type MockDeployError struct {
	Stage string
	Init  StageResult
	Apply StageResult
	Err   error
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

func (h *MockDeployHarness) Run(ctx context.Context, workDir string, env map[string]string) (*MockDeployResult, error) {
	// Deploy flow is intentionally sequenced as reset -> init -> apply -> state snapshot
	// so checks always run against a fresh mock environment.
	if err := h.mock.Reset(ctx); err != nil {
		return nil, &MockDeployError{
			Stage: "reset",
			Err:   err,
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
	}, nil
}
