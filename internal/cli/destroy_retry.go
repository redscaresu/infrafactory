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
// The purge is scoped to projectID, which callers must already have put
// through harness.AssertProjectDeletable. When projectID is empty --
// nothing to scope to -- the first result stands.
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
	result, err := runtime.Deps.SandboxDestroy.Run(ctx, workDir, sandboxEnv)
	if err == nil || projectID == "" {
		return result, nil, err
	}

	secretKey := sandboxEnv["SCW_SECRET_KEY"]
	if secretKey == "" {
		return result, nil, err
	}

	if runtime.Deps.AutoCreated == nil {
		return result, nil, err
	}
	removed, purgeErr := runtime.Deps.AutoCreated.Run(ctx, projectID, secretKey)
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
