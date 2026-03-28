package harness

import (
	"context"
	"errors"
	"fmt"
)

var ErrSandboxDestroyFailed = errors.New("sandbox destroy failed")

type SandboxDestroyHarness struct {
	runner CommandRunner
}

func NewSandboxDestroyHarness(runner CommandRunner) *SandboxDestroyHarness {
	return &SandboxDestroyHarness{runner: runner}
}

type SandboxDestroyResult struct {
	Destroy StageResult
}

type SandboxDestroyError struct {
	Stage   string
	Destroy StageResult
	Err     error
}

func (e *SandboxDestroyError) Error() string {
	if e == nil {
		return ErrSandboxDestroyFailed.Error()
	}
	return fmt.Sprintf("%s: %s: %v", ErrSandboxDestroyFailed, e.Stage, e.Err)
}

func (e *SandboxDestroyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *SandboxDestroyError) Is(target error) bool {
	return target == ErrSandboxDestroyFailed
}

func (h *SandboxDestroyHarness) Run(ctx context.Context, workDir string, env map[string]string) (*SandboxDestroyResult, error) {
	cmd := Command{
		Name: "tofu",
		Args: []string{"destroy", "-auto-approve", "-state=" + LiveStateFilename},
		Dir:  workDir,
		Env:  env,
	}
	destroyResult, err := h.runner.Run(ctx, cmd)
	stage := StageResult{
		Stage:  "destroy",
		Cmd:    []string{"tofu", "destroy", "-auto-approve", "-state=" + LiveStateFilename},
		Stdout: string(destroyResult.Stdout),
		Stderr: string(destroyResult.Stderr),
	}
	if err != nil {
		return nil, &SandboxDestroyError{
			Stage:   "destroy",
			Destroy: stage,
			Err:     err,
		}
	}

	return &SandboxDestroyResult{Destroy: stage}, nil
}
