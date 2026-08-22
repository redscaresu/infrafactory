package harness

import (
	"context"
	"errors"
	"fmt"
)

const LiveStateFilename = "terraform-live.tfstate"

// SandboxStripEnv names the environment keys that must never reach a
// Layer 3 subprocess, whatever the parent shell exports.
//
// SCW_API_URL is the dangerous one. Layer 2 sets it to point the
// provider at mockway (see cloudEnv), mockway's own `make demo-env`
// writes it to /tmp/mockway.env, and any developer driving the mock by
// hand has it exported. Because an Env override map can set but never
// unset, it would otherwise survive into the sandbox apply and silently
// retarget "real Scaleway" at the mock -- which then reports
// sandbox_deploy/apply: pass. A green Layer 3 result would be evidence
// of nothing.
//
// The rest close the same hole by other routes: SCW_INSECURE relaxes
// TLS for a local mock, SCW_DEFAULT_* pins an org/project the run did
// not create, and SCW_PROFILE / SCW_CONFIG_PATH select a profile from
// ~/.config/scw/config.yaml that carries its own api_url and
// project id. Stripping the env alone is necessary but not sufficient
// -- the SDK still reads the default profile from disk, which is why
// the endpoint assertion also inspects the resolved config.
var SandboxStripEnv = []string{
	"SCW_API_URL",
	"SCW_INSECURE",
	"SCW_DEFAULT_PROJECT_ID",
	"SCW_DEFAULT_ORGANIZATION_ID",
	"SCW_PROFILE",
	"SCW_CONFIG_PATH",
}

var ErrSandboxDeployFailed = errors.New("sandbox deploy failed")

type SandboxDeployHarness struct {
	runner CommandRunner
}

func NewSandboxDeployHarness(runner CommandRunner) *SandboxDeployHarness {
	return &SandboxDeployHarness{runner: runner}
}

type SandboxDeployResult struct {
	Init  StageResult
	Plan  StageResult
	Apply StageResult
}

type SandboxDeployError struct {
	Stage string
	Init  StageResult
	Plan  StageResult
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
		Name:     "tofu",
		Args:     []string{"init"},
		Dir:      workDir,
		Env:      env,
		StripEnv: SandboxStripEnv,
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

	planCmd := Command{
		Name:     "tofu",
		Args:     []string{"plan", "-state=" + LiveStateFilename},
		Dir:      workDir,
		Env:      env,
		StripEnv: SandboxStripEnv,
	}
	planResult, err := h.runner.Run(ctx, planCmd)
	planStage := StageResult{
		Stage:  "plan",
		Cmd:    []string{"tofu", "plan", "-state=" + LiveStateFilename},
		Stdout: string(planResult.Stdout),
		Stderr: string(planResult.Stderr),
	}
	if err != nil {
		return nil, &SandboxDeployError{
			Stage: "plan",
			Init:  initStage,
			Plan:  planStage,
			Err:   err,
		}
	}

	applyCmd := Command{
		Name:     "tofu",
		Args:     []string{"apply", "-auto-approve", "-state=" + LiveStateFilename},
		Dir:      workDir,
		Env:      env,
		StripEnv: SandboxStripEnv,
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
			Plan:  planStage,
			Apply: applyStage,
			Err:   err,
		}
	}

	return &SandboxDeployResult{
		Init:  initStage,
		Plan:  planStage,
		Apply: applyStage,
	}, nil
}
