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

// awsClaim is what this process knows of the claim. Its own holder is
// runtime.awsHolder, the one source of it.
type awsClaim struct {
	state awsClaimState
	other string // the holder that has it, for awsClaimHeldByOther
}

// known is the state the reap advice can act on for holder, this
// process's: a held or unknown claim with no holder to name, or another
// holder with no name, is awsClaimHolderUnknown.
func (c awsClaim) known(holder string) awsClaimState {
	switch {
	case (c.state == awsClaimHeld || c.state == awsClaimUnknown) && holder == "",
		c.state == awsClaimHeldByOther && c.other == "":
		return awsClaimHolderUnknown
	}
	return c.state
}

// Who runtime.awsHolder belongs to, set by whoever mints it.
const (
	awsActorRun  = "this run"
	awsActorReap = "this reap"
)

// awsActor is who holds runtime.awsHolder; a run (or test) unless reap
// said otherwise.
func awsActor(runtime *CommandRuntime) string {
	if runtime.awsActor == "" {
		return awsActorRun
	}
	return runtime.awsActor
}

// awsReapAdvice names the reap that works for what this process knows of
// the claim: plain reap refuses a held claim, and --take-over refuses a
// claim its holder does not hold.
func awsReapAdvice(runtime *CommandRuntime, claim awsClaim) string {
	return awsReapAdviceFor(runtime, reapCommand(runtime.ConfigPath, runtime.scenarioPath), claim)
}

// awsReapAdviceFor is awsReapAdvice for plain, the reap command already
// built. It reads only what is fixed for the process.
func awsReapAdviceFor(runtime *CommandRuntime, plain string, claim awsClaim) string {
	holder := runtime.awsHolder
	const does = "sweeps the scope, destroys what is left and releases the claim"
	switch claim.known(holder) {
	case awsClaimHeld:
		return fmt.Sprintf("`%s` %s", awsTakeOverOf(plain, holder), does)
	case awsClaimUnknown:
		return fmt.Sprintf("If %s holds the claim, `%s` %s; if no one holds it, `%s` does",
			awsActor(runtime), awsTakeOverOf(plain, holder), does, plain)
	case awsClaimHeldByOther:
		return fmt.Sprintf("Once that run has ended, `%s` %s", awsTakeOverOf(plain, claim.other), does)
	case awsClaimHolderUnknown:
		return fmt.Sprintf("The claim's holder is unknown here: if no one holds it, `%s` %s; "+
			"if someone does, it refuses, naming the holder and the --take-over that takes the claim over", plain, does)
	}
	return fmt.Sprintf("`%s` %s", plain, does)
}

// awsTakeOverOf is plain taking the claim over from holder, or plain
// alone for an empty holder: there is nothing to take over.
func awsTakeOverOf(plain, holder string) string {
	if holder == "" {
		return plain
	}
	return plain + " --take-over " + shellQuote(holder)
}

// awsClaimHead says what this process knows of the claim, for every
// message that names a reap.
func awsClaimHead(runtime *CommandRuntime, claim awsClaim) string {
	holder, actor := runtime.awsHolder, awsActor(runtime)
	switch claim.known(holder) {
	case awsClaimHeld:
		return actor + " keeps the aws scope's claim for " + holder
	case awsClaimHeldByOther:
		return fmt.Sprintf("%s does not hold the aws scope's claim: %s does", actor, claim.other)
	case awsClaimNotHeld:
		return actor + " does not hold the aws scope's claim"
	case awsClaimHolderUnknown:
		return actor + " may hold the aws scope's claim"
	}
	if actor == awsActorReap {
		return actor + " may hold the aws scope's claim as " + holder
	}
	return actor + " may hold the aws scope's claim for " + holder
}

// firstSignalNotice is what the first signal prints at once, before any
// teardown: if the process is killed before it finishes, this is what
// the operator has. It is built before fn starts, from what is fixed for
// the process, so the watcher that prints it reads nothing fn writes.
func firstSignalNotice(runtime *CommandRuntime, cloud layer3Cloud, scenarioPath string) string {
	plain := reapCommand(runtime.ConfigPath, scenarioPath)
	switch cloud {
	case layer3AWS:
		claim := awsClaim{state: awsClaimUnknown}
		return fmt.Sprintf("\nInterrupted — finishing teardown before exit. Should the process die first, %s. %s.\n",
			awsClaimHead(runtime, claim), awsReapAdviceFor(runtime, plain, claim))
	case layer3Scaleway:
		return fmt.Sprintf("\nInterrupted — finishing cleanup before exit. Should the process die first, "+
			"if this run applied anything, `%s` cleans it up.\n", plain)
	}
	return ""
}

// awsRunEndNotice is what an aws run that did not reach its target
// prints when the claim may still be its own; "" otherwise. A process
// with no holder never claimed.
func awsRunEndNotice(runtime *CommandRuntime) string {
	claim := runtime.awsClaim
	if runtime.awsHolder == "" || (claim.state != awsClaimHeld && claim.state != awsClaimUnknown) {
		return ""
	}
	return fmt.Sprintf("\nRun ended: %s. %s.\n", awsClaimHead(runtime, claim), awsReapAdvice(runtime, claim))
}

// awsInterruptNotice is what an interrupted aws command prints once its
// teardown has settled the claim.
func awsInterruptNotice(runtime *CommandRuntime) string {
	claim := runtime.awsClaim
	tail := ", and what it applied may still exist"
	switch claim.known(runtime.awsHolder) {
	case awsClaimNotHeld:
		tail = ": it never took it, or its sweep proved the scope empty and released it"
	case awsClaimHeldByOther:
		tail = ""
	}
	return fmt.Sprintf("\nInterrupted: %s%s. %s.\n", awsClaimHead(runtime, claim), tail, awsReapAdvice(runtime, claim))
}

// mintAWSClaimHolder sets runtime.awsHolder, this process's claim holder,
// when it may claim the aws scope: Layer 3 on and the scenario's cloud
// aws. Otherwise it stays "", and nothing is minted that could fail a run
// that never claims. A minted holder has not claimed yet.
func mintAWSClaimHolder(runtime *CommandRuntime, cloud layer3Cloud, runID string) error {
	if cloud != layer3AWS || !runtime.Config.Validation.Layers.SandboxDeploy.Enabled {
		return nil
	}
	holder, err := harness.NewAWSClaimHolder(runID)
	if err != nil {
		return err
	}
	runtime.awsHolder, runtime.awsActor = holder, awsActorRun
	runtime.awsClaim = awsClaim{state: awsClaimNotHeld}
	return nil
}

// ensureAWSScopeClaim is ensureRunProject's aws arm, run after the STS
// preflight: the stamp, then the default VPC, then the claim. It returns
// aws.account_id whenever the claim may be held, which is also the case
// for ErrAWSClaimOutcomeUnknown, and then fails with it.
func ensureAWSScopeClaim(ctx context.Context, runtime *CommandRuntime) (string, []StageSummary, []FailureSummary) {
	account := runtime.Config.AWS.AccountID
	holder := runtime.awsHolder
	// Not held until the take below says otherwise: every early return
	// is before any write. A claim this process already holds, from an
	// earlier iteration of run, stays held.
	if runtime.awsClaim.state != awsClaimHeld {
		runtime.awsClaim = awsClaim{state: awsClaimNotHeld}
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
	runtime.awsClaim = awsClaimAfterTake(err)
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
func awsClaimAfterTake(err error) awsClaim {
	var byOther *harness.AWSScopeClaimedError
	switch {
	case err == nil:
		return awsClaim{state: awsClaimHeld}
	case errors.Is(err, harness.ErrAWSClaimOutcomeUnknown):
		return awsClaim{state: awsClaimUnknown}
	case errors.As(err, &byOther):
		return awsClaim{state: awsClaimHeldByOther, other: byOther.Holder}
	}
	// The put failed and no claim is stored.
	return awsClaim{state: awsClaimNotHeld}
}

// awsClaimAfterRelease is what ReleaseAWSClaim's err says of the claim: a
// failed release may have landed (a lost DeleteParameter response), so it
// is unknown unless it found no claim or names the holder that has it.
func awsClaimAfterRelease(err error) awsClaim {
	var byOther *harness.AWSScopeClaimedError
	switch {
	case err == nil, errors.Is(err, harness.ErrAWSNoClaimHeld):
		return awsClaim{state: awsClaimNotHeld}
	case errors.As(err, &byOther):
		return awsClaim{state: awsClaimHeldByOther, other: byOther.Holder}
	}
	return awsClaim{state: awsClaimUnknown}
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
		return awsTeardownSkipped(runtime, reason), nil
	}

	env, err := awsCommandEnvForAccount(runtime, runtime.Config.AWS.AccountID)
	if err != nil {
		return awsScopeClaimKept(runtime, nil, nil, err.Error())
	}
	return awsDestroyAndRelease(ctx, runtime, outputDir, env)
}

// awsTeardownSkipped is the stage a deliberately skipped teardown adds,
// one sentence for each thing the process knows of the claim. Only a
// held claim is kept, on purpose.
func awsTeardownSkipped(runtime *CommandRuntime, reason string) []StageSummary {
	holder, claim := runtime.awsHolder, runtime.awsClaim
	advice := awsReapAdvice(runtime, claim)
	stage := "aws_scope_claim"
	var detail string
	switch claim.known(holder) {
	case awsClaimHeld:
		stage = StageAWSScopeClaimKept
		detail = fmt.Sprintf("kept the aws scope's claim for %s on purpose (%s): what this run applied is still there. %s",
			holder, reason, advice)
	case awsClaimHeldByOther:
		detail = fmt.Sprintf("skipped the teardown (%s); the aws scope's claim is held by %s, not this run. %s",
			reason, claim.other, advice)
	case awsClaimNotHeld:
		detail = fmt.Sprintf("skipped the teardown (%s); this run does not hold the aws scope's claim. %s", reason, advice)
	default:
		stage = StageAWSScopeClaimKept
		detail = fmt.Sprintf("skipped the teardown (%s) though this run may hold the aws scope's claim%s, "+
			"so what it applied may still be there. %s", reason, awsHolderSuffix(holder), advice)
	}
	return []StageSummary{{Layer: "sandbox_deploy", Stage: stage, Status: StageStatusSkip, Detail: detail}}
}

// awsHolderSuffix is " for <holder>", or nothing with no holder to name.
func awsHolderSuffix(holder string) string {
	if holder == "" {
		return ""
	}
	return " for " + holder
}

// awsDestroyAndRelease runs with runtime.awsHolder holding the claim: it destroys
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
	claim := runtime.awsClaim
	switch claim.known(runtime.awsHolder) {
	case awsClaimNotHeld, awsClaimHeldByOther:
		// Not this run's to keep: the claim stage's own failure, and the
		// run ends for its own reason, not as a kept claim.
		return append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "aws_scope_claim", Status: StageStatusFail}),
			append(failures, FailureSummary{
				Layer: "sandbox_deploy", Stage: "aws_scope_claim", Check: "claim",
				Command: "release aws scope",
				Detail:  fmt.Sprintf("%s: %s. %s", awsClaimHead(runtime, claim), reason, awsReapAdvice(runtime, claim)),
			})
	}
	return append(stages, StageSummary{Layer: "sandbox_deploy", Stage: StageAWSScopeClaimKept, Status: StageStatusFail}),
		append(failures, FailureSummary{
			Layer: "sandbox_deploy", Stage: StageAWSScopeClaimKept, Check: "claim",
			Command: "release aws scope",
			Detail: fmt.Sprintf("%s: %s, so resources may still exist in the scope. %s",
				awsClaimHead(runtime, claim), reason, awsReapAdvice(runtime, claim)),
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
	holder := runtime.awsHolder
	err := harness.ReleaseAWSClaim(requestCtx, env, runtime.Deps.AWSSSM, "", holder)
	runtime.awsClaim = awsClaimAfterRelease(err)
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
