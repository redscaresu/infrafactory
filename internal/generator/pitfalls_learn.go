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

	// Captures the offending attribute name from either:
	//   `does not have an attribute named "X"`
	//   `no argument, nested block, or exported attribute named "X"`
	unsupportedAttrNameRe = regexp.MustCompile(`(?i)attribute\s+named\s+"([A-Za-z0-9_]+)"`)

	// Captures the offending argument name from the wrapped Unsupported
	// argument shape: `An argument named "X" is not expected here.`
	// The legacy unsupportedArgRe ("Unsupported argument.*\"X\"") only
	// matches single-line shapes because `.*` doesn't span newlines —
	// real diagnostics wrap, so they fell through to the descriptive
	// fallback and the LLM kept reintroducing the same bad argument
	// (gcp-cloud-run deletion_protection was the motivating case).
	unsupportedArgNameRe = regexp.MustCompile(`(?i)argument\s+named\s+"([A-Za-z0-9_]+)"`)

	// Captures the resource type from `in resource "TYPE" "NAME":`
	// (always present in terraform's Unsupported-argument diagnostic
	// for the owning block — preferred over extractResource which
	// picks up cross-resource references).
	inResourceTypeRe = regexp.MustCompile(`in\s+resource\s+"([a-z][a-z0-9_]*)"`)

	// Terraform's "Did you mean Y?" suggestion, when present.
	didYouMeanRe = regexp.MustCompile(`(?i)Did you mean "([A-Za-z0-9_]+)"\??`)

	// Terraform's diagnostic frame characters — `│` is on every wrapped
	// line of the error body and silently breaks `\s+` bridging in the
	// matchers above.
	boxDrawingStripRe = regexp.MustCompile("[╷╵│─]+")

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

	// mockActionableSignals are substrings that — when present in a
	// failure detail — indicate the failure is a mock-server bug, NOT
	// an LLM-side HCL mistake. Writing such failures to
	// pitfalls/<cloud>.yaml teaches the LLM to avoid valid resources
	// just because the mock is incomplete; that makes the system
	// narrower over time. The correct destination is docs/mock-gaps.md
	// (filed as a ticket against the matching mock repo).
	//
	// Detection is intentionally conservative (substring match, not
	// regex) — false negatives leave the existing pitfall path active;
	// false positives drop a learning that arguably belongs in the
	// pitfalls file. We bias toward false negatives.
	//
	// Signals come from real failures observed in the 2026-05-31 sweep
	// + earlier sessions. Add more here as new mock-gap shapes surface.
	mockActionableSignals = []string{
		// 501 Not Implemented — fakeaws/fakegcp/mockway return this
		// from their default-not-found handler when no route matches.
		// Always a missing-mock-handler symptom, never an LLM bug.
		"501 not implemented",
		"501: not implemented",
		"\"reason\":\"notimplemented\"",
		"reason: \"notimplemented\"",

		// "Plugin did not respond" — the provider crashed parsing the
		// mock's response (usually missing/wrong field shape that the
		// SDK nil-derefs on). Mock-side fidelity bug.
		"plugin did not respond",
		"the plugin encountered an error",

		// OAuth / auth escape — the lib client used a code path that
		// bypasses the per-service custom_endpoint override and
		// escaped to the real cloud. Either a missing endpoint flag
		// (T-D-2 v3 pattern) or a missing route. Either way mock-side.
		"access_token_type_unsupported",
		"request had invalid authentication credentials",

		// Wait-loop "couldn't find resource (N retries)" — provider's
		// Read polled after a successful Create and the mock either
		// 404s the Describe* path or returns null. Mock-side state
		// divergence.
		"couldn't find resource",
		"resourcenotfoundexception",

		// Generic "unimplemented" envelope variants (each mock has its
		// own wire shape).
		"\"error\":\"unimplemented\"",
	}
)

// MockGap is a structured entry written to docs/mock-gaps.md when
// the learning pipeline detects a mock-server-side failure. The
// markdown writer formats one row per gap, dedup-keyed by (cloud,
// signal, resource) so re-running the same scenario doesn't grow
// the file linearly.
type MockGap struct {
	Cloud     string // "aws" | "gcp" | "scaleway"
	Signal    string // matched mock-actionable signal (e.g. "501 not implemented")
	Resource  string // extracted resource, empty when none parseable
	Scenario  string // scenario name where the failure surfaced
	Detail    string // first ~240 chars of the failure detail for context
	Timestamp string // RFC3339; the run's timestamp (caller passes)
}

// AppendMockGap writes a structured mock-gap entry to
// docs/mock-gaps.md inside the given docs directory, creating the
// file with a header on first call. Dedup-keyed by (cloud, signal,
// resource) — a re-run that produces the same gap shape is ignored
// instead of growing the file.
//
// File format is markdown so it doubles as a human-readable backlog
// for the matching mock repo's maintainers. Each entry is a row in
// a per-cloud table.
//
// Caller pattern: see IsMockActionable godoc.
func AppendMockGap(docsDir string, gap MockGap) error {
	if docsDir == "" {
		return fmt.Errorf("docs dir is required")
	}
	if gap.Cloud == "" || gap.Signal == "" {
		return fmt.Errorf("cloud and signal are required")
	}
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		return fmt.Errorf("mkdir docs dir: %w", err)
	}
	path := filepath.Join(docsDir, "mock-gaps.md")

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read mock-gaps: %w", err)
	}
	content := string(existing)

	// Dedup: same (cloud, signal, resource) triple already recorded?
	// Markdown row format includes these three columns; substring
	// match on the resource cell is sufficient because the cloud +
	// signal columns are stable per row group.
	dedupKey := fmt.Sprintf("| %s | `%s` |", gap.Resource, gap.Signal)
	if gap.Resource == "" {
		dedupKey = fmt.Sprintf("| _(none)_ | `%s` |", gap.Signal)
	}
	if strings.Contains(content, dedupKey) {
		return nil
	}

	if content == "" {
		content = "# Mock-server gaps\n\n" +
			"This file collects failures the auto-learning pipeline detects\n" +
			"as mock-server bugs (missing routes, wrong response shapes,\n" +
			"auth escape, stale state) rather than LLM-generated HCL\n" +
			"mistakes. They belong against the matching mock repo\n" +
			"(`fakeaws`, `fakegcp`, `mockway`), NOT in `pitfalls/<cloud>.yaml`.\n\n" +
			"Entries dedup on (cloud, signal, resource). See\n" +
			"`internal/generator/pitfalls_learn.go::IsMockActionable` for\n" +
			"the detection signals.\n\n"
	}

	// Per-cloud heading + table. Idempotent: only adds the heading
	// if the cloud doesn't already have a section.
	heading := fmt.Sprintf("## %s\n\n", gap.Cloud)
	if !strings.Contains(content, heading) {
		content += heading +
			"| Resource | Signal | Scenario | Detail | First seen |\n" +
			"|---|---|---|---|---|\n"
	}

	resourceCell := gap.Resource
	if resourceCell == "" {
		resourceCell = "_(none)_"
	}
	detail := strings.TrimSpace(gap.Detail)
	if len(detail) > 240 {
		detail = detail[:237] + "..."
	}
	// Escape pipe + newline so the markdown table doesn't break.
	detail = strings.ReplaceAll(detail, "|", "\\|")
	detail = strings.ReplaceAll(detail, "\n", " ")

	row := fmt.Sprintf("| %s | `%s` | %s | %s | %s |\n",
		resourceCell, gap.Signal, gap.Scenario, detail, gap.Timestamp)

	// Insert the row at the end of the appropriate cloud section.
	// Sections are delimited by the `## ` heading; the row goes at
	// the END of the section's table. Simple approach: append to the
	// file end if the table is at the file end; otherwise insert
	// before the next `## ` heading.
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

// FirstMockSignal returns the first mock-actionable signal that
// matched the given detail (lowercased, exactly as in
// mockActionableSignals). Returns "" if no signal matches — callers
// typically guard with IsMockActionable first.
func FirstMockSignal(detail string) string {
	if detail == "" {
		return ""
	}
	lower := strings.ToLower(detail)
	for _, sig := range mockActionableSignals {
		if strings.Contains(lower, sig) {
			return sig
		}
	}
	return ""
}

// ExtractResourceFromDetail is a public wrapper around the package-
// private resource-extraction helper. Used by run_command.go's mock-
// gap routing to populate the Resource cell.
func ExtractResourceFromDetail(detail string) string {
	return extractResource(detail)
}

// IsMockActionable reports whether a failure detail is rooted in a
// mock-server gap (missing route, wrong response shape, auth escape,
// stale state) rather than an LLM-generated HCL mistake. Mock-
// actionable failures should NOT be auto-learned into
// pitfalls/<cloud>.yaml — they belong in docs/mock-gaps.md so the
// matching mock repo can fix the gap at source.
//
// Detection is substring-based against a small, hand-curated signal
// set (see mockActionableSignals above). Conservative by design.
//
// Caller pattern (run_command.go):
//
//	if generator.IsMockActionable(failure.Detail) {
//	    generator.AppendMockGap(docsDir, cloud, gap)  // routes here
//	    continue
//	}
//	learned := generator.ExtractLearnedPitfall(failure.Detail, scenario)
//	// ... existing path
func IsMockActionable(detail string) bool {
	if detail == "" {
		return false
	}
	lower := strings.ToLower(detail)
	for _, sig := range mockActionableSignals {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}

// ExtractLearnedPitfall analyzes a failure detail string and extracts
// a pitfall rule if the error is specific enough to be useful.
// Returns nil if the error is too vague (e.g., "test checks failed").
func ExtractLearnedPitfall(failureDetail, scenarioName string) *LearnedPitfall {
	if failureDetail == "" {
		return nil
	}

	lower := strings.ToLower(failureDetail)

	// Box-drawing characters (`╷`, `│`, `╵`, `─`) frame every line of a
	// terraform diagnostic. Any regex below that uses `.*` or `\s+` to
	// bridge content across lines silently fails on multi-line shapes
	// because `.*` doesn't match newlines and `\s+` doesn't match `│`.
	// Use a cleaned view for regex matching while preserving the raw
	// failureDetail for substring checks and the descriptive fallback's
	// rule text.
	cleanedDetail := boxDrawingStripRe.ReplaceAllString(failureDetail, " ")

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

	// Try specific pattern: Unsupported argument. Match against the
	// box-drawing-stripped view because `Unsupported argument.*"X"` has
	// `.*` between the marker and the quoted name, and `.*` doesn't
	// span newlines — terraform's wrapped diagnostics put `X` on a
	// later line in most real failures.
	if matches := unsupportedArgRe.FindStringSubmatch(cleanedDetail); len(matches) >= 2 {
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

	// Try specific pattern: "at least one of". Same reason as above —
	// the constraint list typically wraps to a second line in the
	// diagnostic body and `\s+` won't bridge `│`.
	if matches := atLeastOneOfRe.FindStringSubmatch(cleanedDetail); len(matches) >= 2 {
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
	// (6) "Unsupported attribute" / "no exported attribute named" — the
	// LLM wrote `resource.X.private_ip` when the attribute is actually
	// `private_ips` (plural). Common shape across all 3 clouds: the
	// terraform error often includes a "Did you mean Y" suggestion we
	// can hand straight to the LLM.
	if pitfall := matchUnsupportedAttribute(failureDetail, scenarioName); pitfall != nil {
		return pitfall
	}
	// (7) Scaleway VPC attachment — "X is not attached to a private
	// network via scaleway_instance_private_nic". Counterpart to
	// matchMissingSubnetwork on GCP. The 2026-05-30 sweep showed the
	// LLM hitting this in web-app-paris and private-lb-db-paris and
	// getting only the descriptive fallback ("X failed"), which it
	// failed to act on; a prescriptive rule converges in one shot.
	if pitfall := matchScalewayMissingPrivateNic(failureDetail, scenarioName); pitfall != nil {
		return pitfall
	}
	// (8) Unsupported argument with wrapped diagnostic — the legacy
	// unsupportedArgRe at line ~130 only catches single-line shapes;
	// the real terraform output wraps the argument name onto a later
	// `An argument named "X" is not expected here.` line which the
	// single-line regex misses. This template fires on the wrapped
	// shape and emits a prescriptive rule that names BOTH the resource
	// type AND the arg to remove — motivated by gcp-cloud-run where
	// the LLM kept reintroducing `deletion_protection = false` on
	// `google_cloud_run_v2_service` because the descriptive fallback
	// (raw stderr dump) was unactionable.
	if pitfall := matchUnsupportedArgument(failureDetail, scenarioName); pitfall != nil {
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

// resourceOwningAttribute finds the resource type whose `.attr` access
// triggered an Unsupported attribute / no exported attribute error.
// Returns the empty string when no such chain is in the detail.
func resourceOwningAttribute(detail, attr string) string {
	pattern := `\b((?:scaleway|google|aws)_\w+)[\w.\[\]\*]*\.` + regexp.QuoteMeta(attr) + `\b`
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}
	m := re.FindStringSubmatch(detail)
	if len(m) >= 2 {
		return m[1]
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

// matchScalewayMissingPrivateNic — Scaleway VPC-attachment failure
// surfaced by the vpc_required policy: a `scaleway_instance_server`
// is not attached to a private network via `scaleway_instance_private_nic`.
// The descriptive fallback echoes the policy message verbatim ("X is
// not attached…") but the LLM treats that as a status, not a
// prescription; the M97 sweep on 2026-05-30 showed it converging on
// either oscillating or stuck without the explicit "declare these
// resources" guidance. This template emits the same shape of
// actionable rule as matchMissingSubnetwork does for GCP.
func matchScalewayMissingPrivateNic(detail, scenario string) *LearnedPitfall {
	lower := strings.ToLower(detail)
	if !strings.Contains(lower, "scaleway_instance_private_nic") {
		return nil
	}
	if !strings.Contains(lower, "not attached") && !strings.Contains(lower, "must be attached") {
		return nil
	}
	resource := extractResource(detail)
	if resource == "" {
		// Fall back to the resource the rule applies to even when the
		// caller's detail mentions only `scaleway_instance_private_nic`.
		resource = "scaleway_instance_server"
	}
	rule := "Always declare a `scaleway_vpc_private_network` AND a `scaleway_instance_private_nic` for EACH `scaleway_instance_server`. The private NIC has the shape: `resource \"scaleway_instance_private_nic\" \"NAME\" { server_id = scaleway_instance_server.SERVER.id; private_network_id = scaleway_vpc_private_network.PN.id }`. The `vpc_required` policy fails any instance without this attachment. Add one NIC per instance (use `count` to mirror the `count` on the instance) — do NOT rely on the instance's `routed_ip_enabled` flag, which the provider doesn't accept."
	return &LearnedPitfall{Resource: resource, Rule: rule, DiscoveredFrom: scenario}
}

// matchUnsupportedAttribute — terraform "Unsupported attribute" /
// "no argument, nested block, or exported attribute named ..." errors.
// Cloud-agnostic: applies to scaleway_*, google_*, aws_* identically.
//
// Two real shapes seen across the three mocks:
//
//	This object does not have an attribute named "private_ip".
//	This object has no argument, nested block, or exported attribute
//	named "private_ip". Did you mean "private_ips"?
//
// When terraform offers a "Did you mean Y" suggestion we forward it
// verbatim — the LLM's most common failure mode here is guessing
// `private_ip` (singular) when the provider exports `private_ips`
// (plural), or `id`/`self_link`/`arn` confusion across clouds. A
// prescriptive rule that names BOTH the wrong attribute AND the
// suggested replacement converges in one shot.
func matchUnsupportedAttribute(detail, scenario string) *LearnedPitfall {
	lower := strings.ToLower(detail)
	if !strings.Contains(lower, "unsupported attribute") &&
		!strings.Contains(lower, "exported attribute named") {
		return nil
	}
	// Terraform diagnostics embed box-drawing chars (`│`) at the start
	// of every wrapped line of the error body. Without stripping these
	// first, `\s+` between "attribute named" and the quoted attribute
	// won't span the line break — the regex misses on every multi-line
	// shape (which is most of them). Use a cleaned view for matching;
	// raw `detail` is retained for the rule body where the suggestion
	// extraction still works because "Did you mean" tends to fit on one
	// line.
	cleaned := boxDrawingStripRe.ReplaceAllString(detail, " ")
	m := unsupportedAttrNameRe.FindStringSubmatch(cleaned)
	if len(m) < 2 {
		return nil
	}
	attr := m[1]
	// Resource type: prefer the one whose `.<attr>` chain actually
	// triggered the error. A typical detail mentions TWO resources —
	// the containing block (`scaleway_lb_backend.web`) and the
	// referenced resource being accessed
	// (`scaleway_instance_server.web[*].private_ip`). extractResource
	// just returns the first match (the containing block), which is
	// the wrong resource to attribute the pitfall to. Find the one
	// followed by `.<bad attr>` instead.
	resource := resourceOwningAttribute(detail, attr)
	if resource == "" {
		resource = extractResource(detail)
	}
	if resource == "" {
		return nil
	}
	suggestion := ""
	if s := didYouMeanRe.FindStringSubmatch(detail); len(s) >= 2 {
		suggestion = s[1]
	}
	var rule string
	if suggestion != "" {
		rule = fmt.Sprintf("`%s` has no attribute `%s` — use `%s` instead. The provider error suggested this directly: `Did you mean \"%s\"?`. Update every reference (`server_ips`, outputs, depends_on, etc.) to the suggested attribute name.", resource, attr, suggestion, suggestion)
	} else {
		rule = fmt.Sprintf("`%s` has no attribute named `%s`. Check the provider docs for the exact exported-attribute name (plural vs singular is the most common trap: `private_ips`, `public_ips`, `network_interfaces`). Do NOT keep retrying with the same name.", resource, attr)
	}
	return &LearnedPitfall{Resource: resource, Rule: rule, DiscoveredFrom: scenario}
}

// matchUnsupportedArgument — terraform "Unsupported argument" diagnostic
// in its real wrapped form. The legacy single-line regex
// `Unsupported argument.*"(\w+)"` (unsupportedArgRe) misses the wrapped
// shape because `.*` doesn't span newlines and terraform always wraps
// the bad-arg line under the marker:
//
//	╷
//	│ Error: Unsupported argument
//	│
//	│   on main.tf line 6, in resource "google_cloud_run_v2_service" "api":
//	│    6:   deletion_protection = false
//	│
//	│ An argument named "deletion_protection" is not expected here.
//	╵
//
// The descriptive fallback fired for this shape and produced a verbatim
// stderr dump that the LLM couldn't parse. This template catches the
// wrapped shape, prefers the `in resource "TYPE"` block as the owning
// resource (over extractResource which would pick up the first
// occurrence — fine here but fragile when references to other
// resources appear in the same diagnostic), and emits a prescriptive
// rule that names both the resource and the arg to remove.
func matchUnsupportedArgument(detail, scenario string) *LearnedPitfall {
	lower := strings.ToLower(detail)
	if !strings.Contains(lower, "unsupported argument") &&
		!strings.Contains(lower, "is not expected here") {
		return nil
	}
	cleaned := boxDrawingStripRe.ReplaceAllString(detail, " ")
	m := unsupportedArgNameRe.FindStringSubmatch(cleaned)
	if len(m) < 2 {
		return nil
	}
	argName := m[1]
	var resource string
	if rm := inResourceTypeRe.FindStringSubmatch(cleaned); len(rm) >= 2 {
		resource = rm[1]
	} else {
		resource = extractResource(detail)
	}
	if resource == "" {
		return nil
	}
	rule := fmt.Sprintf("`%s` does NOT accept the argument `%s` — the provider rejects it with \"An argument named \"%s\" is not expected here.\" Remove the `%s = ...` line from every `%s` block; do NOT reintroduce it across iterations. Check the provider docs for the canonical argument set (common traps: deprecated/renamed args, v1→v2 service rewrites where the old arg moved or disappeared).", resource, argName, argName, argName, resource)
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

	// Verbatim → prescriptive upgrade: when the new candidate is a
	// prescriptive rule (no terraform box-drawing chars, no raw stderr
	// prefix) and a same-resource entry is the old descriptive fallback
	// (verbatim diagnostic dump), REPLACE the verbatim entry rather than
	// dedup-skipping. The old shape would otherwise permanently shadow
	// the prescriptive form because isDuplicate matches on shared words
	// — and a verbatim dump shares the resource type / argument name
	// with any subsequent prescriptive rule about the same failure.
	if !isVerbatimFallback(pitfall.Rule) {
		for i, entry := range pf.Pitfalls {
			if entry.Resource == pitfall.Resource && isVerbatimFallback(entry.Rule) {
				pf.Pitfalls[i] = PitfallEntry{
					Resource:       pitfall.Resource,
					Rule:           pitfall.Rule,
					Source:         "learned",
					DiscoveredFrom: pitfall.DiscoveredFrom,
				}
				return writePitfallsFile(pitfallsDir, filePath, cloud, &pf)
			}
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

	return writePitfallsFile(pitfallsDir, filePath, cloud, &pf)
}

// writePitfallsFile marshals the pitfalls file and writes it atomically
// via a same-directory temp + rename. Used by AppendPitfall on both the
// append path and the verbatim→prescriptive upgrade path.
func writePitfallsFile(pitfallsDir, filePath, cloud string, pf *PitfallsFile) error {
	out, err := yaml.Marshal(pf)
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

// isVerbatimFallback returns true if a rule is a raw terraform stderr
// dump (the descriptive fallback ExtractLearnedPitfall returns when no
// M97 template fires). Detection signals: terraform box-drawing chars
// or the "exit status 1 | stderr:" envelope prefix. AppendPitfall uses
// this to allow a later prescriptive rule to UPGRADE an older verbatim
// entry rather than dedup-skipping. Without this, once a verbatim entry
// is in the file for a resource, the same-3-word-share dedup keeps
// blocking the prescriptive form forever.
func isVerbatimFallback(rule string) bool {
	return strings.ContainsAny(rule, "│╷╵─") ||
		strings.Contains(rule, "exit status 1 | stderr:")
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
