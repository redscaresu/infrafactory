package generator

import (
	"os"
	"path/filepath"
	"regexp"
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

// accountIDLeak is the shape a real AccessDenied gives a learned rule.
const accountIDLeak = "User arn:aws:iam::123456789012:user/x is not authorized to perform iam:CreateRole in account 123456789012"

var twelveDigitRun = regexp.MustCompile(`[0-9]{12}`)

func TestScrubAccountIDs(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"arn:aws:iam::123456789012:user/x":           "arn:aws:iam::ACCOUNT_ID:user/x",
		"account 123456789012":                       "account ACCOUNT_ID",
		"role_123456789012 123456789012.dkr.ecr.aws": "role_ACCOUNT_ID ACCOUNT_ID.dkr.ecr.aws",
		"thirteen 1234567890123":                     "thirteen 1234567890123",
		"eleven 12345678901":                         "eleven 12345678901",
		"digest ab123456789012cd":                    "digest ab123456789012cd",
		"id 550e8400-e29b-41d4-a716-446655440000":    "id 550e8400-e29b-41d4-a716-446655440000",
		"bucket logs-123456789012-eu":                "bucket logs-ACCOUNT_ID-eu",
		"\xffarn:aws:iam::123456789012:":             "\xffarn:aws:iam::ACCOUNT_ID:",
	} {
		assert.Equal(t, want, scrubAccountIDs(in), in)
	}
}

func assertAccountIDsScrubbed(t *testing.T, path string, wantPlaceholders int) {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.False(t, twelveDigitRun.Match(body), "12-digit run in %s:\n%s", path, body)
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

// yaml.v3 writes a string that is not valid UTF-8 as base64 !!binary,
// where a scrub of !!str scalars never looks.
func TestPitfallWriteScrubsInvalidUTF8Rule(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Through the writer, not AppendPitfall: that scrubs the candidate
	// first and would hide a writer that skips !!binary.
	require.NoError(t, WritePitfalls(dir, "aws", &PitfallsFile{Provider: "aws", Pitfalls: []PitfallEntry{
		{Resource: "aws_iam_role", Rule: "\xff " + accountIDLeak, Source: "static"},
	}}))
	entries, err := LoadPitfallEntries(dir, "aws")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.NotContains(t, entries[0].Rule, "123456789012")
	assert.Equal(t, 2, strings.Count(entries[0].Rule, accountIDPlaceholder))
}

// Evidence pointers name run artifacts; scrubbing one breaks the pointer.
func TestAvoidLedgerKeepsEvidencePointers(t *testing.T) {
	t.Parallel()

	const pointer = "runs/20261009-123456789012/run.json"
	dir := t.TempDir()
	require.NoError(t, appendAvoidLedgerRecord(dir, "aws", &AvoidLedger{Provider: "aws"}, AvoidLedgerRecord{
		Status: AvoidRecordRelearned, Resource: "aws_iam_role", Attributes: []string{"name"},
		LearnedLayer: "live", LayerEvidence: pointer, Rule: accountIDLeak, At: "2026-10-09T00:00:00Z",
		Check: &AvoidCheck{ID: "c1", At: "2026-10-09T00:00:00Z", From: pointer, Detail: accountIDLeak},
	}))
	ledger, err := ReadAvoidLedger(dir, "aws")
	require.NoError(t, err)
	require.Len(t, ledger.Records, 1)
	rec := ledger.Records[0]
	assert.Equal(t, pointer, rec.LayerEvidence)
	require.NotNil(t, rec.Check)
	assert.Equal(t, pointer, rec.Check.From)
	assert.NotContains(t, rec.Rule, "123456789012")
	assert.NotContains(t, rec.Check.Detail, "123456789012")
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

func TestGapRowsScrubResourceAndScenario(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, AppendPolicyGap(dir, PolicyGap{
		Cloud: "aws", Policy: "aws.iam_scoped", Resource: "role_123456789012",
		Scenario: "web-123456789012", Detail: "d", Timestamp: "20261009T120000Z",
	}))
	assertAccountIDsScrubbed(t, filepath.Join(dir, "policy-gaps.md"), 2)

	require.NoError(t, AppendMockGap(dir, MockGap{
		Cloud: "aws", Signal: "s", Resource: "role_123456789012",
		Scenario: "web-123456789012", Detail: "d", Timestamp: "20261009T120000Z",
	}))
	assertAccountIDsScrubbed(t, filepath.Join(dir, "mock-gaps.md"), 2)
}
