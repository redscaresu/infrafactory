package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// LearnedPitfall represents a pitfall discovered from run feedback.
type LearnedPitfall struct {
	Resource       string
	Rule           string
	DiscoveredFrom string // scenario name
}

var (
	// Matches Terraform resource names like scaleway_redis_cluster or
	// google_compute_instance. Without the google_ branch, GCP-side
	// auto-learning never extracts a resource and (after the cross-cloud
	// guard added in the run loop) silently produces zero pitfalls.
	// M92: aws_* added so AWS scenarios can auto-learn. Prior regex
	// matched only scaleway_*/google_* — every AWS run that hit
	// stuck-detection failed to extract a resource and silently
	// dropped the lesson. M88's sweep showed 11/11 AWS scenarios
	// failed without growing pitfalls/aws.yaml; M86+M90 fixes alone
	// weren't enough because the regex never matched.
	resourceNameRe = regexp.MustCompile(`((?:scaleway|google|aws)_\w+)`)

	// Matches "Unsupported argument" errors with argument name in quotes.
	unsupportedArgRe = regexp.MustCompile(`Unsupported argument.*"(\w+)"`)

	// Matches "at least one of" constraint errors.
	atLeastOneOfRe = regexp.MustCompile(`at least one of\s+(.+?)(?:\s+must|\s+should|\s*\()`)

	// Generic errors that are not learnable.
	genericPatterns = []string{
		"test checks failed",
		"validation failed",
		"exit status",
		"context deadline exceeded",
		"resource with ID",
		"resource not found",
		"command failed",
		"connection refused",
		"timeout",
	}
)

// ExtractLearnedPitfall analyzes a failure detail string and extracts
// a pitfall rule if the error is specific enough to be useful.
// Returns nil if the error is too vague (e.g., "test checks failed").
func ExtractLearnedPitfall(failureDetail, scenarioName string) *LearnedPitfall {
	if failureDetail == "" {
		return nil
	}

	lower := strings.ToLower(failureDetail)

	// Check specific patterns BEFORE generic rejection, since specific
	// errors may be embedded inside generic wrappers like "exit status 1 | stderr: ..."

	// Try specific pattern: password constraint.
	if strings.Contains(lower, "password does not respect constraint") ||
		strings.Contains(lower, "password") && strings.Contains(lower, "complexity") {
		// Only fall back to the Scaleway-flavoured default when the
		// failure detail names no resource at all. This used to be a
		// blind `scaleway_redis_cluster` default that, after the
		// cross-cloud guard in run_command.go, silently dropped the
		// learning on GCP. Now: extract whichever cloud's resource is
		// in the detail; otherwise skip rather than fabricate.
		resource := extractResource(failureDetail)
		if resource == "" {
			return nil
		}
		return &LearnedPitfall{
			Resource:       resource,
			Rule:           "The password must meet the provider's complexity requirements (minimum length, uppercase, lowercase, digit, and special character).",
			DiscoveredFrom: scenarioName,
		}
	}

	// Try specific pattern: K8s version / auto_upgrade mismatch.
	if strings.Contains(lower, "minor version") && strings.Contains(lower, "auto upgrade") ||
		strings.Contains(lower, "auto_upgrade") && strings.Contains(lower, "version") {
		resource := extractResource(failureDetail)
		if resource == "" {
			return nil
		}
		return &LearnedPitfall{
			Resource:       resource,
			Rule:           "Version and auto_upgrade MUST be consistent: WITHOUT auto_upgrade use a full patch version like \"1.31.2\"; WITH auto_upgrade enabled use ONLY a minor version like \"1.31\".",
			DiscoveredFrom: scenarioName,
		}
	}

	// Try specific pattern: Unsupported argument.
	if matches := unsupportedArgRe.FindStringSubmatch(failureDetail); len(matches) >= 2 {
		resource := extractResource(failureDetail)
		argName := matches[1]
		if resource != "" {
			return &LearnedPitfall{
				Resource:       resource,
				Rule:           fmt.Sprintf("The argument %q is not supported. Do not use it.", argName),
				DiscoveredFrom: scenarioName,
			}
		}
	}

	// Try specific pattern: "at least one of".
	if matches := atLeastOneOfRe.FindStringSubmatch(failureDetail); len(matches) >= 2 {
		resource := extractResource(failureDetail)
		constraint := strings.TrimSpace(matches[1])
		if resource != "" {
			return &LearnedPitfall{
				Resource:       resource,
				Rule:           fmt.Sprintf("At least one of %s must be set.", constraint),
				DiscoveredFrom: scenarioName,
			}
		}
	}

	// M97 prescriptive templates: pattern-match common error shapes
	// and emit ACTIONABLE rules ("do Y") instead of descriptive ones
	// ("X failed because..."). M95 showed that descriptive rules
	// accumulate but don't help the LLM converge — it sees the same
	// rule in its prompt context but can't translate "X failed" into
	// "write HCL Y instead." The templates below cover the 5 most
	// common error shapes seen in M88/M94/M95 sweeps. Coverage is
	// intentionally narrow + specific; we'd rather miss a shape (and
	// fall through to the descriptive fallback) than overgeneralize
	// to a misleading prescriptive rule.

	// (1) Missing subnetwork on compute instance / GKE cluster — the
	// most common failure shape in gcp-full-stack and friends.
	if pitfall := matchMissingSubnetwork(failureDetail, scenarioName); pitfall != nil {
		return pitfall
	}
	// (2) CMEK encryption on bucket/SQL — fakegcp doesn't model KMS.
	if pitfall := matchMissingEncryption(failureDetail, scenarioName); pitfall != nil {
		return pitfall
	}
	// (3) "501 Not implemented" — the resource type isn't modelled.
	if pitfall := matchNotImplemented(failureDetail, scenarioName); pitfall != nil {
		return pitfall
	}
	// (4) "401 OAuth credentials" — provider escaped to the real cloud
	// because no custom_endpoint covers this resource type.
	if pitfall := matchOAuthEscape(failureDetail, scenarioName); pitfall != nil {
		return pitfall
	}
	// (5) AWS deletion_protection / force_destroy that block destroy.
	if pitfall := matchDestroyBlockers(failureDetail, scenarioName); pitfall != nil {
		return pitfall
	}

	// Fallback FIRST: if a `scaleway_*` / `google_*` resource is named
	// AND the detail is long enough to be meaningful, capture it.
	// Generic-pattern rejection runs LAST so a detail like
	// "exit status 1 | stderr: ... google_project_service.redis ..."
	// learns from the actionable substring instead of being
	// substring-rejected by the "exit status" envelope (M86 fix —
	// every tofu apply failure starts with that envelope, so the
	// prior ordering silently dropped every apply-time learning).
	resource := extractResource(failureDetail)
	if resource != "" && len(failureDetail) > 40 {
		rule := failureDetail
		if len(rule) > 300 {
			rule = rule[:297] + "..."
		}
		return &LearnedPitfall{
			Resource:       resource,
			Rule:           rule,
			DiscoveredFrom: scenarioName,
		}
	}

	// Reject remaining generic errors that are not actionable. Only
	// reached when no resource could be extracted.
	for _, pattern := range genericPatterns {
		if strings.Contains(lower, pattern) {
			return nil
		}
	}

	return nil
}

// extractResource finds a Terraform resource name in the text. Matches
// scaleway_* or google_* prefixes (mockway / fakegcp providers).
func extractResource(text string) string {
	if match := resourceNameRe.FindString(text); match != "" {
		return match
	}
	return ""
}

// M97 prescriptive-rule templates.
//
// Each match* function inspects the failure detail for one specific
// error shape. On hit it returns a *LearnedPitfall whose Rule is
// ACTIONABLE — tells the LLM what HCL to write — rather than just
// echoing the failure ("X has no Y"). The descriptive fallback in
// ExtractLearnedPitfall remains for shapes none of these match.

// matchMissingSubnetwork — "no network or subnetwork" / "no
// network_interface.subnetwork" failures on compute instances + GKE
// clusters. The single most common shape in gcp-full-stack runs.
func matchMissingSubnetwork(detail, scenario string) *LearnedPitfall {
	lower := strings.ToLower(detail)
	if !strings.Contains(lower, "subnetwork") && !strings.Contains(lower, "network_interface") {
		return nil
	}
	if !strings.Contains(lower, "no network") && !strings.Contains(lower, "must be attached") && !strings.Contains(lower, "must reference") {
		return nil
	}
	resource := extractResource(detail)
	if resource == "" {
		return nil
	}
	rule := "Always declare a `google_compute_network` AND at least one `google_compute_subnetwork` in the scenario's region. Then on `google_compute_instance` set `network_interface { subnetwork = google_compute_subnetwork.NAME.id }`. On `google_container_cluster` set BOTH `network = google_compute_network.NAME.id` AND `subnetwork = google_compute_subnetwork.NAME.id`. Do NOT use the `default` VPC — it is often disabled by org policy."
	return &LearnedPitfall{Resource: resource, Rule: rule, DiscoveredFrom: scenario}
}

// matchMissingEncryption — DISABLED 2026-05-28: previous template
// was in direct conflict with `policies/gcp/encryption.rego` (template
// said "omit CMEK", policy requires CMEK). Auto-learning that tells
// the LLM the OPPOSITE of what the gates enforce makes runs worse,
// not better. The correct prescriptive rule needs to know whether
// CMEK is required by the active policy set for this scenario — and
// when it IS required, the rule should be "set encryption.default_
// kms_key_name = google_kms_crypto_key.NAME.id (and ensure the KMS
// keyring/key are also declared)." Until M98 lands cross-policy
// awareness in the templates, this matcher is a no-op so we don't
// poison the learning loop with wrong advice.
func matchMissingEncryption(_, _ string) *LearnedPitfall {
	return nil
}

// matchNotImplemented — 501 from a mock surface that doesn't model
// the resource. The fix is to remove the resource, not to retry.
func matchNotImplemented(detail, scenario string) *LearnedPitfall {
	lower := strings.ToLower(detail)
	if !strings.Contains(lower, "501") && !strings.Contains(lower, "not implemented") {
		return nil
	}
	resource := extractResource(detail)
	if resource == "" {
		return nil
	}
	rule := fmt.Sprintf("`%s` is not implemented by the matching mock server. Do NOT use this resource type — pick an alternative that the mock supports. Check the mock's regression_manifest.go for the LandedServices list.", resource)
	return &LearnedPitfall{Resource: resource, Rule: rule, DiscoveredFrom: scenario}
}

// matchOAuthEscape — 401 OAuth credentials means the provider routed
// to the real cloud because no *_custom_endpoint covers this resource
// type. The fix is to either add an endpoint override or to remove
// the resource.
func matchOAuthEscape(detail, scenario string) *LearnedPitfall {
	lower := strings.ToLower(detail)
	if !strings.Contains(lower, "401") {
		return nil
	}
	if !strings.Contains(lower, "oauth") && !strings.Contains(lower, "authentication credentials") && !strings.Contains(lower, "invalid_grant") {
		return nil
	}
	resource := extractResource(detail)
	if resource == "" {
		return nil
	}
	rule := fmt.Sprintf("`%s` is escaping to the REAL cloud API (401 OAuth/auth error). The provider block in `providers.tf` is missing a `*_custom_endpoint` override for this resource's service. Either add the matching custom_endpoint OR remove `%s` from this scenario (the mock doesn't intercept it).", resource, resource)
	return &LearnedPitfall{Resource: resource, Rule: rule, DiscoveredFrom: scenario}
}

// matchDestroyBlockers — AWS resources with deletion_protection or
// missing force_destroy that block tofu destroy.
func matchDestroyBlockers(detail, scenario string) *LearnedPitfall {
	lower := strings.ToLower(detail)
	switch {
	case strings.Contains(lower, "deletion_protection") && (strings.Contains(lower, "enabled") || strings.Contains(lower, "true")):
		resource := extractResource(detail)
		if resource == "" {
			return nil
		}
		rule := fmt.Sprintf("`%s` defaults `deletion_protection = true`. For test scenarios, explicitly set `deletion_protection = false` so `tofu destroy` can tear the resource down.", resource)
		return &LearnedPitfall{Resource: resource, Rule: rule, DiscoveredFrom: scenario}
	case strings.Contains(lower, "bucketnotempty") || (strings.Contains(lower, "force_destroy") && strings.Contains(lower, "bucket")):
		resource := extractResource(detail)
		if resource == "" {
			return nil
		}
		rule := fmt.Sprintf("`%s` must set `force_destroy = true` for test scenarios. The provider default is `false`, which makes destroy fail with BucketNotEmpty when objects/keys remain.", resource)
		return &LearnedPitfall{Resource: resource, Rule: rule, DiscoveredFrom: scenario}
	case strings.Contains(lower, "skip_final_snapshot") || (strings.Contains(lower, "final snapshot") && strings.Contains(lower, "must be specified")):
		resource := extractResource(detail)
		if resource == "" {
			return nil
		}
		rule := fmt.Sprintf("`%s` requires `skip_final_snapshot = true` (test) or an explicit `final_snapshot_identifier`. Default behavior blocks destroy with 'final snapshot must be specified'.", resource)
		return &LearnedPitfall{Resource: resource, Rule: rule, DiscoveredFrom: scenario}
	}
	return nil
}

// AppendPitfall appends a learned pitfall to the YAML file if it doesn't
// already exist (deduplication by resource + similar rule text).
func AppendPitfall(pitfallsDir, cloud string, pitfall LearnedPitfall) error {
	if pitfallsDir == "" || cloud == "" {
		return nil
	}

	filePath := filepath.Join(pitfallsDir, cloud+".yaml")

	// Ensure directory exists.
	if err := os.MkdirAll(pitfallsDir, 0o755); err != nil {
		return fmt.Errorf("create pitfalls directory: %w", err)
	}

	// Load existing file or start fresh.
	var pf PitfallsFile
	data, err := os.ReadFile(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read pitfalls file: %w", err)
		}
		pf = PitfallsFile{Provider: cloud}
	} else {
		if err := yaml.Unmarshal(data, &pf); err != nil {
			return fmt.Errorf("parse pitfalls file: %w", err)
		}
	}

	// Deduplication: check if a similar pitfall already exists.
	if isDuplicate(pf.Pitfalls, pitfall) {
		return nil
	}

	// Append the new pitfall.
	pf.Pitfalls = append(pf.Pitfalls, PitfallEntry{
		Resource:       pitfall.Resource,
		Rule:           pitfall.Rule,
		Source:         "learned",
		DiscoveredFrom: pitfall.DiscoveredFrom,
	})

	// Marshal and write atomically via temp file.
	out, err := yaml.Marshal(&pf)
	if err != nil {
		return fmt.Errorf("marshal pitfalls: %w", err)
	}

	// Use os.CreateTemp so two concurrent learn-paths racing on the
	// same provider can't clobber each other's tmp file before either
	// rename completes. Mirrors the editPitfalls API handler.
	tmp, err := os.CreateTemp(pitfallsDir, cloud+"-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("create temp pitfalls file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupPath := tmpPath
	defer func() {
		if cleanupPath != "" {
			_ = os.Remove(cleanupPath)
		}
	}()
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp pitfalls file: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp pitfalls file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp pitfalls file: %w", err)
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		return fmt.Errorf("rename pitfalls file: %w", err)
	}
	cleanupPath = ""

	return nil
}

// isDuplicate returns true if any existing pitfall has the same resource
// and shares 3+ significant words with the new rule.
func isDuplicate(existing []PitfallEntry, candidate LearnedPitfall) bool {
	candidateWords := significantWords(candidate.Rule)
	for _, entry := range existing {
		if entry.Resource != candidate.Resource {
			continue
		}
		existingWords := significantWords(entry.Rule)
		shared := 0
		for w := range candidateWords {
			if existingWords[w] {
				shared++
			}
		}
		if shared >= 3 {
			return true
		}
	}
	return false
}

// significantWords extracts lowercase words of 4+ characters from text,
// excluding common stop words.
func significantWords(text string) map[string]bool {
	stopWords := map[string]bool{
		"that": true, "this": true, "with": true, "from": true,
		"have": true, "will": true, "when": true, "must": true,
		"does": true, "should": true, "would": true, "could": true,
		"than": true, "also": true, "into": true, "only": true,
		"each": true, "such": true, "they": true, "been": true,
		"like": true, "then": true, "some": true, "your": true,
		"them": true, "more": true, "make": true, "very": true,
	}

	words := make(map[string]bool)
	for _, word := range strings.Fields(strings.ToLower(text)) {
		// Strip punctuation.
		word = strings.Trim(word, ".,;:!?\"'`()[]{}/<>")
		if len(word) >= 4 && !stopWords[word] {
			words[word] = true
		}
	}
	return words
}
