package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

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
	markerPath := filepath.Join(workDir, harness.RunProjectMarkerFilename)
	_, stateErr := os.Stat(statePath)
	_, markerErr := os.Stat(markerPath)
	if errors.Is(stateErr, os.ErrNotExist) && errors.Is(markerErr, os.ErrNotExist) {
		_, _ = fmt.Fprintf(out, "No %s or %s in %s — nothing to reap.\n",
			harness.LiveStateFilename, harness.RunProjectMarkerFilename, workDir)
		return nil
	}

	// The marker, not the state: ADR-0025 took the project out of
	// Terraform, so the state no longer names it.
	marker, err := harness.ReadRunProjectMarker(workDir)
	if err != nil {
		return &CLIError{Op: "reap", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"%v, so there is no way to tell which project this run created. Refusing to destroy anything", err)}
	}
	projectID := marker.ProjectID

	// Scoped to the project the marker names: the apply ran with it as
	// the provider default, so the destroy that inverts it must too.
	sandboxEnv, err := sandboxCommandEnvForProject(runtime, projectID)
	if err != nil {
		return &CLIError{Op: "reap", Code: errorCodeCommandFailed, Err: err}
	}
	// The guard that stands between this command and real infrastructure.
	// reap only ever destroys the project recorded in the state file it
	// was handed -- never one named on the command line, never the
	// organization default.
	if err := assertRunProjectDeletable(ctx, runtime, workDir, projectID, sandboxEnv); err != nil {
		return &CLIError{Op: "reap", Code: errorCodeCommandFailed, Err: err}
	}

	_, _ = fmt.Fprintf(out, "Live state: %s\n", statePath)
	_, _ = fmt.Fprintf(out, "Run project: %s\n", projectID)

	if dryRun {
		_, _ = fmt.Fprintf(out, "\n--dry-run: nothing destroyed. Re-run without the flag to tear this down.\n")
		return nil
	}

	sweepTarget, sweepTargetErr := harness.CaptureSweepTarget(workDir)
	destroyResult, purged, destroyErr := destroySandbox(ctx, runtime, workDir, sandboxEnv, sweepTargetProjectID(sweepTarget))
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
		projectStages, projectFailures := releaseRunProject(ctx, runtime, workDir, projectID, sandboxEnv)
		stages = append(stages, projectStages...)
		failures = append(failures, projectFailures...)

		stages, failures = appendOrphanSweepResult(ctx, stages, failures, runtime, sweepTarget, sweepTargetErr, sandboxEnv)
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

	// Scoped to the run's project, so the destroy runs with the same
	// provider default the apply did.
	sandboxEnv, envErr := sandboxCommandEnvForProject(runtime, marker.ProjectID)
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
		_, purged, destroyErr := destroySandbox(
			context.Background(), runtime, workDir, sandboxEnv, sweepTargetProjectID(cleanupTarget))
		if destroyErr != nil {
			reportAbandonedResources(out, statePath, destroyErr)
			return err
		}
		if len(purged) > 0 {
			_, _ = fmt.Fprintf(out, "%s\n", autoCreatedPurgeStage(purged).Detail)
		}
		_, _ = fmt.Fprintf(out, "Cleanup destroy completed.\n")
	}

	// The project last, because tofu cannot delete it and nothing else
	// will: an interrupt is the one exit with no summary to report a
	// kept project in.
	if markerErr != nil {
		return err
	}
	_, projectFailures := releaseRunProject(
		context.Background(), runtime, workDir, marker.ProjectID, sandboxEnv)
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
