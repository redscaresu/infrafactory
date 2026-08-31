package cli

import (
	"context"
	"fmt"
	"strings"
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
		// "not serving" would be a lie for the version findings: a
		// deployment can answer perfectly and still be running something
		// other than what the record claims, which is the more dangerous
		// of the two because it looks fine. The summary must not
		// contradict the failure beneath it.
		return &CLIError{Op: "live observe", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"%d live deployment(s) did not observe clean", len(failures))}
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
	// Skipped here and FAILED by the unreadable loop in the caller --
	// List returns an undecodable record in both slices. The skip says
	// this deployment was not probed; the failure says why, and it is
	// what makes the command exit non-zero. Neither alone would do both.
	if d.Undecodable {
		return skip("record could not be decoded; reported as a failure below")
	}

	// Not skipped for being expired. An expired deployment that is still
	// answering is a fact worth recording -- it means the reaper has not
	// run, which is exactly the kind of thing this command exists to
	// surface.

	// A LIVE deployment that cannot be probed is a failure, not a skip.
	//
	// Skipping read as "nothing to see here" and exited zero, which is
	// the false green this project refuses everywhere else: the record
	// says something is running and this command just said it could not
	// tell. Two ways to get here and both matter -- `registerDeployment`
	// captures the address best-effort, so an apply that never produced a
	// load balancer address leaves a live deployment nobody can monitor;
	// and a record written before S154 carries no port at all.
	if missing := missingProbeTarget(d); missing != "" {
		return fail("target", fmt.Sprintf(
			"%s is live but cannot be observed: %s. Its project %s may be running and unmonitored -- "+
				"tear it down with `infrafactory live teardown %s`, or if it is already gone clear the "+
				"record with `infrafactory live forget %s`",
			d.ID, missing, d.ProjectID, d.ID, d.ID))
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

	// The version check is separate from health on purpose: a service can
	// be perfectly healthy and running something other than what the
	// record claims, which is the more dangerous of the two because it
	// looks fine.
	versionDetail := ""
	if d.VersionPath != "" {
		observation.Version, versionDetail = checkRunningVersion(ctx, runtime, d)
	}

	switch {
	case result.Healthy:
		observation.Status = livestore.ObservationHealthy
	case result.Reachable:
		observation.Status = livestore.ObservationUnhealthy
	default:
		observation.Status = livestore.ObservationUnreachable
	}

	// Re-read before writing. `live observe` is the command most likely
	// to be on a cron, so it is the one most likely to be mid-probe when
	// an operator runs `live teardown` -- and a read-modify-write over a
	// probe that took seconds would put `state: live` back over a record
	// teardown had just released, resurrecting a deployment that is
	// already gone.
	//
	// This narrows the window rather than closing it; the store has no
	// compare-and-swap. Narrowing it to the microseconds around one write
	// is worth doing, and claiming more than that would be wrong.
	fresh, err := store.Get(d.ID)
	if err != nil {
		return fail("record", fmt.Sprintf(
			"%s was observed as %s but the record could not be re-read to save it: %v",
			d.ID, observation.Status, err))
	}
	if fresh.State == livestore.StateReleased {
		return skip("released while it was being probed")
	}

	fresh.RecordObservation(observation)
	if err := store.Put(fresh); err != nil {
		// The probe ran and the answer is being lost. Reported rather
		// than dropped, because a silently unrecorded observation is
		// indistinguishable from one that never happened -- and S156's
		// reproduction gate counts observations.
		return fail("record", fmt.Sprintf(
			"%s was observed as %s but the result could not be recorded: %v",
			d.ID, observation.Status, err))
	}

	// A record that misstates what is running is a finding even when the
	// service is up, and it is reported BEFORE health so it cannot be
	// hidden behind a green probe.
	if observation.Version == livestore.VersionUnconfirmed {
		return fail("version", fmt.Sprintf("%s: %s", d.ID, versionDetail))
	}

	if observation.Healthy() {
		detail := fmt.Sprintf("%s is serving (%s)", d.ID, d.Address)
		switch observation.Version {
		case livestore.VersionConfirmed:
			detail += fmt.Sprintf(" and confirms %s", imageRef(d))
		case livestore.VersionUnchecked:
			// Said out loud. Silence here would read as confirmation,
			// and the record's version is a claim nobody checked.
			detail += "; running version unchecked (no version_path declared)"
		}
		return StageSummary{
			Layer: "live", Stage: "observe", Status: StageStatusPass, Detail: detail,
		}, nil
	}

	return fail(string(observation.Status), fmt.Sprintf("%s: %s", d.ID, observation.Detail))
}

// checkRunningVersion asks the service what it is running and compares it
// with what the record claims.
//
// The comparison is deliberately weak: the response must MENTION the tag.
// That verifies a cooperating service and cannot verify an uncooperative
// one -- so a failure to probe is unchecked, never unconfirmed. Claiming
// a contradiction on a probe that did not happen would be the same
// falsehood in the other direction.
func checkRunningVersion(ctx context.Context, runtime *CommandRuntime, d livestore.Deployment) (livestore.VersionCheck, string) {
	result, err := runtime.Deps.ServiceProbe.Probe(ctx, d.Address, d.Port, d.VersionPath)
	if err != nil || !result.Reachable {
		return livestore.VersionUnchecked, ""
	}
	if d.Tag == "" {
		return livestore.VersionUnchecked, ""
	}

	if strings.Contains(result.Body, d.Tag) {
		return livestore.VersionConfirmed, ""
	}
	return livestore.VersionUnconfirmed, fmt.Sprintf(
		"the record claims %s but %s does not mention %q. The record states intent, not fact -- "+
			"an upgrade to a version nobody confirmed is running proves nothing",
		imageRef(d), d.VersionPath, d.Tag)
}

// missingProbeTarget names what the record lacks, or "" when it can be
// probed. Named rather than inlined so the message says WHICH half is
// missing: "no address" and "no port" have different causes and
// different fixes.
func missingProbeTarget(d livestore.Deployment) string {
	switch {
	case d.Address == "" && d.Port == 0:
		return "it records neither an address nor a port"
	case d.Address == "":
		return "it records no address, so the apply never produced one to monitor"
	case d.Port == 0:
		return "it records no service port (records written before S154 carry none)"
	}
	return ""
}
