package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
)

// tearDownDeployment destroys one live deployment, verifies the account
// with the same real-API sweep a normal run uses, and marks the record
// released only if both succeeded.
//
// The registry is not authority for what gets destroyed. It says *which*
// deployment; the state file in that deployment's own workdir says which
// project, and harness.AssertProjectDeletable refuses when the two
// disagree. So a stale, hand-edited or tampered record cannot aim this
// at a project it did not create -- including the organization default.
//
// A deployment whose workdir or state has gone missing is deliberately
// NOT released. Its resources may well still be running, and marking it
// released would retire the only record that says so, converting a
// visible problem into an invisible one.
func tearDownDeployment(
	ctx context.Context,
	runtime *CommandRuntime,
	store *livestore.FilesystemStore,
	d livestore.Deployment,
) ([]StageSummary, []FailureSummary) {
	var stages []StageSummary
	var failures []FailureSummary

	unreclaimable := func(detail string) ([]StageSummary, []FailureSummary) {
		stages = append(stages, StageSummary{Layer: "live", Stage: "teardown", Status: StageStatusFail})
		return stages, append(failures, FailureSummary{
			Layer:   "live",
			Stage:   "teardown",
			Check:   "reclaimable",
			Command: "live teardown " + d.ID,
			Detail:  detail,
		})
	}

	if d.WorkDir == "" {
		return unreclaimable(fmt.Sprintf(
			"deployment %s records no work_dir, so there is no state to destroy from. "+
				"Its project %s may still be running: destroy it by hand and delete the record",
			d.ID, d.ProjectID))
	}

	statePath := filepath.Join(d.WorkDir, harness.LiveStateFilename)
	if _, err := os.Stat(statePath); errors.Is(err, os.ErrNotExist) {
		return unreclaimable(fmt.Sprintf(
			"deployment %s has no %s (looked in %s). Its project %s may still be running; "+
				"the record is kept rather than released so the leak stays visible",
			d.ID, harness.LiveStateFilename, d.WorkDir, d.ProjectID))
	}

	stateProjectID, err := harness.RunProjectIDFromState(d.WorkDir)
	if err != nil {
		return unreclaimable(fmt.Sprintf("read live state for %s: %v", d.ID, err))
	}

	sandboxEnv, err := sandboxCommandEnv(runtime)
	if err != nil {
		return unreclaimable(fmt.Sprintf("sandbox credentials for %s: %v", d.ID, err))
	}

	// The record says which deployment; the state says which project.
	// Passing both makes a disagreement fatal rather than silent.
	if err := harness.AssertProjectDeletable(
		stateProjectID, d.ProjectID, sandboxEnv["SCW_DEFAULT_ORGANIZATION_ID"],
	); err != nil {
		return unreclaimable(fmt.Sprintf(
			"refusing to destroy for deployment %s: %v", d.ID, err))
	}

	sweepTarget, sweepTargetErr := harness.CaptureSweepTarget(d.WorkDir)
	destroyResult, purged, destroyErr := destroySandbox(
		ctx, runtime, d.WorkDir, sandboxEnv, sweepTargetProjectID(sweepTarget))
	stages, failures = appendSandboxDestroyResult(stages, failures, destroyResult, destroyErr)
	if len(purged) > 0 {
		stages = append(stages, autoCreatedPurgeStage(purged))
	}
	if destroyErr == nil {
		stages, failures = appendOrphanSweepResult(ctx, stages, failures, runtime, sweepTarget, sweepTargetErr, sandboxEnv)
	}

	if len(failures) > 0 {
		return stages, failures
	}

	// Released only once destroy AND the sweep agreed. Marking it earlier
	// would retire the record while resources might still exist.
	if err := store.MarkReleased(d.ID); err != nil {
		stages = append(stages, StageSummary{Layer: "live", Stage: "release", Status: StageStatusFail})
		return stages, append(failures, FailureSummary{
			Layer:   "live",
			Stage:   "release",
			Check:   "record",
			Command: "live teardown " + d.ID,
			Detail: fmt.Sprintf(
				"%s was destroyed and swept, but the record could not be marked released: %v. "+
					"It will be attempted again on the next reap", d.ID, err),
		})
	}

	stages = append(stages, StageSummary{
		Layer:  "live",
		Stage:  "release",
		Status: StageStatusPass,
		Detail: fmt.Sprintf("%s released (project %s destroyed)", d.ID, d.ProjectID),
	})

	return stages, failures
}

func runLiveTeardownCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	store := livestore.NewFilesystemStore(runtime.LiveStoreRoot())

	d, err := store.Get(args[0])
	if err != nil {
		return &CLIError{Op: "live teardown", Code: errorCodeUsage, Err: err}
	}

	stages, failures := tearDownDeployment(cmd.Context(), runtime, store, d)

	return finishLiveCommand(cmd, "live teardown", d.Scenario, stages, failures,
		errors.New("teardown did not leave the account provably clean"))
}

// runLiveReapCommand destroys every deployment whose TTL has run out.
//
// This is what makes persistence safe rather than a slow leak, and per
// ADR-0024 it ships in the same slice as the ability to persist. It
// reports what it removed: a reaper that tears things down silently is
// indistinguishable from one that did nothing, which is the D6 lesson.
func runLiveReapCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	store := livestore.NewFilesystemStore(runtime.LiveStoreRoot())
	out := cmd.OutOrStdout()

	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return &CLIError{Op: "live reap", Code: errorCodeUsage, Err: fmt.Errorf("read --dry-run flag: %w", err)}
	}

	expired, unreadable, err := store.Reapable(time.Now())
	if err != nil {
		return &CLIError{Op: "live reap", Code: errorCodeCommandFailed, Err: err}
	}

	var stages []StageSummary
	var failures []FailureSummary

	// Surfaced before any teardown: an unreadable record may describe
	// running infrastructure this command is about to report as handled.
	for _, readErr := range unreadable {
		stages = append(stages, StageSummary{Layer: "live", Stage: "scan", Status: StageStatusFail})
		failures = append(failures, FailureSummary{
			Layer:   "live",
			Stage:   "scan",
			Check:   "readable",
			Command: "live reap",
			Detail:  fmt.Sprintf("%v — this record may describe running infrastructure that reap cannot reach", readErr),
		})
	}

	if len(expired) == 0 {
		_, _ = fmt.Fprintln(out, "Nothing has expired.")
		stages = append(stages, StageSummary{Layer: "live", Stage: "reap", Status: StageStatusPass, Detail: "no expired deployments"})
		return finishLiveCommand(cmd, "live reap", "n/a", stages, failures,
			errors.New("reap could not account for every live record"))
	}

	if dryRun {
		for _, d := range expired {
			_, _ = fmt.Fprintf(out, "would tear down %s (scenario %s, project %s, expired %s)\n",
				d.ID, d.Scenario, d.ProjectID, d.ExpiresAt.Format(time.RFC3339))
		}
		_, _ = fmt.Fprintf(out, "\n--dry-run: nothing destroyed. %d deployment(s) would be torn down.\n", len(expired))
		return nil
	}

	for _, d := range expired {
		_, _ = fmt.Fprintf(out, "tearing down %s (scenario %s, project %s)\n", d.ID, d.Scenario, d.ProjectID)
		deploymentStages, deploymentFailures := tearDownDeployment(cmd.Context(), runtime, store, d)
		stages = append(stages, deploymentStages...)
		failures = append(failures, deploymentFailures...)
	}

	return finishLiveCommand(cmd, "live reap", "n/a", stages, failures,
		errors.New("reap did not leave the account provably clean"))
}

// finishLiveCommand renders the shared output contract and maps failures
// onto a non-zero exit.
func finishLiveCommand(
	cmd *cobra.Command,
	op string,
	scenario string,
	stages []StageSummary,
	failures []FailureSummary,
	failErr error,
) error {
	status := CommandStatusSuccess
	if len(failures) > 0 {
		status = CommandStatusFailed
	}

	if err := writeCommandOutput(cmd, OutputResult{
		Command:  op,
		Scenario: scenario,
		Status:   status,
		Stages:   stages,
		Failures: failures,
	}); err != nil {
		return err
	}

	if status == CommandStatusFailed {
		return &CLIError{Op: op, Code: errorCodeCommandFailed, Err: failErr}
	}

	return nil
}
