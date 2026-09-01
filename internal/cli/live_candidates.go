package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/redscaresu/infrafactory/internal/feedback"
	"github.com/redscaresu/infrafactory/internal/livestore"
)

// runLiveCandidatesCommand reports the observations that have reproduced
// enough to be worth learning from (S156b).
//
// It deliberately produces **candidates and not pitfalls**. Turning one
// into a rule is S156c's job, and keeping the two apart means the gate
// can be judged on whether it promotes the right things without also
// arguing about rule text — which is the harder and much more subjective
// half.
//
// It is also the only way to see the gate's verdict before anything acts
// on it. A promotion mechanism whose output is invisible until it has
// already written to the corpus is one nobody can calibrate.
func runLiveCandidatesCommand(cmd *cobra.Command, _ []string, runtime *CommandRuntime) error {
	consecutive, err := cmd.Flags().GetInt("consecutive")
	if err != nil {
		return &CLIError{Op: "live candidates", Code: errorCodeUsage, Err: err}
	}
	distinct, err := cmd.Flags().GetInt("deployments")
	if err != nil {
		return &CLIError{Op: "live candidates", Code: errorCodeUsage, Err: err}
	}

	store := livestore.NewFilesystemStore(runtime.LiveStoreRoot())
	deployments, unreadable, err := store.List()
	if err != nil {
		return &CLIError{Op: "live candidates", Code: errorCodeCommandFailed, Err: err}
	}

	rule := livestore.PromotionRule{
		ConsecutiveProbes:   consecutive,
		DistinctDeployments: distinct,
		// The same normalizer the run loop groups failures by, so a
		// live signal and a run signal describing one problem look
		// alike rather than merely similar.
		Normalize: feedback.NormalizeDetail,
	}
	candidates := livestore.PromotionCandidates(deployments, rule)

	stages := []StageSummary{{
		Layer: "live", Stage: "candidates", Status: StageStatusPass,
		Detail: candidateSummary(candidates, rule),
	}}
	for _, c := range candidates {
		stages = append(stages, StageSummary{
			Layer: "live", Stage: "candidates", Status: StageStatusPass,
			Detail: describeCandidate(c),
		})
	}

	var failures []FailureSummary
	// Same rule `live ls` and `live observe` apply: a record that will
	// not parse may describe running infrastructure, and its
	// observations are missing from every count above.
	for _, u := range unreadable {
		stages = append(stages, StageSummary{Layer: "live", Stage: "candidates", Status: StageStatusFail})
		failures = append(failures, FailureSummary{
			Layer: "live", Stage: "candidates", Check: "record",
			Command: "live candidates",
			Detail: fmt.Sprintf(
				"a live record could not be read, so whatever it observed is missing from this verdict: %v", u),
		})
	}

	status := CommandStatusSuccess
	if len(failures) > 0 {
		status = CommandStatusFailed
	}
	if err := writeCommandOutput(cmd, OutputResult{
		Command: "live candidates", Status: status, Stages: stages, Failures: failures,
	}); err != nil {
		return err
	}
	if status == CommandStatusFailed {
		return &CLIError{Op: "live candidates", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"%d live record(s) could not be read, so this verdict is incomplete", len(failures))}
	}
	return nil
}

func candidateSummary(candidates []livestore.Candidate, rule livestore.PromotionRule) string {
	threshold := fmt.Sprintf("%d consecutive probes, or %d distinct deployments",
		rule.ConsecutiveProbes, rule.DistinctDeployments)
	if len(candidates) == 0 {
		return "nothing has reproduced (" + threshold + ")"
	}
	return fmt.Sprintf("%d observation(s) have reproduced (%s)", len(candidates), threshold)
}

// describeCandidate leads with the evidence rather than the text: what
// promoted it is what a reader has to judge.
func describeCandidate(c livestore.Candidate) string {
	attribution := "version UNCONFIRMED, so nothing may be blamed on a tag"
	if c.Attributable {
		attribution = "version confirmed"
	}
	return fmt.Sprintf("%s across %d deployment(s), longest run %d — %s; %s: %s",
		c.Reason, len(c.Deployments), c.LongestRun, attribution,
		c.Status, truncateRule(strings.TrimSpace(c.Example)))
}
