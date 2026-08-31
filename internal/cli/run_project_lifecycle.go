package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// ensureRunProject creates the run's own project and returns the id to
// hand the provider as SCW_DEFAULT_PROJECT_ID.
//
// Unconditional since the cutover: there is one model, so there is no
// flag. The old dual-model flag was scaffolding, and two code paths is
// where this arc's defects came from.
//
// Failure to create is fatal to the run rather than a fallback to the
// shared project: the caller asked for a run-owned project, and silently
// applying into the shared one instead would put this run's strays next
// to every other run's.
func ensureRunProject(ctx context.Context, runtime *CommandRuntime, scenario, workDir string) (string, []StageSummary, []FailureSummary) {
	if runtime.Deps.RunProject == nil {
		return "", []StageSummary{{Layer: "sandbox_deploy", Stage: "run_project", Status: StageStatusFail}},
			[]FailureSummary{{
				Layer: "sandbox_deploy", Stage: "run_project", Check: "dependency",
				Command: "create run project",
				Detail:  "no run-project client is configured, so the run has no project to apply into",
			}}
	}

	secretKey := strings.TrimSpace(os.Getenv("SCW_SECRET_KEY"))
	orgID := strings.TrimSpace(os.Getenv("SCW_DEFAULT_ORGANIZATION_ID"))

	project, err := runtime.Deps.RunProject.Create(ctx, secretKey, orgID, scenario, runProjectStamp(time.Now()))
	if err != nil {
		return "", []StageSummary{{Layer: "sandbox_deploy", Stage: "run_project", Status: StageStatusFail}},
			[]FailureSummary{{
				Layer: "sandbox_deploy", Stage: "run_project", Check: "create",
				Command: "create run project",
				Detail:  err.Error(),
			}}
	}

	// The marker is the guard's witness. Written before anything is
	// applied, and a failure to write it is fatal: without it no teardown
	// can prove this run created the project, so the project would be
	// unreclaimable the moment it had resources in it.
	if err := harness.WriteRunProjectMarker(workDir, project); err != nil {
		// The project exists and nothing has been applied into it yet, so
		// the safe move is to remove it now rather than hand back an
		// empty id and leave the cleanup with nothing to act on. Without
		// the marker no later teardown could authorise deleting it, so
		// this is the only moment it can be reclaimed automatically.
		detail := fmt.Sprintf("created project %s but could not record it in %s: %v",
			project.ID, harness.RunProjectMarkerFilename, err)

		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runProjectTimeout)
		defer cancel()
		if delErr := runtime.Deps.RunProject.Delete(cleanupCtx, secretKey, project.ID); delErr != nil {
			detail += fmt.Sprintf("; deleting it also failed: %v. Destroy it by hand -- no teardown can prove this run owns it", delErr)
		} else {
			detail += "; it was deleted again, so nothing was left behind"
		}

		return "", []StageSummary{{Layer: "sandbox_deploy", Stage: "run_project", Status: StageStatusFail}},
			[]FailureSummary{{
				Layer: "sandbox_deploy", Stage: "run_project", Check: "marker",
				Command: "record run project", Detail: detail,
			}}
	}

	return project.ID, []StageSummary{{
		Layer: "sandbox_deploy", Stage: "run_project", Status: StageStatusPass,
		Detail: fmt.Sprintf("created %s (%s) before the apply; it is the provider default project for this run",
			project.ID, project.Name),
	}}, nil
}

// releaseRunProject deletes the run's project after its resources are
// gone.
//
// `tofu destroy` cannot do this any more: under ADR-0025 the project is
// not a Terraform resource. A project left behind is empty and free, but
// it accumulates and it is exactly the kind of residue the orphan sweep
// exists to make impossible, so a failed delete is reported rather than
// swallowed.
func releaseRunProject(ctx context.Context, runtime *CommandRuntime, projectID string) ([]StageSummary, []FailureSummary) {
	if strings.TrimSpace(projectID) == "" || runtime.Deps.RunProject == nil {
		return nil, nil
	}

	// A FRESH context, bounded on its own. If the run was cancelled --
	// Ctrl-C, a timeout -- the inherited context is already done, and
	// Delete would never reach the API, leaving the project behind on
	// exactly the runs that most need cleaning up. Same reasoning as the
	// interrupt guard's destroy, which also runs after cancellation.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runProjectTimeout)
	defer cancel()

	secretKey := strings.TrimSpace(os.Getenv("SCW_SECRET_KEY"))
	if err := runtime.Deps.RunProject.Delete(cleanupCtx, secretKey, projectID); err != nil {
		return []StageSummary{{Layer: "sandbox_deploy", Stage: "run_project_delete", Status: StageStatusFail}},
			[]FailureSummary{{
				Layer: "sandbox_deploy", Stage: "run_project_delete", Check: "delete",
				Command: "delete run project",
				Detail: fmt.Sprintf(
					"the run's resources were destroyed but its project %s was not deleted: %v. "+
						"An empty project is free but does not clean itself up", projectID, err),
			}}
	}

	return []StageSummary{{
		Layer: "sandbox_deploy", Stage: "run_project_delete", Status: StageStatusPass,
		Detail: fmt.Sprintf("deleted %s", projectID),
	}}, nil
}

// assertRunProjectDeletable is the single place a teardown asks whether
// it may destroy a project: read the marker, ask the API, apply the
// guard. Every destroy path goes through it so none can accidentally
// carry a weaker check.
//
// An unreachable API is an error, not an absence -- refusing costs a
// retry, proceeding wrongly costs a project nobody meant to destroy.
func assertRunProjectDeletable(ctx context.Context, runtime *CommandRuntime, workDir, targetProjectID string, sandboxEnv map[string]string) error {
	marker, err := harness.ReadRunProjectMarker(workDir)
	if err != nil {
		return fmt.Errorf("%w: %v", harness.ErrProtectedProject, err)
	}

	if runtime.Deps.RunProject == nil {
		return fmt.Errorf("%w: no run-project client, so provenance cannot be checked", harness.ErrProtectedProject)
	}

	provenance, err := runtime.Deps.RunProject.Describe(ctx, sandboxEnv["SCW_SECRET_KEY"], targetProjectID)
	if err != nil {
		return fmt.Errorf("%w: could not verify project %s with the API: %v", harness.ErrProtectedProject, targetProjectID, err)
	}

	return harness.AssertRunProjectDeletable(marker, targetProjectID, sandboxEnv["SCW_DEFAULT_ORGANIZATION_ID"], provenance)
}

// runProjectReleased reports whether the destroy path already deleted the
// run's project, so the fallback cleanup does not try again and report a
// second, confusing result.
func runProjectReleased(stages []StageSummary) bool {
	for _, stage := range stages {
		if stage.Stage == "run_project_delete" && stage.Status == StageStatusPass {
			return true
		}
	}
	return false
}

// runProjectStamp keeps generated project names unique per run without
// depending on the scenario name alone.
func runProjectStamp(now time.Time) string {
	return now.UTC().Format("20060102t150405z")
}
