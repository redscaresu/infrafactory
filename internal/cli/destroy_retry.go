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

// privateNICDetachTimeout bounds each HTTP call the NIC detach makes.
// They are all small list/delete requests -- there is nothing to wait
// for, which is one of the ways this differs from the poweroff this
// replaced -- see ADR-0031 for why that was the wrong fix.
const privateNICDetachTimeout = 15 * time.Second

// nicDetachSkipReasonPrefix marks a result entry that is a REASON NOTHING
// HAPPENED rather than a server that was detached. Carried in the same
// slice so no call site changes; marked so the stage cannot render it as
// an accomplishment.
const nicDetachSkipReasonPrefix = "skipped: "

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
) (*harness.SandboxDestroyResult, []string, []string, error) {
	// PRECONDITION, not remediation. Provider 2.81.0 deletes a private
	// NIC through Instance v2alpha1, and that endpoint answers 412 "Can't
	// delete a private network interface attached to a server" -- which
	// is true of every NIC there is, so the provider can never destroy
	// one. Removing them here through v1 first leaves the provider's own
	// delete nothing to fail on. See ADR-0031.
	//
	// Unlike the purge below, this is known in advance for every stack
	// that declares private networking, so it runs first rather than
	// after a failure nobody could have predicted.
	// Resolved ONCE, and used by both remediations. Callers pass
	// sweepTargetProjectID(...), which is empty whenever
	// CaptureSweepTarget failed -- and on 2026-09-10 that silently
	// disabled the purge AND the detach together, leaving a run with
	// neither stage to say so. Recovering it inside only one of them
	// would have fixed half the outage.
	projectID = resolveRunProjectID(workDir, projectID)

	detached, nicDetachSkipped := detachRunProjectNICs(ctx, runtime, workDir, projectID, sandboxEnv)

	// Logged, not just staged.
	//
	// The previous attempt reported skip reasons into a StageSummary --
	// and `run` never persists sandbox stages. iteration.json holds only
	// generate/validate/test, so a real run on 2026-09-10 produced the
	// string "poweroff" exactly ZERO times anywhere on disk (that was
	// this step's predecessor), and the
	// question "did it run?" needed a live API probe to answer. A guard
	// that says why into a channel with no reader is worse than one that
	// says nothing, because it looks fixed.
	//
	// runtime.Logger is the channel every other teardown diagnostic uses
	// -- layer3_auto_destroy and layer3_cleanup_unverified both surface
	// through it, into app.log and the UI event stream.
	logNICDetachOutcome(runtime, projectID, detached, nicDetachSkipped)

	// Captured BEFORE the skip sentinel is appended below. `detached`
	// doubles as the display slice, and a skip reason rides in it as a
	// single marked entry -- so len(detached) > 0 does NOT mean anything
	// was actually detached. Reading it that way made every "no detach
	// dependency" run retry the destroy as though state had changed.

	if nicDetachSkipped != "" && len(detached) == 0 {
		// Carried in the same slice so no call site has to change, and
		// marked so privateNICDetachStage renders it as a SKIP rather
		// than claiming instances were detached.
		detached = []string{nicDetachSkipReasonPrefix + nicDetachSkipped}
	}

	result, err := runtime.Deps.SandboxDestroy.Run(ctx, workDir, sandboxEnv)
	if err == nil || projectID == "" {
		if err != nil {
			logPurgeOutcome(runtime, "skipped", fmt.Sprintf("no project id (workDir=%s), so the auto-created purge cannot be scoped", workDir))
		}
		return result, nil, detached, err
	}

	secretKey := sandboxEnv["SCW_SECRET_KEY"]
	if secretKey == "" {
		return result, nil, detached, err
	}

	if runtime.Deps.AutoCreated == nil {
		logPurgeOutcome(runtime, "skipped", "no auto-created purge dependency is wired into this runtime")
		return result, nil, detached, err
	}
	// The marker plus API provenance, not the state file: under ADR-0025
	// the project is not a Terraform resource, so the state never names
	// it. Same guarantee, one forgeable half and one that is not.
	if assertErr := assertRunProjectDeletable(ctx, runtime, workDir, projectID, sandboxEnv); assertErr != nil {
		logPurgeOutcome(runtime, "skipped", fmt.Sprintf("project %s did not pass the deletable check: %v", projectID, assertErr))
		return result, nil, detached, err
	}
	removed, purgeErr := runtime.Deps.AutoCreated.Run(ctx, projectID, secretKey)
	logPurgeOutcome(runtime, "success", fmt.Sprintf("project=%s removed=%d err=%v", projectID, len(removed), purgeErr))

	if purgeErr != nil || len(removed) == 0 {
		// Nothing was auto-created, so the destroy failed for its own
		// reasons. Report the original error rather than a retry's.
		return result, nil, detached, err
	}

	retryResult, retryErr := runtime.Deps.SandboxDestroy.Run(ctx, workDir, sandboxEnv)
	return retryResult, removed, detached, retryErr
}

// detachRunProjectNICs is nil-safe on the dependency, the credential and
// the project id, because destroySandbox runs on paths that have all
// three and paths that have none -- the interrupt handler among them.
//
// Guarded by the same AssertProjectDeletable as the purge. This changes
// the state of real servers over HTTP with Terraform nowhere in the
// loop, and a state file that is stale, hand-edited, or names the
// organization's default project must not reach it.
// workDir, NOT runtime.OutputDir(). `live teardown` and `live reap` call
// destroySandbox with the DEPLOYMENT's directory and need not have loaded
// a scenario at all, so the runtime's output dir is either empty or some
// other run's. Guarding against the wrong state file would have skipped
// the detach on exactly the paths that tear down long-lived
// infrastructure -- the ones where a failed destroy costs the most.
func detachRunProjectNICs(ctx context.Context, runtime *CommandRuntime, workDir, projectID string, sandboxEnv map[string]string) (detached []string, skipped string) {
	if projectID == "" {
		return nil, fmt.Sprintf("no project id from the caller and no usable run-project marker in %s, so there is nothing safe to scope a NIC detach to", workDir)
	}
	if runtime.Deps.NICDetach == nil {
		return nil, "no private-NIC detach dependency is wired into this runtime"
	}
	secretKey := sandboxEnv["SCW_SECRET_KEY"]
	if secretKey == "" {
		return nil, "no SCW_SECRET_KEY in the sandbox environment"
	}
	if err := assertRunProjectDeletable(ctx, runtime, workDir, projectID, sandboxEnv); err != nil {
		return nil, fmt.Sprintf("project %s did not pass the deletable check (%v), so its servers are not this run's to stop", projectID, err)
	}
	detached, err := runtime.Deps.NICDetach.Run(ctx, projectID, secretKey)
	if err != nil {
		// Best-effort by design. The destroy runs regardless and reports
		// for itself; the sweep is what fails closed.
		return detached, fmt.Sprintf("private-NIC detach reported %v", err)
	}
	return detached, ""
}

// privateNICDetachSkippedStage says WHY no NIC was removed.
//
// A guard that stops without saying why is half a guard -- the durable
// finding of the Layer 3 arc, and one this function was written in
// violation of. The first version had three silent `return nil` paths,
// so when a real run failed its destroy with no such stage at all,
// the log could not distinguish "no project id" from "guard refused"
// from "found nothing to stop". That cost a real apply to learn nothing.
func privateNICDetachSkippedStage(reason string) StageSummary {
	return StageSummary{
		Layer:  "sandbox_deploy",
		Stage:  "private_nic_detach",
		Status: StageStatusSkip,
		Detail: fmt.Sprintf("no private NIC was removed before the destroy: %s", reason),
	}
}

// privateNICDetachStage records which private NICs were removed before
// the destroy. Only emitted when some were, because a teardown that
// quietly deleted things nobody asked it to would be worse than the
// failure it prevents.
//
// FAILS when a NIC could not be deleted. That is the case where the
// destroy is ABOUT TO FAIL, and reporting it inside a passing stage
// headed "removed N private NIC(s)" would assert the opposite.
func privateNICDetachStage(detached []string) StageSummary {
	if len(detached) == 1 && strings.HasPrefix(detached[0], nicDetachSkipReasonPrefix) {
		return privateNICDetachSkippedStage(strings.TrimPrefix(detached[0], nicDetachSkipReasonPrefix))
	}
	status := StageStatusPass
	verb := "removed"
	for _, d := range detached {
		if strings.Contains(d, "could NOT be deleted") {
			status = StageStatusFail
			verb = "could not remove every private NIC; the destroy that follows is expected to fail, because provider 2.81.0 deletes them through v2alpha1 and that endpoint refuses every time. Attempted"
			break
		}
	}
	return StageSummary{
		Layer:  "sandbox_deploy",
		Stage:  "private_nic_detach",
		Status: status,
		Detail: fmt.Sprintf("%s %d private NIC(s) via the v1 API, which the provider cannot do for itself: %s",
			verb, len(detached), strings.Join(detached, "; ")),
	}
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

// logNICDetachOutcome puts the detach on the record either way.
//
// Success AND skip, because "found nothing to stop" and "never ran" were
// indistinguishable before -- both produced no stage, no log line, and
// no way to tell them apart without probing the live API.
func logNICDetachOutcome(runtime *CommandRuntime, projectID string, detached []string, skipped string) {
	if runtime == nil || runtime.Logger == nil {
		return
	}
	entry := LogEntry{
		Level:   logLevelInfo,
		Command: "run",
		Event:   "layer3_private_nic_detach",
		Status:  "success",
	}
	switch {
	case skipped != "":
		entry.Level = logLevelError
		entry.Status = "skipped"
		entry.Detail = fmt.Sprintf("project=%s %s", projectID, skipped)
	case len(detached) == 0:
		// Not an error: a stack with no compute has nothing to stop. But
		// it must be DISTINGUISHABLE from a skip, which is the whole
		// point of logging the empty case at all.
		entry.Status = "no_instances"
		entry.Detail = fmt.Sprintf("project=%s: no private NICs to remove", projectID)
	default:
		entry.Detail = fmt.Sprintf("project=%s removed %d private NIC(s): %s", projectID, len(detached), strings.Join(detached, "; "))
	}
	runtime.Logger.Log(entry)
}

// logPurgeOutcome does for the purge what logNICDetachOutcome does for
// the NIC detach. Both were stage-only, and `run` does not persist sandbox
// stages -- so after a failed teardown the record could not say whether
// either remediation had even been attempted.
func logPurgeOutcome(runtime *CommandRuntime, status, detail string) {
	if runtime == nil || runtime.Logger == nil {
		return
	}
	level := logLevelInfo
	if status == "skipped" {
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
