package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/redscaresu/infrafactory/internal/livestore"
)

// sandboxRunEnabled reports whether this run could have created a run
// project at all. A Layer 2 run creates none, so checking would be
// noise.
func sandboxRunEnabled(runtime *CommandRuntime) bool {
	return runtime != nil && runtime.Config.Validation.Layers.SandboxDeploy.Enabled
}

// reportStrayRunProjects names run projects the organization holds that
// nothing explains.
//
// # Why a run is the place to do it
//
// Because it is the only moment anybody looks. `live reconcile` has done
// this comparison since S157a and is invoked by hand, which is to say
// never -- eight stray projects accumulated over two days, four still
// running an instance, and they surfaced only because a human happened
// to list projects.
//
// The leak is structural, not incidental: a failed run KEEPS its project
// on purpose (the id is the handle to whatever survived), the orphan
// sweep only ever inspects the current run's project, and `reap` needs a
// state file the next iteration has already overwritten. Every
// individual decision is right and the aggregate leaks.
//
// # Why it reports and never deletes
//
// Same reason `live reconcile` does not: a project this system's records
// cannot explain is by definition something it does not understand, and
// deleting what you do not understand is how a reconciler becomes the
// incident. The blast radius is somebody's running infrastructure.
//
// # Why a stray fails the run
//
// ADR-0024: a run that cannot prove the account is clean must not report
// success. A stray run project is billing infrastructure with no TTL and
// nothing that will reap it, so a green summary alongside one would be
// false in the way this project keeps removing. A run that leaves no
// stray is unaffected, and a run that inherits somebody else's stray
// finds out at the only moment anyone is watching.
func reportStrayRunProjects(ctx context.Context, runtime *CommandRuntime) ([]StageSummary, []FailureSummary) {
	if runtime.Deps.RunProject == nil {
		return nil, nil
	}
	secretKey := strings.TrimSpace(os.Getenv("SCW_SECRET_KEY"))
	orgID := strings.TrimSpace(os.Getenv("SCW_DEFAULT_ORGANIZATION_ID"))
	if secretKey == "" || orgID == "" {
		// Silent here, unlike `live reconcile` which fails. A Layer 3
		// run cannot reach this point without credentials -- the
		// sandbox preflight refuses first -- so an absence means the
		// environment is shaped in a way this check cannot read, not
		// that the estate is clean. Reporting a failure the operator
		// cannot act on would train them to ignore this stage.
		return nil, nil
	}

	listed, err := runtime.Deps.RunProject.List(ctx, secretKey, orgID)
	if err != nil {
		// "Could not check" and "nothing leaked" must never look alike.
		return nil, []FailureSummary{{
			Layer: "live", Stage: "stray_run_projects", Check: "reconcile",
			Command: "run",
			Detail: fmt.Sprintf(
				"could not list the organization's projects, so stray run projects cannot be ruled out: %v", err),
		}}
	}

	store := livestore.NewFilesystemStore(runtime.LiveStoreRoot())
	deployments, unreadable, err := store.List()
	if err != nil {
		return nil, []FailureSummary{{
			Layer: "live", Stage: "stray_run_projects", Check: "reconcile",
			Command: "run",
			Detail: fmt.Sprintf(
				"could not read the live store, so a project it explains would be misreported as stray: %v", err),
		}}
	}

	// An unreadable record is not a clean estate. The record might be the
	// one explaining a project that is about to be called stray, or the
	// one whose absence hides a real leak -- either way this check
	// cannot do its job and must say so rather than return a pass.
	var failures []FailureSummary
	for _, u := range unreadable {
		failures = append(failures, FailureSummary{
			Layer: "live", Stage: "stray_run_projects", Check: "record",
			Command: "run",
			Detail: fmt.Sprintf(
				"a live record could not be read, so a project it explains cannot be told from a stray: %v", u),
		})
	}

	stamped := stampedProjects(listed)
	result := livestore.Reconcile(stamped, deployments)
	// `len(failures) == 0` matters as much as the unrecorded count: an
	// unreadable record already produced a failure above, and returning
	// the pass here would throw it away and report a clean estate that
	// was never fully read.
	if len(result.Unrecorded) == 0 && len(failures) == 0 {
		// The STAMPED count, not every project in the organization.
		// Saying "12 projects carry infrafactory's stamp" when eleven of
		// them are somebody else's overstates what was checked, and this
		// line is the evidence an operator reads for "nothing leaked".
		ours := 0
		for _, p := range stamped {
			if p.Ours {
				ours++
			}
		}
		return []StageSummary{{
			Layer: "live", Stage: "stray_run_projects", Status: StageStatusPass,
			Detail: fmt.Sprintf("%d project(s) carry infrafactory's stamp and all are accounted for (%d in the organization)",
				ours, len(listed)),
		}}, nil
	}

	for _, p := range result.Unrecorded {
		failures = append(failures, FailureSummary{
			Layer: "live", Stage: "stray_run_projects", Check: "unrecorded",
			Command: "run",
			Detail: fmt.Sprintf(
				"project %s (%s) carries infrafactory's stamp but nothing explains it — it has no TTL and "+
					"nothing will reap it. Nothing was destroyed. Inspect it and tear it down: "+
					"`scw account project delete project-id=%s`, or `live reconcile` for the full picture",
				p.ProjectID, p.Name, p.ProjectID),
		})
	}
	return []StageSummary{{
		Layer: "live", Stage: "stray_run_projects", Status: StageStatusFail,
		Detail: fmt.Sprintf("%d run project(s) nothing explains, %d live record(s) unreadable",
			len(result.Unrecorded), len(unreadable)),
	}}, failures
}
