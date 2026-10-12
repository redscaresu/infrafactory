package harness

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	awsScopeADRFile = "../../docs/decisions/0040-aws-whole-scope-ownership.md"
	adr0023File     = "../../docs/decisions/0023-layer3-sealed-environment-and-orphan-verification.md"
)

// ADR-0040 records the sweep table and the reap order; a row or a pair
// added without the ADR fails here.
func TestADR0040NamesEverySweepRowAndPrecedencePair(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(awsScopeADRFile)
	require.NoError(t, err)
	adr := string(data)

	require.NotEmpty(t, AWSSweepCollections)
	for _, row := range AWSSweepCollections {
		assert.Contains(t, adr, "`"+row.Name+"`", "ADR-0040 must name the sweep row")
	}
	require.NotEmpty(t, awsReapPrecedence)
	for _, pair := range awsReapPrecedence {
		assert.Contains(t, adr, "`"+pair[0]+" → "+pair[1]+"`", "ADR-0040 must name the precedence pair")
	}
}

func TestADR0023AmendmentRecordsTheAWSImplementation(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(adr0023File)
	require.NoError(t, err)
	for _, want := range []string{"ADR-0040", "aws-layer3-seal-and-dispatch", "ADR-0025 is not carried over"} {
		assert.Contains(t, string(data), want)
	}
}

// The hold amendment names its flags, paths, modes and rules; deleting
// the section or softening a rule fails here.
func TestADR0040RecordsTheHold(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(awsScopeADRFile)
	require.NoError(t, err)
	// Fields folds the ADR's line wraps so a phrase can span them.
	adr := strings.ToLower(strings.Join(strings.Fields(string(data)), " "))

	for _, want := range []string{
		"--hold", "--release-hold", ".infrafactory/holds/", "0700", "0600", "awsReleaseAfterCleanSweep",
		"the hold holds only a passing, uninterrupted run with its copy written",
		"a failed or interrupted hold destroys and releases",
		"the copy is written before the destroy and the release refuses without it",
		"`reap <scenario> --take-over <holder>`",
		"generate, run and test are refused for that scenario",
		"nothing expires a hold",
	} {
		assert.Contains(t, adr, strings.ToLower(want))
	}
	assert.NotContains(t, adr, "exception to rule 6")
}
