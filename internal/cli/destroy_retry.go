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

// instancePowerOffTimeout bounds each HTTP call the poweroff makes, not
// the whole wait. The wait for `stopped` is bounded separately, by
// ScalewayInstancePowerOff's own poll budget, because a poweroff takes
// tens of seconds while each individual request should not.
const instancePowerOffTimeout = 15 * time.Second

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
	// PRECONDITION, not remediation. A private NIC is deletable only while
	// its server is powered off, and `tofu destroy` deletes the NIC before
	// the server -- so a running Instance makes the destroy fail before it
	// has done anything. Unlike the purge below, this is known in advance
	// for every stack that declares compute, so it runs first rather than
	// after a failure nobody could have predicted.
	stopped := powerOffRunInstances(ctx, runtime, workDir, projectID, sandboxEnv)

	result, err := runtime.Deps.SandboxDestroy.Run(ctx, workDir, sandboxEnv)
	if err == nil || projectID == "" {
		return result, nil, stopped, err
	}

	secretKey := sandboxEnv["SCW_SECRET_KEY"]
	if secretKey == "" {
		return result, nil, stopped, err
	}

	if runtime.Deps.AutoCreated == nil {
		return result, nil, stopped, err
	}
	// The marker plus API provenance, not the state file: under ADR-0025
	// the project is not a Terraform resource, so the state never names
	// it. Same guarantee, one forgeable half and one that is not.
	if assertErr := assertRunProjectDeletable(ctx, runtime, workDir, projectID, sandboxEnv); assertErr != nil {
		return result, nil, stopped, err
	}
	removed, purgeErr := runtime.Deps.AutoCreated.Run(ctx, projectID, secretKey)
	if purgeErr != nil || len(removed) == 0 {
		// Nothing was auto-created, so the destroy failed for its own
		// reasons. Report the original error rather than a retry's.
		return result, nil, stopped, err
	}

	retryResult, retryErr := runtime.Deps.SandboxDestroy.Run(ctx, workDir, sandboxEnv)
	return retryResult, removed, stopped, retryErr
}

// powerOffRunInstances is nil-safe on the dependency, the credential and
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
// the poweroff on exactly the paths that tear down long-lived
// infrastructure -- the ones where a failed destroy costs the most.
func powerOffRunInstances(ctx context.Context, runtime *CommandRuntime, workDir, projectID string, sandboxEnv map[string]string) []string {
	if projectID == "" || runtime.Deps.InstanceStop == nil {
		return nil
	}
	secretKey := sandboxEnv["SCW_SECRET_KEY"]
	if secretKey == "" {
		return nil
	}
	if err := assertRunProjectDeletable(ctx, runtime, workDir, projectID, sandboxEnv); err != nil {
		return nil
	}
	stopped, err := runtime.Deps.InstanceStop.Run(ctx, projectID, secretKey)
	if err != nil {
		// Best-effort by design. The destroy runs regardless and reports
		// for itself; the sweep is what fails closed.
		return stopped
	}
	return stopped
}

// instancePowerOffStage records what was stopped. Only emitted when
// something was, because a teardown that quietly changed the state of
// real servers would be worse than the failure it prevents.
//
// FAILS when any server did not reach `stopped`. Reporting that inside a
// passing stage headed "powered off N instance(s)" would be a false
// green of the precise kind this project exists to remove -- a server
// still running is the case where the destroy is ABOUT TO FAIL, and the
// summary would be asserting the opposite.
//
// The failed STAGE deliberately does not, on its own, fail the command.
// Whether the account is clean is decided by the destroy and then by
// ScalewayOrphanSweep, which fails closed -- the same division the purge
// already uses. A server that would not stop while the destroy
// nonetheless succeeded means the teardown worked and the poweroff was
// moot; failing the command there would be a false negative, and this
// path exists to remove false statements in both directions, not to
// trade one for the other.
func instancePowerOffStage(stopped []string) StageSummary {
	status := StageStatusPass
	verb := "powered off"
	for _, s := range stopped {
		if strings.Contains(s, harness.PowerOffNotStoppedMarker) {
			status = StageStatusFail
			verb = "could not power off every instance; the destroy that follows is expected to fail on any private NIC still attached to a running server. Attempted"
			break
		}
	}
	return StageSummary{
		Layer:  "sandbox_deploy",
		Stage:  "instance_poweroff",
		Status: status,
		Detail: fmt.Sprintf("%s %d instance(s) so their private NICs could be deleted: %s",
			verb, len(stopped), strings.Join(stopped, "; ")),
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
