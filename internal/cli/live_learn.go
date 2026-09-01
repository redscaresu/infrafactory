package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/redscaresu/infrafactory/internal/feedback"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/livestore"
)

// runLiveLearnCommand writes reproduced observations into the corpus as
// `source: live` pitfalls (S156c).
//
// This is the step that closes the loop, and the one place in the arc
// where the system writes something it will later act on. Two properties
// follow from that, and both are refusals rather than features:
//
//   - **Nothing is written without a resource.** The corpus is keyed by
//     resource and the generator steers on it, but a live observation
//     names none: "the thing at this address returned 503" is about an
//     endpoint. `ExtractDescriptivePitfall` already refuses to invent one
//     ("skip rather than fabricate"), and this refuses for the same
//     reason — using the resource the address was RESOLVED from, which is
//     a fact recorded at deploy time, or writing nothing.
//   - **Nothing is written that cannot later be retired.** Every entry
//     carries `last_seen`, so S156a's retirement can act on it. An entry
//     the corpus cannot shed is one that steers generation forever.
func runLiveLearnCommand(cmd *cobra.Command, _ []string, runtime *CommandRuntime) error {
	consecutive, err := cmd.Flags().GetInt("consecutive")
	if err != nil {
		return &CLIError{Op: "live learn", Code: errorCodeUsage, Err: err}
	}
	distinct, err := cmd.Flags().GetInt("deployments")
	if err != nil {
		return &CLIError{Op: "live learn", Code: errorCodeUsage, Err: err}
	}
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return &CLIError{Op: "live learn", Code: errorCodeUsage, Err: err}
	}

	store := livestore.NewFilesystemStore(runtime.LiveStoreRoot())
	deployments, unreadable, err := store.List()
	if err != nil {
		return &CLIError{Op: "live learn", Code: errorCodeCommandFailed, Err: err}
	}

	candidates := livestore.PromotionCandidates(deployments, livestore.PromotionRule{
		ConsecutiveProbes:   consecutive,
		DistinctDeployments: distinct,
		Normalize:           feedback.NormalizeDetail,
	})

	resources := addressResources(deployments)
	clouds := deploymentClouds(deployments)
	now := time.Now()

	var stages []StageSummary
	var failures []FailureSummary
	written := 0

	for _, c := range candidates {
		resource := resourceForCandidate(c, resources)
		if resource == "" {
			// Said out loud rather than dropped. A reproduced failure
			// nobody can attribute is a real signal that this system
			// cannot yet use, and hiding that would make the corpus look
			// like it had learned everything available.
			stages = append(stages, StageSummary{
				Layer: "live", Stage: "learn", Status: StageStatusSkip,
				Detail: fmt.Sprintf(
					"reproduced across %d deployment(s) but no resource can be attributed, so nothing is written: %s",
					len(c.Deployments), truncateRule(strings.TrimSpace(c.Example))),
			})
			continue
		}

		cloud := agreedValue(c.Deployments, clouds)
		if cloud == "" {
			// The corpus is per-cloud. A candidate spanning two of them
			// is not one lesson, and picking either would file it where
			// half its evidence does not apply.
			stages = append(stages, StageSummary{
				Layer: "live", Stage: "learn", Status: StageStatusSkip,
				Detail: fmt.Sprintf(
					"reproduced across %d deployment(s) that do not agree on a cloud, so nothing is written",
					len(c.Deployments)),
			})
			continue
		}

		rule := liveRuleText(c)
		if dryRun {
			stages = append(stages, StageSummary{
				Layer: "live", Stage: "learn", Status: StageStatusSkip,
				Detail: fmt.Sprintf("--dry-run: would learn for %s — %s", resource, truncateRule(rule)),
			})
			continue
		}

		pitfall := generator.LearnedPitfall{
			Resource:       resource,
			Rule:           rule,
			Source:         generator.LiveSource,
			DiscoveredFrom: strings.Join(c.Scenarios, ", "),
		}
		// c.Key() is the gate's OWN identity: status, drift and the
		// normalized detail. Persisting anything narrower would collapse
		// distinctions the gate had just been careful to preserve --
		// `unhealthy` and `unreachable` with the same words are two
		// reproduced failures, and one must not overwrite the other.
		if err := generator.AppendLivePitfall(runtime.Config.Paths.Pitfalls, cloud, c.Key(), pitfall, now); err != nil {
			failures = append(failures, FailureSummary{
				Layer: "live", Stage: "learn", Check: "append",
				Command: "live learn",
				Detail:  fmt.Sprintf("could not record the lesson for %s: %v", resource, err),
			})
			continue
		}
		written++
		stages = append(stages, StageSummary{
			Layer: "live", Stage: "learn", Status: StageStatusPass,
			Detail: fmt.Sprintf("%s: %s", resource, truncateRule(rule)),
		})
	}

	for _, u := range unreadable {
		failures = append(failures, FailureSummary{
			Layer: "live", Stage: "learn", Check: "record",
			Command: "live learn",
			Detail: fmt.Sprintf(
				"a live record could not be read, so whatever it observed was not considered: %v", u),
		})
	}

	stages = append([]StageSummary{{
		Layer: "live", Stage: "learn", Status: StageStatusPass,
		Detail: learnSummary(len(candidates), written, dryRun),
	}}, stages...)

	status := CommandStatusSuccess
	if len(failures) > 0 {
		status = CommandStatusFailed
	}
	if err := writeCommandOutput(cmd, OutputResult{
		Command: "live learn", Status: status, Stages: stages, Failures: failures,
	}); err != nil {
		return err
	}
	if status == CommandStatusFailed {
		return &CLIError{Op: "live learn", Code: errorCodeCommandFailed, Err: fmt.Errorf(
			"%d lesson(s) could not be recorded", len(failures))}
	}
	return nil
}

func learnSummary(candidates, written int, dryRun bool) string {
	if candidates == 0 {
		return "nothing has reproduced, so there is nothing to learn"
	}
	if dryRun {
		return fmt.Sprintf("--dry-run: %d candidate(s) considered, nothing written", candidates)
	}
	return fmt.Sprintf("%d candidate(s) considered, %d written as source: live", candidates, written)
}

// addressResources maps deployment id to the resource its probed address
// came from.
func addressResources(deployments []livestore.Deployment) map[string]string {
	out := map[string]string{}
	for _, d := range deployments {
		if d.AddressResource != "" {
			out[d.ID] = d.AddressResource
		}
	}
	return out
}

// resourceForCandidate returns the resource every exhibiting deployment
// agrees on, or "" when they do not.
//
// Unanimity is the bar on purpose. If one deployment served from an
// `scaleway_lb_ip` and another from an `scaleway_instance_ip`, the
// failure they share is not a fact about either — attributing it to
// whichever happened to be first would be a guess dressed as a finding.
func resourceForCandidate(c livestore.Candidate, byDeployment map[string]string) string {
	return agreedValue(c.Deployments, byDeployment)
}

// agreedValue returns the value every id maps to, or "" if any is
// missing or they disagree.
func agreedValue(ids []string, byID map[string]string) string {
	agreed := ""
	for _, id := range ids {
		v := byID[id]
		if v == "" {
			return ""
		}
		if agreed == "" {
			agreed = v
			continue
		}
		if agreed != v {
			return ""
		}
	}
	return agreed
}

// deploymentClouds maps deployment id to the cloud it was applied to.
func deploymentClouds(deployments []livestore.Deployment) map[string]string {
	out := map[string]string{}
	for _, d := range deployments {
		if d.Cloud != "" {
			out[d.ID] = d.Cloud
		}
	}
	return out
}

// liveRuleText phrases what was observed for a reader who will meet it
// as generation guidance, months later, with none of this context.
//
// It says what happened and what the evidence was, and stops there. It
// does NOT say what to do: a descriptive rule that invents a remedy is
// worse than one that admits it has none, and the prescriptive form
// comes from an upgrade diff in S156d.
func liveRuleText(c livestore.Candidate) string {
	evidence := fmt.Sprintf("%s, across %d deployment(s), longest run %d",
		c.Reason, len(c.Deployments), c.LongestRun)

	if c.VersionDrift {
		return fmt.Sprintf(
			"Observed on a RUNNING deployment: the service answered normally while serving a version "+
				"other than the one deployed (%s). An apply reaching its desired state does not mean the "+
				"service restarted or picked up the new configuration. Evidence: %s.",
			strings.TrimSpace(c.Example), evidence)
	}

	attribution := ""
	if !c.Attributable {
		attribution = " The running version was never confirmed, so nothing here may be attributed to a particular image tag."
	}
	return fmt.Sprintf(
		"Observed on a RUNNING deployment, after the apply reported success: %s. Evidence: %s.%s",
		strings.TrimSpace(c.Example), evidence, attribution)
}
