package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// autoCreatedPurgeTimeout bounds the purge's HTTP calls. It walks every
// Instance zone, so it is a handful of small requests, not one long one.
const autoCreatedPurgeTimeout = 15 * time.Second

// runProjectTimeout bounds the two Account API calls that bracket a
// Layer 3 run. Both are single small requests.
const runProjectTimeout = 20 * time.Second

// destroySandbox runs the sandbox destroy, and on failure clears
// API-auto-created resources out of the run's project and tries once
// more.
//
// The retry exists because `tofu destroy` cannot delete a project that
// still contains anything, and Scaleway puts things in the project
// without being asked: the first Instance in a fresh project gets a
// "Default security group" that Terraform never created and never
// removes. Destroy then fails on the project delete with
//
//	precondition failed: resource is still in use
//
// leaving a project behind on every run that declares compute. Purging
// only what the API auto-created and retrying turns that into a clean
// teardown, while a genuine destroy bug still fails both attempts.
//
// The purge is scoped to projectID and guarded by
// harness.AssertProjectDeletable here rather than at the call sites.
// reap asserted it; run, test and the interrupt path did not, and this
// deletes real resources over HTTP with Terraform nowhere in the loop --
// so the guard belongs where it cannot be forgotten. A state file that
// is stale, hand-edited, or names the organization's default project as
// its scaleway_account_project gets no purge at all.
//
// When projectID is empty -- nothing to scope to -- the first result
// stands.
// The returned slice names what the purge removed, so callers can put it
// in the stage summary. A teardown that silently deleted things nobody
// asked it to delete would be worse than the leak it fixes.
func destroySandbox(
	ctx context.Context,
	runtime *CommandRuntime,
	workDir string,
	sandboxEnv map[string]string,
	projectID string,
) (*harness.SandboxDestroyResult, []string, error) {
	// Resolved ONCE, and used by both remediations. Callers pass
	// sweepTargetProjectID(...), which is empty whenever
	// CaptureSweepTarget failed -- and on 2026-09-10 that silently
	// disabled the purge AND the detach together, leaving a run with
	// neither stage to say so. Recovering it inside only one of them
	// would have fixed half the outage.
	projectID = resolveRunProjectID(workDir, projectID)

	// Logged, not just staged.
	result, err := runtime.Deps.SandboxDestroy.Run(ctx, workDir, sandboxEnv)
	if err == nil || projectID == "" {
		if err != nil {
			logPurgeOutcome(runtime, "skipped", fmt.Sprintf("no project id (workDir=%s), so the auto-created purge cannot be scoped", workDir))
		}
		return result, nil, err
	}

	secretKey := sandboxEnv["SCW_SECRET_KEY"]
	if secretKey == "" {
		return result, nil, err
	}

	if runtime.Deps.AutoCreated == nil {
		logPurgeOutcome(runtime, "skipped", "no auto-created purge dependency is wired into this runtime")
		return result, nil, err
	}
	// The marker plus API provenance, not the state file: under ADR-0025
	// the project is not a Terraform resource, so the state never names
	// it. Same guarantee, one forgeable half and one that is not.
	if assertErr := assertRunProjectDeletable(ctx, runtime, workDir, projectID, sandboxEnv); assertErr != nil {
		logPurgeOutcome(runtime, "skipped", fmt.Sprintf("project %s did not pass the deletable check: %v", projectID, assertErr))
		return result, nil, err
	}
	removed, purgeErr := runtime.Deps.AutoCreated.Run(ctx, projectID, secretKey)
	purgeStatus := "success"
	if purgeErr != nil {
		// The status field is what a filter reads. Logging a failed
		// purge as success made the one path that gives up without
		// retrying look like the one that worked.
		purgeStatus = "failed"
	}
	logPurgeOutcome(runtime, purgeStatus, fmt.Sprintf("project=%s removed=%d err=%v", projectID, len(removed), purgeErr))

	if purgeErr != nil || len(removed) == 0 {
		// Nothing was auto-created, so the destroy failed for its own
		// reasons. Report the original error rather than a retry's.
		return result, nil, err
	}

	retryResult, retryErr := runtime.Deps.SandboxDestroy.Run(ctx, workDir, sandboxEnv)
	return retryResult, removed, retryErr
}

// autoCreatedPurgeStage records a purge in the stage list. Only emitted
// when something was actually removed.
func autoCreatedPurgeStage(removed []string) StageSummary {
	return StageSummary{
		Layer:  "sandbox_deploy",
		Stage:  "auto_created_purge",
		Status: StageStatusPass,
		Detail: fmt.Sprintf("destroy was blocked by %d resource(s) the API created but Terraform did not own: %s",
			len(removed), strings.Join(removed, "; ")),
	}
}

// sweepTargetProjectID is nil-safe: capture can fail, and a failed
// capture must not stop the destroy it precedes.
func sweepTargetProjectID(target *harness.SweepTarget) string {
	if target == nil {
		return ""
	}
	return target.ProjectID
}

// resolveRunProjectID falls back to the run-project marker when the
// caller has no project id.
//
// The marker is the same provenance assertRunProjectDeletable reads, so
// this is not a weaker source -- it is the source, one step earlier.
// Callers derive their id from CaptureSweepTarget, which yields nothing
// when the live state is unreadable, and both remediations are scoped by
// that id, so one failed capture disables the purge and the detach at
// once.
func resolveRunProjectID(workDir, projectID string) string {
	if projectID != "" {
		return projectID
	}
	marker, err := harness.ReadRunProjectMarker(workDir)
	if err != nil {
		return ""
	}
	return marker.ProjectID
}

// logPurgeOutcome puts the auto-created purge on the record for
// the NIC detach. Both were stage-only, and `run` does not persist sandbox
// stages -- so after a failed teardown the record could not say whether
// either remediation had even been attempted.
func logPurgeOutcome(runtime *CommandRuntime, status, detail string) {
	if runtime == nil || runtime.Logger == nil {
		return
	}
	level := logLevelInfo
	if status != "success" {
		level = logLevelError
	}
	runtime.Logger.Log(LogEntry{
		Level:   level,
		Command: "run",
		Event:   "layer3_auto_created_purge",
		Status:  status,
		Detail:  detail,
	})
}
