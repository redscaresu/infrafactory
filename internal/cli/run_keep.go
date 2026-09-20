package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

// assertKeepable refuses a --keep the run could not honour.
//
// Called before generation on purpose: every one of these is knowable
// from the flags and the scenario file, and discovering them at the end
// means the operator has paid for an LLM loop and a real apply to be
// told the keep was never possible -- at which point the run destroys
// the stack, the exact opposite of the request.
func assertKeepable(keep bool, runtime *CommandRuntime, sc scenario.Scenario) error {
	if !keep {
		return nil
	}

	usage := func(format string, args ...any) error {
		return &CLIError{Op: "run", Code: errorCodeUsage, Err: fmt.Errorf(format, args...)}
	}

	// Keeping a mock is meaningless: mockway holds no resources, costs
	// nothing and is reset by the next run regardless.
	if !runtime.Config.Validation.Layers.SandboxDeploy.Enabled {
		return usage(
			"--keep leaves real infrastructure running and needs validation.layers.sandbox_deploy.enabled; " +
				"without it the run only touches the mock, which there is nothing to keep")
	}

	// A keep with no deadline is a leak with a nicer name. ADR-0024
	// makes the TTL mandatory for exactly this reason, and service.ttl
	// is where it lives.
	if sc.Service == nil {
		return usage(
			"scenario %q declares no service: block, so a kept stack would have no TTL and nothing would ever "+
				"reap it. Add service.ttl, or run without --keep", sc.Name)
	}
	if _, err := sc.Service.TimeToLive(); err != nil {
		return usage("scenario %q has an unusable service.ttl, so a kept stack could not be given a deadline: %w",
			sc.Name, err)
	}

	return nil
}

// registerKeptRun turns what a --keep run left running into a live
// deployment: a TTL, a row on the estate page, and `live teardown`.
//
// Without the record this would be indistinguishable from the leak the
// stray-project check exists to catch -- a real project, really
// billing, that nothing knows about.
func registerKeptRun(runtime *CommandRuntime, sc scenario.Scenario) ([]StageSummary, []FailureSummary) {
	ttl, err := sc.Service.TimeToLive()
	if err != nil {
		// assertKeepable already parsed this before the run started, so
		// reaching here means the scenario changed underneath us.
		return keepFailed(runtime, "", fmt.Sprintf("service.ttl became unusable mid-run: %v", err))
	}

	store := livestore.NewFilesystemStore(runtime.LiveStoreRoot())
	deploymentID := newDeploymentID(sc.Name, time.Now())

	marker, markerErr := harness.ReadRunProjectMarker(runtime.OutputDir())
	runProjectID := ""
	if markerErr == nil {
		runProjectID = marker.ProjectID
	}

	// The run's output dir is reused by the NEXT run of this scenario,
	// which regenerates the HCL and overwrites the state in place. The
	// state is the only thing that can destroy these resources, so the
	// deployment gets its own copy -- the same per-deployment workdir
	// `deploy` uses, and the reason teardown can be run days later.
	//
	// Whole tree, not just the HCL that copyDeploySource takes: this
	// state is already applied, so the copy has to carry the state, the
	// run-project marker and the initialised .terraform that `tofu
	// destroy` needs, because nothing re-runs init before a teardown.
	workDir := deploymentWorkDir(store.Root, deploymentID)
	copyErr := os.CopyFS(workDir, os.DirFS(runtime.OutputDir()))
	if copyErr != nil {
		// Registered anyway, against the run's own directory. A record
		// whose state the next run may overwrite is still far better
		// than no record: the project id alone is enough for teardown
		// to delete the project and sweep the account.
		workDir = runtime.OutputDir()
	}

	stages, failures := registerDeployment(store, sc, deploymentID, workDir, runProjectID, ttl)
	if len(failures) > 0 {
		return stages, failures
	}

	if copyErr != nil {
		stages = append(stages, StageSummary{Layer: "live", Stage: "keep_workdir", Status: StageStatusFail})
		failures = append(failures, FailureSummary{
			Layer: "live", Stage: "keep_workdir", Check: "isolated_state",
			Command: "live teardown " + deploymentID,
			Detail: fmt.Sprintf(
				"%s is kept and tracked, but its state could not be copied out of %s: %v. The next run of %s "+
					"overwrites that state, so tear this down BEFORE running the scenario again",
				deploymentID, runtime.OutputDir(), copyErr, sc.Name),
		})
		return stages, failures
	}

	stages = append(stages, StageSummary{
		Layer: "live", Stage: "keep", Status: StageStatusPass,
		Detail: fmt.Sprintf("kept as %s; expires in %s, or `infrafactory live teardown %s`",
			deploymentID, ttl, deploymentID),
	})
	return stages, failures
}

// keepFailed reports a stack that is running and NOT tracked, which is
// the one outcome --keep must never produce quietly.
func keepFailed(runtime *CommandRuntime, deploymentID, detail string) ([]StageSummary, []FailureSummary) {
	projectID := "unknown"
	if marker, err := harness.ReadRunProjectMarker(runtime.OutputDir()); err == nil {
		projectID = marker.ProjectID
	}
	return []StageSummary{{Layer: "live", Stage: "keep", Status: StageStatusFail}},
		[]FailureSummary{{
			Layer: "live", Stage: "keep", Check: "registered", Command: "run --keep",
			Detail: fmt.Sprintf(
				"%s. The stack in project %s is still running and nothing tracks it: destroy it by hand",
				detail, projectID),
		}}
}
