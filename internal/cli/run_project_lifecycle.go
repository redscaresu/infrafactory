package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// ensureRunProject creates the run's own project when ADR-0025's flag is
// on, and returns the id to hand the provider as SCW_DEFAULT_PROJECT_ID.
//
// With the flag off it returns an empty id and no stages, so the
// pre-ADR-0025 path is untouched.
//
// Failure to create is fatal to the run rather than a fallback to the
// shared project: the caller asked for a run-owned project, and silently
// applying into the shared one instead would put this run's strays next
// to every other run's.
func ensureRunProject(ctx context.Context, runtime *CommandRuntime, scenario string) (string, []StageSummary, []FailureSummary) {
	if !runtime.Config.Scaleway.CreateRunProject {
		return "", nil, nil
	}

	if runtime.Deps.RunProject == nil {
		return "", []StageSummary{{Layer: "sandbox_deploy", Stage: "run_project", Status: StageStatusFail}},
			[]FailureSummary{{
				Layer: "sandbox_deploy", Stage: "run_project", Check: "dependency",
				Command: "create run project",
				Detail:  "scaleway.create_run_project is on but no run-project client is configured",
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

// runProjectStamp keeps generated project names unique per run without
// depending on the scenario name alone.
func runProjectStamp(now time.Time) string {
	return now.UTC().Format("20060102t150405z")
}
