package generator

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	bearerTokenPattern     = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._-]+`)
	openRouterTokenPattern = regexp.MustCompile(`\bsk-or-v1-[A-Za-z0-9_-]+\b`)
	scenarioPattern        = regexp.MustCompile(`(?i)scenario:\s*[A-Za-z0-9._-]+`)
	secretAssignPattern    = regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password)\b\s*[:=]\s*("[^"]*"|'[^']*'|[^\s,;]+)`)
)

func redactTransportDetail(detail string, prompt string, env map[string]string, extraSecrets ...string) string {
	redacted := detail

	secrets := make([]string, 0, len(extraSecrets)+2)
	if prompt != "" {
		secrets = append(secrets, prompt)
	}
	for _, secret := range extraSecrets {
		if secret != "" {
			secrets = append(secrets, secret)
		}
	}
	for _, v := range env {
		if v != "" {
			secrets = append(secrets, v)
		}
	}
	for _, secret := range secrets {
		redacted = strings.ReplaceAll(redacted, secret, "[REDACTED]")
	}

	redacted = bearerTokenPattern.ReplaceAllString(redacted, "Bearer [REDACTED]")
	redacted = openRouterTokenPattern.ReplaceAllString(redacted, "sk-or-v1-[REDACTED]")
	redacted = scenarioPattern.ReplaceAllString(redacted, "scenario: [REDACTED]")
	redacted = secretAssignPattern.ReplaceAllString(redacted, "$1=[REDACTED]")

	return strings.TrimSpace(redacted)
}

// RedactSecretLikeText applies deterministic secret redaction suitable for
// persisted diagnostics and operator-visible logs.
func RedactSecretLikeText(input string) string {
	return redactTransportDetail(input, "", nil)
}

// digitRunPattern finds the runs ScrubAccountIDs inspects. Only a digit
// bounds a run, so an id glued to letters (`cloudtrail123456789012`, a
// common bucket name) is still found.
var digitRunPattern = regexp.MustCompile(`[0-9]+`)

// groupedIDPattern is the console's form of an id: 1234-5678-9012, or
// with single spaces.
var groupedIDPattern = regexp.MustCompile(`[0-9]{4}[- ][0-9]{4}[- ][0-9]{4}`)

// uuidPattern matches a UUID, whose last group can be twelve digits.
var uuidPattern = regexp.MustCompile(`[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}`)

// alnumTokenPattern and hexTokenPattern find whole hex tokens of 16+
// characters: digests, commit shas and AWS resource ids
// (snap-0a123456789012bcd), which hold a 12-digit run by chance.
var (
	alnumTokenPattern = regexp.MustCompile(`[0-9A-Za-z]+`)
	hexTokenPattern   = regexp.MustCompile(`^[0-9a-f]{16,}$`)
)

const (
	accountIDPlaceholder = "ACCOUNT_ID"
	accountIDDigits      = 12
)

// ScrubAccountIDs replaces every run of exactly 12 digits (not touching
// another digit), and every 4-4-4 grouped run, with ACCOUNT_ID, the
// placeholder iam-policy.json uses. Learned pitfalls and policy gaps copy
// real failure text into published files, and an AWS error names the
// account in more shapes than a list could hold, so this matches the
// class. A UUID and a whole hex token of 16+ characters are not ids and
// are left alone. A bare 12-digit number (a byte count) is
// indistinguishable from an id and is scrubbed: failing closed costs a
// number, failing open an account. It runs at the publish sinks only;
// cuts on the way there use CutText, which never splits a digit run.
func ScrubAccountIDs(s string) string {
	keep := append(uuidPattern.FindAllStringIndex(s, -1), hexTokenSpans(s)...)
	var spans [][]int
	for _, m := range digitRunPattern.FindAllStringIndex(s, -1) {
		if m[1]-m[0] == accountIDDigits && !insideAny(m, keep) {
			spans = append(spans, m)
		}
	}
	for _, m := range groupedIDPattern.FindAllStringIndex(s, -1) {
		if !digitAt(s, m[0]-1) && !digitAt(s, m[1]) && !insideAny(m, keep) {
			spans = append(spans, m)
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })

	var b strings.Builder
	last := 0
	for _, m := range spans {
		b.WriteString(s[last:m[0]])
		b.WriteString(accountIDPlaceholder)
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

func hexTokenSpans(s string) [][]int {
	var spans [][]int
	for _, m := range alnumTokenPattern.FindAllStringIndex(s, -1) {
		if hexTokenPattern.MatchString(s[m[0]:m[1]]) {
			spans = append(spans, m)
		}
	}
	return spans
}

func digitAt(s string, i int) bool {
	return i >= 0 && i < len(s) && s[i] >= '0' && s[i] <= '9'
}

func insideAny(span []int, spans [][]int) bool {
	for _, o := range spans {
		if span[0] >= o[0] && span[1] <= o[1] {
			return true
		}
	}
	return false
}

// CutText returns the longest prefix of s of at most n bytes that ends on
// a rune boundary and outside any digit run (a 4-4-4 grouped run counts
// as one). A cut inside an account id would leave a shorter run that no
// scrub recognises, so the cut backs off to before the run instead.
func CutText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	inRun := digitAt(s, cut) || isGroupSep(s[cut]) && digitAt(s, cut+1)
	for inRun && cut > 0 && (digitAt(s, cut-1) || isGroupSep(s[cut-1]) && digitAt(s, cut-2)) {
		cut--
	}
	return s[:cut]
}

func isGroupSep(c byte) bool { return c == '-' || c == ' ' }
