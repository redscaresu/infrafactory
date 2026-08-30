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
		detail := fmt.Sprintf(
			"deployment %s records no work_dir, so there is no state to destroy from. "+
				"Its project %s may still be running: destroy it by hand, then clear the record with "+
				"`infrafactory live forget %s`", d.ID, d.ProjectID, d.ID)
		if d.Undecodable {
			detail = fmt.Sprintf(
				"deployment %s could not be decoded, so nothing can be destroyed from it. Its resources may "+
					"still be running -- inspect %s.json by hand, destroy what it names, then clear it with "+
					"`infrafactory live forget %s`", d.ID, d.ID, d.ID)
		}
		return unreclaimable(detail)
	}

	statePath := filepath.Join(d.WorkDir, harness.LiveStateFilename)
	if _, err := os.Stat(statePath); errors.Is(err, os.ErrNotExist) {
		return unreclaimable(fmt.Sprintf(
			"deployment %s has no %s (looked in %s). Its project %s may still be running; "+
				"the record is kept rather than released so the leak stays visible",
			d.ID, harness.LiveStateFilename, d.WorkDir, d.ProjectID))
	}

	// An empty state is the signature of a destroy that already ran, so
	// re-running destroy would do nothing. It is NOT evidence the account
	// is clean, and releasing on it alone would launder a previously
	// FAILED orphan sweep into a green result: destroy succeeds, the
	// sweep finds orphans and fails before release, and the next pass
	// sees an empty state and retires the record without ever re-running
	// the sweep. So the sweep is re-run here, against the project id the
	// record carries, and the record is released only if it passes.
	if !liveStateMayHoldResources(d.WorkDir) {
		sandboxEnv, envErr := sandboxCommandEnv(runtime)
		if envErr != nil {
			return unreclaimable(fmt.Sprintf(
				"%s appears already destroyed, but the account cannot be verified: %v", d.ID, envErr))
		}

		stages, failures = appendOrphanSweepResult(ctx, stages, failures, runtime,
			&harness.SweepTarget{ProjectID: d.ProjectID}, nil, sandboxEnv)
		if len(failures) > 0 {
			return stages, failures
		}

		if err := store.MarkReleased(d.ID); err != nil {
			return unreclaimable(fmt.Sprintf(
				"%s was already destroyed and the account verified clean, but the record could not be released: %v",
				d.ID, err))
		}
		stages = append(stages, StageSummary{
			Layer: "live", Stage: "release", Status: StageStatusPass,
			Detail: fmt.Sprintf("%s was already destroyed; account re-verified, record released", d.ID),
		})
		return stages, failures
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
					"Its resources are GONE -- delete the record by hand; do not expect a later reap to "+
					"re-verify it, because destroy has already emptied the state it would read", d.ID, err),
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

// runLiveForgetCommand releases a record WITHOUT destroying anything.
//
// The escape hatch for a record the tooling cannot act on: one whose
// bytes will not decode, or whose workdir is gone. Those are reapable by
// design (ADR-0024 rule 3) but not reclaimable, so without this they
// fail every pass forever and `live ls`/`live reap` stay red with no way
// out -- while `live teardown` cannot even load them.
//
// It destroys nothing and verifies nothing. It is the operator asserting
// they have dealt with the resources by hand, so it says plainly what it
// is giving up, and the unparseable bytes are preserved beside the
// released record rather than replaced.
func runLiveForgetCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	store := livestore.NewFilesystemStore(runtime.LiveStoreRoot())
	id := args[0]
	out := cmd.ErrOrStderr()

	known := "unreadable"
	if d, err := store.Get(id); err == nil {
		known = fmt.Sprintf("scenario %s, project %s", d.Scenario, d.ProjectID)
	}

	if err := store.MarkReleased(id); err != nil {
		return &CLIError{Op: "live forget", Code: errorCodeCommandFailed, Err: err}
	}

	_, _ = fmt.Fprintf(out,
		"Released %s (%s) WITHOUT destroying anything and WITHOUT verifying the account.\n"+
			"If its resources still exist they are now untracked — nothing will reap them.\n", id, known)

	return finishLiveCommand(cmd, "live forget", "n/a",
		[]StageSummary{{
			Layer: "live", Stage: "forget", Status: StageStatusPass,
			Detail: fmt.Sprintf("%s released without destroy or verification (%s)", id, known),
		}}, nil, nil)
}

func runLiveTeardownCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	store := livestore.NewFilesystemStore(runtime.LiveStoreRoot())

	d, err := store.Get(args[0])
	if err != nil {
		// An undecodable record still reaches teardown, which reports it
		// as unreclaimable and names `live forget`. Failing at load
		// returned a *usage* error, which reads like the operator
		// mistyped the id rather than like the store cannot parse it.
		if !errors.Is(err, os.ErrNotExist) {
			d = livestore.Deployment{ID: args[0], Undecodable: true}
		} else {
			return &CLIError{Op: "live teardown", Code: errorCodeUsage, Err: err}
		}
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
	// Progress goes to stderr: stdout carries the output contract, and
	// interleaving human lines there makes --output json unparseable
	// from byte 0.
	progress := cmd.ErrOrStderr()

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
		_, _ = fmt.Fprintln(progress, "Nothing has expired.")
		stages = append(stages, StageSummary{Layer: "live", Stage: "reap", Status: StageStatusPass, Detail: "no expired deployments"})
		return finishLiveCommand(cmd, "live reap", "n/a", stages, failures,
			errors.New("reap could not account for every live record"))
	}

	if dryRun {
		for _, d := range expired {
			_, _ = fmt.Fprintf(progress, "would tear down %s (scenario %s, project %s, expired %s)\n",
				d.ID, d.Scenario, d.ProjectID, d.ExpiresAt.Format(time.RFC3339))
		}
		_, _ = fmt.Fprintf(progress, "\n--dry-run: nothing destroyed. %d deployment(s) would be torn down.\n", len(expired))
		// Through finishLiveCommand, not `return nil`. Returning early
		// discarded the failures already recorded for unreadable
		// records, so a dry run exited 0 while something that may be
		// running was unaccounted for -- and skipped the output
		// contract entirely, so --output json emitted no JSON.
		stages = append(stages, StageSummary{
			Layer: "live", Stage: "reap", Status: StageStatusSkip,
			Detail: fmt.Sprintf("--dry-run: %d deployment(s) would be torn down", len(expired)),
		})
		return finishLiveCommand(cmd, "live reap", "n/a", stages, failures,
			errors.New("reap could not account for every live record"))
	}

	for _, d := range expired {
		_, _ = fmt.Fprintf(progress, "tearing down %s (scenario %s, project %s)\n", d.ID, d.Scenario, d.ProjectID)
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
