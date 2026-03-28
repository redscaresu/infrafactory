package harness

import (
	"context"
	"errors"
	"fmt"
)

const LiveStateFilename = "terraform-live.tfstate"

var ErrSandboxDeployFailed = errors.New("sandbox deploy failed")

type SandboxDeployHarness struct {
	runner CommandRunner
}

func NewSandboxDeployHarness(runner CommandRunner) *SandboxDeployHarness {
	return &SandboxDeployHarness{runner: runner}
}

type SandboxDeployResult struct {
	Init  StageResult
	Apply StageResult
}

type SandboxDeployError struct {
	Stage string
	Init  StageResult
	Apply StageResult
	Err   error
}

func (e *SandboxDeployError) Error() string {
	if e == nil {
		return ErrSandboxDeployFailed.Error()
	}
	return fmt.Sprintf("%s: %s: %v", ErrSandboxDeployFailed, e.Stage, e.Err)
}

func (e *SandboxDeployError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *SandboxDeployError) Is(target error) bool {
	return target == ErrSandboxDeployFailed
}

func (h *SandboxDeployHarness) Run(ctx context.Context, workDir string, env map[string]string) (*SandboxDeployResult, error) {
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
		return nil, &SandboxDeployError{
			Stage: "init",
			Init:  initStage,
			Err:   err,
		}
	}

	applyCmd := Command{
		Name: "tofu",
		Args: []string{"apply", "-auto-approve", "-state=" + LiveStateFilename},
		Dir:  workDir,
		Env:  env,
	}
	applyResult, err := h.runner.Run(ctx, applyCmd)
	applyStage := StageResult{
		Stage:  "apply",
		Cmd:    []string{"tofu", "apply", "-auto-approve", "-state=" + LiveStateFilename},
		Stdout: string(applyResult.Stdout),
		Stderr: string(applyResult.Stderr),
	}
	if err != nil {
		return nil, &SandboxDeployError{
			Stage: "apply",
			Init:  initStage,
			Apply: applyStage,
			Err:   err,
		}
	}

	return &SandboxDeployResult{
		Init:  initStage,
		Apply: applyStage,
	}, nil
}
