package generator

import (
	"regexp"
	"strings"
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

// uuidPattern matches a UUID, whose last group can be twelve digits.
var uuidPattern = regexp.MustCompile(`[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}`)

const (
	accountIDPlaceholder = "ACCOUNT_ID"
	accountIDDigits      = 12
)

// ScrubAccountIDs replaces every run of exactly 12 digits (not touching
// another digit) with ACCOUNT_ID, the placeholder iam-policy.json uses.
// Learned pitfalls and policy gaps copy real failure text into published
// files, and an AWS error names the account in more shapes than a list
// could hold, so this matches the class. A UUID's last group is not an
// id and is left alone. A bare 12-digit number (a byte count) is
// indistinguishable from an id and is scrubbed: failing closed costs a
// number, failing open an account. Scrub BEFORE any cut: a cut inside an
// id leaves a shorter run this no longer recognises.
func ScrubAccountIDs(s string) string {
	uuids := uuidPattern.FindAllStringIndex(s, -1)
	var b strings.Builder
	last := 0
	for _, m := range digitRunPattern.FindAllStringIndex(s, -1) {
		if m[1]-m[0] != accountIDDigits || insideAny(m, uuids) {
			continue
		}
		b.WriteString(s[last:m[0]])
		b.WriteString(accountIDPlaceholder)
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

func insideAny(span []int, spans [][]int) bool {
	for _, o := range spans {
		if span[0] >= o[0] && span[1] <= o[1] {
			return true
		}
	}
	return false
}
