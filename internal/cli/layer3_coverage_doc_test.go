package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// docs/layer3-coverage.md is the file someone reads to decide what to
// spend real money on next, and its numbers are hand-maintained. Over
// one arc they drifted four separate times -- a scenario counted in the
// numerator and not the denominator, a gated remainder that did not
// match the table, a "three families" claim that had become four, and an
// enumerated allowlist that had fallen behind the config.
//
// Every one of those was a paragraph disagreeing with a table two
// screens away. So check the paragraphs against the table.
func layer3CoverageDoc(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "layer3-coverage.md"))
	require.NoError(t, err)
	return string(raw)
}

// allowlistEntryRe must accept digits: scaleway_k8s_cluster is a real
// candidate for admission and [a-z_*]+ drops it silently, which would
// make the sync test compare an incomplete documented set against the
// config and fail for the wrong reason.
var allowlistEntryRe = regexp.MustCompile("`(scaleway_[a-z0-9_*]+)`")

var coverageRowRe = regexp.MustCompile(`(?m)^\| ` + "`" + `([a-z0-9-]+)` + "`" + ` \| ([^|]+) \|`)

// The status totals must match the rows they summarise.
func TestLayer3CoverageDocTotalsMatchItsTable(t *testing.T) {
	t.Parallel()
	doc := layer3CoverageDoc(t)

	counts := map[string]int{}
	rows := coverageRowRe.FindAllStringSubmatch(doc, -1)
	for _, m := range rows {
		counts[strings.TrimSpace(m[2])]++
	}
	require.NotEmpty(t, rows, "no scenario rows parsed -- the table format changed")

	run := counts["**runnable**"]
	keyOnly := counts["key only"]
	both := counts["allowlist + key"]

	// assert.Contains on the whole document prints the whole document
	// on failure, which buries the one number that is wrong.
	wantTotals := sprintTotals(run, keyOnly, both)
	assert.True(t, strings.Contains(doc, wantTotals),
		"the totals line must match the table (%d rows). Expected to find:\n  %s", len(rows), wantTotals)

	wantGated, ok := gatedSentence(keyOnly + both)
	require.True(t, ok,
		"no spelled-out word for %d gated scenarios. Add it to numberWords -- "+
			"without one this check degrades to matching \" scenarios remain gated\", "+
			"which any count satisfies", keyOnly+both)
	assert.True(t, strings.Contains(doc, wantGated),
		"the gated remainder must equal key-only + both = %d. Expected to find:\n  %q",
		keyOnly+both, wantGated)

	assert.Equal(t, len(rows), run+keyOnly+both,
		"every row must fall into exactly one of the three buckets")
}

// The enumerated allowlist must be the allowlist.
func TestLayer3CoverageDocAllowlistMatchesConfig(t *testing.T) {
	t.Parallel()
	doc := layer3CoverageDoc(t)

	start := strings.Index(doc, "Repo default")
	end := strings.Index(doc, "2. **The")
	require.Greater(t, end, start, "could not locate the enumerated allowlist")

	documented := map[string]bool{}
	for _, m := range allowlistEntryRe.FindAllStringSubmatch(doc[start:end], -1) {
		documented[m[1]] = true
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", "infrafactory.yaml"))
	require.NoError(t, err)
	var cfg struct {
		Validation struct {
			Layers struct {
				SandboxDeploy struct {
					AllowResourceTypes []string `yaml:"allow_resource_types"`
				} `yaml:"sandbox_deploy"`
			} `yaml:"layers"`
		} `yaml:"validation"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &cfg))

	actual := map[string]bool{}
	for _, e := range cfg.Validation.Layers.SandboxDeploy.AllowResourceTypes {
		actual[e] = true
	}

	assert.Equal(t, actual, documented,
		"docs/layer3-coverage.md enumerates the repo-default allowlist; it must match infrafactory.yaml exactly")
}

func sprintTotals(run, keyOnly, both int) string {
	return fmt.Sprintf("**Current: %d have run, %d are blocked by the key alone, %d by both.**", run, keyOnly, both)
}

// numberWords covers the plausible range for this table. A count outside
// it is a signal to extend the map, never to skip the check.
var numberWords = map[int]string{
	0: "No", 1: "One", 2: "Two", 3: "Three", 4: "Four", 5: "Five",
	6: "Six", 7: "Seven", 8: "Eight", 9: "Nine", 10: "Ten",
	11: "Eleven", 12: "Twelve", 13: "Thirteen", 14: "Fourteen",
	15: "Fifteen", 16: "Sixteen", 17: "Seventeen", 18: "Eighteen",
	19: "Nineteen", 20: "Twenty",
}

// gatedSentence reports the sentence the doc must contain, and whether a
// word exists for n at all.
//
// Returning the miss rather than an empty word matters: with words[n]
// empty the expected substring became " scenarios remain gated", which
// every possible count satisfies -- so the guard would have gone on
// passing while silently checking nothing, at exactly the moment the
// table grew past its range.
func gatedSentence(n int) (string, bool) {
	word, ok := numberWords[n]
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%s scenarios remain gated", word), true
}

func TestAllowlistEntryRegexAcceptsDigits(t *testing.T) {
	t.Parallel()

	found := allowlistEntryRe.FindAllStringSubmatch(
		"`scaleway_k8s_cluster`, `scaleway_lb*` and `scaleway_block_volume`", -1)

	var got []string
	for _, m := range found {
		got = append(got, m[1])
	}
	assert.Equal(t, []string{"scaleway_k8s_cluster", "scaleway_lb*", "scaleway_block_volume"}, got,
		"dropping a digit-bearing type would silently shrink the documented set")
}

// The degenerate case the explicit miss exists to prevent.
func TestGatedSentenceReportsUnknownCounts(t *testing.T) {
	t.Parallel()

	_, ok := gatedSentence(999)

	assert.False(t, ok, "an unmapped count must be reported, not turned into a substring that always matches")
}
