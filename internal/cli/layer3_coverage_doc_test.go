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

	wantGated := gatedSentence(keyOnly + both)
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
	for _, m := range regexp.MustCompile("`(scaleway_[a-z_*]+)`").FindAllStringSubmatch(doc[start:end], -1) {
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

func gatedSentence(n int) string {
	words := map[int]string{
		11: "Eleven", 12: "Twelve", 13: "Thirteen", 14: "Fourteen",
		15: "Fifteen", 16: "Sixteen", 17: "Seventeen",
	}
	return fmt.Sprintf("%s scenarios remain gated", words[n])
}
