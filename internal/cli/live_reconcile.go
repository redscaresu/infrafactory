package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
)

// runLiveReconcileCommand compares the organization's projects against
// the live store (S157a).
//
// # Why this exists
//
// ADR-0024 promised it and S153 did not deliver it. `livestore.go` said,
// as a statement of fact, that the reaper "reconciles against the API
// rather than trusting this file alone" -- and nothing did. `live reap`
// calls `store.Reapable()` and never contacts Scaleway.
//
// The store lives at `.infrafactory/live`, INSIDE the working directory.
// Wipe it, switch branches, or run from a fresh clone, and the records
// are gone while the load balancer, the instance and the public IPv4s
// keep running with a TTL nobody will ever enforce. Every signal in the
// system reports clean, because every signal reads the store.
//
// That is the shape of D6: a leak whose only symptom is the bill.
//
// # Why it never destroys anything
//
// An unrecorded project is BY DEFINITION something this system's records
// do not explain. Destroying what you cannot explain is how a reconciler
// becomes the incident, and the blast radius here is somebody's running
// infrastructure. It reports project ids and a human decides.
func runLiveReconcileCommand(cmd *cobra.Command, _ []string, runtime *CommandRuntime) error {
	if runtime.Deps.RunProject == nil {
		return &CLIError{Op: "live reconcile", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"no project client is configured, so the cloud cannot be read")}
	}

	secretKey := strings.TrimSpace(os.Getenv("SCW_SECRET_KEY"))
	orgID := strings.TrimSpace(os.Getenv("SCW_DEFAULT_ORGANIZATION_ID"))
	if secretKey == "" || orgID == "" {
		// Fail rather than report a clean estate. A missing credential
		// makes every project invisible, which renders as "nothing
		// unaccounted for" -- the exact false green this command exists
		// to prevent, and the S139 lesson.
		return &CLIError{Op: "live reconcile", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"reconciliation needs SCW_SECRET_KEY and SCW_DEFAULT_ORGANIZATION_ID; " +
				"without them the cloud reads as empty and every deployment would look accounted for")}
	}

	listed, err := runtime.Deps.RunProject.List(cmd.Context(), secretKey, orgID)
	if err != nil {
		return &CLIError{Op: "live reconcile", Code: errorCodeCommandFailed, Err: err}
	}

	store := livestore.NewFilesystemStore(runtime.LiveStoreRoot())
	deployments, unreadable, err := store.List()
	if err != nil {
		return &CLIError{Op: "live reconcile", Code: errorCodeCommandFailed, Err: err}
	}

	result := livestore.Reconcile(stampedProjects(listed), deployments)

	stages := []StageSummary{{
		Layer: "live", Stage: "reconcile", Status: StageStatusPass,
		Detail: reconcileSummary(result, len(listed)),
	}}
	var failures []FailureSummary

	for _, p := range result.Unrecorded {
		// A failure, not a note. Something is running that nothing will
		// reap, and a summary line an operator can scroll past is how
		// the D6 leak survived for weeks.
		failures = append(failures, FailureSummary{
			Layer: "live", Stage: "reconcile", Check: "unrecorded",
			Command: "live reconcile",
			Detail: fmt.Sprintf(
				"project %s (%s) carries infrafactory's stamp but no live record explains it — "+
					"it will never expire and nothing will reap it. Nothing was destroyed; inspect it and "+
					"tear it down deliberately",
				p.ProjectID, p.Name),
		})
	}

	for _, d := range result.Vanished {
		failures = append(failures, FailureSummary{
			Layer: "live", Stage: "reconcile", Check: "vanished",
			Command: "live reconcile",
			Detail: fmt.Sprintf(
				"deployment %s (scenario %s) names project %s, which the API says does not exist — "+
					"the record outlived its infrastructure and `live ls` is reporting something that is gone",
				d.ID, d.Scenario, d.ProjectID),
		})
	}

	for _, u := range unreadable {
		failures = append(failures, FailureSummary{
			Layer: "live", Stage: "reconcile", Check: "record",
			Command: "live reconcile",
			Detail: fmt.Sprintf(
				"a live record could not be read, so a project it explains would be reported as unrecorded: %v", u),
		})
	}

	status := CommandStatusSuccess
	if len(failures) > 0 {
		status = CommandStatusFailed
	}
	if err := writeCommandOutput(cmd, OutputResult{
		Command: "live reconcile", Status: status, Stages: stages, Failures: failures,
	}); err != nil {
		return err
	}
	if status == CommandStatusFailed {
		return &CLIError{Op: "live reconcile", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"the cloud and the live store disagree in %d place(s)", len(failures))}
	}
	return nil
}

// stampedProjects applies the SAME ownership test that guards teardown,
// so "infrafactory created this" has one definition rather than one here
// and another in the guard.
func stampedProjects(listed []harness.ListedProject) []livestore.StampedProject {
	out := make([]livestore.StampedProject, 0, len(listed))
	for _, l := range listed {
		out = append(out, livestore.StampedProject{
			ID:   l.ID,
			Name: l.Name,
			Ours: l.Provenance().IsInfrafactoryRunProject(),
		})
	}
	return out
}

// reconcileSummary states what was EXAMINED as well as what was found.
//
// "0 unrecorded" out of zero projects and out of forty read identically
// and mean opposite things, and the first is what a broken credential or
// a wrong organization looks like.
func reconcileSummary(r livestore.Reconciliation, projectsSeen int) string {
	base := fmt.Sprintf("examined %d project(s) in the organization and %d live record(s)",
		projectsSeen, r.Accounted+len(r.Vanished))
	if r.Clean() {
		return base + "; the cloud and the store agree"
	}
	return fmt.Sprintf("%s; %d unrecorded project(s), %d record(s) whose project is gone",
		base, len(r.Unrecorded), len(r.Vanished))
}
