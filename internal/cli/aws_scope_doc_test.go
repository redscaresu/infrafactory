package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The AWS scope is built by hand from docs/operations.md § Layer 3 (AWS),
// which the stamp's and default VPC's errors point at, so the runbook
// must name what the code reads.
func TestAWSScopeSetupRunbookNamesWhatTheCodeReads(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../docs/operations.md")
	require.NoError(t, err)
	_, section, found := strings.Cut(string(data), "\n## Layer 3 (AWS)\n")
	require.True(t, found, "docs/operations.md has no ## Layer 3 (AWS)")
	section, _, _ = strings.Cut(section, "\n## ")

	for _, want := range []string{
		"### Scope setup",
		"AWSClaimParameter", harness.AWSClaimParameter,
		"AWSStampParameter", harness.AWSStampParameter,
		"~/" + awsCredentialFile, "0600",
		"docs/aws-layer3/iam-policy.json", "docs/aws-layer3/scp.json",
		"arn:aws:iam::*:role/OrganizationAccountAccessRole",
	} {
		assert.Contains(t, section, want)
	}
}
