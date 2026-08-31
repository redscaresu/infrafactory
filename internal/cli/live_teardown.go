package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	// No early bail on a missing state file. Since ADR-0025 the project
	// is created BEFORE the apply, so "no state" is the ordinary shape of
	// a deploy that failed at preflight, init or plan: nothing to destroy
	// with tofu, but a real project to delete. That case falls through to
	// the no-resources path below, which deletes it and verifies.
	// An empty state is the signature of a destroy that already ran, so
	// re-running destroy would do nothing. It is NOT evidence the account
	// is clean, and releasing on it alone would launder a previously
	// FAILED orphan sweep into a green result: destroy succeeds, the
	// sweep finds orphans and fails before release, and the next pass
	// sees an empty state and retires the record without ever re-running
	// the sweep. So the sweep is re-run here, against the project id the
	// record carries, and the record is released only if it passes.
	if !liveStateMayHoldResources(d.WorkDir) {
		// A sweep that previously failed cannot be re-run faithfully: it
		// found strays OUTSIDE the run project, and CaptureSweepTarget
		// computes those from the state that destroy has since emptied.
		// Re-running against the project id alone would find the project
		// gone, report clean, and release the record while the strays keep
		// billing untracked -- laundering the failure this branch exists
		// to prevent. Refuse, and point at the escape hatch.
		if d.SweepVerificationFailed {
			return unreclaimable(fmt.Sprintf(
				"%s had an orphan sweep FAIL before its state was emptied, so strays outside project %s "+
					"cannot be recomputed and this pass cannot prove the account is clean. Verify by hand, "+
					"then clear the record with `infrafactory live forget %s`", d.ID, d.ProjectID, d.ID))
		}

		sandboxEnv, envErr := sandboxCommandEnv(runtime)
		if envErr != nil {
			return unreclaimable(fmt.Sprintf(
				"%s has nothing to destroy, but the account cannot be verified: %v", d.ID, envErr))
		}

		// tofu never created the project and cannot remove it, so the
		// delete happens here -- before the sweep, which exists to verify
		// the project is gone. A project already deleted answers 404 and
		// that is success, so this is safe to re-run.
		//
		// releaseRunProject runs the deletability guard itself, so a
		// record with neither state nor marker fails closed here rather
		// than releasing quietly.
		projectStages, projectFailures := releaseRunProject(ctx, runtime, d.WorkDir, d.ProjectID, sandboxEnv)
		stages = append(stages, projectStages...)
		failures = append(failures, projectFailures...)

		stages, failures = appendOrphanSweepResult(ctx, stages, failures, runtime,
			&harness.SweepTarget{ProjectID: d.ProjectID}, nil, sandboxEnv)
		if len(failures) > 0 {
			return stages, failures
		}

		if err := store.MarkReleased(d.ID); err != nil {
			return unreclaimable(fmt.Sprintf(
				"%s was cleaned up and the account verified clean, but the record could not be released: %v",
				d.ID, err))
		}
		stages = append(stages, StageSummary{
			Layer: "live", Stage: "release", Status: StageStatusPass,
			Detail: fmt.Sprintf(
				"%s held no resources; its project was deleted, the account verified, record released", d.ID),
		})
		return stages, failures
	}

	sandboxEnv, err := sandboxCommandEnv(runtime)
	if err != nil {
		return unreclaimable(fmt.Sprintf("sandbox credentials for %s: %v", d.ID, err))
	}

	// The record says which deployment; the marker and the API say which
	// project. A disagreement between any of them is fatal rather than
	// silent, so neither a stale record nor a forged marker can aim this
	// at infrastructure the run did not create.
	if err := assertRunProjectDeletable(ctx, runtime, d.WorkDir, d.ProjectID, sandboxEnv); err != nil {
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
		// Before the sweep, for the same reason as the run path: the
		// sweep verifies the project is GONE, and tofu no longer deletes
		// it. Deleting afterwards would make every clean teardown report
		// a leak.
		projectStages, projectFailures := releaseRunProject(ctx, runtime, d.WorkDir, d.ProjectID, sandboxEnv)
		stages = append(stages, projectStages...)
		failures = append(failures, projectFailures...)

		failuresBeforeSweep := len(failures)
		stages, failures = appendOrphanSweepResult(ctx, stages, failures, runtime, sweepTarget, sweepTargetErr, sandboxEnv)
		if len(failures) > failuresBeforeSweep {
			// Sticky, and written before returning: the next pass sees an
			// empty state and must not treat that as evidence of a clean
			// account. A failed write is itself a failure -- dropping it
			// leaves the flag false, so the next pass takes the
			// empty-state path and releases while the strays it could not
			// see keep billing.
			d.SweepVerificationFailed = true
			if err := store.Put(d); err != nil {
				stages = append(stages, StageSummary{Layer: "live", Stage: "sweep_marker", Status: StageStatusFail})
				failures = append(failures, FailureSummary{
					Layer: "live", Stage: "sweep_marker", Check: "record",
					Command: "live teardown " + d.ID,
					Detail: fmt.Sprintf(
						"%s had an orphan sweep fail AND the marker recording that could not be written: %v. "+
							"Do not re-run teardown expecting it to refuse -- verify project %s by hand",
						d.ID, err, d.ProjectID),
				})
			}
		}
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

		// Refuse a deployment teardown could still handle. This command
		// exists for records the tooling CANNOT act on; pointed at a
		// healthy one it would mark it released, make Reapable() false
		// forever, and leave the project billing with nothing that will
		// ever destroy it -- a one-command permanent leak, which is the
		// failure class this whole subsystem exists to prevent.
		if reclaimable(d) {
			return &CLIError{Op: "live forget", Code: errorCodeUsage, Err: fmt.Errorf(
				"%s is reclaimable (%s): use `infrafactory live teardown %s`, which destroys and verifies. "+
					"forget abandons tracking and is only for records teardown cannot act on", id, known, id)}
		}
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

// reclaimable reports whether teardown could still act on this record --
// it decodes, it is not already released, and its run-project marker is
// on disk.
//
// The marker, not the state file: since ADR-0025 a deploy that failed
// before writing state still left a real project behind, and teardown
// can delete it. Gating on state would send exactly those records to
// `live forget`, retiring the record while the project kept existing.
// The marker is also what the teardown guard reads, so a record without
// one is the one teardown genuinely cannot act on.
func reclaimable(d livestore.Deployment) bool {
	if d.Undecodable || d.State == livestore.StateReleased || d.WorkDir == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(d.WorkDir, harness.RunProjectMarkerFilename)); err != nil {
		return false
	}
	// Teardown needs a project id: without one AssertProjectDeletable
	// refuses and nothing can be destroyed or released. Treating such a
	// record as teardown's business left it rejected by both commands --
	// the same dead end, one class along.
	if strings.TrimSpace(d.ProjectID) == "" {
		return false
	}
	// A record whose sweep failed before its state was emptied is exactly
	// what teardown refuses and tells the operator to forget. Counting it
	// as reclaimable made the two commands point at each other with no way
	// out -- a dead end introduced while closing the previous one.
	if d.SweepVerificationFailed && !liveStateMayHoldResources(d.WorkDir) {
		return false
	}
	return true
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
