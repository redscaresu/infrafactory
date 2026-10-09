package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/spf13/cobra"
)

// runReapCommand destroys real Scaleway resources left behind by an
// interrupted Layer 3 run.
//
// Every other guarantee in ADR-0023 assumes the run reaches its destroy
// step. A run killed mid-apply -- Ctrl-C, a context timeout, a crash --
// breaks that assumption: terraform-live.tfstate records what was
// created and nothing ever tears it down. The signal handler installed
// by withSandboxInterruptGuard covers the cases where the process gets
// to run its own cleanup; reap covers the ones where it did not.
//
// It refuses to touch any project the run did not create
// (harness.AssertProjectDeletable), and verifies the result with the
// same real-API sweep a normal run uses. A reap that cannot prove the
// account is clean fails.
func runReapCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	sc, err := runtime.LoadScenario(args[0])
	if err != nil {
		return &CLIError{Op: "reap", Code: errorCodeUsage, Err: err}
	}

	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return &CLIError{Op: "reap", Code: errorCodeUsage, Err: fmt.Errorf("read --dry-run flag: %w", err)}
	}

	workDir := runtime.OutputDir()
	statePath := filepath.Join(workDir, harness.LiveStateFilename)

	// Before anything on disk is read: a marker here may be a stale
	// Scaleway one, and acting on it would reap an account this
	// scenario never applied to.
	cloud := layer3TeardownCloud(sc.Cloud)
	if cloud == layer3AWS {
		return runAWSReap(cmd, runtime, sc.Name, dryRun)
	}
	if cloud != layer3Scaleway {
		return &CLIError{Op: "reap", Code: errorCodeCommandFailed, Err: errors.New(layer3TeardownNotBuilt(cloud, statePath))}
	}

	markerPath := filepath.Join(workDir, harness.RunProjectMarkerFilename)
	_, stateErr := os.Stat(statePath)
	_, markerErr := os.Stat(markerPath)
	if errors.Is(stateErr, os.ErrNotExist) && errors.Is(markerErr, os.ErrNotExist) {
		_, _ = fmt.Fprintf(out, "No %s or %s in %s — nothing to reap.\n",
			harness.LiveStateFilename, harness.RunProjectMarkerFilename, workDir)
		return nil
	}

	// The marker, not the state: ADR-0025 took the project out of
	// Terraform, so the state no longer names it. And only the marker --
	// falling back to a scaleway_account_project in state for a
	// pre-cutover workdir is the dual model the cutover dropped, for a
	// case with no instance. Refusing outright is the right answer here
	// specifically: reap's contract is "destroy this run's project and
	// prove the account is clean", and without a marker it can do
	// neither half.
	marker, err := harness.ReadRunProjectMarker(workDir)
	if err != nil {
		return &CLIError{Op: "reap", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"%v, so there is no way to tell which project this run created. Refusing to destroy anything", err)}
	}
	projectID := marker.ProjectID

	// Scoped to the project the marker names: the apply ran with it as
	// the provider default, so the destroy that inverts it must too.
	sandboxEnv, err := sandboxCommandEnvForProject(runtime, cloud, projectID)
	if err != nil {
		return &CLIError{Op: "reap", Code: errorCodeCommandFailed, Err: err}
	}
	// The guard that stands between this command and real infrastructure.
	// reap only ever destroys the project recorded in the state file it
	// was handed -- never one named on the command line, never the
	// organization default.
	if err := assertRunProjectDeletable(ctx, runtime, cloud, workDir, projectID, sandboxEnv); err != nil {
		return &CLIError{Op: "reap", Code: errorCodeCommandFailed, Err: err}
	}

	_, _ = fmt.Fprintf(out, "Live state: %s\n", statePath)
	_, _ = fmt.Fprintf(out, "Run project: %s\n", projectID)

	if dryRun {
		_, _ = fmt.Fprintf(out, "\n--dry-run: nothing destroyed. Re-run without the flag to tear this down.\n")
		return nil
	}

	sweepTarget, sweepTargetErr := harness.CaptureSweepTarget(workDir)
	destroyResult, purged, destroyErr := destroySandbox(ctx, runtime, cloud, workDir, sandboxEnv, sweepTargetProjectID(sweepTarget))
	stages, failures := appendSandboxDestroyResult(nil, nil, destroyResult, destroyErr)
	if len(purged) > 0 {
		stages = append(stages, autoCreatedPurgeStage(purged))
	}
	if destroyErr == nil {
		// The project goes BEFORE the sweep, for the same reason it does
		// in `test` and `live teardown`: since ADR-0025 `tofu destroy`
		// cannot delete it -- it is not a Terraform resource -- and the
		// sweep's whole job is to verify it is gone. Deleting it
		// afterwards would make every clean reap report a leak.
		projectStages, projectFailures := releaseRunProject(ctx, runtime, cloud, workDir, projectID, sandboxEnv)
		stages = append(stages, projectStages...)
		failures = append(failures, projectFailures...)

		stages, failures = appendOrphanSweepResult(ctx, stages, failures, runtime, cloud, sweepTarget, sweepTargetErr, sandboxEnv)
	}

	status := CommandStatusSuccess
	if len(failures) > 0 {
		status = CommandStatusFailed
	}
	result := OutputResult{
		Command:  "reap",
		Scenario: sc.Name,
		Status:   status,
		Stages:   stages,
		Failures: failures,
	}
	if err := writeCommandOutput(cmd, result); err != nil {
		return err
	}
	if status == CommandStatusFailed {
		return &CLIError{Op: "reap", Code: errorCodeCommandFailed, Err: errors.New("reap did not leave the account provably clean")}
	}
	return nil
}

// runAWSReap is reap for the aws Layer 3 scope (ADR-0023 rule 4). It
// needs no state and no marker: the scope is the account, and the claim
// says who may clean it. STS proves the key, the stamp proves the
// account is the scope, and only then is the claim read or written. What
// state records is destroyed, the sweep's findings are deleted, and the
// claim is released only when the verdict sweep finds the scope empty.
func runAWSReap(cmd *cobra.Command, runtime *CommandRuntime, scenarioName string, dryRun bool) error {
	ctx := cmd.Context()
	takeOver, err := cmd.Flags().GetString("take-over")
	if err != nil {
		return &CLIError{Op: "reap", Code: errorCodeUsage, Err: fmt.Errorf("read --take-over flag: %w", err)}
	}
	fail := func(err error) error { return &CLIError{Op: "reap", Code: errorCodeCommandFailed, Err: err} }

	if err := assertAWSCredentials(runtime); err != nil {
		return fail(err)
	}
	account := runtime.Config.AWS.AccountID
	env, err := awsCommandEnvForAccount(runtime, account)
	if err != nil {
		return fail(err)
	}
	if err := harness.AssertAWSScopeStamp(ctx, env, runtime.Deps.AWSSSM, "", account); err != nil {
		return fail(err)
	}

	if dryRun {
		doers := harness.AWSDoers{EC2: runtime.Deps.AWSEC2, SSM: runtime.Deps.AWSSSM}
		if _, err := harness.SweepAWSScope(ctx, env, doers, harness.AWSEndpoints{}, awsSettleWait(ctx, runtime)); err != nil {
			return fail(fmt.Errorf("--dry-run, nothing deleted: %w", err))
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "--dry-run: every swept collection in account %s is empty; nothing to reap.\n", account)
		return nil
	}

	holder, err := harness.NewAWSClaimHolder(awsReapHolderPrefix + time.Now().UTC().Format("20060102T150405Z0700"))
	if err != nil {
		return fail(err)
	}
	if err := claimAWSScopeForReap(ctx, runtime, env, holder, takeOver); err != nil {
		return fail(err)
	}
	runtime.awsClaim = awsClaim{holder: holder, state: awsClaimHeld}
	stages, failures := reapClaimedAWSScope(ctx, runtime, env, holder)

	status := CommandStatusSuccess
	if len(failures) > 0 {
		status = CommandStatusFailed
	}
	result := OutputResult{Command: "reap", Scenario: scenarioName, Status: status, Stages: stages, Failures: failures}
	if err := writeCommandOutput(cmd, result); err != nil {
		return err
	}
	if status == CommandStatusFailed {
		return fail(errors.New("reap did not leave the aws scope provably empty"))
	}
	return nil
}

// claimAWSScopeForReap takes the claim for holder: over from takeOver
// when it is set, otherwise only when no one holds it. A held claim is
// refused naming its holder and the command that takes it over, with
// nothing written.
func claimAWSScopeForReap(ctx context.Context, runtime *CommandRuntime, env map[string]string, holder, takeOver string) error {
	if takeOver != "" {
		return harness.TakeOverAWSClaim(ctx, env, runtime.Deps.AWSSSM, "", takeOver, holder)
	}
	current, held, err := harness.ReadAWSClaimHolder(ctx, env, runtime.Deps.AWSSSM, "")
	switch {
	case err != nil:
		return err
	case held:
		return fmt.Errorf("refusing to reap: %w by %s. Once that run has ended, `%s` takes the claim over from it",
			harness.ErrAWSScopeClaimed, current, awsTakeOverCommand(runtime, current))
	}
	return harness.TakeAWSClaim(ctx, env, runtime.Deps.AWSSSM, "", holder)
}

// awsReapHolderPrefix starts every holder reap mints.
const awsReapHolderPrefix = "reap-"

// awsTakeOverCommand is plain reap for an empty holder: there is nothing
// to take over.
func awsTakeOverCommand(runtime *CommandRuntime, holder string) string {
	if holder == "" {
		return reapCommand(runtime.ConfigPath, runtime.scenarioPath)
	}
	return reapCommand(runtime.ConfigPath, runtime.scenarioPath) + " --take-over " + shellQuote(holder)
}

// reapClaimedAWSScope runs with holder holding the claim. Neither a
// failed destroy nor a failed delete stops it: the verdict sweep in
// awsReleaseAfterCleanSweep decides whether the claim is released, and
// a kept claim names the command that takes it over.
func reapClaimedAWSScope(ctx context.Context, runtime *CommandRuntime, env map[string]string, holder string) ([]StageSummary, []FailureSummary) {
	cfg := runtime.Config.AWS
	stages := []StageSummary{{Layer: "sandbox_deploy", Stage: "aws_scope_claim", Status: StageStatusPass,
		Detail: fmt.Sprintf("claimed account %s for %s", cfg.AccountID, holder)}}
	var failures []FailureSummary
	if liveStateMayHoldResources(runtime.OutputDir()) {
		result, _, destroyErr := destroyAWSSandbox(ctx, runtime, runtime.OutputDir(), env)
		stages, failures = appendSandboxDestroyResult(stages, failures, result, destroyErr)
	}

	doers := harness.AWSDoers{EC2: runtime.Deps.AWSEC2, SSM: runtime.Deps.AWSSSM, STS: runtime.Deps.AWSSTS}
	sleep := awsSettleWait(ctx, runtime)
	// Its error goes unreported: the verdict sweep reports its own.
	strays, _ := harness.SweepAWSScope(ctx, env, doers, harness.AWSEndpoints{}, sleep)
	if len(strays) > 0 {
		names := make([]string, len(strays))
		for i, stray := range strays {
			names[i] = stray.String()
		}
		err := harness.ReapAWSScope(ctx, env, doers, harness.AWSEndpoints{}, sleep, cfg.AccountID, cfg.PrincipalARN, holder, strays)
		if err != nil {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "aws_scope_reap", Status: StageStatusFail})
			failures = append(failures, FailureSummary{
				Layer: "sandbox_deploy", Stage: "aws_scope_reap", Check: "delete",
				Command: "aws scope reap", Detail: err.Error(),
			})
		} else {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "aws_scope_reap", Status: StageStatusPass,
				Detail: "deleted " + strings.Join(names, "; ")})
		}
	}

	releaseStages, releaseFailures := awsReleaseAfterCleanSweep(ctx, runtime, env, holder)
	stages, failures = append(stages, releaseStages...), append(failures, releaseFailures...)
	if len(releaseFailures) > 0 {
		return awsScopeClaimKept(runtime, stages, failures, "the scope was not proven empty and released")
	}
	return stages, failures
}

// withSandboxInterruptGuard runs fn with a SIGINT/SIGTERM handler that
// destroys real resources before the process exits.
//
// Without it, Ctrl-C between apply and destroy leaves billable resources
// with nothing tracking them but a state file on disk. The context
// passed to fn is cancelled on the first signal so the in-flight tofu
// call unwinds; destroy then runs on a FRESH context, because the whole
// point is to do work after cancellation.
//
// A second signal gives up immediately and tells the operator exactly
// how to finish the job by hand -- an operator hammering Ctrl-C needs to
// understand why the process is not exiting, and what state they are
// being left in.
func withSandboxInterruptGuard(
	cmd *cobra.Command,
	runtime *CommandRuntime,
	cloud layer3Cloud,
	notify func(ctx context.Context, sigs ...os.Signal) (context.Context, context.CancelFunc),
	fn func(ctx context.Context) error,
) error {
	if !runtime.Config.Validation.Layers.SandboxDeploy.Enabled {
		return fn(cmd.Context())
	}

	sigCtx, stop := notify(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := fn(sigCtx)
	if sigCtx.Err() == nil {
		return err
	}

	// Interrupted. Anything the apply created is live and unowned -- and
	// since ADR-0025 that includes the project itself, which exists
	// before the apply and outlives `tofu destroy`.
	out := cmd.ErrOrStderr()
	workDir := runtime.OutputDir()
	statePath := filepath.Join(workDir, harness.LiveStateFilename)

	// Before the state and marker reads, for reap's reason: a marker
	// here may be a stale Scaleway one. aws tears nothing down here: the
	// run's own teardown has already swept, and released the claim only
	// if the scope was empty; whatever it kept is reap's.
	if cloud == layer3AWS {
		_, _ = fmt.Fprint(out, awsInterruptNotice(runtime))
		return err
	}
	if cloud != layer3Scaleway {
		_, _ = fmt.Fprintf(out, "\nInterrupted: %s.\n", layer3TeardownNotBuilt(cloud, statePath))
		return err
	}

	_, stateErr := os.Stat(statePath)
	hasState := !errors.Is(stateErr, os.ErrNotExist)

	// The marker, because "no state" no longer means "nothing exists".
	marker, markerErr := harness.ReadRunProjectMarker(workDir)
	if !hasState && markerErr != nil {
		_, _ = fmt.Fprintf(out, "\nInterrupted before any real resources were created — nothing to clean up.\n")
		return err
	}

	if hasState {
		_, _ = fmt.Fprintf(out, "\nInterrupted with real resources live. Destroying before exit — press Ctrl-C again to abandon.\n")
	} else {
		_, _ = fmt.Fprintf(out,
			"\nInterrupted before anything was applied, but project %s exists. Deleting it before exit — press Ctrl-C again to abandon.\n",
			marker.ProjectID)
	}

	// stop() restores default signal handling, so a second Ctrl-C kills
	// the process outright rather than being swallowed here.
	stop()

	// An unreadable marker with state on disk is the one shape this
	// cannot proceed on. marker is the zero value there, so building the
	// env from it would silently scope the destroy to the SHARED
	// fallback -- which is not the inverse of the apply that created
	// these resources. A post-cutover run always writes the marker
	// (failing to is fatal at creation), so its absence here means the
	// workdir is damaged, and the honest answer is to hand the operator
	// the recovery command rather than destroy against a guess.
	if markerErr != nil {
		reportAbandonedResources(out, statePath, fmt.Errorf(
			"cannot tell which project this run owns (%v), and destroying against the shared "+
				"fallback project would not be the inverse of the apply", markerErr))
		return err
	}

	// Scoped to the run's project, so the destroy runs with the same
	// provider default the apply did.
	sandboxEnv, envErr := sandboxCommandEnvForProject(runtime, cloud, marker.ProjectID)
	if envErr != nil {
		reportAbandonedResources(out, statePath, envErr)
		return err
	}

	if hasState {
		// Through destroySandbox, not the raw harness: an interrupted run
		// is exactly when a project the API made undeletable matters
		// most, because nothing else is coming to clean it up. Capture
		// can fail here -- the state may be mid-write -- and an empty
		// project id just means no purge, never a skipped destroy.
		cleanupTarget, _ := harness.CaptureSweepTarget(workDir)
		destroyResult, purged, destroyErr := destroySandbox(
			context.Background(), runtime, cloud, workDir, sandboxEnv, sweepTargetProjectID(cleanupTarget))
		if destroyErr != nil {
			reportAbandonedResources(out, statePath, destroyErr)
			return err
		}
		if len(purged) > 0 {
			_, _ = fmt.Fprintf(out, "%s\n", autoCreatedPurgeStage(purged).Detail)
		}
		if destroyResult != nil && destroyResult.WithoutConfig != "" {
			_, _ = fmt.Fprintf(out, "%s\n", withoutConfigStage(destroyResult.WithoutConfig).Detail)
		}
		_, _ = fmt.Fprintf(out, "Cleanup destroy completed.\n")
	}

	// The project last, because tofu cannot delete it and nothing else
	// will: an interrupt is the one exit with no summary to report a
	// kept project in.
	_, projectFailures := releaseRunProject(
		context.Background(), runtime, cloud, workDir, marker.ProjectID, sandboxEnv)
	if len(projectFailures) > 0 {
		_, _ = fmt.Fprintf(out, "%s\n", projectFailures[0].Detail)
		return err
	}
	_, _ = fmt.Fprintf(out, "Run project %s deleted.\n", marker.ProjectID)
	return err
}

func reportAbandonedResources(out interface{ Write([]byte) (int, error) }, statePath string, cause error) {
	_, _ = fmt.Fprintf(out, strings.TrimSpace(`
CLEANUP FAILED — real resources may still be running and billing.
  cause: %v
  state: %s
  fix:   infrafactory reap <scenario>
`)+"\n", cause, statePath)
}
