package generator

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
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
// Nothing registered still scrubs ARN account fields and plain 12-digit
// runs (accountSpans).
//
// It returns a func that puts the registry back as it was before this
// call. Production callers ignore it; tests use it (via scrubtest) so a
// registration cannot leak into the next test.
func RegisterScrubbedAccounts(ids ...string) (restore func()) {
	accountsMu.Lock()
	defer accountsMu.Unlock()
	before := slices.Clone(knownAccountIDs)
	restore = func() {
		accountsMu.Lock()
		defer accountsMu.Unlock()
		knownAccountIDs = before
		knownAccountPattern.Store(nil)
		if len(before) > 0 {
			knownAccountPattern.Store(knownAccountsPattern(before))
		}
	}
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
	return restore
}

// RegisterConfigAccount registers aws.account_id from the config at path
// for a tool that does not load the full config (pitfall-merge,
// extract-pitfall). It reads only that field, leniently, and never
// fails: a missing or broken config is reported on warn, naming the
// path, and the scrub falls back to its ARN and plain 12-digit layers.
func RegisterConfigAccount(path string, warn io.Writer) {
	var cfg struct {
		AWS struct {
			AccountID string `yaml:"account_id"`
		} `yaml:"aws"`
	}
	body, err := os.ReadFile(path)
	if err == nil {
		err = yaml.Unmarshal(body, &cfg)
	}
	if err != nil {
		fmt.Fprintf(warn, "WARN: config %s not read (%v); scrubbing ARN account fields and plain 12-digit ids only\n", path, err)
		return
	}
	RegisterScrubbedAccounts(cfg.AWS.AccountID)
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
// replaces, in three layers:
//
//	(a) every occurrence of a registered id, in any form;
//	(b) every ARN account field (one followed by a digit is not one);
//	(c) any other run of exactly 12 digits not touching another digit,
//	    unless its whole alphanumeric token is lowercase hex of 16+
//	    characters (a digest, a commit sha, snap-/vol-/eni- ids), it is
//	    a UUID's last group, or it is a public AMI owner (publicAMIOwners).
//
// (a) and (b) are precise; (c) keeps the scrub failing closed when no
// account is registered or an error names another account in prose. A
// bare 12-digit byte count is scrubbed by (c): that costs a number,
// failing open would cost an account.
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
	notIDs := append(hexTokenSpans(s), uuidPattern.FindAllStringIndex(s, -1)...)
	for _, m := range digitRunPattern.FindAllStringIndex(s, -1) {
		if m[1]-m[0] == accountIDDigits && !insideAny(m, notIDs) && !publicAMIOwners[s[m[0]:m[1]]] {
			spans = append(spans, m)
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

// ScrubAccountIDs replaces the spans accountSpans finds (registered ids,
// ARN account fields, other standalone 12-digit runs) with ACCOUNT_ID, the placeholder
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

// publicAMIOwners are the published AMI-owner accounts of OS vendors.
// They are content, not secrets: a learned rule's `owners` filter names
// them, and scrubbing one would break the rule. Only layer (c) honours
// this; a registered id or an ARN account field is scrubbed even if
// listed. Each is confirmed by the vendor's own documentation:
var publicAMIOwners = map[string]bool{
	"099720109477": true, // Canonical (Ubuntu); documentation.ubuntu.com "Find Ubuntu images on AWS"
	"309956199498": true, // Red Hat (RHEL); access.redhat.com/solutions/15356
	"136693071363": true, // Debian; wiki.debian.org/Cloud/AmazonEC2Image
	"801119661308": true, // Amazon's Windows AMIs; AWS Tools for PowerShell user guide, "Find an AMI"
}

var (
	digitRunPattern   = regexp.MustCompile(`[0-9]+`)
	uuidPattern       = regexp.MustCompile(`[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}`)
	alnumTokenPattern = regexp.MustCompile(`[0-9A-Za-z]+`)
	hexTokenPattern   = regexp.MustCompile(`^[0-9a-f]{16,}$`)
)

func hexTokenSpans(s string) [][]int {
	var spans [][]int
	for _, m := range alnumTokenPattern.FindAllStringIndex(s, -1) {
		if hexTokenPattern.MatchString(s[m[0]:m[1]]) {
			spans = append(spans, m)
		}
	}
	return spans
}

func insideAny(span []int, spans [][]int) bool {
	for _, o := range spans {
		if span[0] >= o[0] && span[1] <= o[1] {
			return true
		}
	}
	return false
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

// ScrubStrings scrubs, in place, every string field and string element
// reachable from v (a pointer) through structs, pointers, slices and
// arrays, and returns how many it changed. It does not walk maps,
// interfaces or []byte: the pitfalls types hold none, and
// TestScrubbedTypesHoldOnlyWalkableFields fails the day one appears.
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
