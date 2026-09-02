package harness

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
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
	// TF_VAR_* and TF_CLI_ARGS* decide what the configuration actually
	// applies, from outside the configuration. The cost bounds read a
	// variable's DEFAULT to decide whether a volume is 10 GB or 10 TB,
	// and an inherited TF_VAR_volume_size_in_gb silently overrides that
	// default -- so the check would vouch for a number that never
	// reaches the API. TF_CLI_ARGS is broader still: it injects
	// arbitrary flags, `-var` among them.
	"TF_VAR_*",
	"TF_CLI_ARGS*",
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
	// Attempts counts how many times apply ran. >1 means a retry
	// happened, which the stage summary surfaces -- a silent retry
	// would hide a real API flapping.
	Attempts int
}

// sandboxApplyAttempts bounds how many times a Layer 3 apply is tried.
//
// Real Scaleway can return an error *after* the resource exists, leaving
// it tainted with its computed fields unset; a second apply replaces the
// tainted resource and succeeds. That is exactly what the S143 run 2
// canary hit, and with no retry a single API blip fails an otherwise
// correct run -- and under `infrafactory run` burns a repair iteration
// teaching the LLM nothing, because the HCL was fine.
//
// One retry, not more. A genuinely broken plan should fail fast rather
// than bill for every attempt.
const sandboxApplyAttempts = 2

type SandboxDeployError struct {
	Stage string
	// Attempts is how many times the failing stage ran. Only apply
	// retries, so this is 1 for every other stage.
	Attempts int
	Init     StageResult
	Plan     StageResult
	Apply    StageResult
	Err      error
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

// stageProgress reports what the harness is doing, as it does it.
//
// # Why the harness has to be the one reporting
//
// A Layer 3 apply takes minutes, and until this existed the only thing
// written to a caller's progress stream during all of it was nothing.
// `CommandRunner.Run` returns a fully buffered `CommandResult`, so
// `tofu`'s own output does not exist until each process exits -- there
// is no stream to tee. The harness is the only place that knows a stage
// has BEGUN.
//
// Stage granularity rather than raw tofu output, deliberately: it is
// what a reader needs ("where is it up to?"), it is a predictable
// handful of lines rather than a wall of provider chatter, and it does
// not change what `infrafactory deploy` prints beyond a few lines.
//
// A nil writer is the ordinary case for callers that have nowhere to
// send it, and must cost nothing.
type stageProgress struct {
	out io.Writer
}

func (p stageProgress) start(stage string) time.Time {
	if p.out != nil {
		_, _ = fmt.Fprintf(p.out, "  %s: running\n", stage)
	}
	return time.Now()
}

func (p stageProgress) done(stage string, started time.Time) {
	if p.out != nil {
		_, _ = fmt.Fprintf(p.out, "  %s: done in %s\n", stage, time.Since(started).Round(time.Second))
	}
}

// retrying is reported because a silent retry is indistinguishable from
// a stage that is simply slow -- and a Layer 3 apply really does retry
// after a transient provider error, which a reader watching a frozen log
// would otherwise read as hung.
func (p stageProgress) retrying(stage string, attempt, of int, err error) {
	if p.out != nil {
		_, _ = fmt.Fprintf(p.out, "  %s: attempt %d of %d failed (%v); retrying\n", stage, attempt, of, err)
	}
}

func (h *SandboxDeployHarness) Run(ctx context.Context, workDir string, env map[string]string, progress io.Writer) (*SandboxDeployResult, error) {
	report := stageProgress{out: progress}

	initCmd := Command{
		Name:     "tofu",
		Args:     []string{"init"},
		Dir:      workDir,
		Env:      env,
		StripEnv: SandboxStripEnv,
	}
	initStarted := report.start("init")
	initResult, err := h.runner.Run(ctx, initCmd)
	report.done("init", initStarted)
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
	planStarted := report.start("plan")
	planResult, err := h.runner.Run(ctx, planCmd)
	report.done("plan", planStarted)
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
	var applyStage StageResult
	attempts := 0
	for attempts < sandboxApplyAttempts {
		attempts++
		applyStarted := report.start("apply")
		var applyResult CommandResult
		applyResult, err = h.runner.Run(ctx, applyCmd)
		report.done("apply", applyStarted)
		applyStage = StageResult{
			Stage:  "apply",
			Cmd:    []string{"tofu", "apply", "-auto-approve", "-state=" + LiveStateFilename},
			Stdout: string(applyResult.Stdout),
			Stderr: string(applyResult.Stderr),
		}
		if err == nil {
			break
		}
		// Never retry a cancelled run. On the interrupt path the whole
		// point is to stop touching the API and get to destroy, so a
		// retry here would create exactly what the operator asked us to
		// stop creating.
		if ctx.Err() != nil {
			break
		}
		if attempts < sandboxApplyAttempts {
			report.retrying("apply", attempts, sandboxApplyAttempts, err)
		}
	}
	if err != nil {
		return nil, &SandboxDeployError{
			Stage:    "apply",
			Init:     initStage,
			Plan:     planStage,
			Apply:    applyStage,
			Attempts: attempts,
			Err:      err,
		}
	}

	return &SandboxDeployResult{
		Init:     initStage,
		Plan:     planStage,
		Apply:    applyStage,
		Attempts: attempts,
	}, nil
}
