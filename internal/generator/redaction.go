package generator

import (
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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

const (
	accountIDPlaceholder = "ACCOUNT_ID"
	accountIDDigits      = 12
	accountGroupDigits   = 4
)

// arnAccountPattern matches an ARN up to its account field, which is
// group 1: arn:<partition>:<service>:<region>:<account>.
var arnAccountPattern = regexp.MustCompile(`arn:[A-Za-z0-9-]+:[A-Za-z0-9-]+:[A-Za-z0-9-]*:([0-9]{12})`)

var (
	accountsMu          sync.Mutex
	knownAccountIDs     []string
	knownAccountPattern atomic.Pointer[regexp.Regexp]
)

// RegisterScrubbedAccounts adds the account ids the publish sinks scrub
// wherever they appear, in any form. It is called once the config is
// loaded (aws.account_id); an empty or malformed id is ignored, and ids
// accumulate, so a later load never un-scrubs an earlier account.
// Nothing registered means only ARN account fields are scrubbed.
func RegisterScrubbedAccounts(ids ...string) {
	accountsMu.Lock()
	defer accountsMu.Unlock()
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if !isAccountID(id) || slices.Contains(knownAccountIDs, id) {
			continue
		}
		knownAccountIDs = append(knownAccountIDs, id)
	}
	if len(knownAccountIDs) > 0 {
		knownAccountPattern.Store(knownAccountsPattern(knownAccountIDs))
	}
}

func isAccountID(id string) bool {
	return len(id) == accountIDDigits && strings.Trim(id, "0123456789") == ""
}

// knownAccountsPattern matches each id plain, in the console's 4-4-4
// grouping with '-' or ' ', and glued to anything on either side.
func knownAccountsPattern(ids []string) *regexp.Regexp {
	const sep = "[- ]?"
	alts := make([]string, len(ids))
	for i, id := range ids {
		alts[i] = id[:accountGroupDigits] + sep + id[accountGroupDigits:2*accountGroupDigits] + sep + id[2*accountGroupDigits:]
	}
	return regexp.MustCompile(strings.Join(alts, "|"))
}

// accountSpans returns the sorted, merged byte spans ScrubAccountIDs
// replaces: every occurrence of a known id, and every ARN account field
// (one followed by another digit is not an account and is left).
func accountSpans(s string, known *regexp.Regexp) [][]int {
	var spans [][]int
	if known != nil {
		spans = known.FindAllStringIndex(s, -1)
	}
	for _, m := range arnAccountPattern.FindAllStringSubmatchIndex(s, -1) {
		if !digitAt(s, m[3]) {
			spans = append(spans, m[2:4])
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
	var merged [][]int
	for _, sp := range spans {
		if n := len(merged); n > 0 && sp[0] < merged[n-1][1] {
			merged[n-1][1] = max(merged[n-1][1], sp[1])
			continue
		}
		merged = append(merged, []int{sp[0], sp[1]})
	}
	return merged
}

// ScrubAccountIDs replaces every registered account id (any form) and the
// account field of every ARN with ACCOUNT_ID, the placeholder
// iam-policy.json uses. It runs at the publish sinks (pitfall and ledger
// writers, gap writers, the pitfalls PUT); run diagnostics keep the real
// id, and their cuts use CutText so no partial id reaches a sink.
func ScrubAccountIDs(s string) string {
	return scrubWith(s, knownAccountPattern.Load())
}

func scrubWith(s string, known *regexp.Regexp) string {
	spans := accountSpans(s, known)
	if len(spans) == 0 {
		return s
	}
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

func digitAt(s string, i int) bool {
	return i >= 0 && i < len(s) && s[i] >= '0' && s[i] <= '9'
}

// CutText returns the longest prefix of s of at most n bytes that ends on
// a rune boundary and outside every span ScrubAccountIDs would replace:
// a cut inside an account id leaves a shorter run no scrub recognises,
// so the cut backs off to before it. Other text is cut where it falls.
func CutText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	for _, sp := range accountSpans(s, knownAccountPattern.Load()) {
		if sp[0] < cut && cut < sp[1] {
			cut = sp[0]
		}
	}
	return s[:cut]
}

// ScrubStrings scrubs every string reachable from v, which must be a
// pointer, in place, and returns how many it changed. One walk covers
// every field, so no list of fields can fall behind the types.
func ScrubStrings(v any) int {
	return scrubValue(reflect.ValueOf(v))
}

func scrubValue(v reflect.Value) int {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return 0
		}
		return scrubValue(v.Elem())
	case reflect.Interface:
		if v.IsNil() || !v.CanSet() {
			return 0
		}
		return scrubCopy(v.Elem(), v.Set)
	case reflect.Struct:
		n := 0
		for i := 0; i < v.NumField(); i++ {
			n += scrubValue(v.Field(i))
		}
		return n
	case reflect.Slice, reflect.Array:
		n := 0
		for i := 0; i < v.Len(); i++ {
			n += scrubValue(v.Index(i))
		}
		return n
	case reflect.Map:
		n := 0
		for _, k := range v.MapKeys() {
			n += scrubCopy(v.MapIndex(k), func(e reflect.Value) { v.SetMapIndex(k, e) })
		}
		return n
	case reflect.String:
		if !v.CanSet() {
			return 0
		}
		if scrubbed := ScrubAccountIDs(v.String()); scrubbed != v.String() {
			v.SetString(scrubbed)
			return 1
		}
	}
	return 0
}

// scrubCopy scrubs an unaddressable value through an addressable copy.
func scrubCopy(e reflect.Value, set func(reflect.Value)) int {
	c := reflect.New(e.Type()).Elem()
	c.Set(e)
	n := scrubValue(c)
	if n > 0 {
		set(c)
	}
	return n
}
