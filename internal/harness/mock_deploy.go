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
	Apply         StageResult
	StateSnapshot []byte
}

type MockDeployError struct {
	Stage string
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
	if err := h.mock.Reset(ctx); err != nil {
		return nil, &MockDeployError{
			Stage: "reset",
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
			Apply: stage,
			Err:   err,
		}
	}

	stateSnapshot, err := h.mock.State(ctx)
	if err != nil {
		return nil, &MockDeployError{
			Stage: "state",
			Apply: stage,
			Err:   err,
		}
	}

	return &MockDeployResult{
		Apply:         stage,
		StateSnapshot: stateSnapshot,
	}, nil
}
