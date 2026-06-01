package generator

// N8 — policy_pitfall_conflict detection.
//
// The auto-learning loop writes pitfalls for LLM-actionable failures
// (Unsupported argument, missing required field, etc) and routes
// mock-server gaps to docs/mock-gaps.md (T12 channel). There's a
// THIRD class of failure the loop has previously been blind to:
// the LLM's HCL is *correct* (matches an existing prescriptive
// pitfall verbatim) but a policy gate STILL rejects it. That's a
// policy bug, not an LLM bug — and the system can't fix it from
// inside the auto-learning loop because pitfalls only address LLM
// behaviour.
//
// Motivating case: 2026-06-01 deterministic sweep, web-app-paris +
// compute-lb-multi-paris. The LLM produced count-based
// scaleway_instance_server + matching count-based
// scaleway_instance_private_nic (exactly as the prescriptive
// scaleway_instance_server pitfall recommended), but
// policies/scaleway/vpc_required.rego compared planned addresses
// literally against symbolic refs and falsely flagged every
// count-based server as un-NIC-attached. Two iterations of identical
// HCL → stuck termination → no actionable signal.
//
// This file lands DetectPolicyConflict + AppendPolicyGap: when an
// LLM's HCL contains all the keywords from a same-resource
// prescriptive pitfall AND the same policy fires twice, route the
// failure to docs/policy-gaps.md instead of polluting
// pitfalls/<cloud>.yaml with a pitfall the LLM can't act on.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PolicyGap is a structured entry written to docs/policy-gaps.md.
// Dedup-keyed by (cloud, policy, resource).
type PolicyGap struct {
	Cloud     string // "aws" | "gcp" | "scaleway"
	Policy    string // e.g. "scaleway.vpc_required"
	Resource  string // bare resource type, e.g. "scaleway_instance_server"
	Scenario  string
	Detail    string // first ~240 chars of the failure detail
	Timestamp string // run ID
}

// policyNameRe captures the package name from policy-violation
// failure details. Formats observed:
//
//	"... check=policy policy=scaleway.vpc_required detail=..."
//	"scaleway_instance_server.web[0] is not attached..."
//	"google_storage_bucket.app_assets has no encryption.default_kms_key_name"
//
// The first shape is the structured one (FailureSummary.Detail
// composition); the others are raw rego messages. The first shape
// is what stuck-detection has to work with (the FailureSummary
// already strings them together).
var policyNameRe = regexp.MustCompile(`policy=([a-z][a-z0-9_]*\.[a-z][a-z0-9_]*)`)

// resourceAddressRe finds a resource address in the rego deny
// message (e.g. "scaleway_instance_server.web[0]"). Strips the
// instance name and index, leaving the bare type.
var resourceAddressRe = regexp.MustCompile(`((?:scaleway|google|aws)_[a-z_]+)\.[a-z_]+(?:\[\d+\])?`)

// backtickedIdentRe extracts `backticked_identifier` tokens from a
// pitfall rule. These are the keywords we expect the LLM's HCL to
// contain — resource types, attribute names, function names. The
// rule's narrative prose (e.g. "Always declare a") doesn't appear
// in HCL, so backticked terms are the load-bearing signal.
var backtickedIdentRe = regexp.MustCompile("`([a-zA-Z_][a-zA-Z0-9_]+)`")

// DetectPolicyConflict reports a PolicyGap when the LLM's HCL
// matches all the backticked keywords of any same-resource
// prescriptive pitfall AND the failure is a policy violation. The
// caller should already have observed the failure recurring twice
// (stuck-detection precondition); this function only confirms the
// "matches an existing pitfall" half.
//
// Returns nil if:
//   - failureDetail isn't a policy violation (no policy= or no
//     recognised rego shape).
//   - No same-resource pitfall exists in the provided list.
//   - The LLM's HCL doesn't contain the pitfall's keywords (the LLM
//     IS still making a mistake — let the existing pitfall path
//     handle it).
//
// Conservative by design: false negatives leave the existing
// pitfall path active; false positives surface a likely policy bug
// for human review. We bias toward false negatives.
func DetectPolicyConflict(failureDetail, hcl string, pitfalls []PitfallEntry, cloud, scenario, timestamp string) *PolicyGap {
	policy := extractPolicyName(failureDetail)
	resource := extractRegoResource(failureDetail)
	if policy == "" || resource == "" {
		return nil
	}
	// Find same-resource prescriptive pitfalls.
	for _, p := range pitfalls {
		if p.Resource != resource {
			continue
		}
		keywords := extractKeywords(p.Rule)
		if len(keywords) == 0 {
			continue
		}
		if hclContainsAllKeywords(hcl, keywords) {
			return &PolicyGap{
				Cloud:     cloud,
				Policy:    policy,
				Resource:  resource,
				Scenario:  scenario,
				Detail:    failureDetail,
				Timestamp: timestamp,
			}
		}
	}
	return nil
}

func extractPolicyName(detail string) string {
	if m := policyNameRe.FindStringSubmatch(detail); len(m) >= 2 {
		return m[1]
	}
	return ""
}

func extractRegoResource(detail string) string {
	if m := resourceAddressRe.FindStringSubmatch(detail); len(m) >= 2 {
		return m[1]
	}
	return ""
}

// extractKeywords pulls backticked identifiers from a pitfall rule.
// Skips short tokens (≤3 chars) and common narrative words. The
// result is the set of HCL-relevant identifiers the rule prescribes.
func extractKeywords(rule string) []string {
	matches := backtickedIdentRe.FindAllStringSubmatch(rule, -1)
	seen := map[string]bool{}
	var out []string
	skipWords := map[string]bool{
		"count": true, "for": true, "and": true, "the": true, "NOT": true, "not": true,
	}
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		ident := m[1]
		if len(ident) <= 3 || skipWords[ident] || seen[ident] {
			continue
		}
		seen[ident] = true
		out = append(out, ident)
	}
	return out
}

// hclContainsAllKeywords returns true if every keyword appears
// somewhere in the HCL. Simple substring match — false positives
// occur when keywords are unrelated identifiers; we accept that risk
// because the detection is gated on policy=X firing twice for the
// same resource, which is already a strong signal of policy-vs-
// pitfall disagreement.
func hclContainsAllKeywords(hcl string, keywords []string) bool {
	if len(keywords) == 0 {
		return false
	}
	for _, kw := range keywords {
		if !strings.Contains(hcl, kw) {
			return false
		}
	}
	return true
}

// AppendPolicyGap writes a structured policy-gap entry to
// docs/policy-gaps.md inside the given docs directory. Mirrors
// AppendMockGap's file format (per-cloud table, dedup-keyed by
// (cloud, policy, resource)).
func AppendPolicyGap(docsDir string, gap PolicyGap) error {
	if docsDir == "" {
		return fmt.Errorf("docs dir is required")
	}
	if gap.Cloud == "" || gap.Policy == "" || gap.Resource == "" {
		return fmt.Errorf("cloud, policy, and resource are required")
	}
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		return fmt.Errorf("mkdir docs dir: %w", err)
	}
	path := filepath.Join(docsDir, "policy-gaps.md")

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read policy-gaps: %w", err)
	}
	content := string(existing)

	// Dedup-key combines policy + resource so the same conflict
	// detected on multiple sweeps doesn't grow the file.
	dedupKey := fmt.Sprintf("| `%s` | `%s` |", gap.Policy, gap.Resource)
	if strings.Contains(content, dedupKey) {
		return nil
	}

	if content == "" {
		content = "# Policy gaps\n\n" +
			"This file collects cases where the auto-learning pipeline\n" +
			"detected a **policy_pitfall_conflict**: the LLM's HCL matched\n" +
			"all the keywords of an existing prescriptive pitfall, but a\n" +
			"policy gate (Rego deny rule) still fired. That's a policy\n" +
			"bug, not an LLM bug — the policy disagrees with its own\n" +
			"pitfall.\n\n" +
			"Entries belong to `policies/<cloud>/*.rego`, NOT to\n" +
			"`pitfalls/<cloud>.yaml`. See N8 in `docs/NEXT_SESSION.md`\n" +
			"and `internal/generator/policy_gap.go::DetectPolicyConflict`\n" +
			"for the detection mechanism.\n\n"
	}

	heading := fmt.Sprintf("## %s\n\n", gap.Cloud)
	if !strings.Contains(content, heading) {
		content += heading +
			"| Policy | Resource | Scenario | Detail | First seen |\n" +
			"|---|---|---|---|---|\n"
	}

	detail := strings.TrimSpace(gap.Detail)
	if len(detail) > 240 {
		detail = detail[:237] + "..."
	}
	detail = strings.ReplaceAll(detail, "|", "\\|")
	detail = strings.ReplaceAll(detail, "\n", " ")

	row := fmt.Sprintf("| `%s` | `%s` | %s | %s | %s |\n",
		gap.Policy, gap.Resource, gap.Scenario, detail, gap.Timestamp)

	idx := strings.Index(content, heading)
	sectionStart := idx + len(heading)
	nextHeading := strings.Index(content[sectionStart:], "## ")
	if nextHeading < 0 {
		content += row
	} else {
		insertAt := sectionStart + nextHeading
		content = content[:insertAt] + row + "\n" + content[insertAt:]
	}

	return os.WriteFile(path, []byte(content), 0o644)
}
