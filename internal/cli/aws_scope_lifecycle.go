package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// StageAWSScopeClaimKept is the stage a test execution adds when it ends
// still holding the aws Layer 3 scope's claim. Its detail names the reap
// command, which is the only way the claim is released from then on.
const StageAWSScopeClaimKept = "aws_scope_claim_kept"

// awsClaimHolderFor mints this process's claim holder when it may claim
// the aws scope: Layer 3 on and the scenario's cloud aws. Otherwise it is
// "", and nothing is minted that could fail a run that never claims.
func awsClaimHolderFor(runtime *CommandRuntime, cloud layer3Cloud, runID string) (string, error) {
	if cloud != layer3AWS || !runtime.Config.Validation.Layers.SandboxDeploy.Enabled {
		return "", nil
	}
	return harness.NewAWSClaimHolder(runID)
}

// ensureAWSScopeClaim is ensureRunProject's aws arm, run after the STS
// preflight: the stamp, then the default VPC, then the claim. It returns
// aws.account_id whenever the claim may be held, which is also the case
// for ErrAWSClaimOutcomeUnknown, and then fails with it.
func ensureAWSScopeClaim(ctx context.Context, runtime *CommandRuntime, holder string) (string, []StageSummary, []FailureSummary) {
	account := runtime.Config.AWS.AccountID
	env, err := awsLayer3Env(runtime.Config.AWS)
	if err != nil {
		return awsScopeClaimFailed("", "credentials", err)
	}
	if err := harness.AssertAWSScopeStamp(ctx, env, runtime.Deps.AWSSSM, "", account); err != nil {
		return awsScopeClaimFailed("", "stamp", err)
	}
	if err := harness.AssertNoAWSDefaultVPC(ctx, env, runtime.Deps.AWSEC2, ""); err != nil {
		return awsScopeClaimFailed("", "default_vpc", err)
	}
	err = harness.TakeAWSClaim(ctx, env, runtime.Deps.AWSSSM, "", holder)
	switch {
	case err == nil:
		return account, []StageSummary{{
			Layer: "sandbox_deploy", Stage: "aws_scope_claim", Status: StageStatusPass,
			Detail: fmt.Sprintf("claimed account %s for %s after checking its stamp and that it has no default VPC", account, holder),
		}}, nil
	case errors.Is(err, harness.ErrAWSClaimOutcomeUnknown):
		return awsScopeClaimFailed(account, "claim", err)
	}
	return awsScopeClaimFailed("", "claim", err)
}

// awsScopeClaimFailed returns held: the account when the claim may be
// held despite the failure, "" when it is not.
func awsScopeClaimFailed(held, check string, err error) (string, []StageSummary, []FailureSummary) {
	return held, []StageSummary{{Layer: "sandbox_deploy", Stage: "aws_scope_claim", Status: StageStatusFail}},
		[]FailureSummary{{
			Layer: "sandbox_deploy", Stage: "aws_scope_claim", Check: check,
			Command: "claim aws scope", Detail: err.Error(),
		}}
}

// awsScopeTeardown is every exit of a test execution that took the claim
// (ADR-0025's lesson: taken in one place, released in one place). It
// destroys what the state records when destruction is wanted, then
// releases through awsReleaseAfterCleanSweep. Any other ending keeps the
// claim and names the reap command.
func awsScopeTeardown(ctx context.Context, runtime *CommandRuntime, outputDir string, opts testExecutionOptions) ([]StageSummary, []FailureSummary) {
	reap := reapCommand(runtime.ConfigPath, opts.scenarioPath)
	if opts.SkipDestroy || !runtime.Config.Validation.Layers.Destruction.Enabled {
		reason := "destruction is disabled"
		if opts.SkipDestroy {
			reason = "--no-destroy"
		}
		return []StageSummary{{
			Layer: "sandbox_deploy", Stage: StageAWSScopeClaimKept, Status: StageStatusSkip,
			Detail: fmt.Sprintf("kept the aws scope's claim for %s on purpose (%s): what this run applied is still there. "+
				"`%s` destroys it and releases the claim", opts.AWSClaimHolder, reason, reap),
		}}, nil
	}

	env, err := awsCommandEnvForAccount(runtime, runtime.Config.AWS.AccountID)
	if err != nil {
		return awsScopeClaimKept(nil, nil, opts.AWSClaimHolder, reap, err.Error())
	}
	var stages []StageSummary
	var failures []FailureSummary
	if liveStateMayHoldResources(outputDir) {
		// A failed destroy still reaches the sweep: the sweep, not the
		// destroy, decides whether the claim is released.
		result, _, destroyErr := destroyAWSSandbox(ctx, runtime, outputDir, env)
		stages, failures = appendSandboxDestroyResult(stages, failures, result, destroyErr)
	}
	releaseStages, releaseFailures := awsReleaseAfterCleanSweep(ctx, runtime, env, opts.AWSClaimHolder)
	stages = append(stages, releaseStages...)
	failures = append(failures, releaseFailures...)
	if len(releaseFailures) > 0 {
		return awsScopeClaimKept(stages, failures, opts.AWSClaimHolder, reap, "the scope was not proven empty and released")
	}
	return stages, failures
}

func awsScopeClaimKept(stages []StageSummary, failures []FailureSummary, holder, reap, reason string) ([]StageSummary, []FailureSummary) {
	return append(stages, StageSummary{Layer: "sandbox_deploy", Stage: StageAWSScopeClaimKept, Status: StageStatusFail}),
		append(failures, FailureSummary{
			Layer: "sandbox_deploy", Stage: StageAWSScopeClaimKept, Check: "claim",
			Command: "release aws scope",
			Detail: fmt.Sprintf("kept the aws scope's claim for %s: %s, so resources may still exist in the scope. "+
				"`%s` destroys what is left and releases the claim", holder, reason, reap),
		})
}

// awsReleaseAfterCleanSweep is the one place the aws scope's claim is
// released: it sweeps the scope, and releases only when the sweep finds
// it empty. Its requests ignore cancellation, so an interrupted run still
// gets a verdict; the settle waits do not, so Ctrl-C never sits out the
// settle loop.
func awsReleaseAfterCleanSweep(ctx context.Context, runtime *CommandRuntime, env map[string]string, holder string) ([]StageSummary, []FailureSummary) {
	requestCtx := context.WithoutCancel(ctx)
	doers := harness.AWSDoers{EC2: runtime.Deps.AWSEC2, SSM: runtime.Deps.AWSSSM}
	if _, err := harness.SweepAWSScope(requestCtx, env, doers, harness.AWSEndpoints{}, awsSettleWait(ctx, runtime)); err != nil {
		return []StageSummary{{Layer: "sandbox_deploy", Stage: "aws_scope_sweep", Status: StageStatusFail}},
			[]FailureSummary{{
				Layer: "sandbox_deploy", Stage: "aws_scope_sweep", Check: "no_orphans",
				Command: "aws scope sweep", Detail: err.Error(),
			}}
	}
	stages := []StageSummary{{
		Layer: "sandbox_deploy", Stage: "aws_scope_sweep", Status: StageStatusPass,
		Detail: "every swept collection in the scope is empty",
	}}
	if err := harness.ReleaseAWSClaim(requestCtx, env, runtime.Deps.AWSSSM, "", holder); err != nil {
		return append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "aws_scope_release", Status: StageStatusFail}),
			[]FailureSummary{{
				Layer: "sandbox_deploy", Stage: "aws_scope_release", Check: "claim",
				Command: "release aws scope", Detail: err.Error(),
			}}
	}
	return append(stages, StageSummary{
		Layer: "sandbox_deploy", Stage: "aws_scope_release", Status: StageStatusPass,
		Detail: "released the claim " + holder + " held",
	}), nil
}

// awsSettleWait is the sweep's settle wait, ended by interrupt rather
// than by the context the sweep's requests carry.
func awsSettleWait(interrupt context.Context, runtime *CommandRuntime) func(context.Context, time.Duration) error {
	return func(_ context.Context, d time.Duration) error {
		if runtime.Deps.AWSSweepSleep != nil {
			return runtime.Deps.AWSSweepSleep(interrupt, d)
		}
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-interrupt.Done():
			return interrupt.Err()
		case <-timer.C:
			return nil
		}
	}
}
