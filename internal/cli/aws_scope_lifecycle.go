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

// StageAWSAMIResolve is the stage that resolves the AL2023 AMI and its
// root once per aws Layer 3 command, before generation or the gate. Its
// pass detail names both.
const StageAWSAMIResolve = "aws_ami_resolve"

// awsClaimState is what a process knows of the aws scope's claim. The
// zero value is unknown, so a path that never sets it hedges.
type awsClaimState int

const (
	// awsClaimUnknown: the claim may be this process's.
	awsClaimUnknown awsClaimState = iota
	awsClaimHeld
	// awsClaimNotHeld: not yet taken, refused for a reason other than a
	// holder, or released after a clean sweep.
	awsClaimNotHeld
	// awsClaimHeldByOther: another holder, named in awsClaim.other, has it.
	awsClaimHeldByOther
	// awsClaimHolderUnknown is no stored state: it is what a held or
	// unknown claim with no holder to name reads as.
	awsClaimHolderUnknown
)

type awsClaim struct {
	holder string // this process's holder
	state  awsClaimState
	other  string // the holder that has it, for awsClaimHeldByOther
}

// known is the state the reap advice can act on: a held or unknown claim
// with no holder to name, or another holder with no name, is
// awsClaimHolderUnknown.
func (c awsClaim) known() awsClaimState {
	switch {
	case (c.state == awsClaimHeld || c.state == awsClaimUnknown) && c.holder == "",
		c.state == awsClaimHeldByOther && c.other == "":
		return awsClaimHolderUnknown
	}
	return c.state
}

// awsReapAdvice names the reap that works for what this process knows of
// the claim: plain reap refuses a held claim, and --take-over refuses a
// claim its holder does not hold.
func awsReapAdvice(runtime *CommandRuntime) string {
	const does = "sweeps the scope, destroys what is left and releases the claim"
	claim := runtime.awsClaim
	plain := reapCommand(runtime.ConfigPath, runtime.scenarioPath)
	switch claim.known() {
	case awsClaimHeld:
		return fmt.Sprintf("`%s` %s", awsTakeOverCommand(runtime, claim.holder), does)
	case awsClaimUnknown:
		return fmt.Sprintf("If this run holds the claim, `%s` %s; if no one holds it, `%s` does",
			awsTakeOverCommand(runtime, claim.holder), does, plain)
	case awsClaimHeldByOther:
		return fmt.Sprintf("Once that run has ended, `%s` %s", awsTakeOverCommand(runtime, claim.other), does)
	case awsClaimHolderUnknown:
		return fmt.Sprintf("The claim's holder is unknown here: if no one holds it, `%s` %s; "+
			"if someone does, it refuses, naming the holder and the --take-over that takes the claim over", plain, does)
	}
	return fmt.Sprintf("`%s` %s", plain, does)
}

// awsClaimHead says what this process knows of the claim, for every
// message that names a reap.
func awsClaimHead(claim awsClaim) string {
	switch claim.known() {
	case awsClaimHeld:
		return "this run keeps the aws scope's claim for " + claim.holder
	case awsClaimHeldByOther:
		return fmt.Sprintf("this run does not hold the aws scope's claim: %s does", claim.other)
	case awsClaimNotHeld:
		return "this run does not hold the aws scope's claim"
	case awsClaimHolderUnknown:
		return "this run may hold the aws scope's claim"
	}
	return "this run may hold the aws scope's claim for " + claim.holder
}

// awsInterruptNotice is what an interrupted aws command prints once its
// teardown has settled the claim.
func awsInterruptNotice(runtime *CommandRuntime) string {
	claim := runtime.awsClaim
	tail := ", and what it applied may still exist"
	switch claim.known() {
	case awsClaimNotHeld:
		tail = ": it never took it, or its sweep proved the scope empty and released it"
	case awsClaimHeldByOther:
		tail = ""
	}
	return fmt.Sprintf("\nInterrupted: %s%s. %s.\n", awsClaimHead(claim), tail, awsReapAdvice(runtime))
}

// awsClaimHolderFor mints this process's claim holder when it may claim
// the aws scope: Layer 3 on and the scenario's cloud aws. Otherwise it is
// "", and nothing is minted that could fail a run that never claims. A
// minted holder has not claimed yet.
func awsClaimHolderFor(runtime *CommandRuntime, cloud layer3Cloud, runID string) (string, error) {
	if cloud != layer3AWS || !runtime.Config.Validation.Layers.SandboxDeploy.Enabled {
		return "", nil
	}
	holder, err := harness.NewAWSClaimHolder(runID)
	if err == nil {
		runtime.awsClaim = awsClaim{holder: holder, state: awsClaimNotHeld}
	}
	return holder, err
}

// ensureAWSScopeClaim is ensureRunProject's aws arm, run after the STS
// preflight: the stamp, then the default VPC, then the claim. It returns
// aws.account_id whenever the claim may be held, which is also the case
// for ErrAWSClaimOutcomeUnknown, and then fails with it.
func ensureAWSScopeClaim(ctx context.Context, runtime *CommandRuntime, holder string) (string, []StageSummary, []FailureSummary) {
	account := runtime.Config.AWS.AccountID
	// Not held until the take below says otherwise: every early return
	// is before any write. A claim this process already holds, from an
	// earlier iteration of run, stays held.
	if runtime.awsClaim.state != awsClaimHeld || runtime.awsClaim.holder != holder {
		runtime.awsClaim = awsClaim{holder: holder, state: awsClaimNotHeld}
	}
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
	runtime.awsClaim = awsClaimAfterTake(holder, err)
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

// awsClaimAfterTake is what TakeAWSClaim's err says of the claim.
func awsClaimAfterTake(holder string, err error) awsClaim {
	var byOther *harness.AWSScopeClaimedError
	switch {
	case err == nil:
		return awsClaim{holder: holder, state: awsClaimHeld}
	case errors.Is(err, harness.ErrAWSClaimOutcomeUnknown):
		return awsClaim{holder: holder, state: awsClaimUnknown}
	case errors.As(err, &byOther):
		return awsClaim{holder: holder, state: awsClaimHeldByOther, other: byOther.Holder}
	}
	// The put failed and no claim is stored.
	return awsClaim{holder: holder, state: awsClaimNotHeld}
}

// awsClaimAfterRelease is what ReleaseAWSClaim's err says of the claim. A
// release clears the holder with the state; a failed one may have landed
// (a lost DeleteParameter response), so it is unknown unless it found no
// claim or names the holder that has it.
func awsClaimAfterRelease(holder string, err error) awsClaim {
	var byOther *harness.AWSScopeClaimedError
	switch {
	case err == nil, errors.Is(err, harness.ErrAWSNoClaimHeld):
		return awsClaim{state: awsClaimNotHeld}
	case errors.As(err, &byOther):
		return awsClaim{holder: holder, state: awsClaimHeldByOther, other: byOther.Holder}
	}
	return awsClaim{holder: holder, state: awsClaimUnknown}
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
// claim and names the command that takes it over.
func awsScopeTeardown(ctx context.Context, runtime *CommandRuntime, outputDir string, opts testExecutionOptions) ([]StageSummary, []FailureSummary) {
	if opts.SkipDestroy || !runtime.Config.Validation.Layers.Destruction.Enabled {
		reason := "destruction is disabled"
		if opts.SkipDestroy {
			reason = "--no-destroy"
		}
		kept := " (%s): what this run applied may still be there. %s"
		if runtime.awsClaim.known() == awsClaimHeld {
			kept = " on purpose (%s): what this run applied is still there. %s"
		}
		return []StageSummary{{
			Layer: "sandbox_deploy", Stage: StageAWSScopeClaimKept, Status: StageStatusSkip,
			Detail: awsClaimHead(runtime.awsClaim) + fmt.Sprintf(kept, reason, awsReapAdvice(runtime)),
		}}, nil
	}

	env, err := awsCommandEnvForAccount(runtime, runtime.Config.AWS.AccountID)
	if err != nil {
		return awsScopeClaimKept(runtime, nil, nil, err.Error())
	}
	return awsDestroyAndRelease(ctx, runtime, outputDir, env)
}

// awsDestroyAndRelease runs with runtime.awsClaim's holder holding the claim: it destroys
// what the state records, then releases through awsReleaseAfterCleanSweep.
// Anything short of a release keeps the claim and names its take-over.
func awsDestroyAndRelease(ctx context.Context, runtime *CommandRuntime, outputDir string, env map[string]string) ([]StageSummary, []FailureSummary) {
	var stages []StageSummary
	var failures []FailureSummary
	if liveStateMayHoldResources(outputDir) {
		// A failed destroy still reaches the sweep: the sweep, not the
		// destroy, decides whether the claim is released.
		result, _, destroyErr := destroyAWSSandbox(ctx, runtime, outputDir, env)
		stages, failures = appendSandboxDestroyResult(stages, failures, result, destroyErr)
	}
	releaseStages, releaseFailures := awsReleaseAfterCleanSweep(ctx, runtime, env)
	stages = append(stages, releaseStages...)
	failures = append(failures, releaseFailures...)
	if len(releaseFailures) > 0 {
		return awsScopeClaimKept(runtime, stages, failures, "the scope was not proven empty and released")
	}
	return stages, failures
}

// awsScopeClaimKept names the reap awsReapAdvice picks for what this
// process knows of the claim.
func awsScopeClaimKept(runtime *CommandRuntime, stages []StageSummary, failures []FailureSummary, reason string) ([]StageSummary, []FailureSummary) {
	return append(stages, StageSummary{Layer: "sandbox_deploy", Stage: StageAWSScopeClaimKept, Status: StageStatusFail}),
		append(failures, FailureSummary{
			Layer: "sandbox_deploy", Stage: StageAWSScopeClaimKept, Check: "claim",
			Command: "release aws scope",
			Detail: fmt.Sprintf("%s: %s, so resources may still exist in the scope. %s",
				awsClaimHead(runtime.awsClaim), reason, awsReapAdvice(runtime)),
		})
}

// awsReleaseAfterCleanSweep is the one place the aws scope's claim is
// released: it sweeps the scope, and releases only when the sweep finds
// it empty. Its requests ignore cancellation, so an interrupted run still
// gets a verdict; the settle waits do not, so Ctrl-C never sits out the
// settle loop.
func awsReleaseAfterCleanSweep(ctx context.Context, runtime *CommandRuntime, env map[string]string) ([]StageSummary, []FailureSummary) {
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
	holder := runtime.awsClaim.holder
	err := harness.ReleaseAWSClaim(requestCtx, env, runtime.Deps.AWSSSM, "", holder)
	runtime.awsClaim = awsClaimAfterRelease(holder, err)
	if err != nil {
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
