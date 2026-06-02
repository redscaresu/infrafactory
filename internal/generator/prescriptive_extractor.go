package generator

// N10 — diff-based prescriptive-pitfall extractor.
//
// The legacy ExtractLearnedPitfall captures the failure detail
// verbatim as the pitfall rule. That's symptom-only: the LLM learns
// what went wrong but not HOW to fix it. The 2026-06-02 sweep
// motivating case was gcp-storage repeatedly hitting
// `policy=gcp.encryption detail="google_storage_bucket.app_assets
// has no encryption.default_kms_key_name"` — even after the
// learning loop appended that rule to pitfalls/gcp.yaml, the LLM
// kept failing because nothing told it to declare
// `google_kms_crypto_key` + `encryption { default_kms_key_name }`.
//
// This extractor closes that gap by diffing the last-failing
// iteration's HCL against the first-passing iteration's HCL. When
// the LLM's added resources / attributes correlate with a cleared
// failure, the diff becomes a prescriptive pitfall rule.
//
// See `docs/NEXT_SESSION.md` § N10 for the design rationale and
// validation plan.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// PrescriptiveSource is the source tag used in PitfallEntry to
// distinguish diff-learned rules from legacy symptom-only ones.
const PrescriptiveSource = "learned_from_diff"

// PrescriptiveAvoidSource is the source tag for N13's deletion-as-fix
// pitfalls — rules of the shape "Do NOT use <thing>; it causes
// <failure>." N10's PrescriptiveSource captures addition-as-fix
// patterns (the LLM ADDED HCL that resolved a failure). N13 captures
// the dual: the LLM REMOVED HCL that was causing a failure.
const PrescriptiveAvoidSource = "learned_from_diff_avoid"

// resourceHeaderRe matches the opening line of a Terraform resource
// block: `resource "TYPE" "NAME" {`. We capture TYPE and NAME so we
// can build the bare address (`TYPE.NAME`) used for failure
// attribution. Quote-escape sequences inside type/name are forbidden
// by HCL grammar, so a simple match is safe.
var resourceHeaderRe = regexp.MustCompile(`^\s*resource\s+"([a-z][a-z0-9_]*)"\s+"([A-Za-z][A-Za-z0-9_-]*)"\s*\{`)

// addressRe extracts a bare resource address (TYPE.NAME, optionally
// indexed) from a failure detail string. The trailing `[N]` is
// dropped for matching since the diff sees the configuration
// (symbolic) reference, not the planned (indexed) address.
var addressRe = regexp.MustCompile(`((?:scaleway|google|aws|random)_[a-z_]+)\.([A-Za-z][A-Za-z0-9_-]*)(?:\[\d+\])?`)

// PrescriptiveFix is the structured output of the extractor before
// it's converted into a PitfallEntry. Exposed for testing.
type PrescriptiveFix struct {
	Resource string // bare type, e.g. "google_storage_bucket"
	Address  string // full address from failure, e.g. "google_storage_bucket.app_assets"
	Snippet  string // HCL snippet that the LLM added to clear the failure
	Cloud    string // "aws" | "gcp" | "scaleway"
	Scenario string
}

// snippetMaxBytes caps the rule snippet length to keep
// pitfalls/<cloud>.yaml small enough to inject into prompts. 600
// bytes covers a typical CMEK fix (key ring + crypto key + encryption
// block) with margin for trimming.
const snippetMaxBytes = 600

// ExtractPrescriptiveFix returns a PitfallEntry encoding the HCL
// addition the LLM made between failedDir and passingDir that
// resolved `failure`. Returns nil if no productive diff can be
// attributed to the failure.
//
// failureDetail is parsed for a resource address; that address is
// then looked up in both directories' resource maps. The "fix" is
// the union of (new attributes on the failing resource, new sibling
// resources referenced from those new attributes).
func ExtractPrescriptiveFix(failedDir, passingDir string, failureDetail, failureResource, cloud, scenario, timestamp string) (*LearnedPitfall, error) {
	failedResources, err := loadResourceBlocks(failedDir)
	if err != nil {
		return nil, fmt.Errorf("read failed dir %q: %w", failedDir, err)
	}
	passingResources, err := loadResourceBlocks(passingDir)
	if err != nil {
		return nil, fmt.Errorf("read passing dir %q: %w", passingDir, err)
	}

	// Attribution: locate the failing resource address. Prefer the
	// structured `failure.Resource` if non-empty, else extract from
	// the detail string. State-side policy failures (deny_state in
	// the OPA rules) describe the GCP-side resource by name only
	// ("Cloud SQL instance NAME missing X"), not the terraform
	// address — fall back to type-hint inference + diff scoping.
	address := failureResource
	if address == "" {
		address = firstResourceAddress(failureDetail)
	}
	if address == "" {
		if hint := inferResourceTypeFromDetail(failureDetail); hint != "" {
			changed := changedResourcesOfType(failedResources, passingResources, hint)
			if len(changed) == 1 {
				address = changed[0]
			}
		}
	}
	if address == "" {
		return nil, nil
	}
	resourceType, _, ok := splitAddress(address)
	if !ok {
		return nil, nil
	}

	fix := PrescriptiveFix{
		Resource: resourceType,
		Address:  address,
		Cloud:    cloud,
		Scenario: scenario,
	}

	// Build the snippet from two diff slices:
	//   1) what was added INSIDE the failing resource's block
	//      (e.g., a new `encryption {}` block on google_storage_bucket)
	//   2) NEW sibling resources whose addresses are referenced from
	//      the added attributes (e.g., the google_kms_crypto_key the
	//      `encryption.default_kms_key_name` points to).
	var pieces []string

	if passingBlock, ok := passingResources[address]; ok {
		failedBody := ""
		if failedBlock, ok := failedResources[address]; ok {
			failedBody = failedBlock.Body
		}
		// When the address only exists in passing (LLM renamed the
		// resource between iterations, or added it entirely), the whole
		// passing body is "new" relative to nothing. blockAdditions
		// handles both — pre-set failedBody to empty and the function
		// returns every passing line.
		if diff := blockAdditions(failedBody, passingBlock.Body); diff != "" {
			pieces = append(pieces, fmt.Sprintf("resource %q %q {\n%s\n}", resourceType, blockName(passingBlock), diff))
		}
	}
	if len(pieces) == 0 {
		// Failing resource present + unchanged, but new sibling resources
		// might reference its address (e.g. iter 1 had google_storage_bucket
		// only, iter 2 added a google_kms_crypto_key that the bucket then
		// references). Phase 1 doesn't emit in that case; phase 2 might.
		return nil, nil
	}

	// Phase 2 extension: include sibling NEW resources referenced from
	// the failing block's added attributes. The body diff above already
	// captures the references; we surface the sibling block bodies so
	// the LLM sees the full pattern, not just the reference.
	newSiblings := newResourceAddresses(failedResources, passingResources)
	referenced := referencesIn(strings.Join(pieces, "\n"), newSiblings)
	for _, sib := range referenced {
		if blk, ok := passingResources[sib]; ok {
			sibType, sibName, _ := splitAddress(sib)
			pieces = append(pieces, fmt.Sprintf("resource %q %q {\n%s\n}", sibType, sibName, indent(strings.TrimSpace(blk.Body))))
		}
	}

	snippet := strings.Join(pieces, "\n")
	snippet = trimSnippet(snippet, snippetMaxBytes)
	if snippet == "" {
		return nil, nil
	}
	fix.Snippet = snippet

	rule := buildRule(fix, failureDetail)

	return &LearnedPitfall{
		Resource:       fix.Resource,
		Rule:           rule,
		Source:         PrescriptiveSource,
		DiscoveredFrom: scenario,
	}, nil
}

// resourceBlock is the extracted shape of one `resource "T" "N" {…}`
// block: the header line preserved (for the name) and the body
// (everything between the outermost braces, indentation preserved).
type resourceBlock struct {
	Type string
	Name string
	Body string
}

// loadResourceBlocks reads every *.tf file under dir, extracts each
// resource block, and returns a map keyed by bare address.
func loadResourceBlocks(dir string) (map[string]*resourceBlock, error) {
	out := make(map[string]*resourceBlock)
	if dir == "" {
		return out, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tf") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		for _, blk := range extractResourceBlocks(string(data)) {
			addr := blk.Type + "." + blk.Name
			out[addr] = blk
		}
	}
	return out, nil
}

// extractResourceBlocks scans HCL text for `resource "T" "N" { ... }`
// blocks. It tracks brace depth — naïvely sufficient for HCL since
// braces inside strings are escaped and Terraform doesn't permit
// `{` literals outside quoted strings or heredocs at the top level
// of a resource body.
//
// Heredoc-safe enough for current scenarios: if a heredoc contains
// `{` it must be paired with `}` to be valid HCL, so brace counting
// remains balanced even if the parse is technically unaware.
func extractResourceBlocks(src string) []*resourceBlock {
	var out []*resourceBlock
	lines := strings.Split(src, "\n")
	for i := 0; i < len(lines); i++ {
		m := resourceHeaderRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		depth := 1
		var body []string
		j := i + 1
		for ; j < len(lines); j++ {
			depth += strings.Count(lines[j], "{") - strings.Count(lines[j], "}")
			if depth <= 0 {
				break
			}
			body = append(body, lines[j])
		}
		out = append(out, &resourceBlock{
			Type: m[1],
			Name: m[2],
			Body: strings.Join(body, "\n"),
		})
		i = j
	}
	return out
}

// blockAdditions returns the lines present in `passing` but absent
// from `failed`. Line-based diff suffices for HCL because attribute
// assignments are line-scoped and noise (whitespace, comments) is
// stable across iterations when the LLM uses the same prompt.
//
// Returned text is the literal slice of new lines preserving
// indentation, ready to be re-emitted inside a `resource {}` block.
func blockAdditions(failed, passing string) string {
	failedSet := make(map[string]struct{})
	for _, line := range strings.Split(failed, "\n") {
		failedSet[normalizeLine(line)] = struct{}{}
	}
	var out []string
	for _, line := range strings.Split(passing, "\n") {
		key := normalizeLine(line)
		if key == "" {
			continue
		}
		if _, seen := failedSet[key]; seen {
			continue
		}
		// Skip comment-only additions and stray closing braces.
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") || strings.HasPrefix(t, "//") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// normalizeLine trims whitespace + collapses inner runs of spaces so
// reformatting (e.g., the LLM aligning '=') doesn't register as a
// diff.
func normalizeLine(line string) string {
	t := strings.TrimSpace(line)
	if t == "" {
		return ""
	}
	for strings.Contains(t, "  ") {
		t = strings.ReplaceAll(t, "  ", " ")
	}
	return t
}

// blockName returns the resource Name from a resourceBlock.
func blockName(b *resourceBlock) string {
	if b == nil {
		return ""
	}
	return b.Name
}

// newResourceAddresses returns addresses present in `passing` that
// are absent from `failed` — i.e., resources the LLM added across
// the iteration boundary.
func newResourceAddresses(failed, passing map[string]*resourceBlock) []string {
	var out []string
	for addr := range passing {
		if _, existed := failed[addr]; !existed {
			out = append(out, addr)
		}
	}
	sort.Strings(out)
	return out
}

// removedResourceAddresses is the dual of newResourceAddresses:
// addresses present in `failed` that disappear from `passing`. Used
// by the N13 deletion-as-fix extractor to spot top-level resource
// removals that correlate with failure clearance.
func removedResourceAddresses(failed, passing map[string]*resourceBlock) []string {
	var out []string
	for addr := range failed {
		if _, stillThere := passing[addr]; !stillThere {
			out = append(out, addr)
		}
	}
	sort.Strings(out)
	return out
}

// blockRemovals is the dual of blockAdditions: returns lines present
// in `failed` but absent from `passing`. Same normalization rules
// (whitespace-collapse + comment skip) so reformatting doesn't
// register as a deletion.
func blockRemovals(failed, passing string) string {
	passingSet := make(map[string]struct{})
	for _, line := range strings.Split(passing, "\n") {
		passingSet[normalizeLine(line)] = struct{}{}
	}
	var out []string
	for _, line := range strings.Split(failed, "\n") {
		key := normalizeLine(line)
		if key == "" {
			continue
		}
		if _, seen := passingSet[key]; seen {
			continue
		}
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") || strings.HasPrefix(t, "//") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// attributeNameRe matches an HCL attribute assignment line and
// captures the attribute name (left of `=`). Used by the N13
// deletion-as-fix extractor to confirm a removed line's identifier
// appears in the failure detail. Heredoc/nested-block headers are
// rejected because they end in `{`.
var attributeNameRe = regexp.MustCompile(`^\s*([a-z_][a-z0-9_]*)\s*=`)

// extractAttributeName pulls the attribute name (lhs of `=`) from an
// HCL line, or returns "" if the line isn't a scalar assignment.
func extractAttributeName(line string) string {
	m := attributeNameRe.FindStringSubmatch(line)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// attributeAppearsInDetail tests whether an HCL attribute name (always
// snake_case in Terraform) appears in a failure detail string. The
// match is case-insensitive on the snake_case form AND additionally
// tries the camelCase equivalent — AWS provider errors frequently
// echo the JSON-side field name (`MapPublicIpOnLaunch`) while the HCL
// attribute is `map_public_ip_on_launch`. The S63 sweep's aws_subnet
// false-positive surfaced this gap: N13 saw the failing iter remove
// `map_public_ip_on_launch` but couldn't attribute it because
// `strings.Contains(detail, attr)` returned false on the camelCase
// failure detail.
func attributeAppearsInDetail(detail, attr string) bool {
	if strings.Contains(detail, attr) {
		return true
	}
	// Case-insensitive snake_case (cheap; covers errors echoing the
	// HCL attribute back with different casing).
	if strings.Contains(strings.ToLower(detail), attr) {
		return true
	}
	// camelCase equivalent (`foo_bar_baz` → `FooBarBaz` / `fooBarBaz`).
	if camel := snakeToCamel(attr); camel != "" {
		if strings.Contains(detail, camel) {
			return true
		}
		// Lower-camel variant: `MapPublicIpOnLaunch` → `mapPublicIpOnLaunch`.
		lowerCamel := strings.ToLower(camel[:1]) + camel[1:]
		if strings.Contains(detail, lowerCamel) {
			return true
		}
	}
	return false
}

// snakeToCamel converts `map_public_ip_on_launch` → `MapPublicIpOnLaunch`.
// Empty input returns "". Names that don't contain `_` round-trip with
// only the first letter capitalised.
func snakeToCamel(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "_")
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		if len(p) > 1 {
			b.WriteString(p[1:])
		}
	}
	return b.String()
}

// ExtractPrescriptiveAvoid is the N13 deletion-as-fix companion to
// ExtractPrescriptiveFix. When the LLM cleared a failure by REMOVING
// HCL (an attribute the provider rejected, a resource that escapes
// to real cloud, etc.), the legitimate fix is "do NOT use <thing>"
// rather than "add <thing>." This extractor emits the avoid form.
//
// Heuristic for attribution:
//
//	(a) Attribute-level: the LLM removed an attribute from the failing
//	    resource's body, AND the attribute name appears verbatim in
//	    the failure detail. This is the safest signal — the provider
//	    or policy named the offending attribute, the LLM dropped it,
//	    the failure cleared.
//	(b) Resource-level: the LLM removed every top-level resource of
//	    a given type, AND that resource type appears in the failure
//	    detail. Covers patterns like dropping every
//	    `google_project_service` to escape the auth-pipeline preflight.
//
// Returns nil when no productive deletion can be attributed — the
// pipeline never emits noise on cases where ExtractPrescriptiveFix
// already handled the fix.
func ExtractPrescriptiveAvoid(failedDir, passingDir string, failureDetail, failureResource, cloud, scenario, timestamp string) (*LearnedPitfall, error) {
	failedResources, err := loadResourceBlocks(failedDir)
	if err != nil {
		return nil, fmt.Errorf("read failed dir %q: %w", failedDir, err)
	}
	passingResources, err := loadResourceBlocks(passingDir)
	if err != nil {
		return nil, fmt.Errorf("read passing dir %q: %w", passingDir, err)
	}

	address := failureResource
	if address == "" {
		address = firstResourceAddress(failureDetail)
	}
	// Resource type for the entry. Fall back to a removed-resource
	// hint when we can't resolve from failure metadata (rare; usually
	// the address is present for apply/validate errors).
	resourceType := ""
	if address != "" {
		if rt, _, ok := splitAddress(address); ok {
			resourceType = rt
		}
	}

	// Case (a): attribute-level removal.
	var avoidAttrs []string
	if address != "" {
		failedBlock, hasFailed := failedResources[address]
		passingBlock, hasPassing := passingResources[address]
		if hasFailed && hasPassing {
			removed := blockRemovals(failedBlock.Body, passingBlock.Body)
			seen := make(map[string]struct{})
			for _, line := range strings.Split(removed, "\n") {
				attr := extractAttributeName(line)
				if attr == "" {
					continue
				}
				if _, dup := seen[attr]; dup {
					continue
				}
				// Strict attribution: the attribute name MUST appear in
				// the failure detail. This filters unrelated whitespace
				// rewrites + LLM refactors. Matched both as-written (HCL
				// is snake_case) and in camelCase form — AWS API errors
				// echo back the JSON-side name (`MapPublicIpOnLaunch`)
				// even though the HCL attribute is `map_public_ip_on_launch`.
				// The S63 sweep's aws_subnet false-positive surfaced this
				// gap.
				if attributeAppearsInDetail(failureDetail, attr) {
					avoidAttrs = append(avoidAttrs, attr)
					seen[attr] = struct{}{}
				}
			}
		}
	}

	// Case (b): top-level resource-level removal. Looks for resource
	// types where the failing iter had at least one instance and the
	// passing iter has zero, AND the resource type name appears in
	// the failure detail.
	var avoidResourceTypes []string
	removed := removedResourceAddresses(failedResources, passingResources)
	if len(removed) > 0 {
		typeCounts := make(map[string]int)
		for _, addr := range removed {
			if rt, _, ok := splitAddress(addr); ok {
				typeCounts[rt]++
			}
		}
		// Only emit when ALL instances of the type were removed —
		// avoids treating a partial cleanup as an avoid signal.
		passingHasType := make(map[string]bool)
		for addr := range passingResources {
			if rt, _, ok := splitAddress(addr); ok {
				passingHasType[rt] = true
			}
		}
		for rt := range typeCounts {
			if passingHasType[rt] {
				continue
			}
			if !strings.Contains(failureDetail, rt) {
				continue
			}
			avoidResourceTypes = append(avoidResourceTypes, rt)
		}
		sort.Strings(avoidResourceTypes)
	}

	if len(avoidAttrs) == 0 && len(avoidResourceTypes) == 0 {
		return nil, nil
	}

	// Attribute-removal entries are keyed on the FAILING resource type
	// so the LLM sees them when generating that resource. Resource-
	// removal entries are keyed on the removed type itself ("do NOT
	// use this resource type").
	emitResource := resourceType
	if emitResource == "" && len(avoidResourceTypes) > 0 {
		emitResource = avoidResourceTypes[0]
	}
	if emitResource == "" {
		return nil, nil
	}

	rule := buildAvoidRule(emitResource, avoidAttrs, avoidResourceTypes, failureDetail, scenario)
	return &LearnedPitfall{
		Resource:       emitResource,
		Rule:           rule,
		Source:         PrescriptiveAvoidSource,
		DiscoveredFrom: scenario,
	}, nil
}

// buildAvoidRule composes the human-readable "do NOT use" rule. Keeps
// the format close to buildRule's shape so the prompt-injected pitfall
// list reads consistently.
func buildAvoidRule(resource string, attrs, resourceTypes []string, failureDetail, scenario string) string {
	summary := firstSentence(failureDetail)
	if summary == "" {
		summary = fmt.Sprintf("%s failure cleared by deletion.", resource)
	}
	var avoid []string
	for _, a := range attrs {
		avoid = append(avoid, fmt.Sprintf("attribute `%s`", a))
	}
	for _, t := range resourceTypes {
		avoid = append(avoid, fmt.Sprintf("resource type `%s`", t))
	}
	avoidJoined := strings.Join(avoid, " and ")
	return fmt.Sprintf("%s Do NOT use %s on `%s` — observed in scenario %q to cause the failure above.",
		summary, avoidJoined, resource, scenario)
}

// referencesIn returns the subset of `candidates` whose address
// appears (as a literal substring of the form `TYPE.NAME`) inside
// the given HCL text. Used to detect which NEW resources are
// referenced from the failing resource's new body lines — those
// are the prescriptive companion resources we want in the snippet.
func referencesIn(text string, candidates []string) []string {
	var out []string
	for _, c := range candidates {
		if strings.Contains(text, c+".") || strings.Contains(text, c+"\n") || strings.Contains(text, c+" ") {
			out = append(out, c)
		}
	}
	return out
}

// indent prefixes every line of `body` with two spaces, so the body
// nests correctly when wrapped inside a `resource {}` declaration.
func indent(body string) string {
	if body == "" {
		return ""
	}
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		if l == "" {
			continue
		}
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n")
}

// trimSnippet truncates the snippet at the last top-level block
// boundary before the cap and appends an explicit truncation marker.
// Avoids leaving an unbalanced `depends_on = [` or partial nested
// block, which would render the example unparseable as HCL.
//
// The strategy: prefer cutting after a column-0 `}` (closes a
// top-level `resource` block). Fall back to the last full line if no
// such boundary exists within the cap.
func trimSnippet(s string, max int) string {
	if len(s) <= max {
		return s
	}
	// Prefer cut after a column-0 `}` — that's a top-level resource
	// block close. The literal we look for is "\n}\n" which leaves the
	// snippet ending with the closing brace on its own line.
	if idx := strings.LastIndex(s[:max], "\n}\n"); idx > 0 {
		return s[:idx+3] + "# ... (truncated)\n"
	}
	cut := strings.LastIndex(s[:max], "\n")
	if cut <= 0 {
		cut = max
	}
	return s[:cut] + "\n# ... (truncated)"
}

// firstResourceAddress extracts the first `TYPE.NAME` (optionally
// indexed) from a failure detail. Returns "" if none found.
func firstResourceAddress(detail string) string {
	m := addressRe.FindStringSubmatch(detail)
	if len(m) < 3 {
		return ""
	}
	return m[1] + "." + m[2]
}

// detailTypeHints maps human-readable failure detail prefixes to the
// terraform resource type the policy is talking about. Lets the
// extractor handle state-side policy failures (e.g. "Cloud SQL
// instance NAME missing X") that don't include a terraform address.
// Order matters — longer / more specific phrases first so they win
// over generic substrings.
var detailTypeHints = []struct {
	phrase string
	rtype  string
}{
	{"Cloud SQL instance", "google_sql_database_instance"},
	{"storage bucket", "google_storage_bucket"},
	{"Compute Disk", "google_compute_disk"},
	{"compute disk", "google_compute_disk"},
	{"GKE cluster", "google_container_cluster"},
	{"Memorystore", "google_redis_instance"},
}

// inferResourceTypeFromDetail returns the terraform resource type the
// failure detail describes, or "" if no known hint matches. Used as
// fallback attribution when the detail lacks a parseable address.
func inferResourceTypeFromDetail(detail string) string {
	for _, h := range detailTypeHints {
		if strings.Contains(detail, h.phrase) {
			return h.rtype
		}
	}
	return ""
}

// changedResourcesOfType returns addresses in `passing` whose type
// matches `rtype` and whose body differs from the corresponding
// `failed` block (or is new). The caller uses this when address
// attribution from the detail string fails — exactly one match is
// the unambiguous signal that the LLM's fix targets that resource.
func changedResourcesOfType(failed, passing map[string]*resourceBlock, rtype string) []string {
	var out []string
	for addr, p := range passing {
		if p.Type != rtype {
			continue
		}
		f, ok := failed[addr]
		if !ok || normalizeBody(f.Body) != normalizeBody(p.Body) {
			out = append(out, addr)
		}
	}
	sort.Strings(out)
	return out
}

// normalizeBody collapses whitespace per line so trivial reformatting
// (alignment changes the LLM might make) doesn't register as a
// content change.
func normalizeBody(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if n := normalizeLine(line); n != "" {
			out = append(out, n)
		}
	}
	return strings.Join(out, "\n")
}

// splitAddress splits "TYPE.NAME" into ("TYPE", "NAME", true). The
// third return is false if the input doesn't have exactly one dot.
func splitAddress(addr string) (string, string, bool) {
	idx := strings.Index(addr, ".")
	if idx <= 0 || idx == len(addr)-1 {
		return "", "", false
	}
	if strings.Index(addr[idx+1:], ".") >= 0 {
		// Embedded attribute access like google_storage_bucket.app.id —
		// strip everything from the second dot onwards.
		addr = addr[:idx+1+strings.Index(addr[idx+1:], ".")]
	}
	return addr[:idx], addr[idx+1:], true
}

// buildRule composes the human-readable pitfall rule from a fix +
// the original failure detail. The rule starts with the bare
// failure summary so the LLM can match recurrence, then includes
// the HCL snippet that resolved it.
//
// Format kept terse to fit inside pitfalls/<cloud>.yaml without
// blowing up the prompt injection.
func buildRule(fix PrescriptiveFix, failureDetail string) string {
	summary := firstSentence(failureDetail)
	if summary == "" {
		summary = fmt.Sprintf("%s failure cleared by HCL change shown below.", fix.Resource)
	}
	return fmt.Sprintf("%s Fix observed in scenario %q: add the following HCL.\n%s",
		summary, fix.Scenario, fix.Snippet)
}

// firstSentence returns the first reasonable summary line from a
// (potentially multi-line, ANSI-decorated) failure detail. The goal
// is one human-readable sentence the LLM can match against
// recurring failures.
func firstSentence(detail string) string {
	d := strings.TrimSpace(detail)
	if d == "" {
		return ""
	}
	for _, ln := range strings.Split(d, "\n") {
		ln = strings.TrimSpace(strings.TrimLeft(ln, "│╷╵"))
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "Error:") {
			ln = strings.TrimSpace(strings.TrimPrefix(ln, "Error:"))
		}
		if ln != "" {
			if len(ln) > 240 {
				ln = ln[:237] + "..."
			}
			return ln
		}
	}
	return ""
}
