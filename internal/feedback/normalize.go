package feedback

import (
	"regexp"
	"sort"
	"strings"
)

// NormalizeDetail returns a stable canonical form of a failure detail
// string suitable for equality/hashing in oscillation and stuck
// detection. The LLM regenerates HCL on every iteration so the SAME
// underlying bug shows up with shifting line numbers, instance suffixes
// (`web_0` vs `web[*]`), and slightly different provider-error phrasing.
// Without normalization, FailureSignature{}.Detail differs across
// iterations and DetectOscillation never sees a recurring problem.
//
// Strategy:
//
//  1. For known error families (Unsupported attribute / Unsupported
//     argument / "at least one of"), return a compact kernel that
//     encodes only the load-bearing parts (attribute/argument names,
//     resource type). This collapses cosmetic variation across
//     iterations into one signature.
//  2. Otherwise strip noise: the `exit status N | stderr:` envelope,
//     terraform's box-drawing border characters, `line N` references,
//     count.index-style suffixes (`name_0`, `name[1]`, `name[*]`),
//     and collapse runs of whitespace.
//
// The original Detail is preserved on the Failure struct — only the
// signature view is normalized. Downstream learning (ExtractDescriptivePitfall)
// is called with the kernel where it's more robust (templates match
// substrings) or with the original Detail when the all-iterations
// scan in run_command.go iterates raw failures directly.
func NormalizeDetail(detail string) string {
	if detail == "" {
		return ""
	}

	// Terraform diagnostics embed `│` box-drawing characters between
	// every line of the error body — these break regex matchers that
	// expect `\s+` to bridge "attribute named ..." to its quoted
	// argument. Strip the envelope + box-drawing first so kernel
	// detectors see a clean text view.
	cleaned := exitStatusEnvelopeRe.ReplaceAllString(detail, "")
	cleaned = boxDrawingRe.ReplaceAllString(cleaned, " ")

	if k := unsupportedAttributeKernel(cleaned); k != "" {
		return k
	}
	if k := unsupportedArgumentKernel(cleaned); k != "" {
		return k
	}
	if k := atLeastOneOfKernel(cleaned); k != "" {
		return k
	}

	s := lineRefRe.ReplaceAllString(cleaned, "")
	s = instanceSuffixRe.ReplaceAllString(s, "$1")
	s = instanceBracketRe.ReplaceAllString(s, "$1")
	s = whitespaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

var (
	// "exit status 1 | stderr:" — every tofu apply/validate failure
	// is wrapped in this envelope; differing exit codes are noise.
	exitStatusEnvelopeRe = regexp.MustCompile(`(?i)exit status \d+\s*\|\s*stderr:\s*`)

	// Terraform diagnostics frames its errors with these characters.
	boxDrawingRe = regexp.MustCompile("[╷╵│─]+")

	// "on file.tf line 21, in resource ..." (drops the line ref) plus
	// terraform's diagnostic gutter "  21:   server_ips = ..." (drops
	// the "21:" gutter prefix). Go's regexp has no lookahead, so we
	// just match the gutter without asserting the trailing non-space —
	// any false positives (e.g. timestamps) collapse with whitespace.
	lineRefRe = regexp.MustCompile(`(?i)\bline\s+\d+\b[,:]?|\b\d+:\s`)

	// count.index-flattened: `name_0`, `name_1`, ... Strip the trailing
	// digit suffix when preceded by an underscore.
	instanceSuffixRe = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*?)_\d+\b`)

	// for_each / count selectors: `name[0]`, `name[1]`, `name[*]`.
	instanceBracketRe = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\[(?:\d+|\*)\]`)

	whitespaceRe = regexp.MustCompile(`\s+`)

	// Captures the bad attribute name in:
	//   does not have an attribute named "private_ip"
	//   no argument, nested block, or exported attribute named "private_ip"
	attrNameRe = regexp.MustCompile(`(?i)attribute\s+named\s+"([A-Za-z0-9_]+)"`)

	// Captures the bad argument name in:
	//   Unsupported argument "labels"
	unsupportedArgNameRe = regexp.MustCompile(`(?i)Unsupported argument[^"]*"([A-Za-z0-9_]+)"`)

	// All resource type names mentioned in the detail.
	anyResourceRe = regexp.MustCompile(`\b(?:scaleway|google|aws)_[A-Za-z0-9_]+\b`)

	// "at least one of 'ip_net' or 'enable_ipam'" — capture the list.
	atLeastOneOfBodyRe = regexp.MustCompile(`(?i)at least one of\s+(.+?)(?:\s+must|\s+should|\s*\()`)
)

func unsupportedAttributeKernel(detail string) string {
	lower := strings.ToLower(detail)
	// Two distinct phrasings produce this error family:
	//   `Unsupported attribute` (block/index access)
	//   `no argument, nested block, or exported attribute named` (resource attribute)
	if !strings.Contains(lower, "unsupported attribute") &&
		!strings.Contains(lower, "exported attribute named") {
		return ""
	}
	attrs := captureAll(attrNameRe, detail, 1)
	if len(attrs) == 0 {
		return ""
	}
	resources := uniqSorted(anyResourceRe.FindAllString(detail, -1))
	return "kernel:unsupported_attribute:" + strings.Join(resources, ",") + ":" + strings.Join(uniqSorted(attrs), ",")
}

func unsupportedArgumentKernel(detail string) string {
	lower := strings.ToLower(detail)
	if !strings.Contains(lower, "unsupported argument") {
		return ""
	}
	args := captureAll(unsupportedArgNameRe, detail, 1)
	if len(args) == 0 {
		return ""
	}
	resources := uniqSorted(anyResourceRe.FindAllString(detail, -1))
	return "kernel:unsupported_argument:" + strings.Join(resources, ",") + ":" + strings.Join(uniqSorted(args), ",")
}

func atLeastOneOfKernel(detail string) string {
	m := atLeastOneOfBodyRe.FindStringSubmatch(detail)
	if len(m) < 2 {
		return ""
	}
	body := whitespaceRe.ReplaceAllString(strings.TrimSpace(m[1]), " ")
	resources := uniqSorted(anyResourceRe.FindAllString(detail, -1))
	return "kernel:at_least_one_of:" + strings.Join(resources, ",") + ":" + body
}

func captureAll(re *regexp.Regexp, s string, group int) []string {
	matches := re.FindAllStringSubmatch(s, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > group {
			out = append(out, m[group])
		}
	}
	return out
}

func uniqSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
