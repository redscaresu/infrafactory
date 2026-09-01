package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redscaresu/infrafactory/internal/feedback"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/livestore"
)

// learnRepairs turns upgrades that demonstrably fixed something into
// PRESCRIPTIVE corpus entries (S156d).
//
// S156c writes descriptive rules: what was observed, and nothing about
// what to do. ADR-0019 calls that the weakest useful class, and it is all
// a single live failure can support -- there is no "next iteration that
// fixed it" to diff against.
//
// An upgrade supplies one. S155b keeps the configuration a deployment was
// running into `.infrafactory-previous/` for exactly this, and
// `ExtractFixPitfall` turns a before/after HCL pair into a rule that says
// what to DO.
//
// The gate is `livestore.Repairs`, and the whole difficulty is there
// rather than here: an upgrade is a diff between two configurations that
// both applied successfully, so the apply cannot tell a fix from a
// routine version bump. The observations can.
func learnRepairs(runtime *CommandRuntime, deployments []livestore.Deployment, dryRun bool, now time.Time) (stages []StageSummary, failures []FailureSummary, written, considered int) {
	repairs := livestore.Repairs(deployments, feedback.NormalizeDetail)
	considered = len(repairs)

	for _, r := range repairs {
		d := r.Deployment
		if d.Cloud == "" {
			// The corpus is per-cloud, so a record naming none cannot
			// be filed anywhere. `live learn` already reports these on
			// the descriptive path; this path must SKIP them the same
			// way rather than reaching AppendLivePitfall, whose
			// cloud-name guard would fail the whole command over one
			// partial record. `Cloud` is optional on the schema, so
			// older records really do lack it.
			stages = append(stages, StageSummary{
				Layer: "live", Stage: "learn", Status: StageStatusSkip,
				Detail: fmt.Sprintf(
					"%s repaired a failure but names no cloud, so the remedy cannot be filed", d.ID),
			})
			continue
		}
		previous := filepath.Join(d.WorkDir, PreviousHCLDirname)
		if info, err := os.Stat(previous); err != nil || !info.IsDir() {
			// Without the stash there is no "before", and an ABSENT
			// before is not an empty one. `loadResourceBlocks` returns
			// an empty map for a missing directory, so every resource in
			// the current configuration would look newly added: with one
			// resource in the file, the extractor would emit the whole
			// body as "the fix" and the corpus would carry a rule
			// claiming the entire configuration was the remedy.
			//
			// A record can reach here without one -- it predates S155b,
			// or the working directory was cleaned since the upgrade --
			// so this is a real state, not a defensive impossibility.
			stages = append(stages, StageSummary{
				Layer: "live", Stage: "learn", Status: StageStatusSkip,
				Detail: fmt.Sprintf(
					"%s repaired a failure but the configuration it was running is no longer in %s, "+
						"so there is nothing to compare against", d.ID, PreviousHCLDirname),
			})
			continue
		}

		// Attribution is the hard part of a live repair, and the
		// record's `AddressResource` is NOT the answer.
		//
		// A run-loop failure names a resource address. A live health
		// probe says `health path http://… returned HTTP 503` and names
		// nothing. `AddressResource` is where the probe POINTED -- a
		// load balancer IP -- not where the fault was, which is
		// typically a backend block. Handing it to the extractor as a
		// hint narrows the diff to the wrong resource type and skips
		// precisely the upgrades worth learning from.
		//
		// So attribution comes from the diff itself, under the rule the
		// extractor already applies to ambiguity: use it only when
		// exactly ONE resource changed. Several means the diff cannot
		// say which cleared the failure.
		address, attributionErr := singleChangedResource(previous, d.WorkDir)
		if attributionErr != nil {
			failures = append(failures, FailureSummary{
				Layer: "live", Stage: "learn", Check: "diff",
				Command: "live learn",
				Detail: fmt.Sprintf(
					"deployment %s repaired a failure but its configurations could not be read: %v",
					d.ID, attributionErr),
			})
			continue
		}

		pitfall, err := extractRepairPitfall(
			previous, d.WorkDir, r.Example, address, d.Cloud, d.Scenario,
			now.UTC().Format(time.RFC3339))
		if err != nil {
			failures = append(failures, FailureSummary{
				Layer: "live", Stage: "learn", Check: "diff",
				Command: "live learn",
				Detail: fmt.Sprintf(
					"deployment %s repaired a failure but its configurations could not be diffed: %v", d.ID, err),
			})
			continue
		}
		if pitfall == nil {
			// The extractor found no productive diff it could attribute.
			// Said out loud rather than dropped: a repair nobody can
			// explain is a real signal this system cannot yet use, and
			// silence would make the corpus look like it had learned
			// everything available.
			//
			// S156c will still have written the DESCRIPTIVE form of the
			// same failure if it reproduced, so the observation is not
			// lost -- only the remedy is.
			stages = append(stages, StageSummary{
				Layer: "live", Stage: "learn", Status: StageStatusSkip,
				Detail: fmt.Sprintf(
					"%s: an upgrade cleared %q but %s, so no remedy can be written",
					d.ID, truncateRule(strings.TrimSpace(r.Example)), unattributableReason(address)),
			})
			continue
		}

		// Kept as `source: live`, NOT `fix`, and that is deliberate.
		//
		// ADR-0019's source tag carries two things at once: how the
		// knowledge was extracted, and how long it may live. `fix`
		// entries are permanent; `live` entries are retirable by S156a.
		// This rule is prescriptive in SHAPE and live in PROVENANCE --
		// it describes a running service, and what is true of a running
		// service stops being true. Tagging it `fix` would make it
		// immortal, breaking S156c's rule that nothing is written which
		// cannot later be retired.
		pitfall.Source = generator.LiveSource
		// Captured before the evidence wrapper goes on: the extractor's
		// rule is derived deterministically from the two configurations,
		// so it is stable identity. The wrapped text is not -- it states
		// counts that grow (pass 77).
		extracted := pitfall.Rule
		pitfall.Rule = repairRuleText(r, pitfall.Rule)
		pitfall.DiscoveredFrom = d.Scenario

		if dryRun {
			stages = append(stages, StageSummary{
				Layer: "live", Stage: "learn", Status: StageStatusSkip,
				Detail: fmt.Sprintf("--dry-run: would learn a REMEDY for %s/%s — %s",
					d.Cloud, pitfall.Resource, truncateRule(pitfall.Rule)),
			})
			continue
		}

		if err := generator.AppendLivePitfall(
			runtime.Config.Paths.Pitfalls, d.Cloud, repairKey(r, address, extracted), *pitfall, now); err != nil {
			failures = append(failures, FailureSummary{
				Layer: "live", Stage: "learn", Check: "append",
				Command: "live learn",
				Detail:  fmt.Sprintf("could not record the remedy for %s: %v", pitfall.Resource, err),
			})
			continue
		}
		written++
		stages = append(stages, StageSummary{
			Layer: "live", Stage: "learn", Status: StageStatusPass,
			Detail: fmt.Sprintf("%s/%s (remedy): %s", d.Cloud, pitfall.Resource, truncateRule(pitfall.Rule)),
		})
	}

	return stages, failures, written, considered
}

// repairKey identifies a prescriptive live entry.
//
// Deliberately distinct from a descriptive candidate's key, even for the
// same failure: "this is what went wrong" and "this is what fixed it" are
// two entries the corpus should hold at once, and keying them together
// would let the weaker one overwrite the stronger.
//
// It also carries the REMEDY, not only the symptom. Two upgrades can
// clear the same normalized `HTTP 503` on the same resource by different
// changes -- one adds a health check, another corrects a port -- and
// those are two lessons. Keyed on the symptom alone the second would
// silently refresh over the first, and the corpus would quietly hold
// whichever happened to be learned last.
//
// The remedy enters as a digest rather than in full: a snippet runs to
// hundreds of bytes of HCL and identity has to be exact, so the digest
// does the identifying while the address stays in the clear for a human
// reading the file.
func repairKey(r livestore.Repair, address, extracted string) string {
	sum := sha256.Sum256([]byte(extracted))
	return "repair\x00" + r.Deployment.Cloud + "\x00" + r.Detail +
		"\x00" + address + "\x00" + hex.EncodeToString(sum[:8])
}

// repairRuleText states the evidence alongside the remedy.
//
// The extractor's rule says what to write. It cannot say how anyone knows
// it works, and for a live-sourced rule that is the load-bearing part: a
// remedy is only a remedy because a running service was observed failing
// before it and healthy after.
func repairRuleText(r livestore.Repair, extracted string) string {
	return fmt.Sprintf(
		"%s Observed on a RUNNING deployment: this configuration change cleared %q, "+
			"which %d probe(s) had reported before the upgrade and %d probe(s) confirmed gone after it. "+
			"The apply reported success both before and after, so only the running service showed the difference.",
		strings.TrimSpace(extracted), strings.TrimSpace(r.Example),
		r.ObservationsBefore, r.ObservationsAfter)
}

// extractRepairPitfall turns the before/after pair into a rule, trying
// both prescriptive shapes.
//
// A repair is not always an addition. An upgrade that REMOVES something
// is just as much a fix, and the corpus already has a vocabulary for it:
// ADR-0019's `avoid` entries, of the shape "do NOT use X; it causes Y".
// Trying only `ExtractFixPitfall` would report a deletion-as-fix as
// unattributable, which is both wrong and misleading -- there was a
// single attributable change, it simply was not an addition.
//
// Fix first because addition is the commoner shape and its rule is more
// directly actionable. They read opposite sides of the same diff.
//
// A LIMIT worth stating rather than discovering: `ExtractAvoidPitfall`
// attributes strictly -- the removed attribute's name must appear in the
// failure detail, a rule added after a false positive in S63. Provider
// errors name the offending attribute; a health probe reporting
// `returned HTTP 503` names nothing at all. So the avoid path fires for a
// live signal only when the detail happens to carry the attribute name,
// and most live details will not. Loosening that attribution to suit the
// live path would weaken a guard the RUN LOOP depends on, which is not a
// trade this slice is entitled to make.
func extractRepairPitfall(previousDir, currentDir, detail, address, cloud, scenario, timestamp string) (*generator.LearnedPitfall, error) {
	fix, err := generator.ExtractFixPitfall(previousDir, currentDir, detail, address, cloud, scenario, timestamp)
	if err != nil {
		return nil, err
	}
	if fix != nil {
		return fix, nil
	}
	return generator.ExtractAvoidPitfall(previousDir, currentDir, detail, address, cloud, scenario, timestamp)
}

// singleChangedResource returns the one resource that differs between
// the configurations, or "" when zero or several do.
//
// Returning "" rather than an error for the ambiguous case is
// deliberate: several changed resources is a perfectly ordinary upgrade,
// not a fault. It simply carries no attributable remedy, and the caller
// reports that.
func singleChangedResource(previousDir, currentDir string) (string, error) {
	changed, err := generator.ChangedResourceAddresses(previousDir, currentDir)
	if err != nil {
		return "", err
	}
	if len(changed) != 1 {
		return "", nil
	}
	return changed[0], nil
}

// unattributableReason distinguishes the two ways a repair fails to
// produce a rule, because they call for different responses and reading
// them as one hides which happened.
func unattributableReason(address string) string {
	if address == "" {
		return "no single attributable change was found between the configurations"
	}
	return fmt.Sprintf(
		"the change to %s could not be turned into a rule (a removal whose attribute the "+
			"failure never named, most likely)", address)
}
