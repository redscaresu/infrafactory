package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/redscaresu/infrafactory/internal/livestore"
)

// runLiveObserveCommand probes every live deployment's health path once
// and records what it saw against that deployment's record (S154).
//
// This is the first signal infrafactory gets from infrastructure after
// the run that created it has finished. It deliberately does **no
// learning**: an observation is not a lesson until it is reproduced, and
// that gate is S156's. What this slice owes the loop is an honest,
// bounded record of what the service actually did.
//
// One probe per deployment per invocation, no retries. Scheduling is the
// operator's -- a cron entry, the same way `live reap` is scheduled --
// because a daemon would be a second thing to run and supervise for no
// signal a cron does not already give.
func runLiveObserveCommand(cmd *cobra.Command, _ []string, runtime *CommandRuntime) error {
	store := livestore.NewFilesystemStore(runtime.LiveStoreRoot())

	deployments, unreadable, err := store.List()
	if err != nil {
		return &CLIError{Op: "live observe", Code: errorCodeCommandFailed, Err: err}
	}

	var stages []StageSummary
	var failures []FailureSummary
	now := time.Now()

	for _, d := range deployments {
		stage, failure := observeDeployment(cmd.Context(), runtime, store, d, now)
		stages = append(stages, stage)
		if failure != nil {
			failures = append(failures, *failure)
		}
	}

	// Same rule `live ls` applies: a record that will not parse may
	// describe running infrastructure, so it cannot be observed and
	// cannot be passed over quietly either.
	for _, u := range unreadable {
		stages = append(stages, StageSummary{Layer: "live", Stage: "observe", Status: StageStatusFail})
		failures = append(failures, FailureSummary{
			Layer: "live", Stage: "observe", Check: "record",
			Command: "live observe",
			Detail: fmt.Sprintf(
				"a live record could not be read, so whatever it describes went unobserved: %v", u),
		})
	}

	status := CommandStatusSuccess
	if len(failures) > 0 {
		status = CommandStatusFailed
	}
	if err := writeCommandOutput(cmd, OutputResult{
		Command:  "live observe",
		Status:   status,
		Stages:   stages,
		Failures: failures,
	}); err != nil {
		return err
	}

	if status == CommandStatusFailed {
		return &CLIError{Op: "live observe", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"%d live deployment(s) are not serving", len(failures))}
	}
	return nil
}

// observeDeployment probes one deployment and persists the result.
//
// Returns exactly one stage, and a failure when the service is not
// serving. An unhealthy service is reported rather than swallowed: the
// whole point of observing is that somebody finds out.
func observeDeployment(
	ctx context.Context,
	runtime *CommandRuntime,
	store *livestore.FilesystemStore,
	d livestore.Deployment,
	now time.Time,
) (StageSummary, *FailureSummary) {
	skip := func(detail string) (StageSummary, *FailureSummary) {
		return StageSummary{
			Layer: "live", Stage: "observe", Status: StageStatusSkip,
			Detail: fmt.Sprintf("%s: %s", d.ID, detail),
		}, nil
	}
	fail := func(check, detail string) (StageSummary, *FailureSummary) {
		return StageSummary{Layer: "live", Stage: "observe", Status: StageStatusFail},
			&FailureSummary{
				Layer: "live", Stage: "observe", Check: check,
				Command: "live observe " + d.ID,
				Detail:  detail,
			}
	}

	// A released deployment is gone by definition; probing its old
	// address would attribute whatever now answers there to a service
	// that no longer exists.
	if d.State == livestore.StateReleased {
		return skip("released")
	}
	if d.Undecodable {
		return skip("record could not be decoded")
	}

	// Not skipped for being expired. An expired deployment that is still
	// answering is a fact worth recording -- it means the reaper has not
	// run, which is exactly the kind of thing this command exists to
	// surface.
	if d.Address == "" || d.Port == 0 {
		return skip("no address and port recorded, so there is nothing to probe. " +
			"Deployments created before S154 carry neither")
	}

	if runtime.Deps.ServiceProbe == nil {
		return fail("dependency", fmt.Sprintf(
			"%s could not be observed: no service probe is configured", d.ID))
	}

	result, err := runtime.Deps.ServiceProbe.Probe(ctx, d.Address, d.Port, d.HealthPath)
	if err != nil {
		// The probe could not run, which is not the same as the service
		// being down and must not be recorded as if it were.
		return fail("probe", fmt.Sprintf("%s could not be probed: %v", d.ID, err))
	}

	observation := livestore.Observation{At: now, Detail: result.Detail}
	switch {
	case result.Healthy:
		observation.Status = livestore.ObservationHealthy
	case result.Reachable:
		observation.Status = livestore.ObservationUnhealthy
	default:
		observation.Status = livestore.ObservationUnreachable
	}

	d.RecordObservation(observation)
	if err := store.Put(d); err != nil {
		// The probe ran and the answer is being lost. Reported rather
		// than dropped, because a silently unrecorded observation is
		// indistinguishable from one that never happened -- and S156's
		// reproduction gate counts observations.
		return fail("record", fmt.Sprintf(
			"%s was observed as %s but the result could not be recorded: %v",
			d.ID, observation.Status, err))
	}

	if observation.Healthy() {
		return StageSummary{
			Layer: "live", Stage: "observe", Status: StageStatusPass,
			Detail: fmt.Sprintf("%s is serving (%s)", d.ID, d.Address),
		}, nil
	}

	return fail(string(observation.Status), fmt.Sprintf("%s: %s", d.ID, observation.Detail))
}
