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
