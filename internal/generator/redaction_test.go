package generator

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactTransportDetail(t *testing.T) {
	t.Parallel()

	detail := "Authorization: Bearer abc123 prompt=scenario: smoke key=sk-or-v1-super-secret env=my-secret"
	redacted := redactTransportDetail(detail, "scenario: smoke", map[string]string{"X": "my-secret"}, "sk-or-v1-super-secret")

	if strings.Contains(redacted, "abc123") {
		t.Fatalf("expected bearer token to be redacted, got %q", redacted)
	}
	if strings.Contains(redacted, "scenario: smoke") {
		t.Fatalf("expected prompt content to be redacted, got %q", redacted)
	}
	if strings.Contains(redacted, "sk-or-v1-super-secret") {
		t.Fatalf("expected openrouter token to be redacted, got %q", redacted)
	}
	if strings.Contains(redacted, "my-secret") {
		t.Fatalf("expected env secret to be redacted, got %q", redacted)
	}
}

// testAccountID is the account a test registers, as a command registers
// aws.account_id from its config.
const testAccountID = "123456789012"

// accountIDLeak is the shape a real AccessDenied gives a learned rule.
const accountIDLeak = "User arn:aws:iam::123456789012:user/x is not authorized to perform iam:CreateRole in account 123456789012"

func TestScrubAccountIDs(t *testing.T) {
	t.Parallel()

	known := knownAccountsPattern([]string{testAccountID})
	for in, want := range map[string]string{
		"arn:aws:iam::123456789012:user/x":           "arn:aws:iam::ACCOUNT_ID:user/x",
		"account 123456789012":                       "account ACCOUNT_ID",
		"role_123456789012 123456789012.dkr.ecr.aws": "role_ACCOUNT_ID ACCOUNT_ID.dkr.ecr.aws",
		"console 1234-5678-9012.":                    "console ACCOUNT_ID.",
		"console 1234 5678 9012":                     "console ACCOUNT_ID",
		"bucket abcd123456789012":                    "bucket abcdACCOUNT_ID",
		"cloudtrail123456789012":                     "cloudtrailACCOUNT_ID",
		"9123456789012":                              "9ACCOUNT_ID",
		"123456789012123456789012":                   "ACCOUNT_IDACCOUNT_ID",
		"1234-5678-9012 1234-5678-9012":              "ACCOUNT_ID ACCOUNT_ID",
		"\xffarn:aws:iam::123456789012:":             "\xffarn:aws:iam::ACCOUNT_ID:",
		// Any ARN's account field, registered or not.
		"arn:aws:sts::987654321098:assumed-role/x": "arn:aws:sts::ACCOUNT_ID:assumed-role/x",
		"arn:aws-us-gov:iam::987654321098:root":    "arn:aws-us-gov:iam::ACCOUNT_ID:root",
		"arn:aws:iam::987654321098":                "arn:aws:iam::ACCOUNT_ID",
	} {
		assert.Equal(t, want, scrubWith(in, known), in)
	}
}

// With an unrelated account registered, or none, the fail-closed layer
// still scrubs a plain 12-digit id, and leaves digests, resource ids,
// UUIDs and port lists alone.
func TestScrubAccountIDsLeavesOtherDigits(t *testing.T) {
	t.Parallel()

	known := knownAccountsPattern([]string{"111122223333"})
	digest := "ab3f123456789012c" + strings.Repeat("e", 47)
	for _, in := range []string{
		"sha256 " + digest,
		"snap-0a123456789012bcd",
		"ports 8080 8443 9090",
		"id 550e8400-e29b-41d4-a716-446655440000",
		"arn:aws:iam::9876543210987:root",
		"thirteen 1234567890123",
		"eleven 12345678901",
	} {
		assert.Equal(t, in, scrubWith(in, known), in)
		assert.Equal(t, in, scrubWith(in, nil), in)
	}
	for in, want := range map[string]string{
		"account 123456789012":             "account ACCOUNT_ID",
		"denied in 987654321098, not mine": "denied in ACCOUNT_ID, not mine",
		"cloudtrail123456789012":           "cloudtrailACCOUNT_ID",
		// Fail closed: a bare 12-digit count is indistinguishable from an id.
		"bytes 107374182400": "bytes ACCOUNT_ID",
	} {
		assert.Equal(t, want, scrubWith(in, nil), "nothing registered: "+in)
	}
}

func assertAccountIDsScrubbed(t *testing.T, path string, wantPlaceholders int) {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(body), testAccountID, path)
	assert.Equal(t, wantPlaceholders, strings.Count(string(body), "ACCOUNT_ID"), string(body))
}

func TestPitfallWritesScrubAccountIDs(t *testing.T) {
	t.Parallel()

	t.Run("learned pitfall", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, AppendPitfall(dir, "aws", LearnedPitfall{Resource: "aws_iam_role", Rule: accountIDLeak, DiscoveredFrom: "aws-web"}))
		assertAccountIDsScrubbed(t, filepath.Join(dir, "aws.yaml"), 2)
	})

	t.Run("live pitfall", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, AppendLivePitfall(dir, "aws", "unhealthy\x00health\x00probe failed",
			LearnedPitfall{Resource: "aws_lb", Rule: accountIDLeak, DiscoveredFrom: "aws-web"}, time.Now()))
		assertAccountIDsScrubbed(t, filepath.Join(dir, "aws.yaml"), 2)
	})

	t.Run("avoid ledger", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, appendAvoidLedgerRecord(dir, "aws", &AvoidLedger{Provider: "aws"}, AvoidLedgerRecord{
			Status: AvoidRecordRelearned, Resource: "aws_iam_role", Attributes: []string{"name"},
			LearnedLayer: "live", Rule: accountIDLeak, At: "2026-10-09T00:00:00Z",
		}))
		assertAccountIDsScrubbed(t, filepath.Join(avoidChecksDir(dir), "aws.yaml"), 2)
	})
}

// A raw candidate must still match the scrubbed entry written for it, or
// every run appends the same lesson again.
func TestScrubbedPitfallsStillDedup(t *testing.T) {
	t.Parallel()

	t.Run("learned rule", func(t *testing.T) {
		dir := t.TempDir()
		// Too few shared words once scrubbed for the word-share check, so
		// only the exact match can catch it.
		p := LearnedPitfall{Resource: "aws_iam_role", Rule: "arn:aws:iam::123456789012:role/x account 123456789012"}
		require.NoError(t, AppendPitfall(dir, "aws", p))
		require.NoError(t, AppendPitfall(dir, "aws", p))
		entries, err := LoadPitfallEntries(dir, "aws")
		require.NoError(t, err)
		assert.Len(t, entries, 1)
	})

	t.Run("live key", func(t *testing.T) {
		dir := t.TempDir()
		// YAML escapes the NUL as \0, which a scrub of the marshalled text
		// would read as a 13-digit run beside the id.
		key := "unhealthy\x00health\x00123456789012 probe failed"
		p := LearnedPitfall{Resource: "aws_lb", Rule: "probe failed"}
		require.NoError(t, AppendLivePitfall(dir, "aws", key, p, time.Now()))
		require.NoError(t, AppendLivePitfall(dir, "aws", key, p, time.Now()))
		entries, err := LoadPitfallEntries(dir, "aws")
		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assertAccountIDsScrubbed(t, filepath.Join(dir, "aws.yaml"), 1)
	})

	t.Run("live touch", func(t *testing.T) {
		dir := t.TempDir()
		p := LearnedPitfall{Resource: "aws_lb", Rule: accountIDLeak}
		require.NoError(t, AppendLivePitfall(dir, "aws", "probe failed", p, time.Now()))
		assert.NoError(t, TouchLivePitfall(dir, "aws", "aws_lb", accountIDLeak, time.Now()))
	})
}

// straddlingCut puts an account id across byte offset cut, so cutting
// there unscrubbed keeps its first six digits.
func straddlingCut(prefix string, cut int) string {
	return prefix + strings.Repeat("x", cut-7-len(prefix)) + " 123456789012 " + strings.Repeat("y", 300)
}

func TestTruncatedFailureTextKeepsNoAccountIDDigits(t *testing.T) {
	t.Parallel()

	learned := ExtractDescriptivePitfall(straddlingCut("aws_iam_role: ", 297), "aws-web")
	require.NotNil(t, learned)
	assert.NotContains(t, learned.Rule, "123456")

	assert.NotContains(t, firstSentence(straddlingCut("", 237)), "123456")

	dir := t.TempDir()
	require.NoError(t, AppendPolicyGap(dir, PolicyGap{
		Cloud: "aws", Policy: "aws.iam_scoped", Resource: "aws_iam_role",
		Scenario: "aws-web", Detail: straddlingCut("", 237), Timestamp: "20261009T120000Z",
	}))
	body, err := os.ReadFile(filepath.Join(dir, "policy-gaps.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(body), "123456")
}

// yaml.v3 writes a string that is not valid UTF-8 as base64 !!binary; the
// scrub runs on the Go strings first, so the encoding cannot hide an id.
func TestPitfallWriteScrubsInvalidUTF8Rule(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	scrubbed, err := WritePitfalls(filepath.Join(dir, "aws.yaml"), &PitfallsFile{Provider: "aws", Pitfalls: []PitfallEntry{
		{Resource: "aws_iam_role", Rule: "\xff " + accountIDLeak, Source: "static"},
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, scrubbed)
	entries, err := LoadPitfallEntries(dir, "aws")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.NotContains(t, entries[0].Rule, "123456789012")
	assert.Equal(t, 2, strings.Count(entries[0].Rule, accountIDPlaceholder))
}

// A byte cut inside a multi-byte rune leaves invalid UTF-8.
func TestTruncationKeepsValidUTF8(t *testing.T) {
	t.Parallel()

	for _, cut := range []int{maxGapDetailBytes, maxDescriptiveRuleBytes} {
		s := strings.Repeat("│", cut)
		got := ellipsize(s, cut)
		assert.True(t, utf8.ValidString(got), "cut %d: %q", cut, got)
		assert.LessOrEqual(t, len(got), cut)
	}

	learned := ExtractDescriptivePitfall("aws_iam_role: "+strings.Repeat("│", maxDescriptiveRuleBytes), "aws-web")
	require.NotNil(t, learned)
	assert.True(t, utf8.ValidString(learned.Rule))
	assert.True(t, utf8.ValidString(firstSentence("x"+strings.Repeat("é", maxGapDetailBytes))))
}

func TestGapRowsScrubEveryColumn(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, AppendPolicyGap(dir, PolicyGap{
		Cloud: "aws", Policy: "arn:aws:iam::123456789012:policy/p", Resource: "role_123456789012",
		Scenario: "web-123456789012", Detail: "d", Timestamp: "20261009T120000Z",
	}))
	assertAccountIDsScrubbed(t, filepath.Join(dir, "policy-gaps.md"), 3)

	require.NoError(t, AppendMockGap(dir, MockGap{
		Cloud: "aws", Signal: "arn:aws:iam::123456789012:root", Resource: "role_123456789012",
		Scenario: "web-123456789012", Detail: "d", Timestamp: "20261009T120000Z",
	}))
	assertAccountIDsScrubbed(t, filepath.Join(dir, "mock-gaps.md"), 3)
	// Re-appending the same gap dedups against the scrubbed row.
	require.NoError(t, AppendMockGap(dir, MockGap{
		Cloud: "aws", Signal: "arn:aws:iam::123456789012:root", Resource: "role_123456789012",
		Scenario: "web-123456789012", Detail: "d", Timestamp: "20261009T120000Z",
	}))
	assertAccountIDsScrubbed(t, filepath.Join(dir, "mock-gaps.md"), 3)
}

// A digest that holds a 12-digit run by chance, and a check id, are not
// accounts and survive a ledger write byte for byte.
func TestAvoidLedgerKeepsShapeDigest(t *testing.T) {
	t.Parallel()

	digest := "ab3f987654321098c" + strings.Repeat("e", 47)
	require.Len(t, digest, 64)
	const checkID = "20261009T120000Z-aws_iam_role" // the form pitfalls avoid-check writes
	dir := t.TempDir()
	require.NoError(t, appendAvoidLedgerRecord(dir, "aws", &AvoidLedger{Provider: "aws"}, AvoidLedgerRecord{
		Status: AvoidRecordRelearned, Resource: "aws_iam_role", Attributes: []string{"name"},
		LearnedLayer: "live", Rule: accountIDLeak, At: "2026-10-09T00:00:00Z",
		Check: &AvoidCheck{ID: checkID, At: "2026-10-09T00:00:00Z", ShapeSHA256: digest},
	}))
	ledger, err := ReadAvoidLedger(dir, "aws")
	require.NoError(t, err)
	require.Len(t, ledger.Records, 1)
	require.NotNil(t, ledger.Records[0].Check)
	assert.Equal(t, digest, ledger.Records[0].Check.ShapeSHA256)
	assert.Equal(t, checkID, ledger.Records[0].Check.ID)
	assert.NotContains(t, ledger.Records[0].Rule, testAccountID)
}

// No cut splits the registered id (any form) or an ARN account field: it
// is whole or absent. Other digits are cut where the cut falls.
func TestCutTextNeverSplitsAnAccountID(t *testing.T) {
	// The grouped forms are layer (a) only, so this registers the id.
	// scrubtest would be an import cycle from here; this is what it does.
	t.Cleanup(RegisterScrubbedAccounts(testAccountID))

	for _, id := range []string{testAccountID, "1234-5678-9012", "1234 5678 9012", "arn:aws:iam::987654321098"} {
		s := "lead " + id + ":tail"
		for n := 0; n <= len(s); n++ {
			got := CutText(s, n)
			assert.LessOrEqual(t, len(got), n)
			if strings.HasPrefix(id, "arn:") {
				assert.True(t, len(got) <= len("lead arn:aws:iam::") || strings.HasPrefix(got, "lead "+id), "cut %d split %q: %q", n, id, got)
				continue
			}
			assert.True(t, len(got) <= len("lead ") || strings.HasPrefix(got, "lead "+id), "cut %d split %q: %q", n, id, got)
		}
	}

	ports := strings.Repeat("8080 8443 9090 ", 10)
	for n := 0; n <= len(ports); n++ {
		assert.Len(t, CutText(ports, n), n, "digit-heavy text is not over-cut")
	}
}

// A raw entry written before the scrub existed keys the same as its
// scrubbed candidate, so the two never both land.
func TestRawLegacyEntriesDedupAgainstScrubbed(t *testing.T) {
	t.Parallel()

	t.Run("learned pitfall", func(t *testing.T) {
		dir := t.TempDir()
		legacy := "provider: aws\npitfalls:\n  - resource: aws_iam_role\n    rule: \"arn:aws:iam::123456789012:role/x account 123456789012\"\n    source: descriptive\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "aws.yaml"), []byte(legacy), 0o644))
		require.NoError(t, AppendPitfall(dir, "aws", LearnedPitfall{Resource: "aws_iam_role", Rule: "arn:aws:iam::123456789012:role/x account 123456789012"}))
		entries, err := LoadPitfallEntries(dir, "aws")
		require.NoError(t, err)
		assert.Len(t, entries, 1)
	})

	t.Run("gap rows", func(t *testing.T) {
		dir := t.TempDir()
		gap := PolicyGap{Cloud: "aws", Policy: "aws.iam_scoped", Resource: "role_123456789012", Scenario: "s", Detail: "d", Timestamp: "t"}
		require.NoError(t, AppendPolicyGap(dir, gap))
		path := filepath.Join(dir, "policy-gaps.md")
		body, err := os.ReadFile(path)
		require.NoError(t, err)
		// Put the raw id back, as a file from before the scrub would hold it.
		raw := strings.ReplaceAll(string(body), accountIDPlaceholder, testAccountID)
		require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))

		require.NoError(t, AppendPolicyGap(dir, gap))
		body, err = os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, 1, strings.Count(string(body), "| `aws.iam_scoped` |"), string(body))
		assert.NotContains(t, string(body), testAccountID, "the dedup hit still rewrites the legacy file scrubbed")
	})
}

// The writer scrubs a copy: the caller's struct still holds what it
// passed, and only the file is scrubbed.
func TestWritePitfallsLeavesCallerValue(t *testing.T) {
	t.Parallel()

	pf := &PitfallsFile{Provider: "aws", Pitfalls: []PitfallEntry{{Resource: "aws_iam_role", Rule: accountIDLeak, Source: "static"}}}
	path := filepath.Join(t.TempDir(), "aws.yaml")
	scrubbed, err := WritePitfalls(path, pf)
	require.NoError(t, err)
	assert.Equal(t, 1, scrubbed)
	assert.Equal(t, accountIDLeak, pf.Pitfalls[0].Rule)
	assertAccountIDsScrubbed(t, path, 2)
}

// ScrubStrings walks structs, pointers, slices and strings only. A map,
// interface or []byte added to a written type would be skipped silently,
// so this fails loudly instead.
func TestScrubbedTypesHoldOnlyWalkableFields(t *testing.T) {
	t.Parallel()

	var walk func(path string, typ reflect.Type)
	seen := map[reflect.Type]bool{}
	walk = func(path string, typ reflect.Type) {
		if seen[typ] {
			return
		}
		seen[typ] = true
		switch typ.Kind() {
		case reflect.Pointer, reflect.Array:
			walk(path, typ.Elem())
		case reflect.Slice:
			assert.NotEqual(t, reflect.Uint8, typ.Elem().Kind(), "%s is []byte, which ScrubStrings does not walk", path)
			walk(path+"[]", typ.Elem())
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				walk(path+"."+typ.Field(i).Name, typ.Field(i).Type)
			}
		case reflect.Map, reflect.Interface:
			assert.Fail(t, "unwalkable field", "%s is a %s, which ScrubStrings does not walk", path, typ.Kind())
		}
	}
	walk("PitfallsFile", reflect.TypeOf(PitfallsFile{}))
	walk("AvoidLedger", reflect.TypeOf(AvoidLedger{}))
}

// Published OS-vendor AMI owners are content: the plain-run layer leaves
// them, so a learned owners filter stays valid. A registered id or an ARN
// field is scrubbed even if listed.
func TestPublicAMIOwnersSurviveOnlyThePlainRunLayer(t *testing.T) {
	t.Parallel()

	const canonical = "099720109477"
	assert.Equal(t, `owners = ["099720109477"]`, scrubWith(`owners = ["099720109477"]`, nil))
	assert.Equal(t, "arn:aws:iam::ACCOUNT_ID:root", scrubWith("arn:aws:iam::"+canonical+":root", nil))
	assert.Equal(t, "owner ACCOUNT_ID", scrubWith("owner "+canonical, knownAccountsPattern([]string{canonical})))
}

// Tools that do not load the full config read only aws.account_id, and a
// missing, broken or strict-invalid config never stops them.
func TestRegisterConfigAccountIsLenient(t *testing.T) {
	t.Cleanup(RegisterScrubbedAccounts()) // scrubtest would be an import cycle from here
	dir := t.TempDir()
	var warn strings.Builder

	RegisterConfigAccount(filepath.Join(dir, "absent.yaml"), &warn)
	broken := filepath.Join(dir, "broken.yaml")
	require.NoError(t, os.WriteFile(broken, []byte("aws: [unclosed"), 0o644))
	RegisterConfigAccount(broken, &warn)
	assert.Contains(t, warn.String(), "absent.yaml")
	assert.Contains(t, warn.String(), "broken.yaml")

	// Unknown keys would fail config.Load's strict decode; the account is
	// still read.
	cfg := filepath.Join(dir, "infrafactory.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("unknown_key: 1\naws:\n  account_id: \"444455556666\"\n"), 0o644))
	RegisterConfigAccount(cfg, &warn)
	assert.Equal(t, "abcdACCOUNT_ID", ScrubAccountIDs("abcd444455556666"))
}
