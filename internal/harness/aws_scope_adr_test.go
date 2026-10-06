package harness

import (
	"os"
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
