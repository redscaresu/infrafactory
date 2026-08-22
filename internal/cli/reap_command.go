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
	if _, statErr := os.Stat(statePath); errors.Is(statErr, os.ErrNotExist) {
		_, _ = fmt.Fprintf(out, "No %s in %s — nothing to reap.\n", harness.LiveStateFilename, workDir)
		return nil
	}

	projectID, err := harness.RunProjectIDFromState(workDir)
	if err != nil {
		return &CLIError{Op: "reap", Code: errorCodeCommandFailed, Err: fmt.Errorf("read live state: %w", err)}
	}
	if projectID == "" {
		return &CLIError{Op: "reap", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"%s records no %s, so there is no way to tell which project this run created. Refusing to destroy anything",
			harness.LiveStateFilename, harness.ProjectResourceType)}
	}

	sandboxEnv, err := sandboxCommandEnv(runtime)
	if err != nil {
		return &CLIError{Op: "reap", Code: errorCodeCommandFailed, Err: err}
	}
	// The guard that stands between this command and real infrastructure.
	// reap only ever destroys the project recorded in the state file it
	// was handed -- never one named on the command line, never the
	// organization default.
	if err := harness.AssertProjectDeletable(projectID, projectID, sandboxEnv["SCW_DEFAULT_ORGANIZATION_ID"]); err != nil {
		return &CLIError{Op: "reap", Code: errorCodeCommandFailed, Err: err}
	}

	_, _ = fmt.Fprintf(out, "Live state: %s\n", statePath)
	_, _ = fmt.Fprintf(out, "Run project: %s\n", projectID)

	if dryRun {
		_, _ = fmt.Fprintf(out, "\n--dry-run: nothing destroyed. Re-run without the flag to tear this down.\n")
		return nil
	}

	destroyResult, destroyErr := runtime.Deps.SandboxDestroy.Run(ctx, workDir, sandboxEnv)
	stages, failures := appendSandboxDestroyResult(nil, nil, destroyResult, destroyErr)
	if destroyErr == nil {
		stages, failures = appendOrphanSweepResult(ctx, stages, failures, runtime, workDir, sandboxEnv)
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

	// Interrupted. Anything the apply created is live and unowned.
	out := cmd.ErrOrStderr()
	workDir := runtime.OutputDir()
	statePath := filepath.Join(workDir, harness.LiveStateFilename)
	if _, statErr := os.Stat(statePath); errors.Is(statErr, os.ErrNotExist) {
		_, _ = fmt.Fprintf(out, "\nInterrupted before any real resources were created — nothing to clean up.\n")
		return err
	}

	_, _ = fmt.Fprintf(out, "\nInterrupted with real resources live. Destroying before exit — press Ctrl-C again to abandon.\n")

	// stop() restores default signal handling, so a second Ctrl-C kills
	// the process outright rather than being swallowed here.
	stop()

	sandboxEnv, envErr := sandboxCommandEnv(runtime)
	if envErr != nil {
		reportAbandonedResources(out, statePath, envErr)
		return err
	}
	if _, destroyErr := runtime.Deps.SandboxDestroy.Run(context.Background(), workDir, sandboxEnv); destroyErr != nil {
		reportAbandonedResources(out, statePath, destroyErr)
		return err
	}
	_, _ = fmt.Fprintf(out, "Cleanup destroy completed.\n")
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
